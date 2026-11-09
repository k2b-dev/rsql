# rsql

rsql is a multi-tenant SQLite HTTP server written in Go. Each namespace is an
independent SQLite database with its own schema, lazily opened connection pools,
writer lock, storage quota, telemetry, and change stream.

Documentation: [rsql.k2b.dev](https://rsql.k2b.dev)

It provides:

- namespace provisioning, configuration, duplication, import, and export
- tables, views, indexes, typed row CRUD, filtering, and CSV export
- parameterized SQL reads, writes, and atomic transactions
- server-sent change events and a schema changelog
- tenant-facing storage, traffic, latency, contention, and growth statistics
- Go and TypeScript clients

## Install

Native packages are the recommended production installation on systemd-based
Linux distributions.

Debian 12 or newer:

```bash
curl -fLO https://github.com/k2b-dev/rsql/releases/download/v1.1.0/rsql_1.1.0_amd64.deb
sudo apt install ./rsql_1.1.0_amd64.deb
```

Rocky Linux 9 or newer:

```bash
curl -fLO https://github.com/k2b-dev/rsql/releases/download/v1.1.0/rsql-1.1.0-1.x86_64.rpm
sudo dnf install ./rsql-1.1.0-1.x86_64.rpm
```

The packages install the `rsql` CLI, a hardened systemd unit, and
`/etc/rsql/rsql.env`. Set `RSQL_API_TOKEN` in that file before starting the
service:

```bash
sudo systemctl enable --now rsql
```

Static Linux and macOS binaries and the
`ghcr.io/k2b-dev/rsql:1.1.0` container image are also available from the
[GitHub release](https://github.com/k2b-dev/rsql/releases/tag/v1.1.0).

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

Run the published documentation image on port 3000:

```bash
docker run --rm -p 3000:3000 ghcr.io/k2b-dev/rsql-docs:latest
```

SEO URLs default to `https://rsql.k2b.dev`. Set `RSQL_DOCS_SITE_URL` only when
the documentation is deployed at a different origin.

Start with the [quickstart](./docs/en/quickstart.md), then use the
[production installation guide](./docs/en/operations/installation.md),
[HTTP API reference](./docs/en/reference/http-api.md),
[Go client guide](./docs/en/clients/go.md), or
[multi-tenant hosting guide](./docs/en/hosting/multi-tenant.md).

## Clients

```bash
go get github.com/k2b-dev/rsql@v1.1.0
bun add @k2b/rsql
```

See the [Go client guide](./docs/en/clients/go.md) and
[TypeScript client guide](./docs/en/clients/typescript.md).

## License

MIT, see [LICENSE](./LICENSE).
