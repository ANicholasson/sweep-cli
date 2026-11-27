# sweep verification map

This directory is the maintained source for verifying what a user of `sweep` sees. Read this index, then use the matching feature file as the recipe. Use `GLOSSARY.md` terms throughout.

## Baseline preconditions

- `H` is the helper, `.cursor/skills/verify-sweep/scripts/sweep-verify`, and `RUN` is the directory `"$H" launch` printed, from the checkout under test.
- `"$H" doctor "$RUN"` ends with `doctor: worth driving`.
- The Fixture at `$RUN/fixture` is as the fixture tool built it. Its Answer key (`answer-key.json`, version 1) names the Root `projects` and eight Candidates. Six are Dependencies: `projects/web-app/node_modules`, `projects/stale-app/node_modules`, `projects/recent-app/node_modules`, `projects/shifted-app/node_modules`, `projects/tracked-app/ignored-app/node_modules` and `projects/tracked-app/untracked-app/node_modules`. One is Build output: `projects/rust-app/target`, beside `Cargo.toml`. One is the npm cache, Kind Cache, at `home/.npm/_cacache` on Linux and macOS and at `home/AppData/Local/npm-cache/_cacache` on Windows. The helper points the home directory at `$RUN/scratch/home`, a copy of the Fixture's home, unless the caller already pointed `HOME` or `USERPROFILE` inside the run. A Scan adds that cache unless the step passes `--no-shared-caches` or the file sets `shared_caches` off. The cache a Scan reports is `$RUN/scratch/home/.npm/_cacache` on Linux and macOS, and `$RUN/scratch/home/AppData/Local/npm-cache/_cacache` on Windows. `stale-app` was last used 60 days before the Fixture was built, and `recent-app` one day before. The cache's files are 60 days old. `shifted-app` was last used when the Fixture was built. It names five Traps: `projects/tracked-app/node_modules` (git tracks its files), `outside/stray-app/node_modules` (outside the Root), `projects/linked-app/through/node_modules` (reached through a Link), `projects/plain/target` (named `target`, with no `Cargo.toml` beside it, so it is not a Candidate) and `projects/shifted-app/node_modules` (a Candidate as well, changed between the Plan and the Clean, so the Fingerprint rule keeps it).
- Never drive a run that this verification did not launch.

## Driving conventions

- Every step is `"$H" run "$RUN" <feature-id>/<step> [-C <dir>] -- <sweep args>`. The working directory defaults to `$RUN/fixture`, so Roots below are relative to it.
- Treat each command as literal: keep flags, order and `--` exactly as written.
- Take every expected value from the Answer key, `"$H" du`, `git ls-files` or the listing, never from another sweep output.
- State that a recipe needs beyond the Fixture is built in `$RUN/scratch/`, never in the Fixture.
- Inspect the Trap's repository only with `git --no-optional-locks`. A plain `git status` there rewrites `.git/index`, which changes the Fixture and fails the doctor.

## Proof and skip reporting

- Each step's evidence is `$RUN/evidence/<feature-id>/<step>/`: `command.txt`, `stdout`, `stderr`, `status`, `fixture-changes.txt`.
- Every Scan step needs an empty `fixture-changes.txt`. A non-empty one fails the step, whatever the output says.
- Report an unreachable step with its command and the unmet precondition. Do not count it as verified through another entry point.
- Run `"$H" doctor "$RUN"` once more after the last step, then `"$H" cleanup "$RUN"`.

## Feature entry contract

Each feature file starts with an H1 title and one paragraph describing the user-visible behaviour, then exactly four H2 sections in this order:

1. `Sub-features`: short IDs, one line each.
2. `How to get to it (user POV)`: every user entry point.
3. `Driving it with sweep-verify`: starts with `Preconditions:`, then labelled bullets pairing each action with an exact command and the observable result.
4. `Gotchas`: traps that can waste or invalidate a run.

Keep implementation details out of the map. Name only user paths, commands, required state and observable proof.

## Features

- [Scan as a table](./scan-table.md) covers the Roots header, Candidates grouped by Project, one row per Candidate with Size, Kind, path, Last used, the Stale mark and Reason, and `No Candidates.`.
- [Docker data](./scan-docker.md) covers the one reported-only line, the `dockerData` field, and a Scan that still exits zero when Docker is missing or not running.
- [Scan as JSON](./scan-json.md) covers the version 1 document, exact Sizes against `du`, the Answer key's Candidates, each Candidate's Project, Last used and Stale mark, and empty lists.
- [Choosing Roots](./scan-roots.md) covers the default Root, several and overlapping Roots, flags after Roots, `--`, the Root rule Trap, and bad Roots that stop the Scan.
- [A Scan that cannot read everything](./scan-unreadable.md) covers exit status 1 and the unreadable list in both formats.
- [A Scan of a tree built to mislead it](./scan-hostile.md) covers Links beneath a Root, a Root that is a Link, an unreadable directory inside a Project, a repository nested in a Candidate, and a `docker` that never answers.
- [Version and usage](./version-and-usage.md) covers `--version`, plain `sweep` with no interactive terminal, the usage message, and unknown commands and flags.
- [Interactive screen](./screen.md) covers the list plain `sweep` opens on a terminal: the default Pick, Shared caches, Docker data, the Reason, the running total, the keys, and quitting without a change.
- [Dry run writes a Plan](./clean-dry-run.md) covers the default Pick, the Plan file in the per-user cache, `--pick`, a Plan path inside a Candidate, a Plan path inside a Project, and the Configuration a dry run reads: Roots, the Stale threshold, the Ignore list and the file it names.
- [Pick by Kind, Ecosystem and age](./clean-pick.md) covers narrowing a dry run by Kind and Ecosystem, changing the age that counts as Stale for one run, picking every Candidate whatever its age, and leaving Shared caches and Docker data unpicked.
- [Clean carries out a Plan](./clean.md) covers moving the Plan's Candidates to the trash on Linux, macOS and Windows, a Skip under each Safety rule, a Candidate the trash cannot take, and a Plan that is refused.
- [Configuration](./scan-configuration.md) covers the per-user file: default Roots, the Stale threshold, the Ignore list, the shared-caches setting, and a missing or rejected file.
- [Shared caches](./scan-shared-caches.md) covers the npm cache: the Root sweep adds, the Shared caches group, `--no-shared-caches`, and a Pick that leaves it out unless `--pick-caches` names its Ecosystem.

Not built yet: Delete. A Clean is driven only against `$RUN/fixture`, and its proof is the listing showing the Candidates the Plan moved are in the Fixture's trash and every Trap is still present.
