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

Namespaces open lazily. An open namespace uses:

- one serialized writer connection
- up to four concurrent read connections
- one idle read connection

The manager retains up to 128 idle namespace handles and evicts the least
recently used non-busy handle. Active work is never evicted, so the number of
open handles can temporarily exceed 128.

## Isolation boundary

Separate files and handles prevent SQL in one namespace from addressing another
namespace. The query API also rejects internal objects and non-read-only SQL.

rsql does not authenticate individual tenants. Its bearer token can access
every namespace. Network isolation and an authorization gateway are required
for hostile or mutually untrusted callers.

## Lifecycle behavior

Deleting a namespace removes its database and registry entry. Duplicate and
database import operations create a separate file. Startup reconciliation
repairs incomplete namespace initialization, soft-deletes registry entries
whose files disappeared, and reports unknown database files without deleting
them.

See [Namespace API](/en/reference/namespaces-api) for lifecycle requests and
[Multi-tenant hosting](/en/hosting/multi-tenant) for the deployment boundary.
