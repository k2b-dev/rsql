package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ValentinKolb/rsql/internal/domain"
	"github.com/ValentinKolb/rsql/internal/store/control"
	"github.com/ValentinKolb/rsql/internal/store/sqlite"
	"github.com/ValentinKolb/rsql/internal/telemetry"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	svc, err := New(t.TempDir(), 0)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

func TestNamespaceLifecycle(t *testing.T) {
	svc := newTestService(t)

	cfg := domain.NamespaceConfig{JournalMode: "wal", BusyTimeout: 5000, QueryTimeout: 10000, ForeignKeys: domain.BoolPtr(true)}
	created, err := svc.CreateNamespace(domain.NamespaceDefinition{Name: "workspace1", Config: cfg})
	if err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	if created["name"] != "workspace1" {
		t.Fatalf("unexpected namespace name: %v", created["name"])
	}

	list, err := svc.ListNamespaces()
	if err != nil {
		t.Fatalf("list namespaces: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 namespace, got %d", len(list))
	}

	info, err := svc.GetNamespace("workspace1")
	if err != nil {
		t.Fatalf("get namespace: %v", err)
	}
	if info["name"] != "workspace1" {
		t.Fatalf("unexpected get namespace result: %v", info)
	}

	updatedCfg := domain.NamespaceConfig{JournalMode: "wal", BusyTimeout: 6000, QueryTimeout: 15000, ForeignKeys: domain.BoolPtr(true), ReadOnly: false}
	if _, err := svc.UpdateNamespaceConfig("workspace1", updatedCfg); err != nil {
		t.Fatalf("update namespace config: %v", err)
	}

	readCfg, err := svc.GetNamespaceConfig("workspace1")
	if err != nil {
		t.Fatalf("get namespace config: %v", err)
	}
	if readCfg.BusyTimeout != 6000 {
		t.Fatalf("expected busy_timeout 6000, got %d", readCfg.BusyTimeout)
	}

	if err := svc.DeleteNamespace("workspace1"); err != nil {
		t.Fatalf("delete namespace: %v", err)
	}

	if _, err := svc.GetNamespace("workspace1"); err == nil {
		t.Fatal("expected not found after delete")
	}
}

func TestListNamespacesDoesNotOpenTenantDatabases(t *testing.T) {
	svc := newTestService(t)
	for i := range 250 {
		name := fmt.Sprintf("tenant_%03d", i)
		if err := svc.registry.Create(name, control.PathFor(svc.dataDir, name)); err != nil {
			t.Fatalf("create registry record %s: %v", name, err)
		}
	}

	namespaces, err := svc.ListNamespaces()
	if err != nil {
		t.Fatalf("list namespaces: %v", err)
	}
	if len(namespaces) != 250 {
		t.Fatalf("namespace count = %d, want 250", len(namespaces))
	}
	if got := svc.ns.OpenCount(); got != 0 {
		t.Fatalf("list opened %d tenant databases", got)
	}
}

