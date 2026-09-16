#!/usr/bin/env bash
set -euo pipefail

if (( $# != 3 )); then
  echo "usage: $0 BASE_VERSION CURRENT_VERSION APIDIFF_REPORT" >&2
  exit 2
fi

base_version="$1"
current_version="$2"
report="$3"
version_pattern='^[0-9]+\.[0-9]+\.[0-9]+$'

if [[ ! "$current_version" =~ $version_pattern || ! "$base_version" =~ $version_pattern ]]; then
  echo "SDK versions must use numeric MAJOR.MINOR.PATCH values." >&2
  exit 1
fi

IFS=. read -r base_major base_minor base_patch <<<"$base_version"
IFS=. read -r current_major current_minor current_patch <<<"$current_version"

if (( current_major < base_major )) ||
   (( current_major == base_major && current_minor < base_minor )) ||
   (( current_major == base_major && current_minor == base_minor && current_patch < base_patch )); then
  echo "SDK version v$current_version must not be older than v$base_version." >&2
  exit 1
fi

version_change="- Version: value changed from \"$base_version\" to \"$current_version\""
incompatible_changes="$(awk -v ignored="$version_change" '
  /^Incompatible changes:/ { section = "incompatible"; next }
  /^Compatible changes:/ { section = "compatible"; next }
  section == "incompatible" && /^- / && $0 != ignored { print }
' "$report")"
compatible_changes="$(awk '
  /^Incompatible changes:/ { section = "incompatible"; next }
  /^Compatible changes:/ { section = "compatible"; next }
  section == "compatible" && /^- / { print }
' "$report")"

if [[ -n "$incompatible_changes" ]]; then
  printf 'Incompatible changes:\n%s\n' "$incompatible_changes"
  if (( base_major == 0 )); then
    if ! (( current_major > base_major || current_minor > base_minor )); then
      echo "Incompatible pre-1.0 API changes require a minor version bump." >&2
      exit 1
    fi
  elif ! (( current_major > base_major )); then
    echo "Incompatible API changes require a major version bump." >&2
    exit 1
  fi
elif [[ -n "$compatible_changes" ]]; then
  echo "Compatible public API changes detected relative to v$base_version."
  if ! (( current_major > base_major || current_minor > base_minor )); then
    echo "Compatible public API additions require a minor version bump." >&2
    exit 1
  fi
fi
