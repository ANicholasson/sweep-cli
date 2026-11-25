# sweep

Find and safely reclaim disk space used by developer junk: dependency
folders, build output, caches and stale git worktrees. Works on
Windows, Linux and macOS.

Status: early development.

## Usage

    sweep scan [--json] [Root...]

A Scan looks beneath each Root, or beneath the current directory when you
give none, and lists every Candidate a Rule recognises, such as a
`node_modules` beside a `package.json`, with its Size, Kind, path and
Reason. A Root that is a Link is resolved once, and a Link beneath a Root
is not followed. It groups Candidates by Project. Last used is the newest
modification among that Project's own files, outside its Candidates, and
never an access time. A Candidate last used more than 30 days ago is
marked Stale. A Candidate with no Project takes the newest modification
inside it. The Scan leaves out a path that holds a file git tracks, and a
path when git is missing or cannot answer. A path outside a work tree is
still listed, and so is a path inside one that holds nothing git tracks.
It prints the Roots it used, and it changes nothing. `--json` prints the
same as one JSON document with a `version` field, and each Candidate
carries its Project's path, `last_used` and `stale`. A Size is in bytes in
the JSON, and it is what `du -s` reports for the path, with each
hard-linked file counted once.

A Scan also prints one line of Docker data: the space Docker images and
build cache hold, marked `reported only`. It is not a Candidate, it is
never a row of the table, and no flag makes it part of a Pick. `--json`
carries it in `dockerData` (`size` in bytes, and `mark`) and never in
`candidates`. When Docker is missing or not running, the line says so and
the Scan still exits zero. The size is the Images total plus the Build
Cache total from `docker system df`.

The exit status is 0 when the Scan read everything, 1 when something
beneath a Root could not be read or the output could not be written, and 2
when the command line is wrong or a Root is not a directory, in which case
nothing is scanned.

    sweep clean --dry-run [--plan file] [--pick path] [Root...]

`sweep clean --dry-run` picks every Stale Candidate beneath the Roots you
gave and writes a Plan. It prints each Candidate as `picked: Stale` or
`not picked: used recently`, with its Size, Kind and path. The Plan is a
JSON file with a `version` field. It records the Roots and each picked
Candidate with its Fingerprint: the file identity of the Candidate, the
number of directory entries inside it, its Size and its newest
modification. A Link counts as one entry and is not followed. The default
file is `sweep-plan.json` in the current directory, and `--plan` names
another. The Plan is never written inside a Candidate. `--pick` adds one
Candidate the default Pick left out. Nothing is removed. The run ends with
how many Candidates and how much Size are in the Plan, what was left out,
and how to add it with `--pick`.

The exit status is 0 when the Plan is written and the dry run read
everything, 1 when something could not be read or the Plan or the output
could not be written, and 2 when the command line is wrong, a Root is not
a directory, a `--pick` path is not a Candidate, or the Plan would be
written inside a Candidate. A usage error writes no Plan.

## Safety rules

sweep deletes files, so these rules are non-negotiable:

1. Never delete git-tracked files, or a worktree with uncommitted changes.
2. Never follow a link (a symlink, or a junction on Windows) out of a
   root, and never touch a path outside the roots.
3. If anything changed between the plan and the delete, skip that path.

A Scan does not close one case. A directory above the one being listed can be swapped for a Link and then followed. A Clean must.

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

1. `go run ./tools/boundaries` holds the three boundaries of
   [the package layout](docs/adr/0001-hexagonal-packages-with-feature-folders.md):
   `internal/domain` does no IO, `internal/system/trash` is the only
   package that trashes or deletes anything, and `cmd/sweep` is the only
   package that imports an adapter from `internal/system`. It reads every
   Go file, so a file for another operating system is checked on yours.
2. `gofmt -l .` lists no file, so every Go file is formatted.
3. `go build ./...`
4. `go vet ./...`
5. `golangci-lint run ./...` runs errcheck, govet, ineffassign, staticcheck
   and unused, as configured in `.golangci.yml`. The step fails first if
   golangci-lint is missing or is not the version in
   `.golangci-lint-version`.
6. `go test -count=1 ./...` runs every test, none from the cache, because
   the end-to-end test builds the fixture tool, which the cache cannot see.
   That test builds the Fixture and the `sweep` binary, runs one against
   the other and compares each Size with `du`, so `git` and `du` must be on
   the PATH. Git Bash brings both on Windows.

A change is done when the gate passes.
