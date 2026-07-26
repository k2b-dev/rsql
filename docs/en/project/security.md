---
title: Security
navTitle: Security
section: Project
order: 220
description: Deploy rsql within its authentication, network, SQL, and resource boundaries.
tags: [security, authentication, deployment]
updated: 2026-07-26
---

# Security

rsql is an authenticated internal database service, not a public tenant
identity provider or untrusted-code sandbox.

## Deployment requirements

- Keep the rsql listener on a private network or loopback interface.
- Store the server-wide API token as a secret and rotate it through deployment
  configuration.
- Put tenant authentication and namespace authorization in a gateway.
- Never accept a namespace path directly from an untrusted caller.
- Restrict the public `/metrics` endpoint at the network boundary.
- Keep pprof disabled or bound to a protected operations listener.
- Apply request-size, rate, CPU, memory, and stream limits outside rsql.

## SQL controls

Schema and filter identifiers are validated. Values are parameterized. The
query endpoint accepts only one constrained `SELECT` or `WITH` statement,
rejects comments and semicolons, blocks write/DDL keywords, and blocks internal
objects.

These controls reduce accidental and malicious SQL access but do not replace
gateway authorization or operating-system isolation.

## Data files and backups

Protect the data directory and exported `.db` files as sensitive tenant data.
Use encrypted storage and transport where required by the deployment. Database
exports contain complete namespace contents.

## Reporting vulnerabilities

Do not publish security vulnerabilities as public issues. Follow the repository
[security policy](https://github.com/ValentinKolb/rsql/security/policy) to
report them privately.
