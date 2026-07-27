---
title: Development
navTitle: Development
section: Project
order: 200
description: Build, test, document, and validate changes to rsql.
tags: [development, testing, go]
updated: 2026-07-27
---

# Development

## Build and run

```bash
go build -o bin/rsql ./cmd/rsql
go run ./cmd/rsql serve --api-token=dev-token
```

## Go verification

```bash
go test ./...
go test -race ./...
go vet ./...
```

Coverage can be collected with:

```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

## Client verification

```bash
cd client
bun install
bun test
bunx tsc --noEmit
```

## Documentation verification

From the repository root:

```bash
bun install
bun run docs:typecheck
bun run docs:build
bun run docs:dev
```

Fibel serves every page as HTML and as raw Markdown through `.md` and
`.markdown` routes. It also builds search and `llms.txt` indexes.

## Release artifacts

The release builder validates that the tag, CLI version, and TypeScript package
version match. It produces static Linux and macOS archives plus DEB and RPM
packages for `amd64` and `arm64`.

Install the pinned nFPM release, then build into a new output directory:

```bash
go install github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.42.1
./scripts/build-release.sh v1.0.0 /tmp/rsql-release
```

The tag workflow installs the generated packages on Debian and Rocky Linux
before creating the GitHub Release. It also records GitHub artifact
attestations. Container builds target `linux/amd64` and `linux/arm64`.

## Code organization

Keep HTTP handlers focused on transport and service code focused on
orchestration. Put SQLite-specific behavior in `internal/store/sqlite`, and
preserve package boundaries expressed by each package's `doc.go`.

Expected API failures use typed domain errors and stable JSON error codes.
Prefer direct code over speculative abstractions, and add tests for every
behavioral branch introduced by a change.
