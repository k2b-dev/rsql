package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/k2b-dev/rsql/internal/domain"
	driver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

type sqlPolicy struct {
	control, checking, dml, insert, schema, denied, internalWrite, readOnly bool
	changes                                                                 map[string]bool
	existing                                                                map[string]bool
	managed                                                                 map[string]bool
	managedSQL                                                              map[string]string
}

// Explicit built-in allowlist: new extension or filesystem functions are denied
// by default. SQLite's rename helpers are required by ALTER TABLE.
var sqlFunctions = func() map[string]bool {
	m := make(map[string]bool)
	for _, name := range strings.Fields(`
abs avg changes char coalesce concat concat_ws count cume_dist current_date
current_time current_timestamp date datetime dense_rank first_value format
glob group_concat hex if ifnull iif instr julianday lag last_insert_rowid
last_value lead length like likelihood likely ln log log10 log2 lower ltrim
max min mod nth_value ntile nullif octet_length percent_rank pi pow power
printf quote random randomblob rank replace round row_number rtrim sign sin
cos tan asin acos atan atan2 sinh cosh tanh asinh acosh atanh sqrt exp ceil
ceiling floor trunc soundex sqlite_source_id sqlite_version strftime string_agg
substr substring sum time timediff total total_changes trim typeof unhex
unicode unixepoch unlikely upper zeroblob
json json_array json_array_length json_error_position json_extract json_group_array
json_group_object json_insert json_object json_patch json_pretty json_quote
json_remove json_replace json_set json_type json_valid jsonb jsonb_array
jsonb_extract jsonb_group_array jsonb_group_object jsonb_insert jsonb_object
jsonb_patch jsonb_remove jsonb_replace jsonb_set -> ->>
sqlite_rename_column sqlite_rename_table sqlite_rename_test sqlite_drop_column
sqlite_rename_quotefix`) {
		m[name] = true
	}
	return m
}()

func sqlObject(name string) bool {
	return identRe.MatchString(name) && !strings.HasPrefix(name, "_") && !strings.HasPrefix(strings.ToLower(name), "sqlite_")
}

func (p *sqlPolicy) authorize(action driver.AuthorizerActionCode, a, b, db, source string) driver.AuthorizerReturnCode {
	if p.internalWrite && a == "_schema_log" && action == driver.AuthInsert {
		return driver.AuthorizerOK
	}
	allow := false
	if p.readOnly && action != driver.AuthSelect && action != driver.AuthRecursive && action != driver.AuthRead && action != driver.AuthFunction && action != driver.AuthPragma {
		return p.deny()
	}
	// ALTER TABLE checks the isolated, empty TEMP catalog as part of rename. It also
	// issues an UPDATE against that catalog; clients cannot create TEMP objects.
	if p.schema && db == "temp" && a == "sqlite_temp_master" && (action == driver.AuthRead || action == driver.AuthUpdate) {
		return driver.AuthorizerOK
	}
	if db != "" && db != "main" {
		return p.deny()
	}
	switch action {
	case driver.AuthSelect, driver.AuthRecursive:
		allow = true
	case driver.AuthTransaction:
		allow = p.control
	case driver.AuthFunction:
		allow = sqlFunctions[strings.ToLower(b)]
	case driver.AuthRead:
		allow = (sqlObject(a) && (p.existing[a] || hasSQLChange(p.changes, a) || a == "json_each" || a == "json_tree")) || a == "sqlite_master" || a == "sqlite_schema" || (a == "sqlite_sequence" && p.schema)
	case driver.AuthInsert, driver.AuthUpdate, driver.AuthDelete:
		// SQLite itself writes its schema catalog for DDL. Defensive mode prevents
		// explicit catalog mutation, and writable_schema cannot be enabled.
		if a == "sqlite_master" || a == "sqlite_schema" || (a == "sqlite_sequence" && p.schema) {
			return driver.AuthorizerOK
		}
		allow = sqlObject(a)
		if allow && !p.checking {
			p.dml = true
			p.insert = p.insert || action == driver.AuthInsert
			if _, ok := p.changes[a]; !ok {
				p.changes[a] = false
			}
		}
	case driver.AuthCreateTable, driver.AuthCreateView, driver.AuthDropTable, driver.AuthDropView:
		allow = sqlObject(a) && !p.managed[strings.ToLower(a)]
		if allow {
			p.recordSchema(a)
		}
	case driver.AuthCreateIndex, driver.AuthDropIndex:
		// SQLite-generated UNIQUE/PRIMARY KEY indexes are harmless catalog entries.
		allow = (sqlObject(a) || strings.HasPrefix(a, "sqlite_autoindex_")) && sqlObject(b) && !p.managed[strings.ToLower(b)]
		if allow {
			p.recordSchema(b)
		}
	case driver.AuthAlterTable:
		allow = a == "main" && sqlObject(b) && !p.managed[strings.ToLower(b)]
		if allow {
			p.recordSchema(b)
		}
	case driver.AuthReindex:
		allow = sqlObject(a) || strings.HasPrefix(a, "sqlite_autoindex_")
	case driver.AuthPragma:
		switch strings.ToLower(a) {
		case "table_info", "table_xinfo", "index_info", "index_xinfo", "index_list", "foreign_key_list":
			allow = sqlObject(b)
		case "page_count", "page_size", "freelist_count":
			allow = b == ""
		}
		// Attach/detach (including VACUUM INTO), transaction control, TEMP objects,
		// triggers, virtual tables, ANALYZE and every unknown operation fail closed.
	}
	if !allow {
		return p.deny()
	}
	return driver.AuthorizerOK
}

