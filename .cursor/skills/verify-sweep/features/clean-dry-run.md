# Dry run writes a Plan

`sweep clean --dry-run` picks every Stale Candidate beneath the Roots you gave, prints each Candidate as picked or not picked with the reason, and writes a Plan. Nothing is removed. With no `--plan`, the Plan is `sweep/plan.json` in the operating system's per-user cache directory, and the dry run prints that full path. `--plan` names another file. A path inside a Project the Scan found is refused, and nothing is written.

## Sub-features

- `default` writes a Plan of the Stale Candidates.
- `pick` adds a Candidate that is not Stale.
- `inside` refuses a Plan path inside a Candidate, including one reached through a Link above the file's parent.
- `in-project` refuses a Plan path inside a Project.
- `twice` runs twice from inside a Stale Project and writes the same Plan outside it.
- `existing` refuses a file that already exists and is not a Plan.
- `usage` rejects `sweep clean` without `--dry-run`.

## How to get to it (user POV)

- Run `sweep clean --dry-run --plan <file> [Root...]`.
- Run `sweep clean --dry-run --pick <path> [Root...]`.
- Run `sweep clean --dry-run` with no `--plan`. The Plan goes to the per-user cache.
- Run `sweep clean --dry-run` from inside a Project.
- Run `sweep clean --dry-run --plan <path>` with a path inside a Project.
- Run `sweep clean --dry-run --plan <file>` from inside a Candidate.
- Run `sweep clean` with no `--dry-run`.

## Driving it with sweep-verify

Preconditions:

- Baseline preconditions from the [index](./README.md) hold.
- `$RUN/scratch` exists. Plan files for the steps that should leave the Fixture unchanged go there.
- The cache directory for a step with no `--plan` is `$RUN/scratch/cache`, which is not the real per-user directory. On Linux, set `XDG_CACHE_HOME` to that directory. The Plan is `$RUN/scratch/cache/sweep/plan.json`. On macOS the directory is `HOME`, and the Plan is under `Library/Caches/sweep/plan.json`. On Windows it is `LocalAppData`.

- **Default Pick.** Run `"$H" run "$RUN" clean/default -- clean --dry-run --plan "$RUN/scratch/plan.json" projects`. Exit status `0`, stderr empty. stdout has `picked: Stale` and, under it, `projects/stale-app/node_modules`. It has `not picked: used recently` and, under it, the other five Candidates from the Answer key. It ends with `1 Candidate in the Plan`, a Size, `Written to $RUN/scratch/plan.json`, `Left out 5 Candidates`, and `To add one, pass --pick and its path.` The Plan file is JSON `version` 1. Its `roots` entry is the `projects` directory. Its `candidates` paths are exactly `projects/stale-app/node_modules` and no Trap. That Candidate's Fingerprint `size` equals `"$H" du "$RUN" "$RUN/fixture/projects/stale-app/node_modules"`. `fixture-changes.txt` is empty.
- **Pick.** Run `"$H" run "$RUN" clean/pick -- clean --dry-run --plan "$RUN/scratch/picked.json" --pick projects/recent-app/node_modules projects`. Exit status `0`, stderr empty. The Plan's paths are `projects/stale-app/node_modules` and `projects/recent-app/node_modules`. stdout has `picked: named with --pick` and, under it, the recent Candidate. `fixture-changes.txt` is empty.
- **Inside a Candidate.** Run `"$H" run "$RUN" clean/inside -C projects/stale-app/node_modules -- clean --dry-run --plan sweep-plan.json "$RUN/fixture/projects"`. Exit status `2`, stdout empty, stderr names that Candidate. `projects/stale-app/node_modules/sweep-plan.json` does not exist. `fixture-changes.txt` is empty.
- **Inside a Project.** Run `"$H" run "$RUN" clean/in-project -- clean --dry-run --plan projects/stale-app/plan.json projects`. Exit status `2`, stdout empty, stderr names `projects/stale-app/plan.json` and the Project `projects/stale-app`. That file does not exist. `fixture-changes.txt` is empty.
- **Twice from a Stale Project.** With the cache directory pointed at `$RUN/scratch/cache`, run `"$H" run "$RUN" clean/twice -C projects/stale-app -- clean --dry-run .` and the same command again as `clean/twice-again`. Both exit status `0`, stderr empty, and both print `1 Candidate in the Plan` and `Written to` the full Plan path under the cache directory. The Plan's bytes after the second run equal its bytes after the first. No new file appears under `projects/stale-app`. Both `fixture-changes.txt` files are empty.
- **Existing file.** Run `"$H" run "$RUN" clean/existing -- clean --dry-run --plan "$RUN/fixture/answer-key.json" projects`. Exit status `2`, stdout empty, stderr says the file is not a Plan. `answer-key.json` is unchanged. `fixture-changes.txt` is empty.
- **Usage.** Run `"$H" run "$RUN" clean/usage -- clean`. Exit status `2`, stdout empty, stderr contains `--dry-run`. `sweep-plan.json` does not exist in the Fixture. `fixture-changes.txt` is empty.

## Gotchas

- With no `--plan`, the Plan is `sweep/plan.json` in the per-user cache directory, not `sweep-plan.json` in the working directory. Point that directory at `$RUN/scratch/cache` or the dry run writes outside the run.
- The inside step passes `--plan sweep-plan.json` because the working directory is the Candidate. Its Root is the absolute `projects` directory. A relative `projects` would be resolved from inside the Candidate and would not be a Root.
- The existing step uses `answer-key.json`, which sits outside every Project. A path inside a Project is refused first, so it never reaches the check that the file is already a Plan.
- A Candidate is Stale when the newest modification of its Project's own files, outside the Candidate, is more than 30 days ago. In this Fixture that is only `projects/stale-app`.
