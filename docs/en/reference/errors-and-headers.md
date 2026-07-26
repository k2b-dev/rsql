---
title: Errors and Headers
navTitle: Errors and headers
section: API Reference
order: 165
description: Handle rsql authentication, request IDs, preferences, status codes, and stable error values.
tags: [api, errors, headers]
updated: 2026-07-26
---

# Errors and Headers

## Authentication

Every `/v1/*` request requires:

```http
Authorization: Bearer <server-token>
```

Missing or invalid credentials return `401` with error code `unauthorized`.
`/healthz` and `/metrics` do not require authentication.

## Request IDs

Every response includes `X-Request-ID`. If the request already has a value,
rsql preserves it; otherwise rsql generates one. Forward a trusted trace or
request ID from a gateway to correlate application, rsql, and proxy logs.

## Error response

JSON failures use:

```json
{
  "error": "validation_failed",
  "message": "..."
}
```

Stable error codes include:

| Code | Typical status | Meaning |
| --- | --- | --- |
| `invalid_request` | `400` | Malformed JSON, identifier, filter, or input |
| `unauthorized` | `401` | Missing or invalid bearer token |
| `not_found` | `404` | Requested resource does not exist |
| `namespace_not_found` | `404` | Namespace does not exist |
| `table_not_found` | `404` | Table or view does not exist |
| `read_only` | `405` | Mutation attempted against a read-only target |
| `conflict` | `409` | Name or unique-value conflict |
| `validation_failed` | `400` | Row does not satisfy schema validation |
| `sql_not_read_only` | `400` | Query is outside the accepted read-only subset |
| `quota_exceeded` | `507` | Namespace page quota would be exceeded |
| `database_busy` | `503` | SQLite could not acquire required access |
| `internal_error` | `500` | Unexpected server failure |

Use the machine-readable `error` value in application logic. The human-readable
`message` may become more specific over time.

## Prefer

Mutation routes accept one `Prefer` value:

```http
Prefer: return=representation
Prefer: resolution=merge-duplicates
Prefer: resolution=ignore-duplicates
```

`return=representation` returns changed rows where supported. Without it,
delete operations return `204` and other mutations return count-oriented
responses.

Duplicate-resolution preferences apply to inserts with unique conflicts.
`merge-duplicates` updates the conflicting row; `ignore-duplicates` skips it.

## Content types

- JSON requests use `Content-Type: application/json`.
- Namespace and CSV imports use `multipart/form-data` with field `file`.
- SSE uses `text/event-stream`.
- CSV exports use `text/csv; charset=utf-8`.
- Database exports use `application/octet-stream`.

Streaming responses may terminate after headers were sent if a later read or
encoding error occurs. Clients must treat truncated exports and event-stream
disconnects as failures.
