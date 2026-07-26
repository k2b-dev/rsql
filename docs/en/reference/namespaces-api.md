---
title: Namespace API
navTitle: Namespaces
section: API Reference
order: 120
description: Provision, configure, copy, import, export, and delete databases.
tags: [api, namespaces, control-plane]
updated: 2026-07-26
---

# Namespace API

Namespace endpoints are administrative. Do not expose them through a
tenant-facing gateway.

## Create

```http
POST /v1/namespaces
Content-Type: application/json
```

```json
{
  "name": "tenant_01",
  "config": {
    "journal_mode": "wal",
    "busy_timeout": 5000,
    "max_db_size": 1073741824,
    "query_timeout": 10000,
    "foreign_keys": true,
    "read_only": false
  }
}
```

Returns `201` with the namespace record. Omitted creation settings default to
WAL, a 5000 ms busy timeout, a 10000 ms query timeout, foreign keys enabled, no
quota, and read-write mode.

Namespace names must match `[A-Za-z0-9][A-Za-z0-9_-]{0,63}`.

## List and get

```http
GET /v1/namespaces
GET /v1/namespaces/tenant_01
```

The list reads registry and latest telemetry metadata without opening every
database. Records may include creation time, database path, configuration, last
activity, and a compact storage snapshot.

## Update configuration

```http
PUT /v1/namespaces/tenant_01
Content-Type: application/json
```

```json
{
  "config": {
    "journal_mode": "wal",
    "busy_timeout": 5000,
    "max_db_size": 2147483648,
    "query_timeout": 10000,
    "foreign_keys": true,
    "read_only": false
  }
}
```

The configuration is replaced, not patched. Send every setting that must
remain active. Returns the updated record.

## Duplicate and delete

```http
POST /v1/namespaces/tenant_01/duplicate
Content-Type: application/json

{"name":"tenant_01_copy"}
```

Duplicate returns `201`. Delete returns `204`:

```http
DELETE /v1/namespaces/tenant_01_copy
```

## Export

```http
GET /v1/namespaces/tenant_01/export
```

The response is an `application/octet-stream` attachment named
`tenant_01.db`. Consume it as a stream.

## Import

Send multipart form data with one field named `file`.

Database replacement:

```bash
curl -X POST http://rsql:8080/v1/namespaces/tenant_01/import \
  -H 'Authorization: Bearer admin-token' \
  -F 'file=@backup.db'
```

CSV insertion:

```bash
curl -X POST \
  'http://rsql:8080/v1/namespaces/tenant_01/import?table=contacts' \
  -H 'Authorization: Bearer admin-token' \
  -F 'file=@contacts.csv'
```

The multipart parser keeps up to 64 MiB in memory and may spill larger parts to
temporary files. Database import validates the uploaded SQLite file and quota
before replacement.
