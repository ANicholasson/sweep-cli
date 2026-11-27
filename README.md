# sweep

Find and safely reclaim disk space used by developer junk: dependency
folders, build output, caches and stale git worktrees. Works on
Windows, Linux and macOS.

Status: early development.

## Usage

    sweep scan [--json] [Root...]

A Scan looks beneath each Root. Roots come from the command line first,
then the configuration file, then the current directory. It lists every
Candidate a Rule recognises, such as a `node_modules` beside a
`package.json`, with its Size, Kind, path and Reason. A Root that is a
Link is resolved once, and a Link beneath a Root is not followed. It
groups Candidates by Project. Last used is the newest modification among
that Project's own files, outside its Candidates, and never an access
time. A Candidate last used more than 30 days ago is marked Stale, unless
the configuration file sets another threshold. A Candidate with no Project
takes the newest modification inside it. The Scan leaves out a path that
holds a file git tracks, and a path when git is missing or cannot answer.
A path outside a work tree is still listed, and so is a path inside one
that holds nothing git tracks. It leaves out a path on the Ignore list,
and everything beneath it, from the report. Last used still counts the
files there, so an Ignore does not by itself make a Candidate Stale. It prints the configuration file it read, or
`none`, and the Roots it used, and it changes nothing. `--json` prints the
same as one JSON document with a `version` field and a `configuration`
field, and each Candidate carries its Project's path, `last_used` and
`stale`. A Size is in bytes in the JSON, and it is what `du -s` reports
for the path, with each hard-linked file counted once.

The configuration file is `config.toml` in the `sweep` directory of the
operating system's per-user configuration directory: `$XDG_CONFIG_HOME` or
`~/.config` on Linux, `~/Library/Application Support` on macOS, and
`%AppData%` on Windows. A missing file means the defaults. The format is
TOML and the only keys are these:

    roots = ["/path/to/projects"]
    stale_days = 30
    ignore = ["/path/to/keep"]

`roots` and `ignore` are absolute paths. A Link in an Ignore path is
followed, including one earlier than the last component. `stale_days` is a whole number of
days and replaces the 30-day threshold, and zero means a Candidate is
Stale when its Last used is before now. A malformed file is an error that
names the file. An unknown key or a bad value is an error that names the
key, and nothing is scanned. No key can switch off a Safety rule or make
Delete the default.

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

    sweep clean --dry-run [--plan file] [--pick path] [--kind Kind] [--ecosystem Name] [--stale-days n] [--all] [Root...]

`sweep clean --dry-run` reads the same configuration file a Scan reads.
Roots come from the command line first, then the file, then the current
directory. The file's Stale threshold decides what is Stale, unless
`--stale-days` is set. A path on the Ignore list, and everything beneath
it, is left out, so a Plan never holds one. A Candidate that holds an
Ignore path is left out for the same reason. `--pick` of such a path is
refused, and the message names the path. It prints the configuration file
it read, or `none`, and the Roots it used. It picks every Stale Candidate
and writes a Plan. `--kind` keeps only the named Kinds, and `--ecosystem`
keeps only the named Ecosystems. Each may be repeated. A Kind is
Dependencies, Build output, Cache, Models or Worktree. An Ecosystem is
one sweep knows, such as Node or Cargo. An unknown name is a usage error.
`--stale-days` is a whole number of days and replaces the threshold for
this run, including one from the configuration file. Zero means a
Candidate is Stale when its Last used is before now. `--all` picks every
Candidate whatever its age. None of `--kind`, `--ecosystem`,
`--stale-days` or `--all` picks a Shared cache or Docker data. A Shared
cache is picked only when `--pick` names it. `--pick` adds that Candidate
even when a narrowing flag would leave it out. Each row is `picked` or
`not picked` with the reason that decided it: `Stale`, `used recently`,
`named with --pick`, `--all`, `older than --stale-days n`,
`newer than --stale-days n`, `not named with --kind`,
`not named with --ecosystem` or `Shared cache`, and with its Size, Kind
and path. The Plan is a JSON file with a `version` field. It records the
Roots and each picked Candidate with its Fingerprint: the file identity
of the Candidate, the number of directory entries inside it, its Size and
its newest modification. A Link counts as one entry and is not followed.
The default file is `sweep/plan.json` in the operating system's per-user
cache directory: `$XDG_CACHE_HOME` or `~/.cache` on Linux,
`~/Library/Caches` on macOS, and `%LocalAppData%` on Windows. The dry run
prints that full path. `--plan` names another file. The Plan is never
written inside a Candidate, an Ignore path, or a Project the Scan found.
That refusal happens before anything is written. A file that already
exists is replaced only when it is an earlier Plan. Any other existing
file is left as it was, and the dry run writes nothing. Nothing is
removed. The run ends with how many Candidates and how much Size are in
the Plan, what was left out, and how to add one with `--pick`.

The exit status is 0 when the Plan is written and the dry run read
everything, 1 when something could not be read or the Plan or the output
could not be written, and 2 when the command line is wrong, a Root is not
a directory, the configuration file is rejected, a `--pick` path is not a
Candidate (including one on the Ignore list), a Kind or an Ecosystem is
not one sweep knows, `--stale-days` is not a whole number of days, or the
Plan would be written inside a Candidate, an Ignore path, or a Project.
A usage error writes no Plan.

    sweep clean --plan file

`sweep clean --plan file` carries out exactly the Plan in that file.
Immediately before it moves a Candidate it re-checks the Git rule, the
Root rule and the Fingerprint rule. A Candidate that fails one is a Skip,
named with that rule, and nothing inside it is touched. The Root rule
reaches the Candidate from a Root one component at a time and does not
follow a Link, so a directory above the Candidate that was swapped for a
Link is a Skip. A Candidate that holds a directory sweep cannot read is a
Skip under the Git rule, because a repository with tracked files could be
inside it. Every other Candidate is moved to the trash. On Linux that is
`$XDG_DATA_HOME/Trash`, or `~/.local/share/Trash` when `XDG_DATA_HOME` is
unset. On macOS a Candidate on the same volume as the home directory goes
to `~/.Trash`, and one on another volume goes to that volume's
`.Trashes/<user id>`. A Candidate the trash cannot take is reported as a
failure and left in place. sweep does not delete it, and the rest of the
Plan continues.
The summary lists what was moved to the trash, what was skipped and what
failed. Space that was moved is not called reclaimed.

The exit status is 0 when every Candidate in the Plan was moved, 1 when a
Candidate was skipped or could not be moved, and 2 when the Plan is missing
from the command, is malformed or has an unknown version, a Root or a
Candidate is not a clean absolute path, or this operating system has no
trash yet. Windows refuses before anything is moved.
A refused Clean removes nothing.

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
