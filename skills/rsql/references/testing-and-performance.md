# Testing and performance

Use this reference before completing implementation, API, concurrency,
documentation, or performance work.

## Required gates

For any Go behavior change:

```sh
gofmt -w <changed-go-files>
go test ./...
go vet ./...
```

For concurrency, shared state, namespace lifecycle, SSE, telemetry, or storage:

```sh
go test -race ./...
```

For the TypeScript client:

```sh
cd client
bunx tsc --noEmit
bun test ./tests
```

For Fibel documentation:

```sh
bun run docs:typecheck
bun run docs:build
```

Then verify rendered pages, search, raw `.md`, `llms.txt`, and links in a real
browser.

## Test placement

| Behavior | Primary tests |
| --- | --- |
| HTTP route and envelope | `internal/httpapi/*_test.go` |
| Service orchestration and isolation | `internal/service/*_test.go` |
| Handle locking and eviction | `internal/namespace/manager_test.go` |
| SQLite schema, filters, rows, query, CSV | `internal/store/sqlite/*_test.go` |
| Telemetry retention and aggregation | `internal/telemetry/*_test.go` |
| Go client contract | `client/*_test.go` |
| TypeScript/browser contract | `client/tests/*.test.ts` |

Test expected failure paths, not only successful responses. API changes must
verify error codes, status codes, metadata propagation, and both clients.

## Performance framework

The Bun runner under `client/perf` implements S1-S12:

- namespace lifecycle
- schema lifecycle
- point reads
- filter, pagination, and search
- collaborative writes
- bulk/upsert
- read-only SQL
- SSE
- import/export
- overview/changelog
- noisy-neighbor isolation
- cold/warm behavior

Commands:

```sh
cd client
bun run perf:fast
bun run perf:deep
bun run perf:report --run=<run-id-or-path>
```

`fast` is the normal local baseline. `deep` increases scale and can collect
pprof data. Keep benchmark setup flags synchronized with `rsql serve --help`.

Do not treat throughput as valid unless the scenario also validates persisted
counts, tenant isolation, event counts, or import integrity.

## Documentation synchronization

When behavior changes, search all of:

```sh
rg -n '<endpoint-or-field>' README.md docs skills/rsql client/README.md
```

The root README stays an overview. User-facing behavior belongs under
`docs/en`; agent-only architecture and source routing belongs under
`skills/rsql/references`.
