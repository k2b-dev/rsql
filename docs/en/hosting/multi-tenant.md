---
title: Multi-Tenant Hosting
navTitle: Multi-tenant
section: Hosting
order: 80
description: Place rsql behind an authenticated gateway and preserve namespace isolation.
tags: [hosting, tenants, gateway]
updated: 2026-07-26
---

# Multi-Tenant Hosting

rsql can serve as the database control and data plane behind a multi-tenant
product. The orchestrator owns provisioning; an authenticated gateway exposes a
restricted database API to application containers or end users.

## Required gateway boundary

The rsql bearer token is server-wide and can access every namespace. Keep rsql
on a private network and implement this sequence at the gateway:

1. Authenticate the caller.
2. Resolve the caller to exactly one allowed namespace.
3. Construct the rsql path from the resolved namespace.
4. Reject tenant-controlled namespace path segments.
5. Add the private rsql bearer token.
6. Apply request, rate, response-size, and stream limits.

Do not forward `/v1/namespaces` to tenants. The Go client's
`Database.Forward` enforces the data-plane route boundary, but authentication
and namespace resolution remain application responsibilities.

## Provision a tenant

```json
{
  "name": "tenant_01",
  "config": {
    "journal_mode": "wal",
    "busy_timeout": 5000,
    "query_timeout": 10000,
    "foreign_keys": true,
    "read_only": false,
    "max_db_size": 1073741824
  }
}
```

Provision through `POST /v1/namespaces`, migrate the schema, then store the
namespace ID in the orchestrator's tenant record.

## Isolation guarantees

Each namespace uses a separate database file, connection pools, writer lock,
quota, telemetry aggregation, and SSE broker state. SQL cannot attach databases
or access rsql internal objects through the query endpoint.

This is in-process database isolation, not a hostile-code sandbox. CPU, memory,
request rate, query complexity, subscriber count, and host disk remain shared
resources. Enforce those limits in the gateway and scheduler, and shard hot
tenant groups across rsql processes.

## Scaling to hundreds of databases

Provisioned databases do not stay open merely because they exist. rsql opens
them on demand and retains a bounded idle working set. Hundreds of mostly idle
databases are therefore practical.

If hundreds of namespaces are continuously active, distribute them across
multiple rsql instances. One data directory belongs to one running rsql
process; rsql does not coordinate shared storage or multi-primary access.

Continue with [Quotas and overview](/en/hosting/quotas-and-overview) and
[Backups and scaling](/en/hosting/backups-and-scaling).
