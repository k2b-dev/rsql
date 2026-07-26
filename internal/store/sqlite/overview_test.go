package sqlite

import (
	"testing"

	"github.com/k2b-dev/rsql/internal/domain"
)

func TestSchemaSummary(t *testing.T) {
	db, _ := openTestDB(t)
	if err := CreateTableOrView(db, domain.TableCreateRequest{
		Type: "table",
		Name: "documents",
		Columns: []domain.ColumnDefinition{
			{Name: "title", Type: "text"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := CreateTableOrView(db, domain.TableCreateRequest{
		Type: "view",
		Name: "document_titles",
		SQL:  "SELECT title FROM documents",
	}); err != nil {
		t.Fatal(err)
	}
	if err := CreateIndex(db, "documents", domain.IndexCreateRequest{Type: "index", Columns: []string{"title"}}); err != nil {
		t.Fatal(err)
	}
	if err := CreateIndex(db, "documents", domain.IndexCreateRequest{Type: "fts", Columns: []string{"title"}}); err != nil {
		t.Fatal(err)
	}

	summary, err := SchemaSummary(db)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Tables != 1 || summary.Views != 1 || summary.Indexes != 2 || summary.FTSIndexes != 1 {
		t.Fatalf("unexpected schema summary: %#v", summary)
	}
}
