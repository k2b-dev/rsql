package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	driver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/k2b-dev/rsql/internal/domain"
)

// SQL API budgets bound one request's memory and time independently of callers.
const (
	MaxSQLRequestBytes       = 1 << 20
	MaxSQLBytes              = 64 << 10
	MaxSQLParams             = 1000
	MaxTransactionStatements = 100
	MaxSQLResultBytes        = 4 << 20
	MaxSQLResultRows         = 10000
	MaxSQLDuration           = 30 * time.Second
)

// TransactionStatement defaults to exec; query returns rows, including RETURNING.
type TransactionStatement struct {
	SQL    string `json:"sql"`
	Params []any  `json:"params,omitempty"`
	Mode   string `json:"mode,omitempty"`
}

// TransactionRequest is an ordered, atomic group of statements.
type TransactionRequest struct {
	Statements []TransactionStatement `json:"statements"`
	Meta       json.RawMessage        `json:"_meta,omitempty"`
}

// ExecResult describes the direct effects of one SQL statement.
type ExecResult struct {
	Changes         int64  `json:"changes"`
	LastInsertRowid *int64 `json:"lastInsertRowid,omitempty"`
}

// StatementResult contains either rows or an execution result, according to mode.
type StatementResult struct {
	Data *[]map[string]any `json:"data,omitempty"`
	*ExecResult
}

// TransactionResult is returned only after every statement commits.
type TransactionResult struct {
	Results []StatementResult `json:"results"`
}

// SQLChange is a table-level invalidation; raw SQL does not reconstruct row images.
type SQLChange struct {
	Table  string
	Schema bool
}

// StatementError identifies the zero-based failing statement and preserves its cause.
type StatementError struct {
	Index int
	Err   error
}

func (e *StatementError) Error() string { return fmt.Sprintf("statement %d: %v", e.Index, e.Err) }
func (e *StatementError) Unwrap() error { return e.Err }

