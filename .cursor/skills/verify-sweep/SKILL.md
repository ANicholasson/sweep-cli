---
name: verify-sweep
description: Verify sweep, the Go command-line tool in this repo, by building the real binary and driving it against a fresh Fixture, capturing argv, stdout, stderr, exit status and a before/after listing of the Fixture as evidence. Use when proving a change to what a user of `sweep` sees (Scan table or JSON, Roots, exit statuses, usage errors), when the pull request body needs a "what was verified and how" section, or when asked to verify, drive or smoke-test sweep.
---

# Verify sweep

sweep is a short-lived CLI. There is no server to keep alive. With no arguments it opens an interactive screen when standard input and standard output are a terminal, and otherwise says so and points at `sweep scan`. Every other command reads no terminal. Driving a command means running the built binary with captured streams. The helper's `run` captures pipes, so plain `sweep` takes the no-terminal path. The screen itself is driven in a terminal, with tmux. A verification run is: **launch** (build both binaries and one Fixture into a private run directory), **doctor**, **drive** recipes from [`features/`](features/README.md), **cleanup** (evidence survives).

Read `GLOSSARY.md` first and use its words (Root, Candidate, Size, Kind, Reason, Fixture, Trap, Answer key) in every report.

## Safety, before anything else

- Point sweep at the Fixture of your own run, or at the run's `scratch/`, and nowhere else. A Clean (not built yet) runs against the Fixture only, never a real directory.
- The Fixture is made only by the fixture tool, which `launch` runs. Never add, edit or delete a path in it by hand; a recipe that needs more state builds it in `$RUN/scratch/`, and a Candidate that should be covered for good goes into `tools/fixture/fixture.go` and the Answer key in a pull request.
- Read the `Roots` of every Scan output. If any Root lies outside `$RUN`, stop and report it. The helper points `HOME` and `USERPROFILE` at `$RUN/scratch/home`, a copy of the Fixture's home, unless the caller already pointed that variable at a path inside the run. A Scan then adds Shared cache Roots inside the run, a Plan is not written into the Fixture, and a macOS Clean can still point `HOME` at its trash directory.
- Evidence holds absolute paths under the run directory. Before pasting it into a pull request, replace the run directory with `$RUN` so no machine path lands in the public repo.

## Launch

Run from the root of the checkout under test (a worktree works; the helper builds whatever checkout the current directory is in):

```bash
H=.cursor/skills/verify-sweep/scripts/sweep-verify    # or the absolute path to this skill's scripts/
RUN=$("$H" launch | sed -n 's/^RUN=//p'); echo "$RUN"
```

Ready means the last two lines were `ready: sweep and the Fixture are built` and `RUN=<dir>`. `launch` needs `go` and `git`; it builds `bin/sweep` and `bin/fixture` with `go build` (so the binary carries the checkout's git revision), runs the fixture tool into `$RUN/fixture`, and records the built Fixture's listing, the Answer key and the build's revision in `$RUN/evidence/`. Runs live under `${SWEEP_VERIFY_DIR:-${TMPDIR:-/tmp}/sweep-verify}/<UTC time>-<pid>/`, so any number of runs sit side by side without sharing state.

The helper is bash. From zsh or fish, call it as shown; don't build its argument lists in an unquoted variable (zsh doesn't split one).

## Doctor

```bash
"$H" doctor "$RUN"
```

Read-only. Run it first, again whenever a result surprises you, and last before citing a run. Every line must say `ok`, and the last line `doctor: worth driving`:

- `du` and `git` are on the PATH (every Size is checked against `du`, the Trap against `git`).
- `bin/sweep --version` prints `sweep <version>`.
- The checkout is at the same commit with the same uncommitted changes as at launch. The contents of untracked files count, including files inside an untracked directory and the target text of an untracked symlink. A nested repository's `.git` directory and the paths it ignores do not count, and an uninitialised submodule counts as its path. Editing a counted file after launch leaves the binary stale: clean up and launch again.
- The Fixture's listing (mode, size, time, checksum of every path) equals the one the fixture tool built. A FAIL means something wrote into the Fixture; find which step from the `fixture-changes.txt` files and treat it as a finding, not noise.
- The Trap path `projects/tracked-app/node_modules` is clean, and `git ls-files` lists files under it. The same repository holds an ignored Candidate and an untracked Candidate, so a status of the whole repository is not empty.

