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

step go run ./tools/boundaries
step go build ./...
step go vet ./...
step go test ./...

printf '\nThe gate passes.\n'
