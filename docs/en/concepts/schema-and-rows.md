---
title: Schema and Rows
navTitle: Schema and rows
section: Concepts
order: 40
description: Model tables, views, columns, indexes, and typed rows in rsql.
tags: [schema, rows, validation]
updated: 2026-07-26
---

# Schema and Rows

rsql exposes SQLite schema and row operations as JSON APIs. Table metadata is
stored with the namespace so writes can validate and encode logical types.

## Column types

| Logical type | SQLite storage | JSON behavior |
| --- | --- | --- |
| `text` | `TEXT` | string |
| `integer` | `INTEGER` | number |
| `real` | `REAL` | number |
| `boolean` | `INTEGER` | boolean |
| `json` | `TEXT` | parsed JSON |
| `date` | `TEXT` | string |
| `datetime` | `TEXT` | string |
| `select` | `TEXT` | string constrained by `options` |
| `blob` | `BLOB` | binary storage |

Columns may define `not_null`, `unique`, `default`, `index`, validation
constraints, metadata, or a generated `formula`. Formula columns are read-only.

Every table includes:

- `id`: auto-incrementing integer primary key
- `created_at`: automatically initialized UTC timestamp
- `updated_at`: automatically maintained UTC timestamp

These names, `_meta`, and all names beginning with `_` are reserved.

## Tables and views

A `table` request supplies columns. A `view` request supplies a read-only SQL
definition. Tables support row writes; views support reads and CSV exports.

Schema updates can rename an object, add or drop columns, rename columns, update
metadata, or replace view SQL. Schema mutations are recorded in the changelog.

## Indexes and search

Normal indexes cover one or more columns. An FTS index creates an internal FTS5
table used by the `search` row-list parameter. Without FTS, search falls back to
`LIKE` across text-like columns.

## Audit metadata

Write and schema requests may include `_meta`. rsql does not store it as a user
column. It forwards metadata to change events and, for schema changes, the
changelog. Use it for actor IDs, trace IDs, or application-level audit context.

See [Schema API](/en/reference/schema-api) and [Rows API](/en/reference/rows-api).
