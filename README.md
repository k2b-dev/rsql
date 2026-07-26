# rsql

A multi-tenant SQLite HTTP server written in Go.

Each namespace is one isolated SQLite database file, exposed over a REST API for schema operations, row CRUD, read-only SQL queries, SSE change streams, and import/export.

## Features

- Namespace lifecycle: create, list, update config, delete, duplicate
- Schema: tables and views, indexes including FTS5
- Rows: list, get, insert, update, delete, bulk update/delete
- Filter grammar on row lists (`status=eq.active`, `or=(...)`, `like`, `is.null`, …)
- Read-only SQL endpoint (`SELECT` / `WITH`, single or batch)
- SSE change stream per namespace
- Import as `.db` or CSV; export as `.db` (namespace) or CSV (per table, with full filter passthrough)
- Enforced per-namespace storage quotas with safe database imports
- Per-namespace health, storage, traffic, latency, contention, and growth overview
- Bearer-token auth, Prometheus metrics, optional pprof

## Run

```bash
go run ./cmd/rsql serve \
  --listen=127.0.0.1:8080 \
  --api-token=dev-token \
  --data-dir=./data
```

```bash
curl -s http://127.0.0.1:8080/healthz

curl -s -X POST http://127.0.0.1:8080/v1/namespaces \
  -H 'Authorization: Bearer dev-token' \
  -H 'Content-Type: application/json' \
  -d '{"name":"demo"}'
```

Configuration is read from CLI flags, then `RSQL_*` environment variables, then defaults. Print effective config:

```bash
go run ./cmd/rsql config print --format=json --api-token=dev-token
```

## Clients

The repo includes a Go client and a Bun/TypeScript client in [`client/`](./client).

The Go client separates administrative namespace management from namespace-bound database operations:

```go
import rsql "github.com/ValentinKolb/rsql/client"

client, err := rsql.New(rsql.Config{
	BaseURL: "http://127.0.0.1:8080",
	Token:   "dev-token",
})
if err != nil {
	return err
}

db, err := client.Database("demo")
```

The TypeScript client is not published to npm; use it as a workspace dependency or copy `client/src/` into your project.

```ts
import { createRsqlClient } from "./client/src";

const db = createRsqlClient({
  url: "http://127.0.0.1:8080",
  token: "dev-token",
});

await db.namespaces.list();
await db.ns("demo").overview.get({ window: "24h" });
```

## Documentation

- [Quickstart](./docs/quickstart.md)
- [API Overview](./docs/api-overview.md)
- [Development Guide](./docs/development.md)
- [Multi-Tenant Hosting](./docs/hosting.md)
- [Client](./client/README.md)
- [Performance Framework](./client/perf/README.md)
- [Security](./SECURITY.md)

## Status

Pre-1.0. The HTTP API is stabilising; breaking changes between minor versions remain possible until v1.0.

## License

MIT — see [LICENSE](./LICENSE).
