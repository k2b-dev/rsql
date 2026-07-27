---
title: Quickstart
navTitle: Quickstart
section: Start
order: 20
description: Start rsql and perform the first schema and row operations.
tags: [quickstart, curl]
updated: 2026-07-27
---

# Quickstart

This guide starts a local server, creates a namespace and table, inserts a row,
and executes a read-only query.

## Requirements

- The `rsql` CLI from a native package or release archive.
- `curl` for the HTTP examples.

## Start the server

```bash
rsql serve \
  --listen=127.0.0.1:8080 \
  --api-token=dev-token \
  --data-dir=./data
```

The health endpoint is public:

```bash
curl -s http://127.0.0.1:8080/healthz
```

## Create a namespace

```bash
curl -s -X POST http://127.0.0.1:8080/v1/namespaces \
  -H 'Authorization: Bearer dev-token' \
  -H 'Content-Type: application/json' \
  -d '{"name":"demo"}'
```

Namespace names become filenames and URL segments. Use letters, numbers,
hyphens, and underscores; do not accept a raw namespace from an untrusted
caller.

## Create a table

```bash
curl -s -X POST http://127.0.0.1:8080/v1/demo/tables \
  -H 'Authorization: Bearer dev-token' \
  -H 'Content-Type: application/json' \
  -d '{
    "type": "table",
    "name": "contacts",
    "columns": [
      {"name":"name","type":"text","not_null":true},
      {"name":"email","type":"text","unique":true},
      {"name":"status","type":"select","options":["active","inactive"],"index":true}
    ]
  }'
```

rsql adds `id`, `created_at`, and `updated_at` automatically.

## Write and read rows

```bash
curl -s -X POST http://127.0.0.1:8080/v1/demo/tables/contacts/rows \
  -H 'Authorization: Bearer dev-token' \
  -H 'Content-Type: application/json' \
  -d '{"name":"Ada","email":"ada@example.com","status":"active"}'
```

```bash
curl -s \
  'http://127.0.0.1:8080/v1/demo/tables/contacts/rows?status=eq.active&order=id.desc&limit=20' \
  -H 'Authorization: Bearer dev-token'
```

## Run read-only SQL

```bash
curl -s -X POST http://127.0.0.1:8080/v1/demo/query \
  -H 'Authorization: Bearer dev-token' \
  -H 'Content-Type: application/json' \
  -d '{"sql":"SELECT status, COUNT(*) AS count FROM contacts GROUP BY status","params":[]}'
```

Only one `SELECT` or `WITH` statement is accepted. Use placeholders and
`params` for values.

## Continue

- [Production installation](/en/operations/installation)
- [Schema and rows](/en/concepts/schema-and-rows)
- [Filtering and pagination](/en/reference/filtering)
- [Configuration](/en/reference/configuration)
