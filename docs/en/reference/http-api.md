---
title: HTTP API
navTitle: HTTP API
section: API Reference
order: 110
description: Navigate the complete rsql control-plane and data-plane HTTP API.
tags: [http, api, reference]
updated: 2026-07-26
---

# HTTP API

The API root is `/v1`. Every API request requires:

```http
Authorization: Bearer <server-token>
```

`GET /healthz` and `GET /metrics` are public operational endpoints.

## Namespace control plane

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/v1/namespaces` | Create a namespace |
| `GET` | `/v1/namespaces` | List namespace records |
| `GET` | `/v1/namespaces/{ns}` | Get one namespace |
| `PUT` | `/v1/namespaces/{ns}` | Replace namespace configuration |
| `DELETE` | `/v1/namespaces/{ns}` | Delete a namespace |
| `POST` | `/v1/namespaces/{ns}/duplicate` | Copy a namespace |
| `GET` | `/v1/namespaces/{ns}/export` | Stream a database backup |
| `POST` | `/v1/namespaces/{ns}/import` | Import database or CSV data |

See [Namespace API](/en/reference/namespaces-api).

## Namespace data plane

| Group | Routes |
| --- | --- |
| Schema | `/v1/{ns}/tables...` |
| Rows | `/v1/{ns}/tables/{table}/rows...` |
| CSV | `/v1/{ns}/tables/{table}/export` |
| Read-only SQL | `/v1/{ns}/query` |
| SQL writes and transactions | `/v1/{ns}/query/exec`, `/v1/{ns}/query/transaction` |
| SSE | `/v1/{ns}/subscribe` |
| Changelog | `/v1/{ns}/changelog` |
| Overview | `/v1/{ns}/overview` |

See [Schema API](/en/reference/schema-api),
[Rows API](/en/reference/rows-api), and
[Query, events, and overview](/en/reference/query-events-api).

## Error envelope

Non-2xx JSON errors use:

```json
{
  "error": "invalid_request",
  "message": "name is required"
}
```

Common statuses are `400` invalid request, `401` invalid authentication, `404`
missing namespace/table/row, `409` conflict, `405` wrong method, `507` quota
exceeded, and `500` internal failure.

Every response includes `X-Request-ID`. Supply that header to correlate gateway
and rsql logs, or let rsql generate one.

See [Errors and headers](/en/reference/errors-and-headers) for stable error
codes, content types, and mutation preferences.

## Write preferences

Mutation routes support:

- `Prefer: return=representation`
- `Prefer: resolution=merge-duplicates`
- `Prefer: resolution=ignore-duplicates`

Only use duplicate resolution on inserts where a unique constraint can
identify the conflicting row.
