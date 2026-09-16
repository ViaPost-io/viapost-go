#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
temporary="$(mktemp -d "${TMPDIR:-/tmp}/viapost-go-version-policy.XXXXXX")"
trap 'rm -rf -- "$temporary"' EXIT

write_report() {
  local body="$1"
  printf '%s\n' "$body" > "$temporary/report.txt"
}

expect_pass() {
  if ! "$root/scripts/check-api-version-policy.sh" "$1" "$2" "$temporary/report.txt" >/dev/null; then
    echo "expected policy to accept $1 -> $2" >&2
    exit 1
  fi
}

expect_fail() {
  if "$root/scripts/check-api-version-policy.sh" "$1" "$2" "$temporary/report.txt" >/dev/null 2>&1; then
    echo "expected policy to reject $1 -> $2" >&2
    exit 1
  fi
}

write_report ""
expect_pass 0.2.0 0.2.0

write_report $'Incompatible changes:\n- Version: value changed from "0.2.0" to "0.2.1"'
expect_pass 0.2.0 0.2.1

write_report $'Compatible changes:\n- Client.NewMethod: added'
expect_fail 0.2.0 0.2.1
expect_pass 0.2.0 0.3.0

write_report $'Incompatible changes:\n- Client.Method: changed from func(string) to func(int)'
expect_fail 0.2.0 0.2.1
expect_pass 0.2.0 0.3.0

echo "API version policy tests passed."
