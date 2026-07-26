---
title: TypeScript Client
navTitle: TypeScript
section: Clients
order: 70
description: Use the stateless hierarchical TypeScript client in Bun, Node, or browsers.
tags: [typescript, bun, browser]
updated: 2026-07-26
---

# TypeScript Client

`@k2b/rsql` is a fetch-based, browser-compatible client. Its modules are
stateless namespaces built from immutable URL and token configuration.

```bash
bun add @k2b/rsql
```

## Create an explicit client

```ts
import { createRsqlClient } from "@k2b/rsql";

const client = createRsqlClient({
  url: "http://127.0.0.1:8080",
  token: "dev-token",
});

const contacts = client.ns("demo").table<{
  id: number;
  name: string;
  status: "active" | "inactive";
}>("contacts");

const result = await contacts.rows.list({
  status: "eq.active",
  order: "name.asc",
});
```

The namespace hierarchy is:

```txt
client.namespaces
client.ns(name).tables
client.ns(name).table(name).schema
client.ns(name).table(name).indexes
client.ns(name).table(name).rows
client.ns(name).table(name).export(...)
client.ns(name).query
client.ns(name).changelog
client.ns(name).overview
client.ns(name).events
```

## Result values

Operations resolve to `RsqlResult<T>` rather than throwing for HTTP errors:

```ts
const result = await contacts.rows.get(42);

if (!result.ok) {
  console.error(result.status, result.error.error, result.error.message);
  return;
}

console.log(result.data);
```

Network and runtime failures may still reject the promise.

## Default client

The exported `rsql` client reads `RSQL_URL` and `RSQL_API_TOKEN` only when it is
first accessed:

```ts
import { rsql } from "@k2b/rsql";

const result = await rsql.namespaces.list();
```

Lazy resolution keeps module import browser-safe. Browser applications should
usually construct an explicit client with application-provided configuration;
do not ship the server-wide administrative token to a browser.

## Streaming

Database and CSV exports return the native `Response`, so callers choose how to
consume the body. SSE returns an async iterable and an explicit `close`
function:

```ts
const subscription = await client.ns("demo").events.subscribe({
  tables: ["contacts"],
});

if (subscription.ok) {
  for await (const event of subscription.data.stream) {
    console.log(event.table, event.action);
  }
}
```

Pass an `AbortSignal` in the subscription options to integrate cancellation.
