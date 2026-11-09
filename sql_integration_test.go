package rsql

import (
	"context"
	"errors"
	"testing"
)

func TestGoClientSQLTransaction(t *testing.T) {
	client := integrationClient(t)
	ctx := context.Background()
	if _, err := client.Admin().Namespaces.Create(ctx, NamespaceDefinition{Name: "sql_client"}); err != nil {
		t.Fatal(err)
	}
	db, err := client.Database("sql_client")
	if err != nil {
		t.Fatal(err)
	}
	if result, err := db.Query.Exec(ctx, QueryStatement{SQL: `CREATE TABLE records(id INTEGER PRIMARY KEY,value TEXT UNIQUE)`}); err != nil || result.Changes != 0 {
		t.Fatalf("DDL: %+v %v", result, err)
	}
	value := "x'); DROP TABLE records; --"
	result, err := db.Query.Exec(ctx, QueryStatement{SQL: `INSERT INTO records(value) VALUES(?)`, Params: []any{value}})
	if err != nil || result.Changes != 1 || result.LastInsertRowid == nil || *result.LastInsertRowid != 1 {
		t.Fatalf("exec: %+v %v", result, err)
	}
	out, err := db.Query.Transaction(ctx, []TransactionStatement{
		{SQL: `UPDATE records SET value=? WHERE id=?`, Params: []any{"updated", 1}},
		{SQL: `SELECT value FROM records WHERE id=?`, Params: []any{1}, Mode: "query"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 2 || out.Results[0].ExecResult == nil || out.Results[0].Changes != 1 || out.Results[1].Data == nil || (*out.Results[1].Data)[0]["value"] != "updated" {
		t.Fatalf("results: %+v", out.Results)
	}
	_, err = db.Query.Transaction(ctx, []TransactionStatement{
		{SQL: `INSERT INTO records(value) VALUES(?)`, Params: []any{"rollback"}},
		{SQL: `INSERT INTO records(value) VALUES(?)`, Params: []any{"updated"}},
	})
	var api *APIError
	if !errors.As(err, &api) || api.Code != "conflict" || api.StatementIndex == nil || *api.StatementIndex != 1 {
		t.Fatalf("failure: %#v", err)
	}
	check, err := db.Query.Run(ctx, QueryRequest{SQL: `SELECT COUNT(*) AS count FROM records`})
	if err != nil {
		t.Fatal(err)
	}
	if check["data"].([]any)[0].(map[string]any)["count"] != float64(1) {
		t.Fatalf("rollback: %+v", check)
	}
}
