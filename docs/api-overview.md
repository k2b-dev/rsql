# API Overview

Base URL: `http://<host>:<port>/v1`

## Authentication

All `/v1/*` endpoints require bearer token auth:

```http
Authorization: Bearer <token>
```

Public operational endpoints:

- `GET /healthz`
- `GET /metrics`

## Conventions

### Error envelope

```json
{
  "error": "invalid_request",
  "message": "..."
}
```

### `_meta` passthrough

Write operations can include an optional `_meta` object alongside the regular payload. It is forwarded to SSE events for every write, and to the changelog for schema mutations. The leading underscore distinguishes it from any user-defined column named `meta`.

```json
POST /v1/demo/tables/contacts/rows
{ "name": "Ada", "email": "ada@x.com", "_meta": { "actor": "alice" } }
```

### Reserved column names

The following column names are managed by rsql itself and **cannot** be created, renamed, or dropped via the API:

| Name | Why |
| --- | --- |
| `id` | auto-incremented primary key |
| `created_at` | auto-maintained timestamp |
| `updated_at` | auto-maintained timestamp |
| `_meta` | audit-meta passthrough wire key |

Any column name with a leading underscore (`_*`) is also reserved for future rsql-internal use.

### `Prefer` header

Supported values:

- `Prefer: return=representation`
- `Prefer: resolution=merge-duplicates`
- `Prefer: resolution=ignore-duplicates`

## Endpoint Groups

### Namespaces

- `POST /v1/namespaces`
- `GET /v1/namespaces`
- `GET /v1/namespaces/:ns`
- `PUT /v1/namespaces/:ns`
- `DELETE /v1/namespaces/:ns`
- `POST /v1/namespaces/:ns/duplicate`
- `GET /v1/namespaces/:ns/export`
- `POST /v1/namespaces/:ns/import` (`multipart/form-data`)
  - DB import: upload `.db`
  - CSV import: upload file and set `?table=<table_name>`

### Schema (tables/views)

