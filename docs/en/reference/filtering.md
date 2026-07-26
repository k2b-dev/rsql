---
title: Filtering and Pagination
navTitle: Filtering
section: API Reference
order: 150
description: Filter, search, order, select, aggregate, and paginate row results.
tags: [filters, search, pagination]
updated: 2026-07-26
---

# Filtering and Pagination

Row lists, bulk mutations, and CSV export share one query grammar.

## Controls

| Parameter | Default | Behavior |
| --- | --- | --- |
| `select` | `*` | Comma-separated columns or aggregates |
| `order` | `id.asc` | Comma-separated `column.asc` or `column.desc` |
| `limit` | `100` | Page size, capped at 10000 |
| `offset` | `0` | Number of rows to skip |
| `search` | empty | FTS query or `LIKE` fallback |

CSV export omits the 100-row default when `limit` is absent.

## Column filters

Use `column=operator.value`:

| Operator | Example |
| --- | --- |
| `eq`, `neq` | `status=eq.active` |
| `gt`, `gte`, `lt`, `lte` | `score=gte.80` |
| `like`, `ilike` | `name=ilike.*ada*` |
| `in` | `status=in.(active,pending)` |
| `is` | `deleted_at=is.null` |
| `not` | `status=not.eq.archived` |

`like` and `ilike` translate `*` to SQLite `%`. `ilike` uses case-insensitive
comparison.

Each column query key uses only its first value. To express a range on one
column, use a logical expression:

```http
?and=(score.gte.50,score.lte.95)
```

## Logical expressions

```http
?or=(status.eq.active,priority.gte.3)
?and=(status.eq.active,city.ilike.*berlin*)
```

Nested `and=(...)`, `or=(...)`, and `not.` expressions are supported. URL-encode
query values in production clients.

## Selection and aggregation

Select columns:

```http
?select=id,name,status
```

Aggregate all rows:

```http
?select=count()
```

Group by regular selected columns:

```http
?select=status,count(),score.avg()&order=count().desc
```

Supported column aggregates are `count()`, `sum()`, `avg()`, `min()`, and
`max()`.

## Search

`search=term` uses a table's FTS index when present. Without FTS, it performs a
substring `LIKE` search over `text`, `select`, `date`, and `datetime` columns.
Create FTS indexes for large searchable tables and user-facing query latency.
