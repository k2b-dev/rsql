---
title: Query, Events, and Overview API
navTitle: Query and events
section: API Reference
order: 160
description: Execute read-only SQL, subscribe to changes, inspect schema history, and read namespace health.
tags: [api, sql, sse, overview]
updated: 2026-07-26
---

# Query, Events, and Overview API

## Read-only query

```http
POST /v1/{ns}/query
Content-Type: application/json
```

Single query:

```json
{
  "sql": "SELECT * FROM contacts WHERE status = ?",
  "params": ["active"]
}
```

Response:

```json
{"data":[{"id":1,"name":"Ada","status":"active"}]}
```

Batch:

```json
{
  "statements": [
    {"sql":"SELECT COUNT(*) AS count FROM contacts","params":[]},
    {"sql":"SELECT * FROM contacts WHERE id = ?","params":[1]}
  ]
}
```

Batch responses use `{"results":[{"data":[...]},{"data":[...]}]}`. Statements
execute sequentially. They are not a write transaction.

## SSE subscription

```http
GET /v1/{ns}/subscribe?tables=contacts,orders
Accept: text/event-stream
```

The server flushes a connection comment immediately and a keepalive comment
every 25 seconds. Event names match the mutation action. Data is JSON:

```json
{
  "table": "contacts",
  "action": "update",
  "row": {"id":1,"status":"inactive"},
  "_meta": {"actor":"user-1"},
  "timestamp": "2026-07-26T12:00:00Z"
}
```

Close the HTTP request to unsubscribe.

## Schema changelog

```http
GET /v1/{ns}/changelog?table=contacts&limit=50&offset=0
```

The endpoint returns a JSON array of schema events. Default limit is 50.
Entries contain `id`, `timestamp`, `action`, `table`, `detail`, and optional
`_meta`.

## Namespace overview

```http
GET /v1/{ns}/overview?window=24h
```

Windows are `1h`, `24h`, `7d`, and `30d`; the default is `24h`. The response
combines live storage/schema values with bounded telemetry. See
[Quotas and overview](/en/hosting/quotas-and-overview) for field semantics.
