---
title: Go Client
navTitle: Go
section: Clients
order: 60
description: Use the concurrency-safe Go client for administration, data operations, streaming, and gateways.
tags: [go, client, streaming]
updated: 2026-07-26
---

# Go Client

The `github.com/k2b-dev/rsql` package separates administrative
namespace management from operations bound to one database. The client is safe
for concurrent use and accepts a custom `http.Client`.

```bash
go get github.com/k2b-dev/rsql@v0.4.0
```

## Create a client

```go
import (
    "os"

    rsql "github.com/k2b-dev/rsql"
)

client, err := rsql.New(rsql.Config{
    BaseURL: "http://rsql:8080",
    Token:   os.Getenv("RSQL_API_TOKEN"),
})
if err != nil {
    return err
}
defer client.CloseIdleConnections()
```

`BaseURL` must be an HTTP or HTTPS origin. Set request deadlines through the
operation context or a custom `http.Client`.

## Provision through the admin API

```go
config := rsql.DefaultNamespaceConfig()
config.MaxDBSize = 10 << 30

namespace, err := client.Admin().Namespaces.Create(ctx, rsql.NamespaceDefinition{
    Name:   "tenant-123",
    Config: &config,
})
```

`Admin().Namespaces` provides `Create`, `List`, `Get`, `Update`, `Delete`,
`Duplicate`, `Export`, `ImportDatabase`, and `ImportCSV`.

## Bind database operations

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

A `DatabaseClient` exposes `Overview`, `Query`, `Changelog`, `Events`,
`Tables`, and `Table(name)`. A `TableClient` exposes `Schema`, `Indexes`,
`Rows`, and streaming CSV export.

## Handle API errors

Non-2xx responses return `*rsql.APIError` with `Status`, `Code`, `Message`, and
response headers.

```go
var apiErr *rsql.APIError
if errors.As(err, &apiErr) && apiErr.Status == http.StatusInsufficientStorage {
    // Namespace quota reached.
}
```

## Consume SSE

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

Cancellation propagates through the context. Always close streams and exported
response bodies.

## Forward a database API

`Database.Forward` is a restricted streaming reverse proxy for gateway use. It
permits namespace-scoped table, query, subscribe, changelog, and overview
routes, but never `/v1/namespaces`. It replaces caller authorization, strips
hop-by-hop and sensitive forwarding headers, preserves the raw query, and does
not retry.

```go
db, err := client.Database(resolvedNamespace)
if err != nil {
    return err
}

return db.Forward(w, r, rsql.DatabaseRoute{
    Path: strings.TrimPrefix(r.URL.Path, "/database"),
})
```

The application must authenticate the request and resolve
`resolvedNamespace`; never derive it from an untrusted upstream path.
