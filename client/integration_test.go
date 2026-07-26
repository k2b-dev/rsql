package rsql

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/k2b-dev/rsql/internal/httpapi"
	"github.com/k2b-dev/rsql/internal/observability"
	"github.com/k2b-dev/rsql/internal/service"
)

func TestGoClientFullAPI(t *testing.T) {
	client := integrationClient(t)
	ctx := context.Background()
	config := DefaultNamespaceConfig()
	config.MaxDBSize = 8 << 20

	created, err := client.Admin().Namespaces.Create(ctx, NamespaceDefinition{
		Name:   "go_client",
		Config: &config,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "go_client" {
		t.Fatalf("created namespace = %#v", created)
	}

	namespaces, err := client.Admin().Namespaces.List(ctx)
	if err != nil || len(namespaces) != 1 {
		t.Fatalf("list namespaces: %#v, %v", namespaces, err)
	}
	record, err := client.Admin().Namespaces.Get(ctx, "go_client")
	if err != nil || record.Config == nil || record.Config.MaxDBSize != config.MaxDBSize {
		t.Fatalf("get namespace: %#v, %v", record, err)
	}
	config.QueryTimeout = 12000
	if _, err := client.Admin().Namespaces.Update(ctx, "go_client", config); err != nil {
		t.Fatal(err)
	}

	database, err := client.Database("go_client")
	if err != nil {
		t.Fatal(err)
	}
	if database.Namespace() != "go_client" {
		t.Fatalf("namespace = %q", database.Namespace())
	}

	if err := database.Tables.Create(ctx, TableCreateRequest{
		Type: "table",
		Name: "contacts",
		Columns: []ColumnDefinition{
			{Name: "name", Type: "text", NotNull: true},
			{Name: "status", Type: "select", Options: []string{"active", "inactive", "pending"}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	tables, err := database.Tables.List(ctx)
	if err != nil || len(tables) != 1 {
		t.Fatalf("list tables: %#v, %v", tables, err)
	}
	contacts, err := database.Table("contacts")
	if err != nil {
		t.Fatal(err)
	}
	if contacts.Name() != "contacts" {
		t.Fatalf("table name = %q", contacts.Name())
	}
	if _, err := database.Tables.Get(ctx, "contacts"); err != nil {
		t.Fatal(err)
	}
	if err := database.Tables.Update(ctx, "contacts", TableUpdateRequest{
		AddColumns: []ColumnDefinition{{Name: "email", Type: "text"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := contacts.Indexes.Create(ctx, IndexCreateRequest{
		Type:    "index",
		Name:    "idx_contacts_name",
		Columns: []string{"name"},
	}); err != nil {
		t.Fatal(err)
	}

	inserted, err := contacts.Rows.InsertOne(ctx, map[string]any{
		"name": "Ada", "status": "active",
	}, MutateOptions{Prefer: "return=representation", Meta: json.RawMessage(`{"actor":"test"}`)})
	if err != nil || len(inserted.Data) != 1 {
		t.Fatalf("insert one: %#v, %v", inserted, err)
	}
	if _, err := contacts.Rows.Insert(ctx, []map[string]any{
		{"name": "Bob", "status": "inactive"},
		{"name": "Cara", "status": "active"},
	}, MutateOptions{}); err != nil {
		t.Fatal(err)
	}
	rows, err := contacts.Rows.List(ctx, map[string][]string{"status": {"eq.active"}})
	if err != nil || rows.Meta == nil || rows.Meta.FilterCount != 2 {
		t.Fatalf("list rows: %#v, %v", rows, err)
	}
	row, err := contacts.Rows.Get(ctx, 1)
	if err != nil || row["name"] != "Ada" {
		t.Fatalf("get row: %#v, %v", row, err)
	}
	if _, err := contacts.Rows.Update(ctx, 1, map[string]any{"status": "inactive"}, MutateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := contacts.Rows.BulkUpdate(
		ctx,
		map[string][]string{"status": {"eq.active"}},
		map[string]any{"status": "inactive"},
		MutateOptions{},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := contacts.Rows.Delete(
		ctx,
		2,
		MutateOptions{Meta: json.RawMessage(`{"actor":"test"}`)},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := contacts.Rows.Insert(ctx, []map[string]any{
		{"name": "Delete One", "status": "pending"},
		{"name": "Delete Two", "status": "pending"},
	}, MutateOptions{}); err != nil {
		t.Fatal(err)
	}
	deleted, err := contacts.Rows.BulkDelete(
		ctx,
		map[string][]string{"status": {"eq.pending"}},
		MutateOptions{Prefer: PreferReturnRepresentation},
	)
	if err != nil || len(deleted.Data) != 2 {
		t.Fatalf("bulk delete: %#v, %v", deleted, err)
	}

	queryResult, err := database.Query.Run(ctx, QueryRequest{
		SQL:    "SELECT COUNT(*) AS count FROM contacts",
		Params: []any{},
	})
	if err != nil || queryResult["data"] == nil {
		t.Fatalf("query: %#v, %v", queryResult, err)
	}
	if _, err := database.Query.Batch(ctx, []QueryStatement{{SQL: "SELECT 1 AS value"}}); err != nil {
		t.Fatal(err)
	}
	changelog, err := database.Changelog.List(ctx, "contacts", 50, 0)
	if err != nil || len(changelog) == 0 {
		t.Fatalf("changelog: %#v, %v", changelog, err)
	}
	overview, err := database.Overview.Get(ctx, Window1Hour)
	if err != nil || overview.Schema.Tables != 1 {
		t.Fatalf("overview: %#v, %v", overview, err)
	}

	csv, err := contacts.Export(ctx, TableExportOptions{
		Query: map[string][]string{"order": {"id.asc"}},
		BOM:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	csvBody, err := io.ReadAll(csv.Body)
	_ = csv.Body.Close()
	if err != nil || !bytes.HasPrefix(csvBody, []byte{0xef, 0xbb, 0xbf}) ||
		!strings.Contains(string(csvBody), "Ada") {
		t.Fatalf("CSV export = %q, %v", csvBody, err)
	}

	streamCtx, cancelStream := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelStream()
	stream, err := database.Events.Subscribe(streamCtx, SubscribeOptions{Tables: []string{"contacts"}})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if _, err := contacts.Rows.InsertOne(
		ctx,
		map[string]any{"name": "Stream", "status": "active"},
		MutateOptions{},
	); err != nil {
		t.Fatal(err)
	}
	if !stream.Next() || stream.Event().Table != "contacts" || stream.Event().Action != "insert" {
		t.Fatalf("SSE event = %#v, error = %v", stream.Event(), stream.Err())
	}
	if got := stream.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("SSE content type = %q", got)
	}

	exported, err := client.Admin().Namespaces.Export(ctx, "go_client")
	if err != nil {
		t.Fatal(err)
	}
	databaseBytes, err := io.ReadAll(exported.Body)
	_ = exported.Body.Close()
	if err != nil || len(databaseBytes) == 0 {
		t.Fatalf("database export: %d bytes, %v", len(databaseBytes), err)
	}

	if err := client.Admin().Namespaces.Duplicate(ctx, "go_client", "go_copy"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Admin().Namespaces.Create(ctx, NamespaceDefinition{Name: "go_import"}); err != nil {
		t.Fatal(err)
	}
	if err := client.Admin().Namespaces.ImportDatabase(ctx, "go_import", ImportOptions{
		Filename:    "database.db",
		ContentType: "application/octet-stream",
		Body:        bytes.NewReader(databaseBytes),
	}); err != nil {
		t.Fatal(err)
	}
	importedDatabase, err := client.Database("go_import")
	if err != nil {
		t.Fatal(err)
	}
	importedContacts, err := importedDatabase.Table("contacts")
	if err != nil {
		t.Fatal(err)
	}
	importedRows, err := importedContacts.Rows.List(ctx, nil)
	if err != nil || len(importedRows.Data) == 0 {
		t.Fatalf("imported rows: %#v, %v", importedRows, err)
	}

	if err := importedDatabase.Tables.Create(ctx, TableCreateRequest{
		Type:    "table",
		Name:    "csv_rows",
		Columns: []ColumnDefinition{{Name: "name", Type: "text"}},
	}); err != nil {
		t.Fatal(err)
	}
	csvImport, err := client.Admin().Namespaces.ImportCSV(ctx, "go_import", "csv_rows", ImportOptions{
		Filename:    "rows.csv",
		ContentType: "text/csv",
		Body:        strings.NewReader("name\nImported\n"),
	})
	if err != nil || csvImport.Inserted != 1 {
		t.Fatalf("CSV import: %#v, %v", csvImport, err)
	}

	if err := contacts.Indexes.Delete(ctx, "idx_contacts_name", nil); err != nil {
		t.Fatal(err)
	}
	if err := database.Tables.Delete(ctx, "contacts", nil); err != nil {
		t.Fatal(err)
	}
	for _, namespace := range []string{"go_client", "go_copy", "go_import"} {
		if err := client.Admin().Namespaces.Delete(ctx, namespace); err != nil {
			t.Fatalf("delete %s: %v", namespace, err)
		}
	}
}

func integrationClient(t *testing.T) *Client {
	t.Helper()
	svc, err := service.New(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	handler := httpapi.NewHandler(httpapi.Dependencies{
		Token:   "secret",
		Metrics: observability.NewMetrics(),
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Service: svc,
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return mustClient(t, server.URL)
}
