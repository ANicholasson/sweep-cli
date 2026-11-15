# sweep

Find and safely reclaim disk space used by developer junk: dependency
folders, build output, caches and stale git worktrees. Works on
Windows, Linux and macOS.

Status: early development.

## Safety rules

sweep deletes files, so these rules are non-negotiable:

1. Never delete git-tracked files, or a worktree with uncommitted changes.
2. Never follow a link (a symlink, or a junction on Windows) out of a
   root, and never touch a path outside the roots.
3. If anything changed between the plan and the delete, skip that path.

Deleted items go to the OS trash by default. Permanent delete is opt-in.

## The gate

One script decides whether a change is acceptable:

    scripts/verify.sh

It needs Go, bash and golangci-lint at the version in
`.golangci-lint-version`. To install golangci-lint, follow
[its local install guide](https://golangci-lint.run/docs/welcome/install/local/)
and install the binary release of that version, not `go install`. On
Windows, run the gate from Git Bash.
It stops at the first step that fails and exits non-zero:

1. `go run ./tools/boundaries` holds the two boundaries of
   [the package layout](docs/adr/0001-hexagonal-packages-with-feature-folders.md):
   `internal/domain` does no IO, and `internal/system/trash` is the only
   package that trashes or deletes anything. It reads every Go file, so a
   file for another operating system is checked on yours.
2. `gofmt -l .` lists no file, so every Go file is formatted.
3. `go build ./...`
4. `go vet ./...`
5. `golangci-lint run ./...` runs errcheck, govet, ineffassign, staticcheck
   and unused, as configured in `.golangci.yml`. The step fails first if
   golangci-lint is missing or is not the version in
   `.golangci-lint-version`.
6. `go test ./...`

A change is done when the gate passes.
