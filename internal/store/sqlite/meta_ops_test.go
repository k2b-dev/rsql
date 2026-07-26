package sqlite

import (
	"encoding/json"
	"testing"

	"github.com/k2b-dev/rsql/internal/domain"
)

func TestListChangelog(t *testing.T) {
	db, _ := openTestDB(t)

	if err := CreateTableOrView(db, domain.TableCreateRequest{
		Type: "table",
		Name: "kunden",
		Columns: []domain.ColumnDefinition{
			{Name: "firma", Type: "text"},
		},
	}); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if err := CreateTableOrView(db, domain.TableCreateRequest{
		Type: "view",
		Name: "v_kunden",
		SQL:  "SELECT firma FROM kunden",
	}); err != nil {
		t.Fatalf("create view: %v", err)
	}
	if _, _, err := InsertRows(db, "kunden", []map[string]any{{"firma": "A"}}, ""); err != nil {
		t.Fatalf("insert row: %v", err)
	}
	if err := AppendSchemaLog(db, "custom", "kunden", map[string]any{"x": 1}, json.RawMessage(`{"u":"1"}`)); err != nil {
		t.Fatalf("append schema log: %v", err)
	}

	logs, err := ListChangelog(db, "", -1, -1)
	if err != nil {
		t.Fatalf("list changelog: %v", err)
	}
	if len(logs) == 0 {
		t.Fatal("expected changelog entries")
	}

	filtered, err := ListChangelog(db, "kunden", 10, 0)
	if err != nil {
		t.Fatalf("list changelog filtered: %v", err)
	}
	if len(filtered) == 0 {
		t.Fatal("expected filtered changelog entries")
	}

}

func TestMetaOpsErrorPaths(t *testing.T) {
	db, _ := openTestDB(t)

	if _, err := db.Exec(`DROP TABLE _schema_log`); err != nil {
		t.Fatalf("drop schema log: %v", err)
	}
	if _, err := ListChangelog(db, "", 10, 0); err == nil {
		t.Fatal("expected changelog query error")
	}
}