func TestOverviewTelemetryIsNamespaceIsolated(t *testing.T) {
	svc := newTestService(t)
	config := defaultNamespaceConfig()
	for _, name := range []string{"tenant_a", "tenant_b"} {
		if _, err := svc.CreateNamespace(domain.NamespaceDefinition{Name: name, Config: config}); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	svc.RecordRequest(telemetry.Event{Namespace: "tenant_a", Operation: "rows.list", Status: 200, Duration: time.Millisecond})
	svc.RecordRequest(telemetry.Event{Namespace: "tenant_b", Operation: "rows.insert", Write: true, Status: 507, QuotaRejected: true, Duration: 2 * time.Millisecond})

	a, err := svc.Overview("tenant_a", "1h")
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Overview("tenant_b", "1h")
	if err != nil {
		t.Fatal(err)
	}
	if a.Activity.Reads != 1 || a.Activity.Writes != 0 || a.WriteContention.QuotaRejects != 0 {
		t.Fatalf("unexpected tenant A activity: %#v", a.Activity)
	}
	if b.Activity.Reads != 0 || b.Activity.Writes != 1 || b.WriteContention.QuotaRejects != 1 {
		t.Fatalf("unexpected tenant B activity: %#v", b.Activity)
	}
	if _, err := svc.Overview("tenant_a", "invalid"); err == nil {
		t.Fatal("expected invalid overview window error")
	}
}

func TestRejectedQuotaUpdateKeepsPreviousConfig(t *testing.T) {
	svc := newTestService(t)
	config := defaultNamespaceConfig()
	if _, err := svc.CreateNamespace(domain.NamespaceDefinition{Name: "quota_update", Config: config}); err != nil {
		t.Fatal(err)
	}
	tooSmall := config
	tooSmall.MaxDBSize = 1
	if _, err := svc.UpdateNamespaceConfig("quota_update", tooSmall); err == nil {
		t.Fatal("expected quota update to fail")
	}
	stored, err := svc.GetNamespaceConfig("quota_update")
	if err != nil {
		t.Fatal(err)
	}
	if stored.MaxDBSize != config.MaxDBSize {
		t.Fatalf("stored quota = %d, want %d", stored.MaxDBSize, config.MaxDBSize)
	}
}

func TestSchemaRowsQueryAndOverview(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.CreateNamespace(domain.NamespaceDefinition{Name: "work", Config: domain.NamespaceConfig{JournalMode: "wal", BusyTimeout: 5000, QueryTimeout: 10000, ForeignKeys: domain.BoolPtr(true)}})
	if err != nil {
		t.Fatalf("create namespace: %v", err)
	}

	createReq := domain.TableCreateRequest{
		Type: "table",
		Name: "kunden",
		Columns: []domain.ColumnDefinition{
			{Name: "firma", Type: "text", NotNull: true},
			{Name: "umsatz", Type: "real", Min: floatPtr(0)},
			{Name: "email", Type: "text", Unique: true},
			{Name: "status", Type: "select", Options: []string{"active", "inactive"}},
			{Name: "bezahlt", Type: "boolean", Default: false},
		},
	}
	if err := svc.CreateTableOrView("work", createReq); err != nil {
		t.Fatalf("create table: %v", err)
	}

	tables, err := svc.TablesList("work")
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	if len(tables) != 1 || tables[0]["name"] != "kunden" {
		t.Fatalf("unexpected tables: %#v", tables)
	}

	insertRes, err := svc.InsertRows("work", "kunden", []map[string]any{{
		"firma":  "Muller GmbH",
		"umsatz": 1200.5,
		"email":  "info@mueller.de",
		"status": "active",
	}}, "return=representation", json.RawMessage(`{"user_id":"test"}`))
	if err != nil {
		t.Fatalf("insert row: %v", err)
	}
	insertMap := insertRes.(map[string]any)
	if len(insertMap["data"].([]map[string]any)) != 1 {
		t.Fatalf("unexpected insert data: %#v", insertMap)
	}

	_, err = svc.InsertRows("work", "kunden", []map[string]any{{
		"firma":  "Schmidt AG",
		"umsatz": 2200.0,
		"email":  "info@schmidt.de",
		"status": "inactive",
	}, {
		"firma":  "Weber KG",
		"umsatz": 500.0,
		"email":  "info@weber.de",
		"status": "active",
	}}, "", nil)
	if err != nil {
		t.Fatalf("bulk insert: %v", err)
	}

	rowsRes, err := svc.ListRows("work", "kunden", map[string][]string{"status": {"eq.active"}, "limit": {"10"}, "offset": {"0"}})
	if err != nil {
		t.Fatalf("list rows: %v", err)
	}
	rowsWrapped := rowsRes.(domain.ListResponse[map[string]any])
	if rowsWrapped.Meta.FilterCount != 2 {
		t.Fatalf("unexpected filter_count: %d", rowsWrapped.Meta.FilterCount)
	}

	row1, err := svc.GetRow("work", "kunden", int64(1))
	if err != nil {
		t.Fatalf("get row: %v", err)
	}
	if row1["firma"] != "Muller GmbH" {
		t.Fatalf("unexpected row data: %#v", row1)
	}

	if _, err := svc.UpdateRow("work", "kunden", int64(1), map[string]any{"umsatz": map[string]any{"$increment": 200.0}}, "", nil); err != nil {
		t.Fatalf("update row: %v", err)
	}

	queryRes, err := svc.Query("work", sqlite.QueryRequest{SQL: "SELECT COUNT(*) AS c FROM kunden", Params: []any{}})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	queryMap := queryRes.(map[string]any)
	if len(queryMap["data"].([]map[string]any)) != 1 {
		t.Fatalf("unexpected query result: %#v", queryMap)
	}

	overview, err := svc.Overview("work", "24h")
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if overview.Schema.Tables != 1 || overview.Storage.AllocatedBytes <= 0 {
		t.Fatalf("unexpected overview: %#v", overview)
	}

	changelog, err := svc.Changelog("work", "kunden", 50, 0)
	if err != nil {
		t.Fatalf("changelog: %v", err)
	}
	if len(changelog) == 0 {
		t.Fatal("expected changelog entries")
	}

	if _, err := svc.DeleteRow("work", "kunden", int64(2), "", nil); err != nil {
		t.Fatalf("delete row: %v", err)
	}
}

func TestSSEAndDupExportImport(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.CreateNamespace(domain.NamespaceDefinition{Name: "src", Config: domain.NamespaceConfig{JournalMode: "wal", BusyTimeout: 5000, QueryTimeout: 10000, ForeignKeys: domain.BoolPtr(true)}})
	if err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	if err := svc.CreateTableOrView("src", domain.TableCreateRequest{Type: "table", Name: "items", Columns: []domain.ColumnDefinition{{Name: "name", Type: "text", NotNull: true}}}); err != nil {
		t.Fatalf("create table: %v", err)
	}

	subID, ch, err := svc.Subscribe("src", []string{"items"})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer svc.Unsubscribe("src", subID)

	if _, err := svc.InsertRows("src", "items", []map[string]any{{"name": "A"}}, "", json.RawMessage(`{"source":"test"}`)); err != nil {
		t.Fatalf("insert for sse: %v", err)
	}

	select {
	case ev := <-ch:
		if ev.Action != "insert" || ev.Table != "items" {
			t.Fatalf("unexpected event: %#v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for sse event")
	}

	dup, err := svc.DuplicateNamespace("src", "copy")
	if err != nil {
		t.Fatalf("duplicate namespace: %v", err)
	}
	if dup["target"] != "copy" {
		t.Fatalf("unexpected duplicate result: %#v", dup)
	}

	exportPath, err := svc.ExportNamespace("src")
	if err != nil {
		t.Fatalf("export namespace: %v", err)
	}
	content, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("read exported db: %v", err)
	}

	if err := svc.ImportNamespaceDB("copy", bytes.NewReader(content)); err != nil {
		t.Fatalf("import db: %v", err)
	}

	rows, err := svc.ListRows("copy", "items", map[string][]string{"limit": {"10"}, "offset": {"0"}})
	if err != nil {
		t.Fatalf("list copied rows: %v", err)
	}
	if rows.(domain.ListResponse[map[string]any]).Meta.TotalCount == 0 {
		t.Fatal("expected copied data after import")
	}
}

func TestOversizedImportPreservesNamespace(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.CreateNamespace(domain.NamespaceDefinition{
		Name: "limited",
		Config: domain.NamespaceConfig{
			JournalMode:  "wal",
			BusyTimeout:  5000,
			MaxDBSize:    64 << 10,
			QueryTimeout: 10000,
			ForeignKeys:  domain.BoolPtr(true),
		},
	})
	if err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	if err := svc.CreateTableOrView("limited", domain.TableCreateRequest{
		Type: "table",
		Name: "items",
		Columns: []domain.ColumnDefinition{
			{Name: "name", Type: "text", NotNull: true},
		},
	}); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := svc.InsertRows("limited", "items", []map[string]any{{"name": "preserve"}}, "", nil); err != nil {
		t.Fatalf("insert seed: %v", err)
	}

	err = svc.ImportNamespaceDB("limited", bytes.NewReader(make([]byte, (64<<10)+1)))
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != domain.ErrQuotaExceeded {
		t.Fatalf("expected quota error, got %v", err)
	}

	row, err := svc.GetRow("limited", "items", int64(1))
	if err != nil {
		t.Fatalf("read preserved row: %v", err)
	}
	if row["name"] != "preserve" {
		t.Fatalf("unexpected preserved row: %#v", row)
	}
}

func TestSSEViewInvalidationEvent(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.CreateNamespace(domain.NamespaceDefinition{Name: "ws", Config: domain.NamespaceConfig{JournalMode: "wal", BusyTimeout: 5000, QueryTimeout: 10000, ForeignKeys: domain.BoolPtr(true)}})
	if err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	if err := svc.CreateTableOrView("ws", domain.TableCreateRequest{
		Type: "table",
		Name: "kunden",
		Columns: []domain.ColumnDefinition{
			{Name: "firma", Type: "text"},
		},
	}); err != nil {
		t.Fatalf("create source table: %v", err)
	}
	if err := svc.CreateTableOrView("ws", domain.TableCreateRequest{
		Type: "view",
		Name: "v_kunden",
		SQL:  "SELECT firma FROM kunden",
	}); err != nil {
		t.Fatalf("create dependent view: %v", err)
	}

	subID, ch, err := svc.Subscribe("ws", []string{"v_kunden"})
	if err != nil {
		t.Fatalf("subscribe view: %v", err)
	}
	defer svc.Unsubscribe("ws", subID)

	if _, err := svc.InsertRows("ws", "kunden", []map[string]any{{"firma": "A"}}, "", nil); err != nil {
		t.Fatalf("insert row: %v", err)
	}

	select {
	case ev := <-ch:
		if ev.Action != "source_changed" || ev.Table != "v_kunden" {
			t.Fatalf("unexpected invalidation event: %#v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for view invalidation event")
	}
}

func TestImportNamespaceCSVSuccess(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.CreateNamespace(domain.NamespaceDefinition{Name: "csvok", Config: domain.NamespaceConfig{JournalMode: "wal", BusyTimeout: 5000, QueryTimeout: 10000, ForeignKeys: domain.BoolPtr(true)}})
	if err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	if err := svc.CreateTableOrView("csvok", domain.TableCreateRequest{
		Type: "table",
		Name: "items",
		Columns: []domain.ColumnDefinition{
			{Name: "name", Type: "text", NotNull: true},
			{Name: "state", Type: "select", Options: []string{"open", "done"}},
		},
	}); err != nil {
		t.Fatalf("create table: %v", err)
	}

	_, err = svc.ImportNamespaceCSV("csvok", "items", strings.NewReader("name,state\nCSV Item,open\n"), nil)
	if err != nil {
		t.Fatalf("csv import should succeed, got: %v", err)
	}
}

func TestExportTableCSV(t *testing.T) {
	svc := newTestService(t)

	if _, err := svc.CreateNamespace(domain.NamespaceDefinition{
		Name:   "exp",
		Config: domain.NamespaceConfig{JournalMode: "wal", BusyTimeout: 5000, QueryTimeout: 10000, ForeignKeys: domain.BoolPtr(true)},
	}); err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	if err := svc.CreateTableOrView("exp", domain.TableCreateRequest{
		Type: "table",
		Name: "items",
		Columns: []domain.ColumnDefinition{
			{Name: "label", Type: "text"},
			{Name: "score", Type: "integer"},
			{Name: "active", Type: "boolean"},
		},
	}); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := svc.InsertRows("exp", "items", []map[string]any{
		{"label": "Ada", "score": 10, "active": true},
		{"label": "Bob", "score": 20, "active": false},
	}, "", nil); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var buf strings.Builder
	if err := svc.ExportTableCSV("exp", "items", map[string][]string{
		"select": {"label,score,active"},
		"order":  {"id.asc"},
	}, &buf); err != nil {
		t.Fatalf("export: %v", err)
	}
	got := buf.String()
	wantPrefix := "label,score,active\nAda,10,true\nBob,20,false\n"
	if got != wantPrefix {
		t.Fatalf("export body mismatch:\nwant=%q\ngot =%q", wantPrefix, got)
	}

	// Missing namespace → typed 404.
	err := svc.ExportTableCSV("missing", "items", nil, &strings.Builder{})
	assertDomainCode(t, err, domain.ErrNamespaceNotFound)

	// Missing table → typed 404 via mapErr(sql.ErrNoRows).
	err = svc.ExportTableCSV("exp", "missing_table", nil, &strings.Builder{})
	assertDomainCode(t, err, domain.ErrNotFound)
}

func floatPtr(v float64) *float64 { return &v }
