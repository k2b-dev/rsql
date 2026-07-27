# Troubleshooting

Use this reference when an rsql integration fails, stalls, exceeds limits, or
behaves differently under load.

## Establish the failing boundary

Record:

- rsql base URL and route shape, without the token
- namespace and operation
- HTTP status and JSON `error`
- `X-Request-ID`
- caller timeout and cancellation state
- relevant namespace overview window

Check the process first:

```sh
curl -fsS http://127.0.0.1:8080/healthz
curl -fsS http://127.0.0.1:8080/metrics | head
```

Then make one authenticated namespace request with `curl`. This separates
server behavior from client or gateway behavior.

## Common statuses

### 401 unauthorized

- Confirm `Authorization: Bearer <token>`.
- Confirm the gateway replaces caller authorization with the private token.
- Do not solve this by exposing the token to a browser or tenant.

### 404 namespace or table not found

- Verify the orchestrator provisioned the namespace.
- Verify the gateway resolved the expected namespace from trusted state.
- Verify table schema was applied after provisioning or restore.

### 405 read_only

- Do not mutate a view.
- Check whether namespace configuration has `read_only: true`.

### 507 quota_exceeded

- Read `GET /v1/{namespace}/overview`.
- Compare allocated bytes with `quota_bytes`; enforcement uses allocated pages.
- Increase `max_db_size`, delete data and reclaim pages, or move the workload.
- WAL and SHM bytes are reported but are outside the hard page quota.

### 503 database_busy

- Inspect `write_contention.wait`, `busy_errors`, and `timeouts` in overview.
- Reduce concurrent writes to one namespace or batch them.
- Keep write transactions short.
- Shard continuously hot namespaces across rsql processes.

## Timeouts and slow queries

Compare:

- caller timeout
- server `query-timeout-ms`
- namespace `query_timeout`
- overview p50, p95, and p99 latency
- Prometheus endpoint latency

For row lists, inspect filters, `search`, selected columns, ordering, limit, and
offset. Add regular or FTS indexes when query shape and measured latency justify
them. Avoid large offsets for latency-sensitive pagination.

Use optional pprof only on a protected operations listener:

```sh
rsql serve \
  --api-token=dev-token \
  --pprof-enabled \
  --pprof-listen=127.0.0.1:6060
```

## SSE disconnects or missing events

- Confirm the request remains open and its context was not cancelled.
- Confirm the optional `tables` filter includes the event table.
- Expect a keepalive comment every 25 seconds.
- Consume events promptly; subscriber buffers are bounded.
- Reconnect after transport failure.
- Do not rely on SSE for durable replay. Persist business events separately.

Browsers may limit concurrent HTTP/1.1 streams per origin. Terminate or
multiplex subscriptions in a gateway when an application needs many logical
streams.

## Import and export

- Always close export response bodies.
- Stream large exports to storage instead of reading them into memory.
- Database import targets an existing namespace and replaces its database after
  validation.
- CSV import requires `?table=<name>` and multipart field `file`.
- A truncated streaming export is a failed backup even if headers were `200`.

## TypeScript environment failures

The default `rsql` export resolves `RSQL_URL` and `RSQL_API_TOKEN` only on first
access. In browsers, create an explicit client against a trusted gateway and do
not embed the administrative token.

HTTP failures return `RsqlResult<T>` with `ok: false`. Network failures reject
the promise.

## Performance framework

For reproducible local diagnosis:

```sh
cd client
bun run perf:fast
bun run perf:deep
bun run perf:report --run=<run-id-or-path>
```

The scenarios validate correctness while measuring namespace lifecycle,
schema, reads, writes, bulk operations, SQL, SSE, import/export, overview,
noisy-neighbor behavior, and cold/warm state. Compare runs only on the same host
with the same profile and seed.
