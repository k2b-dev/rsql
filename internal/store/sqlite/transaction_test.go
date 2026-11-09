package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/k2b-dev/rsql/internal/domain"
)

func sqlTestConfig() domain.NamespaceConfig {
	return domain.NamespaceConfig{JournalMode: "wal", Synchronous: "full", BusyTimeout: 100, QueryTimeout: 1000}
}

func TestBoundTransactionAndDDL(t *testing.T) {
	db, path := openTestDB(t)
	value := "'); DROP TABLE records; --"
	req := TransactionRequest{Statements: []TransactionStatement{
		{SQL: `CREATE TABLE records (id INTEGER PRIMARY KEY, value TEXT UNIQUE)`},
		{SQL: `INSERT INTO records(value) VALUES (?)`, Params: []any{value}},
		{SQL: `SELECT value FROM records WHERE value=?`, Params: []any{value}, Mode: "query"},
		{SQL: `UPDATE records SET value=? WHERE id=? RETURNING value`, Params: []any{"changed", 1}, Mode: "query"},
		{SQL: `CREATE INDEX records_value ON records(value)`},
	}}
	out, changes, err := ExecuteTransaction(context.Background(), path, sqlTestConfig(), req)
	if err != nil {
		t.Fatal(err)
	}
	if out.Results[0].Changes != 0 || out.Results[1].Changes != 1 || out.Results[4].Changes != 0 {
		t.Fatalf("changes: %+v", out.Results)
	}
	if id := out.Results[1].LastInsertRowid; id == nil || *id != 1 {
		t.Fatalf("row ID: %v", id)
	}
	if (*out.Results[2].Data)[0]["value"] != value || (*out.Results[3].Data)[0]["value"] != "changed" {
		t.Fatalf("data: %+v", out.Results)
	}
	if len(changes) != 1 || changes[0].Table != "records" || !changes[0].Schema {
		t.Fatalf("changes: %+v", changes)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM records`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("persistence: %d %v", count, err)
	}
	for _, statement := range []string{
		`ALTER TABLE records ADD COLUMN active INTEGER DEFAULT 1`,
		`ALTER TABLE records RENAME TO renamed`,
		`CREATE VIEW record_view AS SELECT value FROM renamed`,
		`DROP VIEW record_view`,
		`DROP TABLE renamed`,
	} {
		if _, _, err := ExecuteTransaction(context.Background(), path, sqlTestConfig(), TransactionRequest{Statements: []TransactionStatement{{SQL: statement}}}); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
}

func TestTransactionRollback(t *testing.T) {
	db, path := openTestDB(t)
	if _, err := db.Exec(`CREATE TABLE records(id INTEGER PRIMARY KEY, value TEXT UNIQUE)`); err != nil {
		t.Fatal(err)
	}
	_, changes, err := ExecuteTransaction(context.Background(), path, sqlTestConfig(), TransactionRequest{Statements: []TransactionStatement{
		{SQL: `INSERT INTO records(value) VALUES (?)`, Params: []any{"first"}},
		{SQL: `INSERT INTO records(value) VALUES (?) RETURNING id`, Params: []any{"second"}, Mode: "query"},
		{SQL: `INSERT INTO records(value) VALUES (?)`, Params: []any{"first"}},
	}})
	var st *StatementError
	if !errors.As(err, &st) || st.Index != 2 || len(changes) != 0 {
		t.Fatalf("error: %v, changes: %+v", err, changes)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM records`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rollback: %d %v", count, err)
	}
	_, _, err = ExecuteTransaction(context.Background(), path, sqlTestConfig(), TransactionRequest{Statements: []TransactionStatement{
		{SQL: `CREATE TABLE rolled_back(id INTEGER)`}, {SQL: `SELECT * FROM missing`, Mode: "query"},
	}})
	if err == nil {
		t.Fatal("expected DDL rollback")
	}
	if _, err := db.Exec(`SELECT * FROM rolled_back`); err == nil {
		t.Fatal("DDL persisted after rollback")
	}
}

