---
title: Backups and Scaling
navTitle: Backups and scaling
section: Hosting
order: 100
description: Back up, restore, move, and shard namespace databases safely.
tags: [backup, restore, scaling]
updated: 2026-07-27
---

# Backups and Scaling

The namespace export endpoint produces a consistent SQLite database stream.
The import endpoint accepts a database file for an existing namespace and
validates it before replacement.

## Database backup

```http
GET /v1/namespaces/tenant_01/export
Authorization: Bearer <admin-token>
```

Stream the response directly to durable storage. Do not buffer large exports in
the gateway.

## Restore

Upload a `.db` file as multipart form data:

```bash
curl -X POST http://rsql:8080/v1/namespaces/tenant_01/import \
  -H 'Authorization: Bearer admin-token' \
  -F 'file=@backup.db'
```

Imports are control-plane operations. Coordinate application writes during a
restore and verify the namespace overview afterward.

## CSV movement

CSV import targets one table:

```bash
curl -X POST \
  'http://rsql:8080/v1/namespaces/tenant_01/import?table=contacts' \
  -H 'Authorization: Bearer admin-token' \
  -F 'file=@contacts.csv'
```

CSV export is a data-plane table operation and accepts the row filtering,
selection, ordering, and search grammar:

```http
GET /v1/tenant_01/tables/contacts/export?format=csv&status=eq.active
```

## Process sharding

Assign each namespace to exactly one rsql process and data directory. Route
requests through the orchestrator's placement record. Moving a namespace means
exporting it, importing it into the destination instance, switching routing,
and removing the old copy after verification.

One process can register substantially more namespaces than it keeps open.
`max-open-namespaces` bounds the warm working set; tenant database pools open
on demand and are evicted when the limit or idle timeout is reached. Keep this
limit sized for the instance's memory and file-descriptor budget rather than
setting it to the total fleet size.

Read and write pools initialize independently. A namespace that only receives
reads does not open or retain its writer connection. Namespace files use
deterministic two-level hash directories below `data/namespaces`, avoiding one
directory with an entry for every provisioned database.

The default `namespace-read-connections=1` is tuned for traffic spread across
many namespaces. Increase it only when a small number of namespaces need
parallel reads. More per-namespace connections improve hot-database throughput
but consume more file descriptors and duplicate connection-local SQLite work.

Namespace listing uses cursor pagination. Startup neither lists the registry
nor scans namespace files, so cold fleet size does not determine startup work.
If thousands of namespaces must remain hot simultaneously, split them across
processes and route by placement.

rsql does not provide replication, consensus, distributed transactions, or
automatic failover. Build those properties at the deployment layer or use
durable block storage and a single active process with an external recovery
procedure.

Native package installations store all service data below `/var/lib/rsql`.
Stop `rsql.service` before taking an offline filesystem copy. Online backups
should use the namespace export endpoint instead of copying active SQLite
files.
