#!/usr/bin/env sh
set -eu

before_directory="$(mktemp -d "${TMPDIR:-/tmp}/viapost-go-api-before.XXXXXX")"
after_directory="$(mktemp -d "${TMPDIR:-/tmp}/viapost-go-api-after.XXXXXX")"
trap 'rm -rf "$before_directory" "$after_directory"' EXIT HUP INT TERM

cp -R api/. "$before_directory/"
go generate ./internal/generate
cp -R api/. "$after_directory/"

if ! diff -ru "$before_directory" "$after_directory"; then
	echo "generated client drift detected; run 'make generate' and commit api/" >&2
	exit 1
fi
