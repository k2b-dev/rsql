# rsql Clients

Go and Bun/TypeScript clients for `rsql`.

## Go

The Go client separates namespace administration from operations inside an
already resolved database.

```go
import rsql "github.com/ValentinKolb/rsql/client"

client, err := rsql.New(rsql.Config{
    BaseURL: "http://rsql:8080",
    Token:   os.Getenv("RSQL_API_TOKEN"),
})
if err != nil {
    return err
}
defer client.CloseIdleConnections()
```

The orchestrator uses the explicit control plane:

```go
config := rsql.DefaultNamespaceConfig()
config.MaxDBSize = 10 << 30

namespace, err := client.Admin().Namespaces.Create(ctx, rsql.NamespaceDefinition{
    Name:   "tenant-123",
    Config: &config,
})
```

Application services bind the resolved namespace once:

```go
db, err := client.Database("tenant-123")
if err != nil {
    return err
}

contacts, err := db.Table("contacts")
if err != nil {
    return err
}

rows, err := contacts.Rows.List(ctx, map[string][]string{
    "status": {"eq.active"},
    "order":  {"name.asc"},
})
```

### Gateway forwarding

`Database.Forward` can only construct namespace-scoped data-plane routes. It
cannot access `/v1/namespaces`. Request and response bodies are streamed without
buffering, including multipart imports, CSV/DB exports, and SSE.

```go
namespace, err := resolveNamespace(r.Context(), authenticatedUserID)
if err != nil {
    return err
}

db, err := client.Database(namespace)
if err != nil {
    return err
}

return db.Forward(w, r, rsql.DatabaseRoute{
    Path: strings.TrimPrefix(r.URL.Path, "/database"),
})
```

The forwarder replaces caller authorization with the internal rsql token,
removes sensitive and hop-by-hop headers, preserves the raw query string, and
does not retry requests.

### Typed SSE

```go
stream, err := db.Events.Subscribe(ctx, rsql.SubscribeOptions{
    Tables: []string{"contacts"},
})
if err != nil {
    return err
}
defer stream.Close()

for stream.Next() {
    event := stream.Event()
    log.Printf("%s %s", event.Table, event.Action)
}
return stream.Err()
```

## TypeScript

### Install

```bash
cd client
bun install
```

### Usage

```ts
import { rsql, createRsqlClient } from "./src";

const explicit = createRsqlClient({
  url: "http://127.0.0.1:8080",
  token: "secret",
});

const ns = explicit.ns("crm");
await ns.tables.list();
const overview = await ns.overview.get({ window: "24h" });
console.log(overview);

// Database exports stay streaming; choose how and where to consume the body.
const exported = await explicit.namespaces.exportDb("crm");
if (exported.ok) {
  await exported.data.body?.pipeTo(destination);
}

// Lazy default client, reads env on first property access only
// Required env: RSQL_URL, RSQL_API_TOKEN
await rsql.namespaces.list();
```

## Tests

```bash
cd client
bun test
```

## Performance Benchmarks

```bash
cd client
bun run perf:fast
bun run perf:deep
bun run perf:report --run=<run-id-or-path>
```

Detailed benchmark docs and scenario list:
`client/perf/README.md`