// ExecuteTransaction uses a short-lived physical connection, never a pooled one:
// authorizer state, limits, transaction state, and last row IDs cannot leak to CRUD.
// The caller must hold the namespace writer lock throughout this operation.
func ExecuteTransaction(ctx context.Context, path string, cfg domain.NamespaceConfig, req TransactionRequest) (out TransactionResult, changes []SQLChange, err error) {
	if len(req.Statements) == 0 || len(req.Statements) > MaxTransactionStatements {
		return out, nil, domain.NewError(domain.ErrInvalidRequest, 400, "transaction requires 1 to 100 statements")
	}
	encoded, err := json.Marshal(req)
	if err != nil || len(encoded) > MaxSQLRequestBytes {
		return out, nil, domain.NewError(domain.ErrSQLLimitExceeded, 413, "SQL request exceeds 1 MiB or contains invalid parameters")
	}
	for i, st := range req.Statements {
		if err := validateSQLStatement(st); err != nil {
			return out, nil, &StatementError{i, err}
		}
	}
	if cfg.ReadOnly {
		return out, nil, domain.NewError(domain.ErrReadOnly, 405, "namespace is read-only")
	}
	if strings.EqualFold(cfg.JournalMode, "off") {
		return out, nil, domain.NewError(domain.ErrSQLForbidden, 400, "SQL writes require a journal mode supported by defensive SQLite")
	}
	db, conn, policy, err := openSQLConnection(ctx, path, cfg, false)
	if err != nil {
		return out, nil, err
	}
	defer db.Close()
	defer conn.Close()
	control := func(ctx context.Context, command string) error {
		policy.control = true
		defer func() { policy.control = false }()
		_, e := conn.ExecContext(ctx, command)
		return e
	}
	if err = control(ctx, "BEGIN IMMEDIATE"); err != nil {
		return out, nil, err
	}
	defer func() { _ = control(context.Background(), "ROLLBACK") }()
	out.Results = make([]StatementResult, 0, len(req.Statements))
	budget := resultBudget{bytes: MaxSQLResultBytes, rows: MaxSQLResultRows}
	statementTimeout := 10 * time.Second
	if cfg.QueryTimeout > 0 {
		statementTimeout = time.Duration(min(cfg.QueryTimeout, int(MaxSQLDuration/time.Millisecond))) * time.Millisecond
	}
	for i, st := range req.Statements {
		statementCtx, cancel := context.WithTimeout(ctx, statementTimeout)
		policy.dml, policy.insert, policy.schema, policy.denied = false, false, false, false
		result, e := executeSQLStatement(statementCtx, conn, policy, st, &budget)
		if e == nil && policy.schema {
			e = validateSQLSchema(statementCtx, conn, policy)
		}
		if statementCtx.Err() != nil {
			e = domain.NewError(domain.ErrSQLTimeout, 408, "SQL statement deadline exceeded")
		}
		cancel()
		if e != nil {
			return TransactionResult{}, nil, &StatementError{i, e}
		}
		out.Results = append(out.Results, result)
	}
	tables := make([]string, 0, len(policy.changes))
	for table := range policy.changes {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	for _, table := range tables {
		if !policy.changes[table] {
			continue
		}
		policy.internalWrite = true
		_, logErr := conn.ExecContext(ctx, `INSERT INTO _schema_log(timestamp,action,table_name,detail_json,meta_json) VALUES(?,?,?,?,?)`, time.Now().UTC().Format(time.RFC3339), "sql", table, `{"source":"sql"}`, nullableString(req.Meta))
		policy.internalWrite = false
		if logErr != nil {
			return TransactionResult{}, nil, &StatementError{len(req.Statements) - 1, logErr}
		}
	}
	if err = ctx.Err(); err != nil {
		return TransactionResult{}, nil, err
	}
	if err = commitSQL(ctx, control); err != nil {
		return TransactionResult{}, nil, &StatementError{len(req.Statements) - 1, err}
	}
	for _, table := range tables {
		changes = append(changes, SQLChange{table, policy.changes[table]})
	}
	return out, changes, nil
}

// commitSQL has a single cancellation boundary before COMMIT. Once SQLite starts
// committing, cancellation must not turn a durable success into a retryable error.
func commitSQL(ctx context.Context, control func(context.Context, string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return control(context.WithoutCancel(ctx), "COMMIT")
}

func executeSQLStatement(ctx context.Context, conn *sql.Conn, policy *sqlPolicy, st TransactionStatement, budget *resultBudget) (StatementResult, error) {
	if err := boundSQLResult(conn, st.SQL, budget.bytes); err != nil {
		return StatementResult{}, policy.sqlError(err)
	}
	defer func() { _, _ = driver.Limit(conn, sqlite3.SQLITE_LIMIT_LENGTH, MaxSQLResultBytes) }()
	if err := ctx.Err(); err != nil {
		return StatementResult{}, err
	}
	params := make([]any, len(st.Params))
	for i, p := range st.Params {
		if n, ok := p.(json.Number); ok {
			if v, e := n.Int64(); e == nil {
				params[i] = v
			} else if v, e := n.Float64(); e == nil {
				params[i] = v
			} else {
				return StatementResult{}, domain.NewError(domain.ErrInvalidRequest, 400, "invalid SQL number parameter")
			}
		} else {
			params[i] = p
		}
	}
	if st.Mode == "query" {
		rows, err := conn.QueryContext(ctx, st.SQL, params...)
		if err != nil {
			return StatementResult{}, policy.sqlError(err)
		}
		defer rows.Close()
		data, err := budget.scan(rows)
		return StatementResult{Data: &data}, policy.sqlError(err)
	}
	r, err := conn.ExecContext(ctx, st.SQL, params...)
	if err != nil {
		return StatementResult{}, policy.sqlError(err)
	}
	result := ExecResult{}
	if policy.dml {
		result.Changes, err = r.RowsAffected()
		if err != nil {
			return StatementResult{}, err
		}
		if policy.insert && result.Changes > 0 {
			id, e := r.LastInsertId()
			if e != nil {
				return StatementResult{}, e
			}
			result.LastInsertRowid = &id
		}
	}
	return StatementResult{ExecResult: &result}, nil
}

func validateSQLStatement(st TransactionStatement) error {
	if len(st.SQL) > MaxSQLBytes || len(st.Params) > MaxSQLParams {
		return domain.NewError(domain.ErrSQLLimitExceeded, 413, "SQL text or parameter count exceeds limit")
	}
	if st.Mode != "" && st.Mode != "exec" && st.Mode != "query" {
		return domain.NewError(domain.ErrInvalidRequest, 400, "statement mode must be exec or query")
	}
	for _, p := range st.Params {
		switch p.(type) {
		case nil, string, bool, int, int32, int64, float64, json.Number, []byte:
		default:
			return domain.NewError(domain.ErrInvalidRequest, 400, "SQL parameters must be scalar values")
		}
	}
	// This is only statement framing. Authorization is SQLite's responsibility.
	// Quoted semicolons are values, while comments and multiple statements are rejected.
	q := strings.TrimSpace(st.SQL)
	if q == "" || strings.ContainsRune(q, 0) {
		return domain.NewError(domain.ErrInvalidRequest, 400, "SQL statement is empty or contains NUL")
	}
	var quote byte
	for i := 0; i < len(q); i++ {
		c := q[i]
		if quote != 0 {
			if c == quote {
				if quote != ']' && i+1 < len(q) && q[i+1] == quote {
					i++
				} else {
					quote = 0
				}
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
		case '[':
			quote = ']'
		case '$', '@', ':', '#':
			return domain.NewError(domain.ErrSQLForbidden, 400, "only positional SQL parameters are accepted")
		case ';':
			if i != len(q)-1 {
				return domain.NewError(domain.ErrSQLForbidden, 400, "one SQL statement per entry is required")
			}
		case '-':
			if i+1 < len(q) && q[i+1] == '-' {
				return domain.NewError(domain.ErrSQLForbidden, 400, "SQL comments are not accepted")
			}
		case '/':
			if i+1 < len(q) && q[i+1] == '*' {
				return domain.NewError(domain.ErrSQLForbidden, 400, "SQL comments are not accepted")
			}
		}
	}
	return nil
}

type resultBudget struct{ bytes, rows int }

func (b *resultBudget) scan(rows *sql.Rows) ([]map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0)
	for rows.Next() {
		if b.rows == 0 {
			return nil, domain.NewError(domain.ErrSQLLimitExceeded, 413, "SQL results exceed 10000 rows")
		}
		values := make([]any, len(cols))
		targets := make([]any, len(cols))
		for i := range values {
			targets[i] = &values[i]
		}
		if err := rows.Scan(targets...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		rowBytes := 2
		for i, col := range cols {
			// Bound before copying byte values or JSON escaping (up to 6x).
			var size int
			switch v := values[i].(type) {
			case []byte:
				size = len(v)
			case string:
				size = len(v)
			}
			if size > max(0, (b.bytes-rowBytes-2)/6) {
				return nil, domain.NewError(domain.ErrSQLLimitExceeded, 413, "SQL value exceeds remaining result budget")
			}
			value := normalizeDBValue(values[i])
			encoded, err := json.Marshal(value)
			if err != nil {
				return nil, err
			}
			key, _ := json.Marshal(col)
			rowBytes += len(key) + len(encoded) + 2
			if rowBytes > b.bytes {
				return nil, domain.NewError(domain.ErrSQLLimitExceeded, 413, "SQL results exceed 4 MiB")
			}
			row[col] = value
		}
		b.bytes -= rowBytes + 1
		if b.bytes < 0 {
			return nil, domain.NewError(domain.ErrSQLLimitExceeded, 413, "SQL results exceed 4 MiB")
		}
		b.rows--
		out = append(out, row)
	}
	return out, rows.Err()
}
