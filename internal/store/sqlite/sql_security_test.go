package sqlite

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/k2b-dev/rsql/internal/domain"
)

func TestSQLWideResultsBoundBeforeMaterialization(t *testing.T) {
	_, path := openTestDB(t)
	cols := make([]string, 32)
	for i := range cols {
		cols[i] = fmt.Sprintf("zeroblob(65536) AS c%d", i)
	}
	query := "SELECT " + strings.Join(cols, ",")
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, _, err := ExecuteTransaction(context.Background(), path, sqlTestConfig(), TransactionRequest{Statements: []TransactionStatement{{SQL: query, Mode: "query"}}})
	runtime.ReadMemStats(&after)
	var e *domain.Error
	if !errors.As(err, &e) || e.Code != domain.ErrSQLLimitExceeded {
		t.Fatalf("wide result was not rejected: %v", err)
	}
	// The deliberately small probe is safe even if this regresses; the old path
	// allocated over 64 MiB. The guard must reject before stepping/scanning.
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 16<<20 {
		t.Fatalf("wide result allocated %d bytes before rejection", allocated)
	}
	// The same boundary applies when a SELECT is sent in exec mode.
	_, _, err = ExecuteTransaction(context.Background(), path, sqlTestConfig(), TransactionRequest{Statements: []TransactionStatement{{SQL: query}}})
	if !errors.As(err, &e) || e.Code != domain.ErrSQLLimitExceeded {
		t.Fatalf("exec discarded the output budget: %v", err)
	}
}

