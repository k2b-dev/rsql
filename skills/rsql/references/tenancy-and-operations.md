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
| `read-timeout-ms` | `15000` |
| `write-timeout-ms` | `15000` |
| `idle-timeout-ms` | `120000` |
| `shutdown-timeout-ms` | `10000` |

Environment keys replace hyphens with underscores and add `RSQL_`.
`config print` emits a redacted effective configuration.

There is no public `max-open-namespaces` setting. The service currently creates
the namespace manager with its default retention bound of 128 handles.

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

Handles open lazily and idle or excess non-busy handles are evicted. Active work
is never forcibly evicted. Hundreds of provisioned databases are practical
when the hot working set is bounded.

For hundreds of continuously active tenants, shard namespaces across multiple
rsql processes with separate data directories. Never run two processes against
one data directory.

## Recovery and backups

One process owns one data directory. Supported failover uses an offline copy or
a namespace export restored on another process.

Startup reconciliation:

- removes temporary export files
- removes active registry rows whose database file is missing
- reapplies internal schema and stored config to existing namespace files
- logs unregistered namespace files without deleting them
- continues when one namespace cannot be healed

SSE delivery is at-most-once and has no replay cursor. Consumers that require
durable events need a separate durable event system.
