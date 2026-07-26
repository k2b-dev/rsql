# Tenancy and operations

Use this reference for hosting architecture, security boundaries, config,
telemetry, quotas, scaling, startup, shutdown, and recovery.

## Trust boundary

The bearer token authenticates a trusted service to the entire rsql process.
It is not a tenant credential. Every authenticated caller can address every
namespace.

A tenant-facing deployment requires a gateway that:

1. authenticates the product user
2. resolves the user to one allowed namespace
3. constructs the upstream path itself
4. injects the private rsql token
5. prevents direct access to rsql

Namespace files isolate data and runtime locks. The gateway owns identity,
authorization, request-rate limits, CPU/query policy, and tenant routing.

## Runtime configuration

Precedence is CLI flag, `RSQL_*` environment variable, then default.

| Key | Default |
| --- | --- |
| `listen` | `:8080` |
| `pprof-enabled` | `false` |
| `pprof-listen` | `127.0.0.1:6060` |
| `data-dir` | `data` |
| `api-token` | required |
| `log-level` | `info` |
| `query-timeout-ms` | `10000` |
| `namespace-idle-timeout-ms` | `300000` |
| `max-open-namespaces` | `128` |
| `namespace-read-connections` | `1` |
| `read-timeout-ms` | `15000` |
| `write-timeout-ms` | `15000` |
| `idle-timeout-ms` | `120000` |
| `shutdown-timeout-ms` | `10000` |

Environment keys replace hyphens with underscores and add `RSQL_`.
`config print` emits a redacted effective configuration.

Namespace configuration defaults to WAL with `synchronous=full`. Hosts may
choose `synchronous=normal` per namespace for higher write throughput when
losing the newest WAL transactions after a host crash is acceptable.

`max-open-namespaces` is the soft retention bound for namespace handles.
Non-busy handles are selected by a bounded clock-style eviction pass when the
bound is exceeded; active work is never evicted. Increase it only when the
process has enough memory and file descriptors for the additional SQLite
pools.

`namespace-read-connections` controls read concurrency within each open
database. Keep `1` for broad multi-tenant workloads. Raise it for a small number
of hot namespaces after measuring the deployment, accepting additional file
descriptors and per-connection SQLite overhead.

## Telemetry and overview

Prometheus exposes process, Go runtime, normalized HTTP count, and normalized
HTTP duration metrics. Namespace and table names are never metric labels.

Tenant telemetry is separate in `telemetry.db`:

- request completion updates bounded in-memory five-minute buckets
- dirty buckets flush every five seconds and on shutdown
- five-minute buckets remain for 48 hours
- completed data rolls up hourly and remains for 31 days
- SQL, parameters, bodies, rows, and secrets are not stored

`GET /v1/:ns/overview` combines live storage/schema/SSE state with retained
telemetry. Degraded issues are stable codes:

- `storage_near_quota`
- `elevated_error_rate`
- `write_contention`

Overview is tenant-scoped. Fleet listing reads the control registry and latest
telemetry without opening every namespace database.

## Scaling

Handles open lazily and read/write pools initialize independently. Idle or
excess non-busy handles are evicted without scanning the full cache. Startup
does not enumerate the registry or namespace files, so a large cold fleet adds
no per-namespace startup work.

For hundreds of continuously active tenants, shard namespaces across multiple
rsql processes with separate data directories. Never run two processes against
one data directory.

## Recovery and backups

One process owns one data directory. Supported failover uses an offline copy or
a namespace export restored on another process.

Namespace files use deterministic two-level hash directories below
`data/namespaces`. Missing or corrupt files are reported when that namespace is
addressed; startup deliberately performs no fleet-wide repair or cleanup scan.
Use export/import and external storage checks for administrative recovery.

SSE delivery is at-most-once and has no replay cursor. Consumers that require
durable events need a separate durable event system.
