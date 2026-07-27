#!/bin/sh

set -eu

case "${1:-}" in
	0 | remove)
		if command -v systemctl >/dev/null 2>&1; then
			systemctl disable --now rsql.service >/dev/null 2>&1 || true
		fi
		;;
esac
