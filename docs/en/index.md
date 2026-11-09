---
title: rsql
navTitle: Overview
section: Start
order: 10
description: Provision and operate isolated SQLite databases behind one HTTP service.
tags: [rsql, sqlite, multi-tenant]
updated: 2026-07-27
image: /assets/rsql-social.png
---

# rsql

<div class="rsql-home">
  <section class="rsql-hero">
    <div class="rsql-home-shell rsql-hero-grid">
      <div class="rsql-hero-content">
        <p class="rsql-kicker">SQLite isolation for multi-tenant systems</p>
        <h1>A SQLite database for every tenant.</h1>
        <p class="rsql-hero-copy">rsql provisions independent SQLite databases through one HTTP service. Your gateway resolves the tenant; rsql enforces the namespace boundary, quota, and database lifecycle.</p>
        <div class="rsql-actions">
          <a class="rsql-action rsql-action-primary" href="/en/quickstart">Run the quickstart</a>
          <a class="rsql-action" href="/en/reference/http-api">Read the HTTP API</a>
        </div>
      </div>
      <div class="rsql-trace" aria-label="Request path from application to tenant database">
        <header class="rsql-trace-head">
          <span>Request trace</span>
          <code>GET /contacts?status=active</code>
        </header>
        <ol class="rsql-trace-steps">
          <li>
            <span class="rsql-trace-index">01</span>
            <div><small>Application</small><strong>Authenticated request</strong></div>
            <code>user_83</code>
          </li>
          <li>
            <span class="rsql-trace-index">02</span>
            <div><small>Gateway</small><strong>Resolve namespace</strong></div>
            <code>tenant_acme</code>
          </li>
          <li>
            <span class="rsql-trace-index">03</span>
            <div><small>rsql</small><strong>Authorize data route</strong></div>
            <code>/v1/tenant_acme/...</code>
          </li>
          <li>
            <span class="rsql-trace-index">04</span>
            <div><small>SQLite</small><strong>Open isolated database</strong></div>
            <code>sharded namespace file</code>
          </li>
        </ol>
        <footer class="rsql-trace-foot"><span>Namespace fixed before SQL execution</span><strong>200 OK</strong></footer>
      </div>
    </div>
  </section>

  <section class="rsql-proof" aria-label="Measured performance">
    <div class="rsql-home-shell">
      <div class="rsql-proof-context">
        <div>
          <p class="rsql-kicker">Benchmark results</p>
          <h2>Throughput across parallel tenant workloads.</h2>
        </div>
        <p>The fixed-operation benchmark rotates across 1,000 independent databases and verifies every persisted row. <a href="/en/project/performance">Read the methodology</a>.</p>
      </div>
      <dl class="rsql-proof-grid">
        <div class="rsql-proof-item"><dt>Point reads</dt><dd>54.1k/s <small>aggregate</small></dd></div>
        <div class="rsql-proof-item"><dt>Read latency</dt><dd>12.2 ms <small>p95</small></dd></div>
        <div class="rsql-proof-item"><dt>Batched writes</dt><dd>114k/s <small>rows · FULL</small></dd></div>
        <div class="rsql-proof-item"><dt>Parallel namespaces</dt><dd>1,000 <small>tested</small></dd></div>
      </dl>
      <p class="rsql-proof-note">Median of 3 fixed-operation runs · 20-row write batches · zero request errors</p>
    </div>
  </section>

  <section class="rsql-section">
    <div class="rsql-home-shell rsql-boundary">
      <div class="rsql-section-intro">
        <p class="rsql-kicker">Namespace boundary</p>
        <h2>Isolation is a database file, not a filter.</h2>
        <p>Each namespace owns the state and runtime resources that could otherwise leak or interfere across tenants.</p>
      </div>
      <div class="rsql-boundary-list">
        <article>
          <span class="rsql-boundary-label">Storage</span>
          <div>
            <h3>Independent schema, data, and WAL</h3>
            <p>No shared tenant column and no application-level row filter defines the boundary.</p>
          </div>
          <code>hashed .db file</code>
        </article>
        <article>
          <span class="rsql-boundary-label">Runtime</span>
          <div>
            <h3>Separate pools and writer lock</h3>
            <p>Reads remain concurrent while writes serialize within one database, not across the fleet.</p>
          </div>
          <code>pools on demand</code>
        </article>
        <article>
          <span class="rsql-boundary-label">Limits</span>
          <div>
            <h3>Quota and health per namespace</h3>
            <p>Storage, traffic, latency, contention, schema counts, subscribers, and growth share one overview.</p>
          </div>
          <code>GET /overview</code>
        </article>
        <article>
          <span class="rsql-boundary-label">Movement</span>
          <div>
            <h3>Stream changes, exports, and backups</h3>
            <p>SSE events and streamed database or CSV responses avoid buffering tenant datasets in the gateway.</p>
          </div>
          <code>SSE · CSV · SQLite</code>
        </article>
      </div>
    </div>
  </section>

  <section class="rsql-ownership">
    <div class="rsql-home-shell rsql-ownership-grid">
      <div class="rsql-section-intro">
        <p class="rsql-kicker">Deployment boundary</p>
        <h2>Keep identity in the gateway. Keep data in rsql.</h2>
        <p>The administrative credential never reaches tenant code. The gateway exposes only the data operations the application needs.</p>
      </div>
      <div class="rsql-ownership-columns">
        <article>
          <h3>Your gateway</h3>
          <ul>
            <li>Authenticate the caller</li>
            <li>Map user or account to namespace</li>
            <li>Allow only application routes</li>
            <li>Attach the private rsql token</li>
          </ul>
        </article>
        <article>
          <h3>rsql</h3>
          <ul>
            <li>Provision and migrate databases</li>
            <li>Execute schema, row, and query APIs</li>
            <li>Enforce storage quotas</li>
            <li>Report health and stream changes</li>
          </ul>
        </article>
      </div>
    </div>
  </section>

  <section class="rsql-section">
    <div class="rsql-home-shell">
      <div class="rsql-code-panel">
        <div class="rsql-code-copy">
          <p class="rsql-kicker">Go gateway client</p>
          <h2>Bind once to the resolved namespace.</h2>
          <p>The database client exposes schema, table, query, event, and export operations for that namespace. Namespace administration remains on the parent client.</p>
          <a href="/en/clients/go">Go client guide</a>
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

  <section class="rsql-start">
    <div class="rsql-home-shell rsql-start-grid">
      <div class="rsql-section-intro">
        <p class="rsql-kicker">Install</p>
        <h2>Start with the interface your system already uses.</h2>
        <p>The quickstart creates a local server, namespace, table, and first row.</p>
        <div class="rsql-actions">
          <a class="rsql-action rsql-action-primary" href="/en/quickstart">Open the quickstart</a>
          <a class="rsql-action" href="/en/hosting/multi-tenant">Hosting guide</a>
        </div>
      </div>
      <div class="rsql-install-list">
        <div><span>Debian</span><code>apt install ./rsql_1.1.0_amd64.deb</code></div>
        <div><span>Rocky Linux</span><code>dnf install ./rsql-1.1.0-1.x86_64.rpm</code></div>
        <div><span>TypeScript</span><code>bun add @k2b/rsql</code></div>
        <div><span>Go client</span><code>go get github.com/k2b-dev/rsql@v1.1.0</code></div>
      </div>
    </div>
  </section>
</div>
