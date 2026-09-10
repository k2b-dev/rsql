---
title: Production Installation
navTitle: Installation
section: Operations
order: 168
description: Install and operate rsql as a native systemd service on Debian or Rocky Linux.
tags: [installation, systemd, debian, rocky]
updated: 2026-07-27
---

# Production Installation

Native DEB and RPM packages are the recommended production installation. Each
package installs the `rsql` CLI, a hardened systemd unit, and a protected
environment file. Release packages are smoke-tested on Debian 12 and Rocky
Linux 9 on `amd64`; release artifacts are built for both `amd64` and `arm64`.

The service listens on `127.0.0.1:8080` by default. rsql does not install or
configure a reverse proxy, TLS, firewall rules, DNS, or an external monitoring
system.

## Install on Debian

```bash
curl -fLO https://github.com/k2b-dev/rsql/releases/download/v1.0.0/rsql_1.0.0_amd64.deb
sudo apt install ./rsql_1.0.0_amd64.deb
```

Use `rsql_1.0.0_arm64.deb` on an ARM64 host.

## Install on Rocky Linux

```bash
curl -fLO https://github.com/k2b-dev/rsql/releases/download/v1.0.0/rsql-1.0.0-1.x86_64.rpm
sudo dnf install ./rsql-1.0.0-1.x86_64.rpm
```

Use `rsql-1.0.0-1.aarch64.rpm` on an ARM64 host.

## Configure the service

The package installs `/etc/rsql/rsql.env` with mode `0600`. Set a long random
API token before starting the service:

```bash
openssl rand -hex 32
sudoedit /etc/rsql/rsql.env
```

```bash
RSQL_LISTEN=127.0.0.1:8080
RSQL_DATA_DIR=/var/lib/rsql
RSQL_API_TOKEN=replace-with-a-long-random-value
```

Keep the loopback listener when the calling application or gateway runs on the
same host. Bind to a private interface only when the surrounding network
boundary is already enforced.

rsql owns its database settings, connection limits, timeouts, storage paths,
health endpoints, metrics, and shutdown behavior. The deployment owns TLS,
network access, tenant routing, secret distribution, backup scheduling, and
placement across multiple rsql processes.

## Start and verify

```bash
sudo systemctl enable --now rsql
sudo systemctl status rsql
curl -fsS http://127.0.0.1:8080/healthz
```

The package creates the `rsql` service account. Tenant databases, the control
registry, and telemetry are stored below `/var/lib/rsql`. Logs go to the system
journal:

```bash
sudo journalctl -u rsql
```

The CLI is available system-wide:

```bash
rsql version
sudo sh -c 'set -a; . /etc/rsql/rsql.env; exec rsql config print'
```

## Verify release provenance

Every release contains `checksums.txt`. Verify one downloaded package with:

```bash
grep 'rsql_1.0.0_amd64.deb' checksums.txt | shasum -a 256 --check
```

GitHub also records build provenance for the release artifacts:

```bash
gh attestation verify rsql_1.0.0_amd64.deb --repo k2b-dev/rsql
```

## Upgrade

Back up the service before an upgrade. Package installation preserves a
modified `/etc/rsql/rsql.env` and does not restart a running rsql process.

Debian:

```bash
sudo apt install ./rsql_1.0.1_amd64.deb
sudo systemctl restart rsql
```

Rocky Linux:

```bash
sudo dnf upgrade ./rsql-1.0.1-1.x86_64.rpm
sudo systemctl restart rsql
```

Verify `/healthz`, the logs, and a representative namespace after the restart.
GitHub Releases are not an APT or DNF repository, so distribution-wide update
commands do not discover a newer rsql release automatically.

## Portable binary

Every GitHub release also contains static Linux and macOS tarballs with
SHA-256 checksums. The binary has no runtime SQLite or libc dependency.

```bash
tar -xzf rsql_1.0.0_linux_amd64.tar.gz
./rsql version
```

The portable binary does not install a service account, environment file, or
systemd unit.

## Container installation

The OCI image remains supported for container-managed environments:

```bash
docker pull ghcr.io/k2b-dev/rsql:1.0.0
```

Mount one persistent data directory into `/data` and run exactly one rsql
process against it. Container deployment does not change the
[single-process storage boundary](/en/hosting/backups-and-scaling).