- `GET /v1/:ns/tables`
- `POST /v1/:ns/tables`
- `GET /v1/:ns/tables/:table`
- `PUT /v1/:ns/tables/:table`
- `DELETE /v1/:ns/tables/:table`
- `POST /v1/:ns/tables/:table/indexes`
- `DELETE /v1/:ns/tables/:table/indexes/:index`
- `GET /v1/:ns/tables/:table/export?format=csv` — see [CSV export](#csv-export)

### Rows

- `GET /v1/:ns/tables/:table/rows`
- `POST /v1/:ns/tables/:table/rows`
- `PATCH /v1/:ns/tables/:table/rows`
- `DELETE /v1/:ns/tables/:table/rows`
- `GET /v1/:ns/tables/:table/rows/:id`
- `PUT /v1/:ns/tables/:table/rows/:id`
- `DELETE /v1/:ns/tables/:table/rows/:id`

#### Insert payload shape

A single row is a flat object:

```json
POST /v1/:ns/tables/:table/rows
{ "name": "Ada", "email": "ada@example.com" }
```

Multiple rows are wrapped in a `rows` array (a top-level JSON array is **not** accepted):

```json
POST /v1/:ns/tables/:table/rows
{
  "rows": [
    { "name": "Ada",  "email": "ada@example.com" },
    { "name": "Bob",  "email": "bob@example.com" }
  ]
}
```

Both shapes accept an optional `_meta` field that is forwarded to SSE and (for schema mutations) the changelog.

### Query / Realtime / Observability

- `POST /v1/:ns/query` (read-only SQL)
- `GET /v1/:ns/subscribe` (SSE, optional `?tables=t1,t2`)
- `GET /v1/:ns/changelog?table=<name>&limit=50&offset=0` — schema mutations only
- `GET /v1/:ns/overview?window=1h|24h|7d|30d` (default `24h`)

### Namespace overview

The overview endpoint is the tenant-facing operational read model. It combines
live constant-time SQLite storage/schema values with bounded historical
telemetry:

- health and read/write mode
- logical usage, allocated pages, reclaimable pages, quota, WAL, and disk bytes
- request/read/write/error totals and time series
- read/write latency (`p50`, `p95`, `p99`, average)
- writer wait, busy errors, timeouts, and quota rejections
- table/view/index/FTS counts and active SSE subscribers
- top stable API operations
- storage growth and projected quota exhaustion when enough samples exist

```http
GET /v1/demo/overview?window=24h
```

```json
{
  "health": {
    "status": "healthy",
    "mode": "read_write",
    "issues": []
  },
  "storage": {
    "used_bytes": 49152,
    "allocated_bytes": 65536,
    "reclaimable_bytes": 16384,
    "quota_bytes": 1073741824,
    "remaining_bytes": 1073676288,
    "usage_ratio": 0.000061,
    "database_file_bytes": 65536,
    "wal_bytes": 0,
    "shm_bytes": 0,
    "disk_bytes": 65536
  }
}
```

`remaining_bytes` and `usage_ratio` use allocated pages because that is the
enforced limit; `used_bytes` subtracts reusable free-list pages.
`quota_bytes: 0`, `remaining_bytes: null`, and `usage_ratio: null` mean that no
storage quota is configured. The endpoint does not count table rows or scan
tenant data.

### Storage quota errors

Set `config.max_db_size` in bytes when creating or updating a namespace. rsql
enforces the limit at SQLite page-allocation time on the serialized writer.
Writes and imports that exceed it return:

```http
HTTP/1.1 507 Insufficient Storage
```

```json
{
  "error": "quota_exceeded",
  "message": "namespace storage quota exceeded"
}
```

The quota applies to allocated database pages. WAL and SHM sidecars are shown in
the overview but are not part of the hard SQLite page quota.

## Filtering and Pagination

Row list supports:

- `select=*` (default)
- `order=id.asc` (default)
- `limit=100` (default)
- `offset=0` (default)
- `search=<term>` (FTS if available, otherwise LIKE fallback)

Filter operators (`column=<op>.<value>`):

- `eq`, `neq`, `gt`, `gte`, `lt`, `lte`
- `like`, `ilike`
- `in.(a,b,c)`
- `is.null`, `is.true`, `is.false`
- negation: `not.<op>.<value>`

Each column key takes a single filter value. Repeating the same key (e.g. `?score=gte.50&score=lte.95`) only applies the first occurrence; combine multiple predicates on one column via `and=(…)` instead.

Logical expressions:

- `or=(status.eq.active,priority.gte.3)`
- `and=(status.eq.active,city.ilike.Berlin)`

Example:

```http
GET /v1/demo/tables/contacts/rows?status=eq.active&score=gte.80&order=score.desc&limit=20
```

Range filter via `and`:

```http
GET /v1/demo/tables/contacts/rows?and=(score.gte.50,score.lte.95)&order=score.desc
```

## CSV Export

`GET /v1/:ns/tables/:table/export?format=csv` streams a table or view as RFC 4180 CSV.

The endpoint accepts the same query grammar as `GET /v1/:ns/tables/:table/rows` — every filter operator, logical expression (`or=`, `and=`), `select`, `order`, and `search` works identically. The only differences:

- `format=csv` is required (reserves space for future encoders).
- `limit` and `offset` are honored when present, but **omitting `limit` exports all rows** (the `/rows` endpoint defaults to `limit=100`).
- Aggregate selects (`select=status,count()`) work and produce a CSV pivot table.
- `bom=true` prepends a UTF-8 BOM so Excel renders non-ASCII correctly.

### Type encoding

| Logical type | CSV cell |
| --- | --- |
| `text`, `integer`, `real`, `select`, `date`, `datetime` | as stored |
| `boolean` | `true` / `false` |
| `json` | inline JSON (CSV-quoted) |
| `NULL` | empty cell |

### Response headers

```
Content-Type: text/csv; charset=utf-8
Content-Disposition: attachment; filename="<table>.csv"
X-Content-Type-Options: nosniff
```

### Examples

Filtered export, narrowed columns, deterministic order:

```http
GET /v1/demo/tables/contacts/export?format=csv&status=eq.active&select=name,email,score&order=score.desc
```

Aggregated CSV (counts per status):

```http
GET /v1/demo/tables/contacts/export?format=csv&select=status,count()&order=count().desc
```

Notes:

- The HTTP status is committed to `200` before the first row is sent. Errors during streaming are logged with the request id; the connection is then closed.
- Reads are not published to SSE or the changelog (consistent with `/rows`).

## Read-only SQL Endpoint

`POST /v1/:ns/query` supports:

- Single statement: `{ "sql": "SELECT ...", "params": [] }`
- Batch mode: `{ "statements": [{"sql":"SELECT ...","params":[]}] }`

Rules:

- Only read-only statements (`SELECT`/`WITH`) are allowed
- Internal objects (prefixed with `_`) are blocked
