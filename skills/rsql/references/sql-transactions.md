# SQL Writes and Transactions

Use these routes when an application owns its SQL schema and needs bound DDL,
DML, or an atomic group of reads and writes. Both require the existing
server-wide bearer token and operate on exactly one provisioned namespace.
Keep that token in a trusted backend and resolve the namespace there.

## Execute one statement

```http
POST /v1/{ns}/query/exec
Content-Type: application/json
```

```json
{"sql":"INSERT INTO notes(body) VALUES (?)","params":["a bound value"]}
```

```json
{"changes":1,"lastInsertRowid":12}
```

`changes` counts direct SQLite row changes, excluding trigger and foreign-key
side effects. `lastInsertRowid` is optional and only returned for an insert
that changed rows. It follows SQLite semantics; it is not a generated key for
`WITHOUT ROWID` tables. DDL normally returns `changes: 0`. Use query mode in a
transaction to retrieve rows from `SELECT` or `RETURNING`.

## Execute an atomic transaction

```http
POST /v1/{ns}/query/transaction
Content-Type: application/json
```

```json
{
  "statements": [
    {"sql":"INSERT INTO notes(body) VALUES (?)","params":["first"]},
    {"sql":"SELECT id, body FROM notes WHERE body = ?","params":["first"],"mode":"query"}
  ],
  "_meta": {"actor":"user-1"}
}
```

```json
{"results":[{"changes":1,"lastInsertRowid":12},{"data":[{"id":12,"body":"first"}]}]}
```

Each entry defaults to `mode: "exec"`. `mode: "query"` returns `data`, including
an empty array for no rows. Query mode also supports DML with `RETURNING`.
Results appear in input order and are sent only after commit. All statements
run on one connection under the namespace writer lock. Reads inside the
transaction see earlier writes. Existing readers retain SQLite's isolation
semantics; no other writer can interleave.

Any statement failure, timeout, result limit, quota failure, or failed commit
rolls back the whole transaction, including DDL. Statement errors include the
zero-based `statement_index`:

```json
{"error":"conflict","message":"statement 1: ...","statement_index":1}
```

A commit failure, such as a deferred foreign-key violation, identifies the last
statement. Request-level failures before execution may omit the index. A lost
HTTP connection after commit has an unknown outcome for the caller; use
application-owned idempotency keys before retrying writes.

## Bound values and supported SQL

Pass values in `params`. Values are bound through the SQLite driver; never
interpolate caller values into `sql`. JSON parameters are strings, numbers,
booleans, or null. Object and array parameters are rejected. Parameters cannot
stand in for identifiers.

Each entry contains one statement. A trailing semicolon is accepted; SQL
comments and multiple statements in one entry are rejected. Semicolons inside
quoted values are allowed. Use positional `?` or `?NNN` parameters; named
parameters are not accepted.

The connection authorizer permits ordinary namespace DML, tables, indexes,
views, and table alterations. It denies:

- `ATTACH`, `DETACH`, `VACUUM`, transaction control, and savepoints;
- temporary objects, triggers, virtual-table creation, and `ANALYZE`;
- internal objects beginning with `_`, including namespace configuration;
- extension loading, filesystem functions, and functions outside the built-in
  allowlist in `internal/store/sqlite/sql_policy.go`;
- changes to tables, views, or indexes owned by the structured schema API,
  including indirect changes through renames or removed dependencies. Use that
  API for their DDL so its metadata remains consistent;
- eponymous virtual tables other than `json_each` and `json_tree`.

Raw DML uses SQLite constraints rather than the structured row API's logical
type, formula, read-only-column, or timestamp rules. Tables with internal FTS
triggers cannot be mutated through raw SQL; use the row API for those tables.

Only these PRAGMAs are accepted: `table_info`, `table_xinfo`, `index_info`,
`index_xinfo`, `index_list`, and `foreign_key_list` for a non-internal object;
and argument-free reads of `page_count`, `page_size`, and `freelist_count`.
Namespace `read_only` blocks both new routes, even for a read-only transaction.
The existing `/query` route remains available for reads.

