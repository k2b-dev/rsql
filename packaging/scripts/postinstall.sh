#!/bin/sh

set -eu

if command -v systemctl >/dev/null 2>&1; then
	systemctl daemon-reload >/dev/null 2>&1 || true
fi

printf '%s\n' "rsql installed. Set RSQL_API_TOKEN in /etc/rsql/rsql.env, then run:"
printf '%s\n' "  systemctl enable --now rsql"
