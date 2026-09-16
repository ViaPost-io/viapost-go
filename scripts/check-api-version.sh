#!/usr/bin/env bash
set -euo pipefail

base_tag="${API_DIFF_BASE_TAG:-$(git describe --tags --abbrev=0 HEAD^ 2>/dev/null || true)}"
if [[ -z "$base_tag" ]]; then
  echo "No previous release tag found; skipping API version comparison."
  exit 0
fi

current_version="$(sed -n 's/^[[:space:]]*Version = "\([^"]*\)"/\1/p' client.go)"
base_version="${base_tag#v}"

temporary="$(mktemp -d "${TMPDIR:-/tmp}/viapost-go-apidiff.XXXXXX")"
old_worktree="$temporary/old"
cleanup() {
  git worktree remove --force "$old_worktree" >/dev/null 2>&1 || true
  case "$temporary" in
    "${TMPDIR:-/tmp}"/viapost-go-apidiff.*) rm -rf -- "$temporary" ;;
  esac
}
trap cleanup EXIT

GOTOOLCHAIN=auto GOBIN="$temporary/bin" go install \
  golang.org/x/exp/cmd/apidiff@v0.0.0-20260908205506-85c1c2202aba
apidiff="$temporary/bin/apidiff"
git worktree add --detach "$old_worktree" "$base_tag" >/dev/null
(cd "$old_worktree" && "$apidiff" -m -w "$temporary/old.api" .)
"$apidiff" -m -w "$temporary/new.api" .
"$apidiff" -m "$temporary/old.api" "$temporary/new.api" > "$temporary/report.txt"

"$(dirname "$0")/check-api-version-policy.sh" \
  "$base_version" "$current_version" "$temporary/report.txt"
