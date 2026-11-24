# sweep verification map

This directory is the maintained source for verifying what a user of `sweep` sees. Read this index, then use the matching feature file as the recipe. Use `GLOSSARY.md` terms throughout.

## Baseline preconditions

- `H` is the helper, `.cursor/skills/verify-sweep/scripts/sweep-verify`, and `RUN` is the directory `"$H" launch` printed, from the checkout under test.
- `"$H" doctor "$RUN"` ends with `doctor: worth driving`.
- The Fixture at `$RUN/fixture` is as the fixture tool built it. Its Answer key (`answer-key.json`, version 1) names the Root `projects`, three Candidates, all Dependencies: `projects/web-app/node_modules`, `projects/tracked-app/ignored-app/node_modules` and `projects/tracked-app/untracked-app/node_modules`. It names two Traps: `projects/tracked-app/node_modules` (git tracks its files) and `outside/stray-app/node_modules` (outside the Root).
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

- [Scan as a table](./scan-table.md) covers the Roots header, one row per Candidate with Size, Kind, path and Reason, and `No Candidates.`.
- [Docker data](./scan-docker.md) covers the one reported-only line, the `dockerData` field, and a Scan that still exits zero when Docker is missing or not running.
- [Scan as JSON](./scan-json.md) covers the version 1 document, exact Sizes against `du`, the Answer key's Candidates, and empty lists.
- [Choosing Roots](./scan-roots.md) covers the default Root, several and overlapping Roots, flags after Roots, `--`, the Root rule Trap, and bad Roots that stop the Scan.
- [A Scan that cannot read everything](./scan-unreadable.md) covers exit status 1 and the unreadable list in both formats.
- [Version and usage](./version-and-usage.md) covers `--version`, the usage message, and unknown commands and flags.

Not built yet: Clean, Plan, Pick, Trash and Delete. When one lands, add its file here. A Clean is driven only against `$RUN/fixture`, and its proof is the listing showing the Answer key's Candidates gone and every Trap present.