## Drive

Each step runs the real binary once:

```bash
"$H" run "$RUN" <feature-id>/<step> [-C <dir>] -- <sweep arguments>
```

`-C` sets the working directory: relative to `$RUN/fixture`, or absolute. Without it the step runs in `$RUN/fixture`, so Roots can be written relative to it (`projects`, `projects/web-app`). The helper prints the exit status, stdout, stderr, and whether the Fixture changed.

Expected values come from sources independent of sweep, never from sweep's own output:

| What | Independent source |
| --- | --- |
| A Size | `"$H" du "$RUN" <path>`: bytes from `du -s` in 512-byte blocks, as the end-to-end test measures it |
| Which Candidates must be listed, with their Kind | `$RUN/evidence/answer-key.json` |
| What git tracks | `git -C "$RUN/fixture/projects/tracked-app" --no-optional-locks ls-files` (a plain `git status` there rewrites `.git/index`, which changes the Fixture and fails the doctor) |
| What a run changed | the step's `fixture-changes.txt`, written by the helper from listings before and after |

`jq` makes JSON assertions short when it is installed; Git Bash on Windows lacks it, so read the JSON evidence directly there.

Pick recipes from [`features/README.md`](features/README.md). When a change touches a feature, drive every entry point that feature's file lists, not one convenient path.

## Evidence

Everything lands in `$RUN/evidence/`:

- `build.txt`: the commit, the uncommitted changes, `sweep --version`, and the `vcs.*` stamps from `go version -m`.
- `answer-key.json` and `fixture-listing-built.txt`: the Fixture as built.
- `<feature-id>/<step>/`: `command.txt` (working directory and exact argv), `stdout`, `stderr`, `status`, and `fixture-changes.txt`, which is empty when the Fixture did not change.
- `du.txt`: every `du` measurement, one `<bytes> <path>` per line.

Proof standards:

- Drive the user's path: the built binary with real arguments. Not `go run`, not `run()` from a test, not a fake from `internal/sweeptest`.
- Capture the action and the result: the evidence directory pairs each argv with its streams and status.
- Check side effects alongside output. A Scan changes nothing, so every Scan step needs an empty `fixture-changes.txt`. When Clean lands, its steps must show exactly the Candidates gone and every Trap still present, from the listing, not from sweep's report.
- Report a step that could not be reached (for example, a locked directory when running as root or on Windows) as unreachable with the reason. Never report it as passed through another path.

For the pull request body, state the commit from `build.txt`, each step's argv and status, and each Size beside its `du` value, with the run directory replaced by `$RUN`.

## Cleanup

```bash
"$H" cleanup "$RUN"
```

Removes `$RUN/bin`, `$RUN/fixture` and `$RUN/scratch` (making locked scratch directories writable first) and keeps `$RUN/evidence`. It refuses any directory `launch` did not make. Run it after every attempt, failed ones included. sweep starts no background process, so there is nothing to kill. Delete an old run's evidence with `rm -rf "$RUN"` once nothing cites it.

## Helpers

`scripts/sweep-verify` is the only helper. Run it with no arguments for its usage:

```text
sweep-verify launch                       build binaries and a Fixture; prints RUN=<dir>
sweep-verify doctor  <run>                is this run worth driving?
sweep-verify run     <run> <step> [-C <dir>] -- <sweep args...>
sweep-verify du      <run> <path>         bytes du -s gives, as the tests measure it
sweep-verify cleanup <run>                remove binaries, Fixture and scratch; keep evidence
```

It runs on Linux, macOS and Git Bash on Windows, using only bash, `find`, `ls`, `readlink`, `cksum`, `du`, `git` and `go`.

## How this relates to the gate

`scripts/verify.sh` is still the gate, and `cmd/sweep/scan_test.go` already proves the Scan against the Fixture on three operating systems. This skill is for what the gate doesn't give a reviewer: a hand-driven, readable record of the binary doing the new thing, for the pull request body, and a way to explore behaviour no test covers yet. When a run finds something a test should hold, add the test.
