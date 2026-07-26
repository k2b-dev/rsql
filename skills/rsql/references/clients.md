# Public clients

Use this reference when integrating the Go or TypeScript client, forwarding
gateway traffic, streaming exports, or consuming SSE.

## Go

Import:

```go
import rsql "github.com/k2b-dev/rsql"
```

Construction is explicit:

```go
client, err := rsql.New(rsql.Config{
	BaseURL: "http://rsql:8080",
	Token:   token,
})
```

Control-plane methods live under `client.Admin().Namespaces`: create, list,
get, update, delete, duplicate, database export, database import, and CSV
import.

Bind data-plane operations once:

```go
db, err := client.Database("tenant_01")
table, err := db.Table("contacts")
rows, err := table.Rows.List(ctx, map[string][]string{"status": {"eq.active"}})
```

`DatabaseClient` exposes overview, query, changelog, events, tables, and bound
table modules. Calls accept `context.Context`. `Client` is concurrency-safe.

Exports return `StreamResponse`; callers close `Body`. `EventStream` is a
pull-based SSE iterator without hidden goroutines.

### Gateway forwarding

`Database.Forward` and `Database.RoundTrip` accept only these relative
data-plane roots:

- `/tables`
- `/query`
- `/subscribe`
- `/changelog`
- `/overview`

They reject control-plane and traversal paths, replace caller authorization
with the private rsql token, copy an allowlist of request headers, strip
hop-by-hop response headers, and flush SSE chunks. They do not provide
namespace DB import/export because those are administrative routes.

## TypeScript

Import:

```ts
import { createRsqlClient, rsql } from "@k2b/rsql";
```

The explicit client shape is:

```ts
const client = createRsqlClient({ url, token, fetch });
await client.namespaces.list();
await client.ns("tenant_01").table<Contact>("contacts").rows.list();
```

`client.namespaces` is the control plane. `client.ns(name)` returns immutable,
hierarchical modules for tables, rows, indexes, schema, query, changelog,
overview, and events.

Methods return `RsqlResult<T>` instead of throwing for non-2xx HTTP responses.
Network failures still reject.

`namespaces.exportDb()` and table CSV export return an unconsumed `Response`.
SSE returns an async iterable and explicit `close`.

The default `rsql` proxy reads `RSQL_URL` and `RSQL_API_TOKEN` on first property
access, not at import time, so the package remains browser-loadable.

## Choose a client

- Use Go for trusted gateways, orchestrators, and high-throughput services.
- Use TypeScript in trusted Bun or Node applications.
- In a browser, use an explicit TypeScript client against an authenticated
  gateway and never expose `RSQL_API_TOKEN`.
- Use direct HTTP when another language or a thin integration is sufficient.
