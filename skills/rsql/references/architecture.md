# Architecture

Use this reference only when deployment planning, request tracing, lifecycle
behavior, or a production issue requires implementation detail.

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

github.com/k2b-dev/rsql
  -> public Go client package

client/
  -> Bun/browser TypeScript package @k2b/rsql
```

## Package ownership

| Package | Responsibility |
| --- | --- |
| `github.com/k2b-dev/rsql` | Public Go client |
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
- Namespace names map to stable SHA-256 paths with two directory levels below
  `data/namespaces/`.
- Handles open lazily on first operation; read and writer pools initialize
  independently.
- Idle or excess handles are closed without removing the namespace registry
  record or database file.
- Startup does not enumerate namespace registry rows or tenant files.
- Data-plane requests derive the path without querying `control.db`; lifecycle
  operations continue to use registry metadata.

## Shutdown

`internal/app.RunServe` handles context cancellation, SIGINT, and SIGTERM. It
shuts down the HTTP and optional pprof servers with the configured timeout.
Closing the service flushes telemetry and closes namespace and control stores.

## Deployment responsibility

rsql owns database defaults, process limits, storage layout, API-token
validation, health endpoints, metrics, and shutdown. Native packages own only
the service account, environment file, and systemd integration.

The surrounding deployment owns identity, tenant routing, network policy, TLS,
DNS, proxy selection, secret distribution, backups, monitoring products, and
placement across rsql instances.
