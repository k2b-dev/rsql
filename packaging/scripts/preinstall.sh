#!/bin/sh

set -eu

if ! getent group rsql >/dev/null 2>&1; then
	groupadd --system rsql
fi

if ! getent passwd rsql >/dev/null 2>&1; then
	nologin_shell="$(command -v nologin || printf '%s' /usr/sbin/nologin)"
	useradd \
		--system \
		--gid rsql \
		--home-dir /var/lib/rsql \
		--shell "$nologin_shell" \
		--comment "rsql service account" \
		rsql
fi
