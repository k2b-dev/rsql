---
title: rsql
navTitle: Overview
section: Start
order: 10
description: Run isolated SQLite databases behind one HTTP service.
tags: [rsql, sqlite, multi-tenant]
updated: 2026-07-26
image: /assets/rsql-social.png
---

# rsql

<div class="rsql-home">
  <section class="rsql-hero">
    <div class="rsql-home-shell rsql-hero-grid">
      <div>
        <p class="rsql-eyebrow">Multi-tenant SQLite over HTTP</p>
        <h1>One service. <em>Every database.</em></h1>
        <p class="rsql-hero-copy">Give every tenant an isolated SQLite database without building a database control plane from scratch. Provision centrally, bind requests to one namespace, and operate the fleet with predictable limits.</p>
        <div class="rsql-actions">
          <a class="rsql-action rsql-action-primary" href="/en/quickstart">Start with rsql</a>
          <a class="rsql-action" href="/en/reference/http-api">Explore the HTTP API</a>
        </div>
      </div>
      <div class="rsql-runtime" aria-label="Three isolated namespace databases">
        <div class="rsql-runtime-card">
          <header><span class="rsql-runtime-name">tenant_acme.db</span><span class="rsql-runtime-state">healthy</span></header>
          <div class="rsql-runtime-track" aria-hidden="true"><span style="--usage:56%"></span></div>
          <div class="rsql-runtime-meta"><span>566 MB used</span><span>1 GB quota</span></div>
        </div>
        <div class="rsql-runtime-card">
          <header><span class="rsql-runtime-name">tenant_north.db</span><span class="rsql-runtime-state">healthy</span></header>
          <div class="rsql-runtime-track" aria-hidden="true"><span style="--usage:27%"></span></div>
          <div class="rsql-runtime-meta"><span>274 MB used</span><span>1 GB quota</span></div>
        </div>
        <div class="rsql-runtime-card">
          <header><span class="rsql-runtime-name">tenant_studio.db</span><span class="rsql-runtime-state">healthy</span></header>
          <div class="rsql-runtime-track" aria-hidden="true"><span style="--usage:74%"></span></div>
          <div class="rsql-runtime-meta"><span>744 MB used</span><span>1 GB quota</span></div>
        </div>
      </div>
    </div>
  </section>

  <section class="rsql-proof" aria-label="Measured performance">
    <div class="rsql-home-shell">
      <div class="rsql-proof-context">
        <span>Measured fast profile</span>
        <a href="/en/project/performance">Benchmark methodology →</a>
      </div>
      <div class="rsql-proof-grid">
        <div class="rsql-proof-item"><strong class="rsql-proof-value">3,018/s</strong><span class="rsql-proof-label">point reads</span></div>
        <div class="rsql-proof-item"><strong class="rsql-proof-value">0.38 ms</strong><span class="rsql-proof-label">point-read p95</span></div>
        <div class="rsql-proof-item"><strong class="rsql-proof-value">666/s</strong><span class="rsql-proof-label">mixed writes</span></div>
        <div class="rsql-proof-item"><strong class="rsql-proof-value">0 / 559k</strong><span class="rsql-proof-label">errors / measured ops</span></div>
      </div>
      <p class="rsql-proof-note">10 namespaces · 12 real-world scenarios · machine-specific, not a capacity SLA</p>
    </div>
  </section>

  <section class="rsql-section">
    <div class="rsql-home-shell">
      <div class="rsql-section-head">
        <div>
          <p class="rsql-eyebrow">A complete database boundary</p>
          <h2>The pieces a tenant platform needs.</h2>
        </div>
        <p class="rsql-section-lead">rsql keeps the control plane explicit and the data plane namespace-scoped. Your gateway owns identity and routing; rsql owns the database lifecycle.</p>
      </div>
      <div class="rsql-capabilities">
        <article class="rsql-capability rsql-capability-wide">
          <h3>Isolate by database file, not by convention</h3>
          <p>Every namespace has separate schema, data, connection pools, writer lock, quota accounting, telemetry, and realtime subscribers.</p>
          <span class="rsql-capability-code">/v1/{namespace}/tables/...</span>
        </article>
        <article class="rsql-capability">
          <h3>Model and query through one API</h3>
          <p>Create tables, views, indexes, and FTS search. Use typed row CRUD for application paths and parameterized read-only SQL for joins and aggregates.</p>
          <span class="rsql-capability-code">rows.list · query.run · export</span>
        </article>
        <article class="rsql-capability">
          <h3>Know when a tenant needs attention</h3>
          <p>One overview reports storage, quota, traffic, latency, writer contention, schema counts, subscribers, and growth without scanning tenant rows.</p>
          <span class="rsql-capability-code">GET /v1/{namespace}/overview</span>
        </article>
        <article class="rsql-capability">
          <h3>Stream instead of buffering</h3>
          <p>Consume SSE change events and stream database or CSV exports. The Go gateway forwarder preserves response streaming on allowed data-plane routes.</p>
          <span class="rsql-capability-code">subscribe · Forward · export</span>
        </article>
        <article class="rsql-capability">
          <h3>Choose the client for the boundary</h3>
          <p>Use the concurrency-safe Go client in gateways and orchestrators, the stateless TypeScript client in trusted applications, or the HTTP API directly.</p>
          <span class="rsql-capability-code">Go · TypeScript · HTTP</span>
        </article>
      </div>
    </div>
  </section>

  <section class="rsql-section">
    <div class="rsql-home-shell">
      <div class="rsql-section-head">
        <div>
          <p class="rsql-eyebrow">From signup to query</p>
          <h2>A small, explicit hosting path.</h2>
        </div>
        <p class="rsql-section-lead">The administrative credential stays private. Tenant applications receive only the database operations your gateway chooses to expose.</p>
      </div>
      <div class="rsql-flow">
        <article class="rsql-flow-step">
          <span class="rsql-flow-label">01 / Provision</span>
          <h3>Create a namespace</h3>
          <p>Your orchestrator creates the database, applies its quota, and migrates the initial schema.</p>
        </article>
        <article class="rsql-flow-step">
          <span class="rsql-flow-label">02 / Resolve</span>
          <h3>Bind the tenant</h3>
          <p>Your gateway authenticates the caller, resolves exactly one namespace, and adds the private rsql token.</p>
        </article>
        <article class="rsql-flow-step">
          <span class="rsql-flow-label">03 / Operate</span>
          <h3>Observe and move</h3>
          <p>Track quota and contention, stream backups, and shard hot namespace groups across rsql processes.</p>
        </article>
      </div>
    </div>
  </section>

  <section class="rsql-section">
    <div class="rsql-home-shell">
      <div class="rsql-code-panel">
        <div class="rsql-code-copy">
          <p class="rsql-eyebrow">Go gateway client</p>
          <h2>Resolve once. Stay scoped.</h2>
          <p>Bind a client to the namespace your application authorized. The resulting database client cannot reach the namespace control plane.</p>
        </div>
        <pre class="rsql-code" aria-label="Go client example"><code><span class="token-key">client</span>, err := rsql.New(rsql.Config{
    BaseURL: <span class="token-string">"http://rsql:8080"</span>,
    Token:   os.Getenv(<span class="token-string">"RSQL_API_TOKEN"</span>),
})
<span class="token-key">db</span>, err := client.Database(resolvedNamespace)
contacts, err := db.Table(<span class="token-string">"contacts"</span>)
rows, err := contacts.Rows.List(ctx, map[string][]string{
    <span class="token-string">"status"</span>: {<span class="token-string">"eq.active"</span>},
})</code></pre>
      </div>
    </div>
  </section>

  <section class="rsql-cta">
    <div class="rsql-home-shell">
      <div class="rsql-cta-card">
        <p class="rsql-eyebrow">Build the first namespace</p>
        <h2>Start local. Keep the production boundary clear.</h2>
        <p>Run rsql, create a namespace and table, then choose the HTTP, Go, or TypeScript path that matches your application.</p>
        <div class="rsql-actions">
          <a class="rsql-action rsql-action-primary" href="/en/quickstart">Open the quickstart</a>
          <a class="rsql-action" href="/en/hosting/multi-tenant">Plan multi-tenant hosting</a>
        </div>
      </div>
    </div>
  </section>
</div>
