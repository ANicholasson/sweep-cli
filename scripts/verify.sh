#!/usr/bin/env bash

set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

step() {
	printf '\n==> %s\n' "$*"
	"$@" || {
		printf '\nThe gate fails at: %s\n' "$*" >&2
		exit 1
	}
}

# gofmt -l exits 0 even when it lists files, so the list itself decides.
formatted() {
	local files
	files=$(gofmt -l .) || return 1
	if [ -n "$files" ]; then
		printf '%s\n\nRun gofmt -w on these files.\n' "$files"
		return 1
	fi
}

step go run ./tools/boundaries
step formatted
step go build ./...
step go vet ./...
step go test ./...

printf '\nThe gate passes.\n'
