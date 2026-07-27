---
name: rsql
description: Use, integrate, deploy, or operate the rsql multi-tenant SQLite HTTP server and its Go and TypeScript clients. Use when provisioning namespace databases, designing a tenant gateway, calling schema or row APIs, filtering and querying data, consuming SSE, enforcing storage quotas, reading namespace overview telemetry, importing or exporting databases, configuring the CLI, troubleshooting requests, or planning rsql hosting.
---

# rsql

Help application developers and operators use rsql safely. Start from the
public HTTP and client contracts. Read implementation internals only when they
explain observable behavior or a production problem.

## Choose the path

1. For a first local database, follow the quickstart below and then read
   `references/http-api.md`.
2. For a Go service or gateway, read `references/clients.md` and
   `references/tenancy-and-operations.md`.
3. For a Bun, Node, or browser-compatible TypeScript integration, read
   `references/clients.md`.
4. For schema, rows, filters, search, read-only SQL, or quotas, read
   `references/storage-and-queries.md`.
5. For deployment, backups, telemetry, scaling, or recovery, read
   `references/tenancy-and-operations.md`.
6. For a failing or slow integration, read `references/troubleshooting.md`.
7. Read `references/architecture.md` only when internal request flow or
   concurrency explains a user-visible behavior.

## Non-negotiable trust boundary

The rsql bearer token is a server-wide administrative credential. It can access
every namespace.

- Keep rsql on a private network.
- Never ship the token to an untrusted browser or tenant container.
- Authenticate tenants in a gateway.
- Resolve the allowed namespace from trusted application state.
- Construct the upstream rsql path in the gateway.
- Do not expose `/v1/namespaces` to tenants.

The tenant-facing database surface starts below `/v1/{namespace}`. The
orchestrator owns namespace provisioning and lifecycle.

## Quick local start

```sh
rsql serve \
  --listen=127.0.0.1:8080 \
  --api-token=dev-token \
  --data-dir=./data
```

```sh
curl -s -X POST http://127.0.0.1:8080/v1/namespaces \
  -H 'Authorization: Bearer dev-token' \
  -H 'Content-Type: application/json' \
  -d '{"name":"demo"}'
```

All `/v1/*` calls need `Authorization: Bearer <token>`. `/healthz` and
`/metrics` are public operational endpoints.

## Integration defaults

- Use `Client.Admin().Namespaces` in Go only from an orchestrator.
- Bind application operations with `Client.Database(namespace)`.
- Use `Database.Forward` for streaming gateway passthrough of allowed
  namespace-scoped routes.
- Use `createRsqlClient({ url, token })` for explicit TypeScript configuration.
- Treat the default TypeScript `rsql` export as server-side convenience; it
  reads `RSQL_URL` and `RSQL_API_TOKEN` lazily.
- Handle Go `*rsql.APIError` or TypeScript `RsqlResult<T>` by stable error code.
- Stream exports and SSE; do not buffer complete database files in gateways.
- Apply timeouts and cancellation at the caller.

## Namespace lifecycle

Provision a namespace with the control plane, apply schema, then store its name
in trusted tenant placement data. Use `config.max_db_size` for a page-granular
storage quota. Show tenant storage and health through
`GET /v1/{namespace}/overview`, not the administrative namespace list.

For backups, stream `GET /v1/namespaces/{namespace}/export` to durable storage.
Database and CSV imports remain administrative operations.

## Query guidance

- Prefer row APIs for ordinary application CRUD.
- Use the shared filter grammar for lists, bulk mutations, and CSV export.
- Use parameterized read-only SQL for joins and aggregates.
- Create FTS indexes for large user-facing search workloads.
- Treat SSE as live notification delivery, not a durable queue.

## Response guidance

Expected failures use JSON with `error` and `message`. Important codes include:

- `unauthorized`
- `namespace_not_found`
- `table_not_found`
- `validation_failed`
- `sql_not_read_only`
- `quota_exceeded`
- `database_busy`

Preserve `X-Request-ID` through gateways for log correlation.

## Deep reference routing

- Complete HTTP routes, statuses, payloads, headers, and events:
  `references/http-api.md`
- Go and TypeScript client APIs, streaming, and gateway forwarding:
  `references/clients.md`
- Schema, validation, filtering, query behavior, imports, exports, and quotas:
  `references/storage-and-queries.md`
- Hosting boundary, runtime configuration, telemetry, scaling, and recovery:
  `references/tenancy-and-operations.md`
- Common failures and performance diagnosis:
  `references/troubleshooting.md`
- Optional package and internal request-flow explanation:
  `references/architecture.md`

## If changing rsql itself

This skill is primarily for rsql consumers. If a task explicitly modifies the
rsql source, treat code and tests as authoritative, preserve the control-plane
and data-plane boundary, update both clients when the HTTP contract changes,
and update the matching reference and Fibel page.
