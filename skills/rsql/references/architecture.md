# Architecture

Use this reference for package placement, request tracing, lifecycle changes,
and architecture reviews.

## System shape

```text
cmd/rsql
  -> internal/cli
  -> internal/config
  -> internal/app
  -> internal/httpapi
  -> internal/service
     -> internal/store/control
     -> internal/namespace
        -> internal/store/sqlite
        -> internal/sse
     -> internal/telemetry

client/
  -> public Go package github.com/ValentinKolb/rsql/client
  -> Bun/browser TypeScript package @valentinkolb/rsql-client
```

## Package ownership

| Package | Responsibility |
| --- | --- |
| `cmd/rsql` | Process entrypoint |
| `internal/cli` | Cobra commands and flag binding |
| `internal/config` | Viper environment loading, defaults, validation, redaction |
| `internal/app` | Process startup, server construction, pprof listener, shutdown |
| `internal/httpapi` | Routes, middleware, HTTP decoding and encoding |
| `internal/domain` | Shared wire contracts and typed domain errors |
| `internal/service` | Namespace, schema, row, query, import/export, event, and overview orchestration |
| `internal/store/control` | Global namespace registry in `control.db` |
| `internal/namespace` | Lazy database handles, read/write pools, locks, eviction |
| `internal/store/sqlite` | SQLite DDL, CRUD, filters, read-only SQL, CSV, storage |
| `internal/sse` | Per-namespace subscriber broker |
| `internal/telemetry` | Bounded namespace activity buckets in `telemetry.db` |
| `internal/observability` | Low-cardinality Prometheus process and HTTP metrics |

## Request flow

```text
request
  -> panic recovery
  -> request id
  -> structured access log
  -> Prometheus timing
  -> bearer auth for /v1/*
  -> namespace telemetry classification
  -> route handler
  -> service
  -> registry / namespace manager / SQLite / broker / telemetry
  -> stable JSON, stream, or empty response
```

`GET /healthz` and `GET /metrics` are public. `/v1/*` uses one server-wide
bearer token.

## Control plane and data plane

The control plane is rooted at `/v1/namespaces`. It provisions, lists,
configures, duplicates, imports, exports, and deletes database files.

The data plane is rooted at `/v1/:namespace`. It operates inside one existing
database: schema, indexes, rows, query, changelog, SSE, CSV export, and
overview.

Do not blur this boundary in gateways. The public Go client enforces the same
split with `Admin()` and `Database(namespace)`.

## Namespace lifecycle

- `control.db` stores active namespace name, database path, and creation time.
- Namespace database files live below `data/namespaces/`.
- Handles open lazily on first operation.
- Each handle owns one serialized writer connection and a read pool.
- Idle or excess handles are closed without removing the namespace registry
  record or database file.
- Startup reconciliation purges temporary exports, removes registry rows whose
  files disappeared, reapplies internal schema/config to existing files, and
  reports unregistered files without deleting them.

## Shutdown

`internal/app.RunServe` handles context cancellation, SIGINT, and SIGTERM. It
shuts down the HTTP and optional pprof servers with the configured timeout.
Closing the service flushes telemetry and closes namespace and control stores.
