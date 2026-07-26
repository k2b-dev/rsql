---
title: Queries and Realtime
navTitle: Query and realtime
section: Concepts
order: 50
description: Use read-only SQL, SSE change events, and the schema changelog.
tags: [sql, sse, changelog]
updated: 2026-07-26
---

# Queries and Realtime

rsql provides three complementary read paths: structured row lists,
parameterized read-only SQL, and server-sent change events.

## Read-only SQL

`POST /v1/{namespace}/query` accepts one statement or a batch. Statements must
begin with `SELECT` or `WITH`. Semicolons, SQL comments, write/DDL keywords,
pragmas, attachments, and references to internal objects are rejected.

The endpoint is intended for joins and aggregations that do not fit the row-list
grammar. It is not a general SQLite administration endpoint.

## Change events

`GET /v1/{namespace}/subscribe` opens an SSE stream. The optional `tables`
parameter limits events to a comma-separated table set. Events contain the
table, action, timestamp, optional row or row IDs, and optional `_meta`.

SSE is efficient for backend services and a small number of browser streams.
Browsers may limit concurrent HTTP/1.1 connections per origin. A public gateway
can multiplex or terminate realtime connections when an application opens many
logical subscriptions.

## Changelog

`GET /v1/{namespace}/changelog` returns persisted schema mutations with
pagination and an optional table filter. Row changes are realtime events, not
changelog records.

## Delivery semantics

SSE streams live events from the in-process namespace broker. Treat them as
notifications, not as a durable queue. Consumers that require durable business
events should persist application events in their own table and resume from
that data.

See [Query, events, and overview API](/en/reference/query-events-api).
