# Storage and query behavior

Use this reference for schema, row, filter, SQLite, transaction, query, CSV,
and quota-related changes.

## Per-namespace database

Each namespace owns one SQLite database file. Internal tables and metadata are
created by `internal/store/sqlite.EnsureInternalSchema`. User APIs cannot
address SQLite objects whose names begin with `_`.

The namespace manager opens:

- a query-only read pool with up to four connections and one idle connection
- one writer connection
- a per-namespace writer mutex

Reads may run concurrently with a write under WAL. Writes inside one namespace
are serialized. Separate namespaces have separate locks and pools.

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
allowlist. Review `validateGeneratedFormula` and its regression tests before
changing this boundary.

## Rows and validation

The service resolves namespace configuration and executes store operations
inside `WithRead` or `WithWrite`. Write validation and SQL mutation happen
inside the serialized write callback.

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
