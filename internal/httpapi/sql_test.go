package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/k2b-dev/rsql/internal/domain"
	"github.com/k2b-dev/rsql/internal/store/sqlite"
)

func TestSQLHTTPIsolationAuthAndEvents(t *testing.T) {
	h, svc := newAPIForTest(t)
	for _, ns := range []string{"tenant_one", "tenant_two"} {
		if _, err := svc.CreateNamespace(domain.NamespaceDefinition{Name: ns, Config: domain.NamespaceConfig{JournalMode: "wal", QueryTimeout: 1000, MaxDBSize: 128 << 10}}); err != nil {
			t.Fatal(err)
		}
	}
	call := func(ns, op string, payload any, status int) map[string]any {
		t.Helper()
		rec := doReq(t, h, http.MethodPost, "/v1/"+ns+"/query/"+op, payload, nil)
		if rec.Code != status {
			t.Fatalf("%s %s: %d %s", ns, op, rec.Code, rec.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	call("tenant_one", "exec", map[string]any{"sql": `CREATE TABLE records(id INTEGER PRIMARY KEY,value TEXT UNIQUE)`}, 200)
	call("tenant_two", "exec", map[string]any{"sql": `CREATE TABLE records(id INTEGER PRIMARY KEY,value TEXT UNIQUE)`}, 200)
	id, events, err := svc.Subscribe("tenant_one", []string{"records"})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Unsubscribe("tenant_one", id)
	id2, other, err := svc.Subscribe("tenant_two", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Unsubscribe("tenant_two", id2)
	payload := map[string]any{"statements": []map[string]any{
		{"sql": `INSERT INTO records(value) VALUES (?)`, "params": []any{"injected'); DELETE FROM records; --"}},
		{"sql": `SELECT * FROM records`, "mode": "query"},
	}, "_meta": map[string]any{"actor": "integration"}}
	call("tenant_one", "transaction", payload, 200)
	select {
	case event := <-events:
		if event.Action != "invalidate" || event.Table != "records" || !strings.Contains(string(event.Meta), "integration") {
			t.Fatalf("event: %+v", event)
		}
	default:
		t.Fatal("missing committed SQL event")
	}
	select {
	case e := <-other:
		t.Fatalf("cross-namespace event: %+v", e)
	default:
	}
	failure := call("tenant_one", "transaction", map[string]any{"statements": []map[string]any{
		{"sql": `INSERT INTO records(value) VALUES (?)`, "params": []any{"rollback"}},
		{"sql": `SELECT * FROM _meta`, "mode": "query"},
	}}, 400)
	if failure["statement_index"] != float64(1) || failure["error"] != "sql_forbidden" {
		t.Fatalf("error envelope: %+v", failure)
	}
	select {
	case e := <-events:
		t.Fatalf("rollback event: %+v", e)
	default:
	}
	for ns, want := range map[string]float64{"tenant_one": 1, "tenant_two": 0} {
		result := call(ns, "transaction", map[string]any{"statements": []map[string]any{{"sql": `SELECT COUNT(*) AS count FROM records`, "mode": "query"}}}, 200)
		data := result["results"].([]any)[0].(map[string]any)["data"].([]any)
		if data[0].(map[string]any)["count"] != want {
			t.Fatalf("namespace %s: %+v", ns, result)
		}
	}
	quota := call("tenant_one", "exec", map[string]any{"sql": `INSERT INTO records(value) VALUES(zeroblob(?))`, "params": []any{256 << 10}}, 507)
	if quota["error"] != "quota_exceeded" {
		t.Fatalf("quota error: %+v", quota)
	}
	call("missing", "exec", map[string]any{"sql": "SELECT 1"}, 404)
	for _, op := range []string{"exec", "transaction"} {
		rec := doReq(t, h, http.MethodPost, "/v1/tenant_one/query/"+op, map[string]any{"sql": "SELECT 1"}, map[string]string{"Authorization": "Bearer incorrect"})
		if rec.Code != 401 {
			t.Fatalf("unauthenticated %s: %d", op, rec.Code)
		}
		rec = doReq(t, h, http.MethodGet, "/v1/tenant_one/query/"+op, nil, nil)
		if rec.Code != 405 {
			t.Fatalf("wrong method %s: %d", op, rec.Code)
		}
	}
	logs, err := svc.Changelog("tenant_one", "records", 100, 0)
	if err != nil || len(logs) != 1 || logs[0].Action != "sql" {
		t.Fatalf("atomic schema history: %+v %v", logs, err)
	}
	badParams := call("tenant_one", "exec", map[string]any{"sql": "INSERT INTO records(value) VALUES (?)"}, 400)
	if badParams["error"] != "invalid_request" || badParams["statement_index"] != float64(0) {
		t.Fatalf("parameter error: %+v", badParams)
	}
	// Existing read endpoint and non-transactional batches remain supported.
	rec := doReq(t, h, http.MethodPost, "/v1/tenant_one/query", map[string]any{"statements": []map[string]any{{"sql": "SELECT COUNT(*) AS count FROM records"}}}, nil)
	if rec.Code != 200 {
		t.Fatalf("legacy query: %d %s", rec.Code, rec.Body.String())
	}
}

func TestSQLHTTPRequestLimitsAndReadOnly(t *testing.T) {
	h, svc := newAPIForTest(t)
	if _, err := svc.CreateNamespace(domain.NamespaceDefinition{Name: "bounded", Config: domain.NamespaceConfig{JournalMode: "wal", QueryTimeout: 5}}); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		`{"sql":"SELECT ?","params":["` + strings.Repeat("x", sqlite.MaxSQLRequestBytes) + `"]}`,
		`{"sql":"SELECT 1"}` + strings.Repeat(" ", sqlite.MaxSQLRequestBytes),
	} {
		rec := doReq(t, h, http.MethodPost, "/v1/bounded/query/exec", body, nil)
		if rec.Code != 413 || !strings.Contains(rec.Body.String(), "sql_limit_exceeded") {
			t.Fatalf("body limit: %d %s", rec.Code, rec.Body.String())
		}
	}
	rec := doReq(t, h, http.MethodPost, "/v1/bounded/query/transaction", map[string]any{"statements": []map[string]any{{"sql": `WITH RECURSIVE r(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM r WHERE x<100000000) SELECT SUM(x) FROM r`, "mode": "query"}}}, nil)
	if rec.Code != 408 || !strings.Contains(rec.Body.String(), "sql_timeout") {
		t.Fatalf("timeout: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := svc.UpdateNamespaceConfig("bounded", domain.NamespaceConfig{JournalMode: "wal", ReadOnly: true}); err != nil {
		t.Fatal(err)
	}
	rec = doReq(t, h, http.MethodPost, "/v1/bounded/query/exec", map[string]any{"sql": `CREATE TABLE denied(id INTEGER)`}, nil)
	if rec.Code != 405 || !strings.Contains(rec.Body.String(), "read_only") {
		t.Fatalf("read-only: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := svc.UpdateNamespaceConfig("bounded", domain.NamespaceConfig{JournalMode: "wal"}); err != nil {
		t.Fatal(err)
	}
	rec = doReq(t, h, http.MethodPost, "/v1/bounded/query/exec", map[string]any{"sql": "CREATE TABLE recovered(id INTEGER)"}, nil)
	if rec.Code != 200 {
		t.Fatalf("read-only recovery: %d %s", rec.Code, rec.Body.String())
	}

}

func TestSQLProtectsStructuredMetadata(t *testing.T) {
	h, svc := newAPIForTest(t)
	if _, err := svc.CreateNamespace(domain.NamespaceDefinition{Name: "managed", Config: domain.NamespaceConfig{JournalMode: "wal"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.CreateTableOrView("managed", domain.TableCreateRequest{Type: "table", Name: "records", Columns: []domain.ColumnDefinition{{Name: "value", Type: "text"}}}); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{"ALTER TABLE records ADD COLUMN another TEXT", "DROP TABLE records"} {
		rec := doReq(t, h, http.MethodPost, "/v1/managed/query/exec", map[string]any{"sql": statement}, nil)
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), "sql_forbidden") {
			t.Fatalf("managed DDL: %d %s", rec.Code, rec.Body.String())
		}
	}
	rec := doReq(t, h, http.MethodPost, "/v1/managed/query/exec", map[string]any{"sql": "INSERT INTO records(value) VALUES (?)", "params": []any{"allowed"}}, nil)
	if rec.Code != 200 {
		t.Fatalf("managed DML: %d %s", rec.Code, rec.Body.String())
	}
}
