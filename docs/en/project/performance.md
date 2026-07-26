---
title: Performance Framework
navTitle: Performance
section: Project
order: 210
description: Run deterministic fast and deep real-world rsql workloads.
tags: [performance, benchmarks, profiling]
updated: 2026-07-26
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
