#!/usr/bin/env bash
#
# Delete the monitors that the CI test suite creates in the Cronitor account.
#
# `cronitor discover` names each monitor after the machine it runs on:
#   Linux:   "[<hostname>] <command>"
#   Windows: "<hostname>/<scheduled task name>"
# Every CI run gets a fresh runner hostname, so every run creates a fresh
# set of monitors. This script finds monitors whose name starts with the
# given hostname and deletes them.
#
# Usage: cleanup-ci-monitors.sh [hostname]
#   hostname       Defaults to the current machine hostname.
#   CRONITOR_BIN   Path to the cronitor binary (default: ../cronitor).
#   CRONITOR_API_KEY  Passed to the CLI with -k when set.
#
# Requires jq.
set -euo pipefail

CRONITOR_BIN="${CRONITOR_BIN:-$(dirname "$0")/../cronitor}"
TARGET_HOST="${1:-$(hostname)}"
MAX_ROUNDS=20

api_key_args=()
if [ -n "${CRONITOR_API_KEY:-}" ]; then
  api_key_args=(-k "$CRONITOR_API_KEY")
fi

# discover shortens hostnames longer than 21 chars to "first9...last9".
name_prefix="$TARGET_HOST"
if [ "${#TARGET_HOST}" -gt 21 ]; then
  name_prefix="${TARGET_HOST:0:9}...${TARGET_HOST: -9}"
fi

total_deleted=0
for _ in $(seq 1 $MAX_ROUNDS); do
  keys=$("$CRONITOR_BIN" monitor list --search "$TARGET_HOST" --format json --page-size 100 "${api_key_args[@]}" \
    | jq -r --arg p "$name_prefix" \
        '.monitors[] | select(.name | (startswith("[" + $p + "] ") or startswith($p + "/"))) | .key')

  if [ -z "$keys" ]; then
    break
  fi

  # shellcheck disable=SC2086
  "$CRONITOR_BIN" monitor delete "${api_key_args[@]}" $keys
  total_deleted=$((total_deleted + $(echo "$keys" | wc -l)))
done

echo "Deleted $total_deleted monitor(s) for host $TARGET_HOST"