func TestSQLCommitCancellationBoundary(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	called := false
	err := commitSQL(ctx, func(commitCtx context.Context, command string) error {
		called = true
		cancel() // deterministic cancellation during the commit phase
		if command != "COMMIT" || commitCtx.Err() != nil {
			t.Fatalf("commit still uses caller cancellation: %s %v", command, commitCtx.Err())
		}
		return nil
	})
	if err != nil || !called {
		t.Fatalf("commit result: %v, called %v", err, called)
	}
	if err := commitSQL(ctx, func(context.Context, string) error {
		t.Fatal("already canceled transaction must not commit")
		return nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-commit cancellation: %v", err)
	}
}

func TestSQLProtectsManagedSchemaDependencies(t *testing.T) {
	db, path := openTestDB(t)
	if _, err := db.Exec(`CREATE TABLE raw_items(id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if err := CreateTableOrView(db, domain.TableCreateRequest{Type: "table", Name: "people", Columns: []domain.ColumnDefinition{{Name: "name", Type: "text", Unique: true}}}); err != nil {
		t.Fatal(err)
	}
	if err := CreateTableOrView(db, domain.TableCreateRequest{Type: "view", Name: "managed_view", SQL: `SELECT id FROM raw_items`}); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`CREATE UNIQUE INDEX raw_unique ON people(name)`,
		`CREATE INDEX raw_index ON PEOPLE(name)`,
		`DROP INDEX ux_people_name`,
		`ALTER TABLE raw_items RENAME TO renamed`,
		`DROP TABLE raw_items`,
	} {
		if _, _, err := ExecuteTransaction(context.Background(), path, sqlTestConfig(), TransactionRequest{Statements: []TransactionStatement{{SQL: query}}}); err == nil {
			t.Fatalf("managed schema dependency changed: %s", query)
		}
	}
	if _, err := db.Exec(`SELECT id FROM managed_view`); err != nil {
		t.Fatalf("managed view was not restored: %v", err)
	}
}

func TestSQLFramingAndEponymousTables(t *testing.T) {
	_, path := openTestDB(t)
	for _, query := range []string{`SELECT $a(') ; DELETE FROM records ; SELECT $b(')`, `SELECT :value`, `SELECT @value`, `SELECT name FROM dbstat`, `SELECT * FROM pragma_database_list`} {
		if _, _, err := ExecuteTransaction(context.Background(), path, sqlTestConfig(), TransactionRequest{Statements: []TransactionStatement{{SQL: query, Mode: "query"}}}); err == nil {
			t.Fatalf("unsupported SQL accepted: %s", query)
		}
	}
	for _, query := range []string{`SELECT value FROM json_each('[1,2]')`, `SELECT value FROM json_tree('[1,2]')`, `SELECT '$a(:@#)' AS value`} {
		if _, _, err := ExecuteTransaction(context.Background(), path, sqlTestConfig(), TransactionRequest{Statements: []TransactionStatement{{SQL: query, Mode: "query"}}}); err != nil {
			t.Fatalf("supported SQL rejected: %s: %v", query, err)
		}
	}
}

func TestSQLProtectsManagedForeignKeys(t *testing.T) {
	db, path := openTestDB(t)
	if _, err := db.Exec(`CREATE TABLE raw_parent(id INTEGER PRIMARY KEY); CREATE TABLE managed_child(id INTEGER PRIMARY KEY, parent INTEGER REFERENCES raw_parent(id))`); err != nil {
		t.Fatal(err)
	}
	if err := PutMeta(db, "table_schema", "managed_child", TableSchema{Name: "managed_child", Type: "table"}); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{`ALTER TABLE raw_parent RENAME TO renamed`, `DROP TABLE raw_parent`} {
		_, _, err := ExecuteTransaction(context.Background(), path, sqlTestConfig(), TransactionRequest{Statements: []TransactionStatement{{SQL: query}}})
		var e *domain.Error
		if !errors.As(err, &e) || e.Code != domain.ErrSQLForbidden {
			t.Fatalf("managed FK dependency changed: %s: %v", query, err)
		}
	}
}

func TestViewSQLAndLegacyReadPolicy(t *testing.T) {
	db, path := openTestDB(t)
	if err := CreateTableOrView(db, domain.TableCreateRequest{Type: "view", Name: "safe_view", SQL: `SELECT 1 AS value`}); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{`SELECT 2 AS value; SELECT 3`, `SELECT * FROM _meta`, `SELECT * FROM pragma_database_list`, `SELECT name FROM dbstat`, `SELECT load_extension('no')`, `SELECT $a(') ; SELECT $b(')`} {
		if err := CreateTableOrView(db, domain.TableCreateRequest{Type: "view", Name: "denied_view", SQL: query}); err == nil {
			t.Fatalf("view SQL accepted: %s", query)
		}
		if err := UpdateTableOrView(db, "safe_view", domain.TableUpdateRequest{SQL: query}); err == nil {
			t.Fatalf("updated view SQL accepted: %s", query)
		}
		if _, err := ExecuteReadOnly(db, QueryRequest{SQL: query}); err == nil {
			t.Fatalf("read SQL accepted: %s", query)
		}
	}
	// A previously stored view is also checked transitively on every raw read.
	if _, err := db.Exec(`CREATE VIEW old_view AS SELECT * FROM _meta`); err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteReadOnly(db, QueryRequest{SQL: `SELECT * FROM old_view`}); err == nil {
		t.Fatal("legacy view bypassed read policy")
	}
	if _, err := ExecuteReadOnly(db, QueryRequest{SQL: `SELECT * FROM safe_view`}); err != nil {
		t.Fatalf("failed view update changed existing view: %v", err)
	}
	cfg := sqlTestConfig()
	cfg.QueryTimeout = 5
	started := time.Now()
	_, err := ExecuteReadOnlyContext(context.Background(), path, cfg, QueryRequest{SQL: `WITH RECURSIVE r(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM r WHERE x<100000000) SELECT SUM(x) FROM r`})
	var e *domain.Error
	if !errors.As(err, &e) || e.Code != domain.ErrSQLTimeout || time.Since(started) > 5*time.Second {
		t.Fatalf("legacy query timeout: %v", err)
	}
}
