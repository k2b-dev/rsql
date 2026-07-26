---
title: Architecture
navTitle: Architecture
section: Project
order: 190
description: Understand the rsql process, package boundaries, storage layout, and request flow.
tags: [architecture, internals, go]
updated: 2026-07-26
---

# Architecture

rsql is one Go process that owns a control database, a telemetry database, and
one SQLite file per namespace.

```txt
client or gateway
  -> HTTP middleware
  -> transport handler
  -> service orchestration
  -> namespace manager
  -> SQLite control, telemetry, or tenant store
  -> SSE broker and operational metrics
```

## Package responsibilities

| Package | Responsibility |
| --- | --- |
| `cmd/rsql` | Process entrypoint |
| `internal/cli` | Cobra commands |
| `internal/config` | Viper and environment configuration |
| `internal/app` | Server lifecycle and graceful shutdown |
| `internal/httpapi` | Routes, middleware, request/response transport |
| `internal/service` | Business orchestration and error mapping |
| `internal/store/control` | Namespace registry |
| `internal/store/sqlite` | Tenant schema, rows, queries, imports, exports |
| `internal/namespace` | Lazy handles, pools, writer locks, idle eviction |
| `internal/telemetry` | Bounded namespace activity storage and rollups |
| `internal/sse` | Namespace-local subscriber fanout |
| `internal/observability` | Low-cardinality Prometheus metrics |
| `client` | Public Go client and TypeScript package |

Transport code decodes requests and maps errors. Service code coordinates
namespace lifecycle and mutations. SQLite packages own SQL construction,
validation, transactions, and storage details.

## Files on disk

The configured data directory contains the control registry, telemetry store,
namespace databases, and temporary export files. Namespace files are never
automatically deleted merely because they are absent from the registry; startup
reconciliation reports such files for operator review.

## Concurrency

Each namespace has one serialized writer and a small read pool. Independent
namespaces can execute concurrently. This matches SQLite's single-writer model
while avoiding a server-wide database lock.

## API safety

Identifiers are validated before SQL construction. Values use parameters.
Structured writes validate logical column types. The raw SQL endpoint accepts
only constrained read-only statements and blocks internal objects.

For implementation work, use the repository-local `skills/rsql` agent skill,
which routes agents to source-grounded architecture and API references.