func hasSQLChange(changes map[string]bool, name string) bool {
	_, ok := changes[name]
	return ok
}

func (p *sqlPolicy) deny() driver.AuthorizerReturnCode { p.denied = true; return driver.AuthorizerDeny }
func (p *sqlPolicy) recordSchema(table string) {
	if p.checking {
		return
	}
	p.schema = true
	p.changes[table] = true
}

func (p *sqlPolicy) sqlError(err error) error {
	if err == nil {
		return nil
	}
	if p.denied {
		return domain.NewError(domain.ErrSQLForbidden, 400, "SQL operation is not allowed")
	}
	if strings.HasPrefix(err.Error(), "missing argument with index ") || strings.HasPrefix(err.Error(), "missing named argument ") {
		return domain.NewError(domain.ErrInvalidRequest, 400, "SQL parameters do not match the statement")
	}
	var e *driver.Error
	if errors.As(err, &e) {
		switch e.Code() & 0xff {
		case sqlite3.SQLITE_TOOBIG, sqlite3.SQLITE_NOMEM:
			return domain.NewError(domain.ErrSQLLimitExceeded, 413, "SQLite resource limit exceeded")
		case sqlite3.SQLITE_ERROR, sqlite3.SQLITE_RANGE, sqlite3.SQLITE_MISMATCH:
			return domain.NewError(domain.ErrInvalidRequest, 400, "invalid SQL statement or parameters")
		}
	}
	return err
}

