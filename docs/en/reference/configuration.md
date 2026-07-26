---
title: Configuration
navTitle: Configuration
section: Operations
order: 170
description: Configure rsql through Cobra flags, environment variables, and dotenv files.
tags: [configuration, cobra, viper]
updated: 2026-07-26
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
| `--listen` | `RSQL_LISTEN` | `:8080` |
| `--data-dir` | `RSQL_DATA_DIR` | `data` |
| `--api-token` | `RSQL_API_TOKEN` | required |
| `--log-level` | `RSQL_LOG_LEVEL` | `info` |
| `--query-timeout-ms` | `RSQL_QUERY_TIMEOUT_MS` | `10000` |
| `--namespace-idle-timeout-ms` | `RSQL_NAMESPACE_IDLE_TIMEOUT_MS` | `300000` |
| `--read-timeout-ms` | `RSQL_READ_TIMEOUT_MS` | `15000` |
| `--write-timeout-ms` | `RSQL_WRITE_TIMEOUT_MS` | `15000` |
| `--idle-timeout-ms` | `RSQL_IDLE_TIMEOUT_MS` | `120000` |
| `--shutdown-timeout-ms` | `RSQL_SHUTDOWN_TIMEOUT_MS` | `10000` |
| `--pprof-enabled` | `RSQL_PPROF_ENABLED` | `false` |
| `--pprof-listen` | `RSQL_PPROF_LISTEN` | `127.0.0.1:6060` |

Supported log levels are `debug`, `info`, `warn`, and `error`.

## Inspect the effective configuration

```bash
go run ./cmd/rsql config print \
  --format=json \
  --api-token=dev-token
```

The token is redacted in output.

## Namespace configuration

Server configuration controls the process. Each namespace separately stores
`journal_mode`, `busy_timeout`, `max_db_size`, `query_timeout`,
`foreign_keys`, and `read_only`. Manage those through the
[Namespace API](/en/reference/namespaces-api).
