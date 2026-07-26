# Storage and query behavior

Use this reference when designing schemas, validating rows, building filters,
running SQL, importing or exporting CSV, or configuring storage quotas.

## Per-namespace database

Each namespace owns one SQLite database file. rsql creates private tables and
metadata that user APIs cannot address. SQLite object names beginning with `_`
are reserved.

The namespace manager initializes resources independently:

- a query-only pool on the first read, bounded by
  `namespace-read-connections`
- one writer connection on the first write
- a per-namespace writer mutex

Reads may run concurrently with a write under WAL. Writes inside one namespace
are serialized. Read-only namespaces do not retain writer connections. Separate
namespaces have separate locks and pools.

## Schema

Tables receive managed `id`, `created_at`, and `updated_at` columns. User
columns support:

- text, integer, real, boolean, json
- date and datetime
- select with `options`
- constraints such as not-null, unique, min/max, pattern, and max length
- indexes and generated formulas

Views use a read-only SQL definition. Mutations against views are rejected.
Schema changes append to the internal changelog and publish schema SSE events.

Generated-column formulas enter SQLite DDL and therefore use a strict
allowlist. Treat rejected formulas as unsupported rather than trying to bypass
validation.

## Rows and validation

rsql applies namespace configuration before each operation. Write validation
and SQL mutation happen inside one serialized write transaction.

Bulk changes use one filter expression and return either counts or rows,
depending on `Prefer`. Published SSE events include row data for single changes
and ids/count for bulk changes.

## Filters

Row list, bulk mutation, and CSV export share the query grammar:

- comparison: `eq`, `neq`, `gt`, `gte`, `lt`, `lte`
- matching: `like`, `ilike`
- set/null/boolean: `in`, `is.null`, `is.true`, `is.false`
- negation: `not.<operator>`
- logical groups: `or=(...)`, `and=(...)`
- projection and aggregates through `select`
- `order`, `limit`, `offset`, and `search`

Repeated values for one column are not an implicit range. Use an `and` group.
FTS search is used when a compatible FTS index exists; otherwise search falls
back to LIKE-compatible behavior.

Keep identifier quoting and value parameterization separate. Identifiers pass
strict validation; user values must remain bound parameters.

## Read-only SQL

`internal/store/sqlite.ExecuteReadOnly` accepts a single query or a batch. It
rejects non-read-only statements and access to internal objects. This is
defense in depth for a trusted backend caller, not a sandbox for hostile SQL.

## Storage quotas

`max_db_size` is enforced with SQLite's page limit on the writer connection.
The unit is bytes at the API boundary, but enforcement is page-granular.

Rejected quota reductions keep the previous config. Oversized writes and DB
imports return `507 quota_exceeded`. Failed DB import validation or quota
checks preserve the existing namespace database.

WAL and SHM bytes are reported but are not included in the hard page quota.

## Export and import

- Namespace export uses a consistent temporary SQLite snapshot.
- Namespace DB import validates SQLite integrity and storage before replacing
  the active file.
- CSV import maps header fields to row payloads and uses normal row insertion.
- Table CSV export streams RFC 4180 output and shares row filters.