## Server-enforced limits

These budgets bound SQL request buffering, result materialization, and writer
occupancy. They are fixed API limits, independent of client settings.

| Resource | Limit |
| --- | --- |
| Entire JSON request, including parameters and `_meta` | 1 MiB |
| SQL text per entry | 64 KiB |
| Parameters per entry | 1,000 |
| Statements per transaction | 1–100 |
| Result rows across all query entries | 10,000 |
| Encoded result row data across all query entries | 4 MiB |
| SQLite value or row for statements without result columns | 4 MiB |
| SQLite value or row for statements with result columns | 4 MiB / column count, rounded down |
| Columns / expression depth / compound SELECT terms | 256 / 100 / 100 |
| LIKE/GLOB pattern / compiled VM instructions | 4,096 bytes / 100,000 |
| Per-statement execution | namespace `query_timeout` milliseconds, default 10,000; at most 30,000 |
| Entire SQL request, including reader/writer wait | 30 seconds |

Result metadata is prepared before execution. The per-value limit bounds the
combined output registers before SQLite or the driver materializes a wide row,
even when output is discarded in exec mode. JSON escaping is counted before
values are copied into the response. The SQLite length limit also applies to
intermediate values and sort records; select fewer columns or return smaller
values when it is exceeded. Exec statements use the same fixed value limit
regardless of earlier query results.

The existing read-only `/query` route uses the same request, statement, result,
and execution budgets. Its batches remain sequential and non-atomic. The
`namespace-read-connections` setting also bounds isolated SQL readers; waiting
for a reader slot counts against the request deadline. Structured view definitions are compiled under the read policy before being stored.

SQLite busy waits on the isolated SQL connection are capped at one second.
Cancellation before the final commit phase rolls back the work. Once commit
starts, it completes without the request's cancellation signal so a durable
success is not reported as a timeout. HTTP/proxy deadlines can be shorter
than the SQL budget; set them to allow enough time to receive the result.
Limits do not replace host CPU, memory, disk, and request-concurrency controls.

Read-only response shapes and batch semantics are unchanged. SQL requests that
exceed the new input, output, or execution budgets now fail with a stable error
instead of consuming unbounded resources. Review large-value queries and
application retries before upgrading. Use pagination or exports for larger
results. Existing stored views are checked transitively by SQL queries; a view
using forbidden objects or functions must be replaced through the schema API.

The existing page-granular `config.max_db_size` applies to writes, indexes,
schema changes, and their changelog records. A full namespace returns HTTP
`507`, `quota_exceeded`. WAL/SHM files and temporary sort files remain outside this page quota. Sorters
use temporary files rather than retaining their entire input in memory.

| Error | Status | Recovery |
| --- | --- | --- |
| `sql_forbidden` | 400 | Use supported SQL or the structured API |
| `invalid_request` | 400 | Correct the statement, mode, or parameters |
| `sql_limit_exceeded` | 413 | Reduce parameters/results or split independent work |
| `sql_timeout` | 408 | Reduce work or improve indexes before retrying |
| `read_only` | 405 | Use `/query` for reads or change the namespace configuration |
| `conflict` | 409 | Resolve the constraint violation |
| `quota_exceeded` | 507 | Free pages or increase the namespace quota |
| `database_busy` | 503 | Retry with bounded backoff |

## Events and schema history

After commit, the existing SSE stream receives one table-level `invalidate`
event for each affected table, or `schema_change` for a schema mutation. Events
include `detail: {"source":"sql"}` and the transaction's optional `_meta`.
These are conservative invalidations, not reconstructed row images: consumers
should refetch, and a statement matching zero rows may still invalidate a table.
Structured views with tracked dependencies receive the existing
`source_changed` notifications.

Schema changes also append `action: "sql"` records to `/changelog` in the same
transaction. Rollbacks emit no events and retain no schema records. SSE remains
live delivery, not a durable event queue.

The existing `query.run` and `query.batch` interfaces remain read-only.
`query.batch` is sequential and does not become a transaction; use
`query.transaction` for atomicity.