func loadSQLPolicy(ctx context.Context, conn *sql.Conn) (*sqlPolicy, error) {
	p := &sqlPolicy{changes: make(map[string]bool), existing: make(map[string]bool), managed: make(map[string]bool), managedSQL: make(map[string]string)}
	rows, err := conn.QueryContext(ctx, `SELECT name,tbl_name,coalesce(sql,'') FROM sqlite_schema`)
	if err != nil {
		return nil, err
	}
	type object struct{ name, table, sql string }
	var objects []object
	for rows.Next() {
		var o object
		if err := rows.Scan(&o.name, &o.table, &o.sql); err != nil {
			rows.Close()
			return nil, err
		}
		p.existing[o.name] = true
		objects = append(objects, o)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows, err = conn.QueryContext(ctx, `SELECT entity_name FROM _meta WHERE entity_type='table_schema'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		p.managed[strings.ToLower(name)] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, o := range objects {
		if p.managed[strings.ToLower(o.table)] {
			p.managedSQL[o.name] = o.sql
		}
	}
	return p, nil
}

// SQLite defers compiling view bodies, defaults, CHECKs and foreign keys.
// Compile them before commit so they cannot become a delayed path into internal
// data or forbidden functions when a later structured API uses the schema.
func validateSQLSchema(ctx context.Context, conn *sql.Conn, p *sqlPolicy) error {
	p.checking = true
	defer func() { p.checking = false }()
	rows, err := conn.QueryContext(ctx, `SELECT type,name,tbl_name,coalesce(sql,'') FROM sqlite_schema`)
	if err != nil {
		return p.sqlError(err)
	}
	var tables, views []string
	seen := make(map[string]bool)
	for rows.Next() {
		var typ, name, owner, definition string
		if err := rows.Scan(&typ, &name, &owner, &definition); err != nil {
			rows.Close()
			return err
		}
		seen[name] = true
		original, managed := p.managedSQL[name]
		if (managed && definition != original) || (!managed && p.managed[strings.ToLower(owner)]) {
			rows.Close()
			return domain.NewError(domain.ErrSQLForbidden, 400, "SQL cannot change structured schema definitions")
		}
		if typ != "table" && typ != "view" {
			continue
		}
		if strings.HasPrefix(name, "_") {
			if !p.existing[name] {
				rows.Close()
				return domain.NewError(domain.ErrSQLForbidden, 400, "internal object names are reserved")
			}
			continue
		}
		if strings.HasPrefix(name, "sqlite_") {
			continue
		}
		if !sqlObject(name) {
			rows.Close()
			return domain.NewError(domain.ErrSQLForbidden, 400, "invalid SQL object name")
		}
		if !p.existing[name] {
			p.changes[name] = true
		}
		if typ == "view" {
			views = append(views, name)
		} else {
			tables = append(tables, name)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for name := range p.managedSQL {
		if !seen[name] {
			return domain.NewError(domain.ErrSQLForbidden, 400, "SQL cannot remove structured schema objects")
		}
	}
	prepare := func(query string) error {
		stmt, err := conn.PrepareContext(ctx, query)
		if err != nil {
			return p.sqlError(err)
		}
		// modernc prepares lazily: QueryContext is needed to actually compile EXPLAIN.
		r, err := stmt.QueryContext(ctx)
		if r != nil {
			_ = r.Close()
		}
		_ = stmt.Close()
		return p.sqlError(err)
	}
	for _, view := range views {
		if err := prepare("EXPLAIN SELECT * FROM " + quotedIdentifier(view)); err != nil {
			return err
		}
	}
	for _, table := range tables {
		name := quotedIdentifier(table)
		if !p.managed[strings.ToLower(table)] {
			for _, q := range []string{"EXPLAIN INSERT INTO " + name + " DEFAULT VALUES", "EXPLAIN DELETE FROM " + name} {
				if err := prepare(q); err != nil {
					return err
				}
			}
		}
		fk, err := conn.QueryContext(ctx, "PRAGMA foreign_key_list("+name+")")
		if err != nil {
			return p.sqlError(err)
		}
		for fk.Next() {
			var id, seq int
			var target string
			var from, to, update, del, match any
			if err := fk.Scan(&id, &seq, &target, &from, &to, &update, &del, &match); err != nil {
				fk.Close()
				return err
			}
			if !sqlObject(target) || (p.managed[strings.ToLower(table)] && !seen[target]) {
				fk.Close()
				return domain.NewError(domain.ErrSQLForbidden, 400, "foreign keys cannot reference internal objects")
			}
		}
		if err := fk.Close(); err != nil {
			return err
		}
		if err := fk.Err(); err != nil {
			return err
		}
	}
	return nil
}