func TestSQLAuthorizer(t *testing.T) {
	db, path := openTestDB(t)
	if _, err := db.Exec(`CREATE TABLE records(id INTEGER PRIMARY KEY, value TEXT)`); err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`ATTACH DATABASE ':memory:' AS other`, `DETACH DATABASE main`,
		`VACUUM INTO '/tmp/rsql-escape.db'`, `PRAGMA writable_schema=ON`,
		`PRAGMA max_page_count=1000000`, `PRAGMA query_only=OFF`, `PRAGMA journal_mode=OFF`,
		`SELECT load_extension(?)`, `SELECT writefile('/tmp/rsql-escape','data')`,
		`SELECT * FROM main."_meta"`, "SELECT * FROM main.[_meta]", "SELECT * FROM main.`_meta`",
		`UPDATE main."_meta" SET metadata_json='{}'`,
		`DELETE FROM _schema_log`, `PRAGMA table_info(_meta)`,
		`CREATE TABLE _hidden(id INTEGER)`, `CREATE TEMP TABLE hidden(id INTEGER)`,
		`CREATE VIRTUAL TABLE fts USING fts5(value)`,
		`CREATE TRIGGER bad AFTER INSERT ON records BEGIN DELETE FROM _meta; END`,
		`CREATE VIEW leak AS SELECT * FROM _meta`,
		`CREATE VIEW leak AS SELECT load_extension('bad') AS value`,
		`CREATE TABLE evil(value TEXT DEFAULT (load_extension('bad')))`,
		`CREATE TABLE evil(value TEXT CHECK (load_extension(value)))`,
		`CREATE TABLE evil(value TEXT REFERENCES _meta(entity_name))`,
		`ALTER TABLE records RENAME TO _hidden`,
		`BEGIN`, `COMMIT`, `ROLLBACK`, `SAVEPOINT escape`, `END`,
		`INSERT INTO records(value) VALUES ('ok'); COMMIT`,
		`SELECT 1 /* comment */`, "SELECT 1\x00; COMMIT",
	}
	for _, statement := range statements {
		t.Run(statement, func(t *testing.T) {
			_, _, err := ExecuteTransaction(context.Background(), path, sqlTestConfig(), TransactionRequest{Statements: []TransactionStatement{{SQL: statement}}})
			if err == nil {
				t.Fatalf("forbidden SQL accepted: %s", statement)
			}
		})
	}
	// Safe introspection is still available, with values bound independently.
	for _, statement := range []string{`PRAGMA table_info(records)`, `PRAGMA page_count`, `SELECT 1 AS value;`} {
		if _, _, err := ExecuteTransaction(context.Background(), path, sqlTestConfig(), TransactionRequest{Statements: []TransactionStatement{{SQL: statement, Mode: "query"}}}); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
}

func TestSQLQuotaTimeoutAndBudgets(t *testing.T) {
	db, path := openTestDB(t)
	if _, err := db.Exec(`CREATE TABLE records(value BLOB)`); err != nil {
		t.Fatal(err)
	}
	cfg := sqlTestConfig()
	cfg.MaxDBSize = 64 << 10
	_, _, err := ExecuteTransaction(context.Background(), path, cfg, TransactionRequest{Statements: []TransactionStatement{{SQL: `INSERT INTO records VALUES (zeroblob(?))`, Params: []any{128 << 10}}}})
	if !IsFullError(err) {
		t.Fatalf("quota error: %v", err)
	}
	cfg = sqlTestConfig()
	cfg.QueryTimeout = 5
	_, _, err = ExecuteTransaction(context.Background(), path, cfg, TransactionRequest{Statements: []TransactionStatement{{SQL: `INSERT INTO records VALUES (?)`, Params: []any{"timeout rollback"}}, {Mode: "query", SQL: `WITH RECURSIVE r(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM r WHERE x<100000000) SELECT SUM(x) FROM r`}}})
	var e *domain.Error
	if !errors.As(err, &e) || e.Code != domain.ErrSQLTimeout {
		t.Fatalf("timeout error: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM records`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("timeout rollback: %d %v", count, err)
	}
	for _, req := range []TransactionRequest{
		{Statements: make([]TransactionStatement, MaxTransactionStatements+1)},
		{Statements: []TransactionStatement{{SQL: strings.Repeat(" ", MaxSQLBytes+1)}}},
		{Statements: []TransactionStatement{{SQL: "SELECT ?", Params: make([]any, MaxSQLParams+1)}}},
		{Statements: []TransactionStatement{{SQL: "SELECT ?", Params: []any{strings.Repeat("x", MaxSQLRequestBytes)}}}},
		{Statements: []TransactionStatement{{SQL: "SELECT zeroblob(?)", Params: []any{MaxSQLResultBytes + 1}, Mode: "query"}}},
		{Statements: []TransactionStatement{{SQL: "SELECT hex(zeroblob(?))", Params: []any{3 << 19}, Mode: "query"}, {SQL: "SELECT hex(zeroblob(?))", Params: []any{3 << 19}, Mode: "query"}}},
		{Statements: []TransactionStatement{{SQL: `WITH RECURSIVE r(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM r WHERE x<10001) SELECT x FROM r`, Mode: "query"}}},
	} {
		if _, _, err := ExecuteTransaction(context.Background(), path, sqlTestConfig(), req); err == nil {
			t.Fatal("budget was not enforced")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := ExecuteTransaction(ctx, path, sqlTestConfig(), TransactionRequest{Statements: []TransactionStatement{{SQL: "SELECT 1"}}}); err == nil {
		t.Fatal("canceled context accepted")
	}
	// A timed out/failed transaction must not poison the next connection.
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, _, err := ExecuteTransaction(ctx, path, sqlTestConfig(), TransactionRequest{Statements: []TransactionStatement{{SQL: `INSERT INTO records VALUES (?)`, Params: []any{"ok"}}}}); err != nil {
		t.Fatal(err)
	}
}
