---
title: Configuration
navTitle: Configuration
section: Operations
order: 170
description: Configure rsql through Cobra flags, environment variables, and dotenv files.
tags: [configuration, cobra, viper]
updated: 2026-07-27
---

# Configuration

rsql resolves configuration in this order:

1. CLI flag.
2. `RSQL_*` environment variable.
3. Built-in default.

At startup, `.env` and then `.env.local` are loaded when present. Environment
variables use uppercase names and replace hyphens with underscores.

| Flag | Environment | Default |
| --- | --- | --- |
| `--listen` | `RSQL_LISTEN` | `127.0.0.1:8080` |
| `--data-dir` | `RSQL_DATA_DIR` | `data` |
| `--api-token` | `RSQL_API_TOKEN` | required |
| `--log-level` | `RSQL_LOG_LEVEL` | `info` |
| `--query-timeout-ms` | `RSQL_QUERY_TIMEOUT_MS` | `10000` |
| `--namespace-idle-timeout-ms` | `RSQL_NAMESPACE_IDLE_TIMEOUT_MS` | `300000` |
| `--max-open-namespaces` | `RSQL_MAX_OPEN_NAMESPACES` | `128` |
| `--namespace-read-connections` | `RSQL_NAMESPACE_READ_CONNECTIONS` | `1` |
| `--read-timeout-ms` | `RSQL_READ_TIMEOUT_MS` | `15000` |
| `--write-timeout-ms` | `RSQL_WRITE_TIMEOUT_MS` | `15000` |
| `--idle-timeout-ms` | `RSQL_IDLE_TIMEOUT_MS` | `120000` |
| `--shutdown-timeout-ms` | `RSQL_SHUTDOWN_TIMEOUT_MS` | `10000` |
| `--pprof-enabled` | `RSQL_PPROF_ENABLED` | `false` |
| `--pprof-listen` | `RSQL_PPROF_LISTEN` | `127.0.0.1:6060` |

Supported log levels are `debug`, `info`, `warn`, and `error`.

The default listener is intentionally restricted to loopback. Set it to a
private interface or `0.0.0.0:8080` only when the deployment provides the
required network boundary.

`max-open-namespaces` is a soft retention bound. Active handles are never
evicted, but excess non-busy handles are closed by a bounded clock-style pass.
Higher values require more memory and file descriptors for SQLite pools.

`namespace-read-connections` limits concurrent reads inside each open
namespace. Keep the default of `1` for large multi-tenant fleets. Increase it
for deployments where a small number of databases receive many parallel reads;
each additional connection increases file-descriptor and statement-cache use.

## Inspect the effective configuration

```bash
rsql config print \
  --format=json \
  --api-token=dev-token
```

The token is redacted in output.

## Namespace configuration

Server configuration controls the process. Each namespace separately stores
`journal_mode`, `synchronous`, `busy_timeout`, `max_db_size`, `query_timeout`,
`foreign_keys`, and `read_only`. Manage those through the
[Namespace API](/en/reference/namespaces-api).
