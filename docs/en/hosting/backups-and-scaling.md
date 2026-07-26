---
title: Backups and Scaling
navTitle: Backups and scaling
section: Hosting
order: 100
description: Back up, restore, move, and shard namespace databases safely.
tags: [backup, restore, scaling]
updated: 2026-07-26
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

rsql does not provide replication, consensus, distributed transactions, or
automatic failover. Build those properties at the deployment layer or use
durable block storage and a single active process with an external recovery
procedure.
