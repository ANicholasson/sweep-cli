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

lint() {
	local wanted installed
	wanted=$(tr -d '[:space:]' < .golangci-lint-version) || return 1
	if ! command -v golangci-lint > /dev/null; then
		printf 'golangci-lint is missing. The gate needs version %s, and the gate section of README.md says how to install it.\n' "$wanted"
		return 1
	fi
	installed=$(golangci-lint version --short) || return 1
	if [ "$installed" != "$wanted" ]; then
		printf 'golangci-lint %s is installed, but the gate needs %s. The gate section of README.md says how to install it.\n' "$installed" "$wanted"
		return 1
	fi
	golangci-lint run ./...
}

step go run ./tools/boundaries
step formatted
step go build ./...
step go vet ./...
step lint
step go test ./...

printf '\nThe gate passes.\n'
