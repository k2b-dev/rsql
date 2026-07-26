---
title: Namespaces and Isolation
navTitle: Namespaces
section: Concepts
order: 30
description: Understand rsql database isolation, lifecycle, and runtime limits.
tags: [namespaces, isolation, sqlite]
updated: 2026-07-26
---

# Namespaces and Isolation

A namespace is the unit of data ownership in rsql. Each namespace maps to a
separate SQLite database file and has independent schema, rows, configuration,
connection pools, writer lock, quota accounting, telemetry, changelog, and SSE
subscriber set.

## Control plane and data plane

Control-plane routes start with `/v1/namespaces`. They provision, inspect,
configure, copy, import, export, and delete complete databases.

Data-plane routes start with `/v1/{namespace}`. They operate inside one existing
database: tables, rows, queries, events, changelog, and overview.

This path split is useful for a gateway because database users usually need
only the data plane. The orchestrator retains the control plane.

## Runtime model

Namespaces and their SQLite pools open lazily:

- the read pool opens on the first read
- the serialized writer opens on the first write
- a read-only working set does not retain writer connections

`namespace-read-connections` can raise the per-namespace read limit for a small
number of hot databases. The default minimizes connection setup, file
descriptors, and prepared-statement duplication across large tenant fleets.

The manager retains up to `max-open-namespaces` namespace handles and uses a
bounded clock-style eviction pass for excess or idle non-busy handles. The
default is `128`. Active work is never evicted, so the number of open handles
can temporarily exceed the configured value.

## Isolation boundary

Separate files and handles prevent SQL in one namespace from addressing another
namespace. The query API also rejects internal objects and non-read-only SQL.

rsql does not authenticate individual tenants. Its bearer token can access
every namespace. Network isolation and an authorization gateway are required
for hostile or mutually untrusted callers.

## Lifecycle behavior

Deleting a namespace removes its database and registry entry. Duplicate and
database import operations create a separate file. Database paths are derived
from the namespace name and distributed across two hash-directory levels.

Startup opens only the control and telemetry stores. It does not enumerate the
namespace registry or scan tenant files. Missing or corrupt tenant databases
therefore fail when addressed instead of delaying process startup. The first
write opens the writer pool and applies the stored database configuration.

See [Namespace API](/en/reference/namespaces-api) for lifecycle requests and
[Multi-tenant hosting](/en/hosting/multi-tenant) for the deployment boundary.
