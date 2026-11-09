# @k2b/rsql

Type-safe, fetch-based client for the
[rsql](https://github.com/k2b-dev/rsql) HTTP API. It runs in Bun, Node, and
modern browsers without runtime dependencies.

Documentation: [rsql.k2b.dev](https://rsql.k2b.dev)

## Install

```bash
bun add @k2b/rsql
```

## Usage

```ts
import { createRsqlClient } from "@k2b/rsql";

const client = createRsqlClient({
  url: "http://127.0.0.1:8080",
  token: "secret",
});

const contacts = client.ns("crm").table<{
  id: number;
  name: string;
  status: "active" | "inactive";
}>("contacts");

const result = await contacts.rows.list({
  status: "eq.active",
  order: "name.asc",
});

if (result.ok) {
  console.log(result.data);
}
```

Modules are hierarchical and stateless:

```text
client.namespaces
client.ns(name).tables
client.ns(name).table(name).schema
client.ns(name).table(name).indexes
client.ns(name).table(name).rows
client.ns(name).query
client.ns(name).changelog
client.ns(name).overview
client.ns(name).events
```

HTTP errors resolve to `RsqlResult<T>`. Network and runtime failures may reject
the promise. Export methods return the native streaming `Response`; SSE
subscriptions expose an async iterable and explicit `close` function.

## Default Client

The `rsql` export reads `RSQL_URL` and `RSQL_API_TOKEN` lazily on first access:

```ts
import { rsql } from "@k2b/rsql";

const page = await rsql.namespaces.list({ limit: 100 });
```

Imports remain browser-safe because environment variables are not read during
module evaluation. Browser applications should use an explicit client and must
not expose a server-wide administrative token.

Namespace lists use cursor pagination. Pass `page.data.next_cursor` to the next
`list` call, or call `listAll()` explicitly when the complete fleet must be
loaded into memory.

## Development

```bash
bun install
bun run typecheck
bun test
bun run test:package
```

Performance profiles are available through `bun run perf:fast` and
`bun run perf:deep`.

## SQL writes and transactions

```ts
const db = client.ns("demo");
const written = await db.query.exec({
  sql: "INSERT INTO notes(body) VALUES (?)",
  params: ["bound value"],
});
const transaction = await db.query.transaction([
  { sql: "UPDATE notes SET body=? WHERE id=?", params: ["updated", 12] },
  { sql: "SELECT * FROM notes WHERE id=?", params: [12], mode: "query" },
]);
```

`exec` returns `{ changes, lastInsertRowid? }` inside `RsqlResult`.
`transaction` returns ordered `{ results }` after commit; a failure rolls back
all entries and exposes `error.statement_index` (zero-based). `mode` defaults
to `exec`; use `query` for rows or `RETURNING`. `query.batch` remains read-only
and non-atomic. Parameters are bound, and server quotas, timeouts, statement,
request, and result limits apply. See the
[SQL transaction reference](https://rsql.k2b.dev/en/reference/sql-transactions-api).
