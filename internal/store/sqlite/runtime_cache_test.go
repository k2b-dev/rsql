package sqlite

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/k2b-dev/rsql/internal/domain"
)

func TestNamespaceRuntimeSharesAndInvalidatesSchema(t *testing.T) {
	writeDB, path := openTestDB(t)
	readDB, err := sql.Open("sqlite", path+"?_pragma=query_only(1)")
	if err != nil {
		t.Fatalf("open read pool: %v", err)
	}
	t.Cleanup(func() { _ = readDB.Close() })
	RegisterNamespacePool(path, readDB)
	RegisterNamespacePool(path, writeDB)
	t.Cleanup(func() {
		UnregisterNamespacePool(readDB)
		UnregisterNamespacePool(writeDB)
	})

	initial := TableSchema{
		Name:    "items",
		Type:    "table",
		Columns: []domain.ColumnDefinition{{Name: "name", Type: "text"}},
	}
	if err := PutMeta(writeDB, "table_schema", "items", initial); err != nil {
		t.Fatalf("put initial schema: %v", err)
	}
	got, err := TableSchemaForValidation(readDB, "items")
	if err != nil {
		t.Fatalf("load initial schema: %v", err)
	}
	if len(got.Columns) != 1 {
		t.Fatalf("initial columns = %d, want 1", len(got.Columns))
	}

	updated := initial
	updated.Columns = append(updated.Columns, domain.ColumnDefinition{Name: "state", Type: "text"})
	raw, err := json.Marshal(updated)
	if err != nil {
		t.Fatalf("marshal updated schema: %v", err)
	}
	if _, err := writeDB.Exec(`
		UPDATE _meta SET metadata_json = ?
		WHERE entity_type = 'table_schema' AND entity_name = 'items'
	`, string(raw)); err != nil {
		t.Fatalf("update schema behind cache: %v", err)
	}

	cached, err := TableSchemaForValidation(readDB, "items")
	if err != nil {
		t.Fatalf("load cached schema: %v", err)
	}
	if len(cached.Columns) != 1 {
		t.Fatalf("cached columns = %d, want 1", len(cached.Columns))
	}

	InvalidateRuntime(writeDB)
	refreshed, err := TableSchemaForValidation(readDB, "items")
	if err != nil {
		t.Fatalf("load refreshed schema: %v", err)
	}
	if len(refreshed.Columns) != 2 {
		t.Fatalf("refreshed columns = %d, want 2", len(refreshed.Columns))
	}
}

func TestNamespaceRuntimeInvalidatesNegativeObjectLookup(t *testing.T) {
	writeDB, path := openTestDB(t)
	readDB, err := sql.Open("sqlite", path+"?_pragma=query_only(1)")
	if err != nil {
		t.Fatalf("open read pool: %v", err)
	}
	t.Cleanup(func() { _ = readDB.Close() })
	RegisterNamespacePool(path, readDB)
	RegisterNamespacePool(path, writeDB)
	t.Cleanup(func() {
		UnregisterNamespacePool(readDB)
		UnregisterNamespacePool(writeDB)
	})

	if TableOrViewExists(readDB, "items") {
		t.Fatal("missing table reported as existing")
	}
	if _, err := writeDB.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if TableOrViewExists(readDB, "items") {
		t.Fatal("negative cache changed without invalidation")
	}

	InvalidateRuntime(writeDB)
	if !TableOrViewExists(readDB, "items") {
		t.Fatal("table missing after cache invalidation")
	}
}

func TestNamespaceRuntimeBoundsPreparedStatements(t *testing.T) {
	db, _ := openTestDB(t)
	RegisterNamespacePool("statement-cache", db)
	t.Cleanup(func() { UnregisterNamespacePool(db) })

	for i := 0; i < maxPreparedStatementsPerPool+5; i++ {
		key := string(rune('a' + i))
		stmt, retained, err := cachedStatement(db, key, `SELECT 1`)
		if err != nil {
			t.Fatalf("prepare %d: %v", i, err)
		}
		if !retained {
			_ = stmt.Close()
		}
	}

	runtime, ok := runtimeForDB(db)
	if !ok {
		t.Fatal("runtime not registered")
	}
	runtime.mu.Lock()
	count := len(runtime.statements)
	runtime.mu.Unlock()
	if count != maxPreparedStatementsPerPool {
		t.Fatalf("retained statements = %d, want %d", count, maxPreparedStatementsPerPool)
	}
}
