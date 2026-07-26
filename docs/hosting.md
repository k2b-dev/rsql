# Multi-Tenant Hosting

rsql can be the database control and data plane behind a multi-tenant product.
Each namespace has its own SQLite database file, connection pools, writer lock,
configuration, quota, telemetry, and SSE subscriber set.

## Security Boundary

The built-in bearer token is a server-wide administrative credential. It can
access every namespace and must not be given to tenants.

A hosting product should place an authenticated gateway in front of rsql:

1. Authenticate the tenant.
2. Resolve the tenant to exactly one allowed namespace.
3. Construct the upstream rsql path itself.
4. Reject tenant-controlled namespace path segments.
5. Add the private rsql administrative token.

Run rsql on a private network or loopback interface so tenants cannot bypass the
gateway. Namespace files provide strong data separation inside rsql, but tenant
identity and authorization belong to the gateway.

## Provisioning

Create a database with `POST /v1/namespaces` and a byte quota:

```json
{
  "name": "tenant_01",
  "config": {
    "journal_mode": "wal",
    "busy_timeout": 5000,
    "query_timeout": 10000,
    "foreign_keys": true,
    "read_only": false,
    "max_db_size": 1073741824
  }
}
```

`max_db_size` is enforced through SQLite's page limit on the dedicated
single-connection writer. The limit is deterministic but page-granular rather
than byte-exact. Oversized writes and imports return HTTP `507` with
`quota_exceeded`.

## Tenant Dashboard

Use one endpoint for the database detail page:

```http
GET /v1/tenant_01/overview?window=24h
```

It includes storage/quota state, activity, latency, errors, writer contention,
schema counts, realtime subscribers, top operations, and storage growth.
Supported windows are `1h`, `24h`, `7d`, and `30d`.

Use `GET /v1/namespaces` for the administrative fleet list. It reads the control
registry and latest telemetry only; it does not open every tenant database.

## Telemetry Model

Tenant telemetry is stored separately in `telemetry.db`.

- Requests update in-memory five-minute buckets.
- Dirty buckets flush every five seconds and during graceful shutdown.
- Five-minute data is retained for 48 hours.
- Completed buckets roll up hourly and are retained for 31 days.
- Histograms and operation names are bounded.
- SQL, parameters, row values, request bodies, and secrets are never retained.
- Prometheus metrics remain low-cardinality and do not use namespace or table
  labels.

The overview performs constant-time SQLite pragma/file reads and schema object
counts. It never scans all rows.

## Scaling Model

Namespace databases open lazily. rsql retains at most 128 idle namespace
handles by default and evicts the least recently used non-busy handle. Each
retained namespace has:

- one serialized writer connection
- up to four concurrent read connections
- one idle read connection

Hundreds of provisioned databases are therefore practical when only a subset is
active. More than 128 simultaneously active namespaces can temporarily remain
open because active work is never evicted. If hundreds of tenants are
continuously hot, shard namespaces across multiple rsql processes instead of
raising one process without bounds.

Storage quotas do not limit CPU, request rate, query complexity, SSE subscriber
count, or total platform disk. Enforce those product limits in the gateway and
host scheduler.
