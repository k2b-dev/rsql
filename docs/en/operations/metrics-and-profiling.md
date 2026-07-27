---
title: Metrics and Profiling
navTitle: Metrics and profiling
section: Operations
order: 180
description: Monitor rsql with Prometheus metrics, namespace overview data, and optional pprof.
tags: [prometheus, pprof, operations]
updated: 2026-07-27
---

# Metrics and Profiling

rsql exposes process-wide Prometheus metrics and namespace-scoped operational
overview data. They serve different purposes.

## Prometheus

`GET /metrics` is public and includes:

- `rsql_http_requests_total`
- `rsql_http_request_duration_seconds`
- standard Go runtime collectors
- standard process collectors

HTTP metric labels are `method`, normalized route `path`, and `status`.
Namespace and table names are replaced with route placeholders to keep
cardinality bounded.

Because `/metrics` has no authentication, expose it only on a trusted network
or restrict it at the ingress.

## Namespace telemetry

`GET /v1/{namespace}/overview` is authenticated and designed for tenant database
dashboards. It contains namespace storage, traffic, latency, contention,
schema, realtime, and growth data without tenant-data scans.

Telemetry writes are aggregated in memory and flushed to `telemetry.db`. They
are intentionally not exported as high-cardinality Prometheus labels.

## pprof

Enable pprof only while diagnosing a trusted environment:

```bash
rsql serve \
  --api-token=dev-token \
  --pprof-enabled \
  --pprof-listen=127.0.0.1:6060
```

Profiles are served from the dedicated listener under `/debug/pprof/`. Keep the
listener on loopback or a protected operations network.

```bash
go tool pprof http://127.0.0.1:6060/debug/pprof/profile?seconds=30
go tool pprof http://127.0.0.1:6060/debug/pprof/heap
```

Capture profiles under representative load and compare them with endpoint
latency, process CPU/RSS, and database/WAL growth.
