---
title: Schema API
navTitle: Schema
section: API Reference
order: 130
description: Manage tables, views, columns, indexes, and FTS search.
tags: [api, schema, indexes]
updated: 2026-07-26
---

# Schema API

All paths on this page are scoped to an existing namespace.

## Tables and views

| Method | Path | Success |
| --- | --- | --- |
| `GET` | `/v1/{ns}/tables` | `200`, object list |
| `POST` | `/v1/{ns}/tables` | `201`, created object |
| `GET` | `/v1/{ns}/tables/{table}` | `200`, schema |
| `PUT` | `/v1/{ns}/tables/{table}` | `200` |
| `DELETE` | `/v1/{ns}/tables/{table}` | `204` |

Create a table:

```json
{
  "type": "table",
  "name": "contacts",
  "metadata": {"label":"Contacts"},
  "columns": [
    {"name":"name","type":"text","not_null":true,"max_length":200},
    {"name":"email","type":"text","unique":true},
    {"name":"score","type":"real","min":0,"max":100}
  ],
  "_meta": {"actor":"migration-42"}
}
```

Create a view:

```json
{
  "type": "view",
  "name": "active_contacts",
  "sql": "SELECT id, name, email FROM contacts WHERE status = 'active'"
}
```

Update a table or view:

```json
{
  "rename": "customers",
  "add_columns": [{"name":"company","type":"text"}],
  "drop_columns": ["legacy_code"],
  "rename_columns": {"email":"primary_email"},
  "metadata": {"label":"Customers"},
  "_meta": {"actor":"migration-43"}
}
```

Only include operations required by the update. `sql` replaces view SQL.

## Indexes

```http
POST /v1/{ns}/tables/{table}/indexes
```

Regular index:

```json
{"type":"index","name":"idx_contacts_status","columns":["status"]}
```

Unique index:

```json
{"type":"unique","name":"ux_contacts_external_id","columns":["external_id"]}
```

FTS index:

```json
{"type":"fts","columns":["name","email","notes"]}
```

Delete an index with:

```http
DELETE /v1/{ns}/tables/{table}/indexes/{index}
Content-Type: application/json

{"_meta":{"actor":"migration-44"}}
```

Index creation returns `201`; deletion returns `204`.

## Reserved names

`id`, `created_at`, `updated_at`, `_meta`, and any identifier beginning with
`_` are reserved. Identifiers otherwise use letters, numbers, and underscores
and must begin with a letter or underscore.
