---
title: Rows API
navTitle: Rows
section: API Reference
order: 140
description: Insert, read, update, delete, bulk mutate, and export typed rows.
tags: [api, rows, csv]
updated: 2026-07-26
---

# Rows API

## Endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/v1/{ns}/tables/{table}/rows` | List rows |
| `POST` | `/v1/{ns}/tables/{table}/rows` | Insert rows |
| `PATCH` | `/v1/{ns}/tables/{table}/rows` | Update matching rows |
| `DELETE` | `/v1/{ns}/tables/{table}/rows` | Delete matching rows |
| `GET` | `/v1/{ns}/tables/{table}/rows/{id}` | Get one row |
| `PUT` | `/v1/{ns}/tables/{table}/rows/{id}` | Update one row |
| `DELETE` | `/v1/{ns}/tables/{table}/rows/{id}` | Delete one row |
| `GET` | `/v1/{ns}/tables/{table}/export` | Stream CSV |

## List

```http
GET /v1/demo/tables/contacts/rows?status=eq.active&order=name.asc&limit=20
```

```json
{
  "data": [{"id":1,"name":"Ada","status":"active"}],
  "meta": {
    "total_count": 42,
    "filter_count": 18,
    "limit": 20,
    "offset": 0
  }
}
```

The default limit is 100 and the maximum is 10000. Aggregate selections return
`{"data":[...]}` without pagination metadata. See
[Filtering and pagination](/en/reference/filtering).

## Insert

One row is a flat object:

```json
{"name":"Ada","email":"ada@example.com","_meta":{"actor":"user-1"}}
```

Multiple rows use a wrapper:

```json
{
  "rows": [
    {"name":"Ada","email":"ada@example.com"},
    {"name":"Bob","email":"bob@example.com"}
  ],
  "_meta": {"actor":"import-7"}
}
```

A top-level JSON array is not accepted. Insert returns `201`. Use
`Prefer: return=representation` for inserted rows, or a duplicate resolution
preference for unique conflicts.

## Update

```http
PUT /v1/demo/tables/contacts/rows/42
Content-Type: application/json

{"status":"inactive","_meta":{"actor":"user-1"}}
```

Bulk update applies the same filtering grammar:

```http
PATCH /v1/demo/tables/contacts/rows?status=eq.pending
Content-Type: application/json

{"status":"active","_meta":{"actor":"job-9"}}
```

## Delete

Single and bulk deletes return `204` by default. Send
`Prefer: return=representation` for a `200` response containing deleted rows.
Treat an unfiltered bulk delete as destructive and guard it in application
code.

## CSV export

```http
GET /v1/demo/tables/contacts/export?format=csv&status=eq.active&select=name,email&order=name.asc
```

`format=csv` is required. Filters, logical expressions, `select`, `order`, and
`search` match row listing. Without `limit`, all matching rows are exported.
`bom=true` prepends a UTF-8 BOM.

The response uses RFC 4180 CSV, `text/csv; charset=utf-8`, and a content
disposition attachment. Once streaming begins, a later encoder error terminates
the connection because the `200` status has already been sent.
