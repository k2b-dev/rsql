---
title: rsql Documentation
navTitle: Overview
section: Start
order: 10
description: Operate isolated SQLite databases through one HTTP service.
tags: [rsql, sqlite, http]
updated: 2026-07-26
---

# rsql

rsql runs many independent SQLite databases behind one HTTP service. A
namespace owns one database file and isolates schema, data, runtime handles,
write serialization, quotas, telemetry, and realtime subscribers.

The project is designed for application backends and hosting gateways that need
to provision databases centrally and expose only database operations to their
users.

## What rsql provides

- A control plane for creating, configuring, duplicating, importing, exporting,
  and deleting namespaces.
- A namespace-scoped data plane for schema management, typed row operations,
  read-only SQL, CSV export, SSE, changelog, and operational overview.
- A Go client with typed APIs, streaming, and a restricted reverse-proxy helper.
- A browser-compatible TypeScript client with stateless hierarchical modules.
- Prometheus metrics and optional pprof endpoints for server operations.

## Trust model

All `/v1/*` routes use one server-wide bearer token. It is an administrative
credential, not a tenant credential. Do not expose rsql directly to untrusted
tenants.

A hosting gateway must authenticate the caller, resolve the allowed namespace,
construct the upstream path, and attach the private rsql token. See
[Multi-tenant hosting](/en/hosting/multi-tenant).

## Next steps

1. Follow the [quickstart](/en/quickstart) to create a database and table.
2. Choose the [Go client](/en/clients/go), [TypeScript client](/en/clients/typescript),
   or the [HTTP API](/en/reference/http-api).
3. Read [Namespaces and isolation](/en/concepts/namespaces) before deploying a
   tenant-facing service.
