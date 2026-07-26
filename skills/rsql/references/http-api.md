# HTTP API

Use this reference when calling routes directly, implementing a gateway, or
checking payloads, response types, status codes, headers, and event contracts.

## Common contracts

- API base: `/v1`
- Authentication: `Authorization: Bearer <server-token>`
- JSON errors: `{"error":"<code>","message":"<detail>"}`
- Managed row columns: `id`, `created_at`, `updated_at`
- All leading-underscore column names are reserved.
- Optional write audit metadata uses `_meta`.
- `Prefer: return=representation` returns changed rows.
- Insert conflict modes are `resolution=merge-duplicates` and
  `resolution=ignore-duplicates`.

## Operational endpoints

| Method | Path | Auth | Response |
| --- | --- | --- | --- |
| GET | `/healthz` | No | `200 {"status":"ok"}` |
| GET | `/metrics` | No | Prometheus text |

## Namespace control plane

| Method | Path | Success |
| --- | --- | --- |
| POST | `/v1/namespaces` | `201` name and resolved config |
| GET | `/v1/namespaces` | `200` fleet records with optional latest activity/storage |
| GET | `/v1/namespaces/:ns` | `200` registry metadata and config |
| PUT | `/v1/namespaces/:ns` | `200` name and applied config |
| DELETE | `/v1/namespaces/:ns` | `204` |
| POST | `/v1/namespaces/:ns/duplicate` | `201` source and target |
| GET | `/v1/namespaces/:ns/export` | `200` streamed SQLite file |
| POST | `/v1/namespaces/:ns/import` | `200` DB or CSV import result |

Create body:

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

DB and CSV imports use multipart field `file`. CSV import also requires
`?table=<table>`.

## Schema and indexes

| Method | Path | Success |
| --- | --- | --- |
| GET | `/v1/:ns/tables` | `200` tables and views |
| POST | `/v1/:ns/tables` | `201 {"created":"name","type":"table|view"}` |
| GET | `/v1/:ns/tables/:table` | `200` schema metadata |
| PUT | `/v1/:ns/tables/:table` | `200 {"updated":true}` |
| DELETE | `/v1/:ns/tables/:table` | `204` |
| POST | `/v1/:ns/tables/:table/indexes` | `201 {"created":true}` |
| DELETE | `/v1/:ns/tables/:table/indexes/:index` | `204` |
| GET | `/v1/:ns/tables/:table/export?format=csv` | streamed CSV |

Table create supports `type`, `name`, `metadata`, `columns`, `sql`, and `_meta`.
Table update supports `rename`, `add_columns`, `drop_columns`,
`rename_columns`, `sql`, `metadata`, and `_meta`.

Index types are `index`, `unique`, and `fts`.

## Rows

| Method | Path | Success |
| --- | --- | --- |
| GET | `/v1/:ns/tables/:table/rows` | `200` data and list metadata |
| POST | `/v1/:ns/tables/:table/rows` | `201` inserted count or data |
| PATCH | `/v1/:ns/tables/:table/rows` | `200` updated count or data |
| DELETE | `/v1/:ns/tables/:table/rows` | `204` or `200` data |
| GET | `/v1/:ns/tables/:table/rows/:id` | `200` row |
| PUT | `/v1/:ns/tables/:table/rows/:id` | `200` updated count or data |
| DELETE | `/v1/:ns/tables/:table/rows/:id` | `204` or `200` data |

Single inserts use a flat object. Bulk inserts use `{"rows":[...]}`. Bulk
update and delete filters use the same query grammar as row listing.

## Query, changelog, SSE, and overview

| Method | Path | Success |
| --- | --- | --- |
| POST | `/v1/:ns/query` | `200` single data result or batch results |
| GET | `/v1/:ns/changelog` | `200` schema changelog entries |
| GET | `/v1/:ns/subscribe` | `200` SSE stream |
| GET | `/v1/:ns/overview` | `200` operational summary |

Query accepts one `sql` plus `params`, or `statements`. Only read-only
`SELECT`/`WITH` execution is accepted and internal objects are blocked.

Changelog options are `table`, `limit`, and `offset`. It records schema
mutations, not row mutations.

SSE accepts `tables=t1,t2`. Events contain namespace, table, action, optional
row or bulk ids, optional view invalidation source, `_meta`, and timestamp.
Delivery is at-most-once for a connection.

Overview windows are `1h`, `24h`, `7d`, and `30d`; default is `24h`. The full
wire contract is `internal/domain/overview.go`.
