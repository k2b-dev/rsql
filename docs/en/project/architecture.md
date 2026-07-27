---
title: Architecture
navTitle: Architecture
section: Project
order: 190
description: Understand the rsql process, package boundaries, storage layout, and request flow.
tags: [architecture, internals, go]
updated: 2026-07-27
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
namespace databases, and temporary export files. Namespace names map to stable
SHA-256 paths with two directory-sharding levels below `data/namespaces`.

Startup opens the control and telemetry stores but does not enumerate registry
rows or tenant files. Data-plane requests derive the namespace path directly;
control-plane operations use the registry for lifecycle metadata.

## Concurrency

Each namespace lazily opens one serialized writer and a small read pool. A
read-only namespace does not retain a writer connection. Independent namespaces
can execute concurrently. This matches SQLite's single-writer model while
avoiding a server-wide database lock.

## API safety

Identifiers are validated before SQL construction. Values use parameters.
Structured writes validate logical column types. The raw SQL endpoint accepts
only constrained read-only statements and blocks internal objects.

## Deployment boundary

rsql owns database behavior, runtime limits, storage layout, authentication of
its private API token, health endpoints, metrics, and graceful shutdown. Native
packages provide the service account, systemd unit, and environment file needed
to run that process.

The deployment owns user authentication, tenant routing, network policy, TLS,
DNS, proxy selection, secret distribution, backup scheduling, monitoring
products, and placement across rsql instances. None of those systems is bundled
or configured by rsql.

For implementation work, use the repository-local `skills/rsql` agent skill,
which routes agents to source-grounded architecture and API references.
