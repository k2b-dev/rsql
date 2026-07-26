# rsql

rsql is a multi-tenant SQLite HTTP server written in Go. Each namespace is an
independent SQLite database with its own schema, lazily opened connection pools,
writer lock, storage quota, telemetry, and change stream.

It provides:

- namespace provisioning, configuration, duplication, import, and export
- tables, views, indexes, typed row CRUD, filtering, and CSV export
- parameterized read-only SQL
- server-sent change events and a schema changelog
- tenant-facing storage, traffic, latency, contention, and growth statistics
- Go and TypeScript clients

## Install

```bash
go install github.com/k2b-dev/rsql/cmd/rsql@v0.4.0
```

The release is also available as
`ghcr.io/k2b-dev/rsql:0.4.0` and as static Linux and macOS binaries on the
[GitHub release](https://github.com/k2b-dev/rsql/releases/tag/v0.4.0).

## Run

```bash
rsql serve \
  --listen=127.0.0.1:8080 \
  --api-token=dev-token \
  --data-dir=./data
```

```bash
curl -s http://127.0.0.1:8080/healthz

curl -s -X POST http://127.0.0.1:8080/v1/namespaces \
  -H 'Authorization: Bearer dev-token' \
  -H 'Content-Type: application/json' \
  -d '{"name":"demo"}'
```

Configuration precedence is CLI flags, `RSQL_*` environment variables, then
defaults. Print the resolved configuration with:

```bash
rsql config print --format=json --api-token=dev-token
```

## Documentation

The Fibel documentation source is in [`docs/en`](./docs/en). Start the local
documentation site with:

```bash
bun install
bun run docs:dev
```

Set `RSQL_DOCS_SITE_URL` to the public documentation origin in production so
canonical links, Open Graph URLs, `robots.txt`, and the sitemap use absolute
URLs.

Start with the [quickstart](./docs/en/quickstart.md), then use the
[HTTP API reference](./docs/en/reference/http-api.md),
[Go client guide](./docs/en/clients/go.md), or
[multi-tenant hosting guide](./docs/en/hosting/multi-tenant.md).

The server is pre-1.0. Breaking API changes may occur before v1.0.

## Clients

```bash
go get github.com/k2b-dev/rsql@v0.4.0
bun add @k2b/rsql
```

See the [Go client guide](./docs/en/clients/go.md) and
[TypeScript client guide](./docs/en/clients/typescript.md).

## License

MIT, see [LICENSE](./LICENSE).
