---
title: Quotas and Overview
navTitle: Quotas and overview
section: Hosting
order: 90
description: Enforce namespace storage quotas and build tenant database dashboards.
tags: [quota, telemetry, dashboard]
updated: 2026-07-26
---

# Quotas and Overview

`config.max_db_size` limits allocated SQLite database pages for one namespace.
The limit is deterministic and inexpensive, but page-granular rather than
byte-exact.

## Quota behavior

Set the maximum in bytes when creating or updating a namespace:

```json
{
  "config": {
    "max_db_size": 1073741824
  }
}
```

The serialized writer applies SQLite's page limit before writes and imports.
When a mutation requires more pages, rsql returns:

```http
HTTP/1.1 507 Insufficient Storage
Content-Type: application/json
```

```json
{
  "error": "quota_exceeded",
  "message": "namespace storage quota exceeded"
}
```

WAL and SHM sidecars are reported but are not part of the hard page quota.
Platform disk capacity still needs host-level monitoring.

## Tenant dashboard endpoint

Use one request for a database detail page:

```http
GET /v1/tenant_01/overview?window=24h
```

Supported windows are `1h`, `24h`, `7d`, and `30d`. The response includes:

- health, read/write mode, and detected issues
- used, allocated, reclaimable, quota, remaining, WAL, SHM, and disk bytes
- request, read, write, error, and response-byte totals
- read and write latency at p50, p95, p99, and average
- writer wait, busy errors, timeouts, and quota rejections
- table, view, index, FTS index, and realtime subscriber counts
- bounded top operation groups
- storage growth and projected quota exhaustion when enough samples exist

`remaining_bytes` and `usage_ratio` use allocated pages because that is the
enforced quantity. With no quota, `quota_bytes` is `0` and both values are
`null`.

The endpoint uses SQLite pragmas, file metadata, bounded telemetry, and schema
object counts. It does not scan tenant rows.

## Telemetry retention

Telemetry is stored separately in `telemetry.db`. Five-minute buckets are kept
for 48 hours; completed buckets roll up hourly and remain for 31 days. Request
bodies, SQL, parameters, row values, and secrets are not retained.
