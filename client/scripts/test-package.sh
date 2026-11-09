#!/usr/bin/env bash

set -euo pipefail

package_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
archive="$(cd "$package_root" && npm pack --silent)"
work_dir="$(mktemp -d)"

cleanup() {
  find "$work_dir" -depth -delete
  find "$package_root" -maxdepth 1 -name "$archive" -delete
}
trap cleanup EXIT

printf '{"type":"module"}\n' > "$work_dir/package.json"
npm install "$package_root/$archive" \
  --prefix "$work_dir" \
  --ignore-scripts \
  --no-audit \
  --no-fund \
  --silent

cat > "$work_dir/smoke.ts" <<'EOF'
import { createRsqlClient, rsql, type RsqlClient, type ExecResult, type TransactionResult } from "@k2b/rsql";

const client: RsqlClient = createRsqlClient({
  url: "https://rsql.example",
  token: "test-token",
  fetch: async () =>
    new Response('{"data":[]}', {
      status: 200,
      headers: { "content-type": "application/json" },
    }),
});

const db = client.ns("packed");
if (typeof db.query.exec !== "function" || typeof db.query.transaction !== "function") {
  throw new Error("SQL write APIs are missing from the packed client");
}
const execution: ExecResult = { changes: 1 };
const transaction: TransactionResult = { results: [execution, { data: [] }] };
if (transaction.results.length !== 2) throw new Error("SQL result types are missing");

if (typeof rsql !== "object") {
  throw new Error("default client export is missing");
}

const result = await client.namespaces.list();
if (!result.ok || result.data.data.length !== 0) {
  throw new Error("packed client request failed");
}
EOF

cat > "$work_dir/tsconfig.json" <<'EOF'
{
  "compilerOptions": {
    "target": "ES2022",
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "module": "NodeNext",
    "moduleResolution": "NodeNext",
    "strict": true,
    "noEmit": true
  },
  "include": ["smoke.ts"]
}
EOF

"$package_root/node_modules/.bin/tsc" -p "$work_dir/tsconfig.json"
node --experimental-strip-types "$work_dir/smoke.ts"
bun build "$work_dir/smoke.ts" --target browser --outdir "$work_dir/browser" >/dev/null
