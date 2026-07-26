---
title: Performance Framework
navTitle: Performance
section: Project
order: 210
description: Run deterministic fast and deep real-world rsql workloads.
tags: [performance, benchmarks, profiling]
updated: 2026-07-27
---

# Performance Framework

The benchmark runner in `client/perf` starts an isolated rsql process, seeds
deterministic domain data, executes scenario workloads, samples server metrics,
and writes a bottleneck report.

## Run profiles

```bash
cd client
bun run perf:fast
bun run perf:deep
bun run perf:report --run=<run-id-or-path>
```

Use a short smoke run while changing the framework:

```bash
bun run perf:fast --quick=true
```

`fast` is the normal local feedback profile. `deep` increases namespace counts,
rows, repetitions, load phases, and optional profiling.

## Published 1,000-namespace results

The published run provisions 1,000 databases with 100 rows each. Four
load-generator processes issue 256 concurrent requests while rotating across
the complete fleet. Every repetition executes a fixed operation count and then
checks the exact persisted row count.

The reported value is the median of three repetitions. The run was repeated
after all lifecycle and cache changes, and every repetition verified persisted
row counts.

| Workload | Throughput | p50 | p95 | p99 |
| --- | ---: | ---: | ---: | ---: |
| Point reads | 54,051 requests/s | 3.75 ms | 12.24 ms | 17.98 ms |
| Batched inserts | 5,711 requests/s | 29.43 ms | 152.76 ms | 283.05 ms |

Each insert request contains 20 rows, producing 114,226 written rows/s with the
default `synchronous=full` durability. All requests succeeded and every
repetition persisted the expected number of rows.

The read workload used 225 MB peak RSS and retained about 4,000 open files when
the benchmark deliberately kept all 1,000 namespace handles warm. Before
read/write pools were initialized independently, the same workload used 343 MB
and about 6,000 open files. Production defaults to 128 retained handles. A
deployment may provision far more namespaces than it keeps simultaneously hot.

## Cold fleet startup

A separate startup check populated `control.db` with 100,000 active namespace
records while leaving all tenant databases cold. After a clean checkpoint,
rsql became healthy in 53.65 ms with 22.4 MB RSS and 19 open files. No tenant
database was opened and all 100,000 registry rows remained present.

This check validates the startup architecture rather than namespace request
capacity: startup opens the control and telemetry stores but does not enumerate
registry rows or scan namespace files.

## Read-pool profiles

The default read-pool limit of one connection is optimized for broad
multi-tenant traffic. Raising the limit to four favors many concurrent reads
against one hot database. The pool itself still opens only on the first read.

| Traffic shape | Read connections | Throughput | p95 |
| --- | ---: | ---: | ---: |
| 1,000 rotating namespaces | 1 | 54,051 requests/s | 12.24 ms |
| 1,000 rotating namespaces | 4 | 25,006 requests/s | 34.84 ms |
| One hot namespace | 1 | 27,614 requests/s | 26.62 ms |
| One hot namespace | 4 | 57,812 requests/s | 12.27 ms |

Configure this with `--namespace-read-connections` or
`RSQL_NAMESPACE_READ_CONNECTIONS`. Benchmark the actual tenant distribution
before changing it.

## Durability profile

`synchronous=full` remains the default. Hosts that accept losing the newest WAL
transactions after an operating-system crash or power loss can opt individual
namespaces into `synchronous=normal`.

| Mode | Written rows/s | p95 | p99 |
| --- | ---: | ---: | ---: |
| `full` | 121,071 | 149.38 ms | 277.36 ms |
| `normal` | 188,619 | 96.23 ms | 128.75 ms |

The two modes use the same binary, data, client load, and fixed-operation
protocol. `normal` improved row throughput by 56% in this run. It is a
durability tradeoff, not a free default.

## Process sharding

The same 1,000 namespaces were distributed across independent rsql processes
on one host while total client concurrency remained fixed.

| Processes | Point reads/s | Read p95 | FULL rows/s | Write p95 |
| ---: | ---: | ---: | ---: | ---: |
| 1 | 23,881 | 34.63 ms | 134,902 | 115.94 ms |
| 2 | 38,275 | 26.76 ms | 102,524 | 159.46 ms |
| 4 | 42,635 | 22.00 ms | 93,062 | 247.70 ms |

Read throughput scales across process boundaries. FULL-durable writes do not
scale when every process shares one physical volume; additional writers
increase fsync and disk contention. Use separate volumes or hosts when
sharding write-heavy tenants.

## Scenario coverage

The matrix covers namespace lifecycle, schema changes, point reads,
filters/pagination/search, concurrent writes, bulk/upsert, read-only SQL, SSE
fanout, import/export, overview/changelog polling, noisy-neighbor isolation, and
cold/warm behavior.

Each scenario validates data correctness in addition to latency and throughput.

## Artifacts

A run records:

- host, toolchain, commit, dirty state, configuration, and seed
- operation-level latency/status events
- one-second Prometheus and process samples
- scenario p50/p95/p99, throughput, errors, and variation
- database and WAL growth
- prioritized bottleneck hypotheses
- optional CPU, heap, mutex, and block profiles

Benchmark results are machine-specific. Compare runs on the same host with the
same profile and seed rather than treating one number as a universal capacity
claim.

The complete scenario and report schema remains documented in
`client/perf/README.md` in the repository.
