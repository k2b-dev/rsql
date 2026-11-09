package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/k2b-dev/rsql/internal/domain"
)

var forbiddenSQLKeywords = map[string]struct{}{
	"INSERT":   {},
	"UPDATE":   {},
	"DELETE":   {},
	"CREATE":   {},
	"ALTER":    {},
	"DROP":     {},
	"REPLACE":  {},
	"PRAGMA":   {},
	"ATTACH":   {},
	"DETACH":   {},
	"VACUUM":   {},
	"REINDEX":  {},
	"TRUNCATE": {},
}

// QueryStatement defines one query statement.
type QueryStatement struct {
	SQL    string `json:"sql"`
	Params []any  `json:"params"`
}

// QueryRequest defines a single or batch read-only query request.
type QueryRequest struct {
	SQL        string           `json:"sql"`
	Params     []any            `json:"params"`
	Statements []QueryStatement `json:"statements"`
}

// ExecuteReadOnly preserves the internal convenience entry point. Service calls
// use ExecuteReadOnlyContext to propagate request cancellation and configuration.
func ExecuteReadOnly(db *sql.DB, req QueryRequest) (any, error) {
	var seq int
	var name, path string
	if err := db.QueryRow("PRAGMA database_list").Scan(&seq, &name, &path); err != nil {
		return nil, err
	}
	return ExecuteReadOnlyContext(context.Background(), path, domain.NamespaceConfig{}, req)
}

// ExecuteReadOnlyContext keeps the legacy response and non-atomic batch contract
// while using the same connection policy, deadlines and budgets as transactions.
func ExecuteReadOnlyContext(ctx context.Context, path string, cfg domain.NamespaceConfig, req QueryRequest) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, MaxSQLDuration)
	defer cancel()
	encoded, err := json.Marshal(req)
	if err != nil || len(encoded) > MaxSQLRequestBytes || len(req.Statements) > MaxTransactionStatements {
		return nil, domain.NewError(domain.ErrSQLLimitExceeded, 413, "SQL request exceeds limit")
	}
	statements := req.Statements
	batch := len(statements) > 0
	if !batch {
		statements = []QueryStatement{{SQL: req.SQL, Params: req.Params}}
	}
	for _, st := range statements {
		if !isReadOnlyQuery(st.SQL) || referencesInternalObject(st.SQL) {
			return nil, fmt.Errorf("sql_not_read_only")
		}
		if err := validateSQLStatement(TransactionStatement{SQL: st.SQL, Params: st.Params, Mode: "query"}); err != nil {
			return nil, err
		}
	}
	db, conn, policy, err := openSQLConnection(ctx, path, cfg, true)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	defer conn.Close()
	budget := resultBudget{bytes: MaxSQLResultBytes, rows: MaxSQLResultRows}
	timeout := 10 * time.Second
	if cfg.QueryTimeout > 0 {
		timeout = time.Duration(min(cfg.QueryTimeout, int(MaxSQLDuration/time.Millisecond))) * time.Millisecond
	}
	out := make([]map[string]any, 0, len(statements))
	for _, st := range statements {
		statementCtx, cancel := context.WithTimeout(ctx, timeout)
		result, err := executeSQLStatement(statementCtx, conn, policy, TransactionStatement{SQL: st.SQL, Params: st.Params, Mode: "query"}, &budget)
		if statementCtx.Err() != nil {
			err = domain.NewError(domain.ErrSQLTimeout, 408, "SQL statement deadline exceeded")
		}
		cancel()
		if err != nil {
			if policy.denied {
				return nil, domain.NewError(domain.ErrSQLNotReadOnly, 400, "Only permitted read-only SELECT queries are allowed")
			}
			return nil, err
		}
		out = append(out, map[string]any{"data": *result.Data})
	}
	if batch {
		return map[string]any{"results": out}, nil
	}
	return out[0], nil
}

func isReadOnlyQuery(q string) bool {
	raw := strings.TrimSpace(q)
	if raw == "" {
		return false
	}
	// Block multi-statement and comment-based bypasses.
	if strings.Contains(raw, ";") || strings.Contains(raw, "--") || strings.Contains(raw, "/*") || strings.Contains(raw, "*/") {
		return false
	}

	normalized := normalizeSQL(raw)
	if normalized == "" {
		return false
	}
	tokens := strings.Fields(normalized)
	if len(tokens) == 0 {
		return false
	}
	if tokens[0] != "SELECT" && tokens[0] != "WITH" {
		return false
	}
	for _, token := range tokens {
		if _, blocked := forbiddenSQLKeywords[token]; blocked {
			return false
		}
	}
	return true
}

func referencesInternalObject(q string) bool {
	n := normalizeSQL(q)
	parts := strings.Fields(n)
	for _, p := range parts {
		p = strings.Trim(p, `",;()`) // punctuation
		if strings.HasPrefix(p, "_") {
			return true
		}
	}
	return false
}

func normalizeSQL(q string) string {
	q = strings.TrimSpace(q)
	q = strings.NewReplacer(
		"\n", " ",
		"\t", " ",
		"\r", " ",
		"(", " ( ",
		")", " ) ",
		",", " , ",
	).Replace(q)
	q = " " + strings.ToUpper(q) + " "
	for strings.Contains(q, "  ") {
		q = strings.ReplaceAll(q, "  ", " ")
	}
	return strings.TrimSpace(q)
}
