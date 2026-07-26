---
name: rsql
description: Build, review, debug, document, benchmark, or operate the rsql multi-tenant SQLite HTTP server and its Go and TypeScript clients. Use whenever work touches rsql namespaces, schema or row APIs, filtering, read-only SQL, SSE, storage quotas, namespace overview telemetry, gateway forwarding, CLI configuration, package boundaries, tests, performance scenarios, or files under cmd/rsql, internal, client, docs, or skills/rsql.
---

# rsql

Work from the live implementation and tests. rsql is pre-1.0, so prose and
older examples can drift from the current contract.

## First checks

1. Read `git status --short` and preserve unrelated work.
2. Locate the affected route, service method, store operation, client method,
   and tests before proposing a change.
3. Read only the references relevant to the task.
4. Treat code and tests as authoritative when a reference or user-facing page
   disagrees.

Useful entry points:

```sh
sed -n '1,280p' internal/httpapi/handler.go
sed -n '1,220p' internal/domain/types.go
sed -n '1,260p' internal/domain/overview.go
rg -n 'func \(s \*Service\)|func [A-Z]' internal/service internal/store
rg -n 'export interface|export type' client/src/types.ts
```

## Reference routing

- HTTP endpoints, status codes, payloads, headers, and event contracts:
  `references/http-api.md`
- Go packages, request flow, lifecycle, and responsibility boundaries:
  `references/architecture.md`
- Public Go and TypeScript APIs, streaming, and gateway forwarding:
  `references/clients.md`
- SQLite schema, row validation, filtering, query safety, and write semantics:
  `references/storage-and-queries.md`
- Isolation, quotas, telemetry, configuration, scaling, and recovery:
  `references/tenancy-and-operations.md`
- Required tests, benchmarks, documentation checks, and completion gates:
  `references/testing-and-performance.md`

## Architecture invariants

1. Keep `internal/httpapi` transport-focused.
2. Keep orchestration and domain behavior in `internal/service`.
3. Keep SQLite construction and persistence in `internal/store/sqlite`.
4. Keep namespace registry persistence in `internal/store/control`.
5. Keep per-namespace pools, writer serialization, and eviction in
   `internal/namespace`.
6. Never expose internal SQLite objects or let a tenant select an arbitrary
   namespace through gateway passthrough.
7. Preserve the stable JSON error envelope: `error` and `message`.
8. Treat the server-wide bearer token as an administrative credential.

## Change workflow

1. Identify whether the change affects the control plane
   (`/v1/namespaces/...`), one namespace's data plane (`/v1/:ns/...`), or both.
2. Trace the complete path from handler to service to store/runtime component.
3. State isolation, atomicity, timeout, quota, and event side effects.
4. Implement the smallest complete slice.
5. Update both public clients when the HTTP contract changes.
6. Update user documentation under `docs/en/` and the matching reference file
   in this skill when behavior or architecture changes.
7. Run the gates selected from `references/testing-and-performance.md`.

## Client rules

- Keep Go administration under `Client.Admin().Namespaces`.
- Bind application operations with `Client.Database(namespace)`.
- Keep Go `Database.Forward` limited to namespace-scoped data-plane routes.
- Preserve the TypeScript `client.namespaces` and `client.ns(namespace)` shape.
- Stream exports and SSE. Do not introduce hidden buffering or automatic
  retries.
- Keep browser environment reads lazy; importing the TypeScript client must not
  require `process.env`.

## Documentation rules

- Keep the root README short and navigational.
- Write user-facing documentation in English under `docs/en/`.
- Give each Fibel page frontmatter with `title`, `navTitle`, `section`,
  `order`, and `description`.
- Keep internal implementation detail in this skill's references, not in
  getting-started pages.
- Verify examples against current route and client tests before finishing.
