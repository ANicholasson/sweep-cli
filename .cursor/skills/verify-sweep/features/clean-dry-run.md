# Dry run writes a Plan

`sweep clean --dry-run` picks every Stale Candidate beneath the Roots you gave, prints each Candidate as picked or not picked with the reason, and writes a Plan. Nothing is removed.

## Sub-features

- `default` writes a Plan of the Stale Candidates.
- `pick` adds a Candidate that is not Stale.
- `inside` refuses a Plan path inside a Candidate, including one reached through a Link above the file's parent.
- `existing` refuses a file that already exists and is not a Plan.
- `usage` rejects `sweep clean` without `--dry-run`.

## How to get to it (user POV)

- Run `sweep clean --dry-run --plan <file> [Root...]`.
- Run `sweep clean --dry-run --pick <path> [Root...]`.
- Run `sweep clean --dry-run` from inside a Candidate.
- Run `sweep clean` with no `--dry-run`.

## Driving it with sweep-verify

Preconditions:

- Baseline preconditions from the [index](./README.md) hold.
- `$RUN/scratch` exists. Plan files for the steps that should leave the Fixture unchanged go there.

- **Default Pick.** Run `"$H" run "$RUN" clean/default -- clean --dry-run --plan "$RUN/scratch/plan.json" projects`. Exit status `0`, stderr empty. stdout has `picked: Stale` and, under it, `projects/stale-app/node_modules`. It has `not picked: used recently` and, under it, the other four Candidates from the Answer key. It ends with `1 Candidate in the Plan`, a Size, `Written to $RUN/scratch/plan.json`, `Left out 4 Candidates`, and `To add one, pass --pick and its path.` The Plan file is JSON `version` 1. Its `roots` entry is the `projects` directory. Its `candidates` paths are exactly `projects/stale-app/node_modules` and no Trap. That Candidate's Fingerprint `size` equals `"$H" du "$RUN" "$RUN/fixture/projects/stale-app/node_modules"`. `fixture-changes.txt` is empty.
- **Pick.** Run `"$H" run "$RUN" clean/pick -- clean --dry-run --plan "$RUN/scratch/picked.json" --pick projects/recent-app/node_modules projects`. Exit status `0`, stderr empty. The Plan's paths are `projects/stale-app/node_modules` and `projects/recent-app/node_modules`. stdout has `picked: named with --pick` and, under it, the recent Candidate. `fixture-changes.txt` is empty.
- **Inside a Candidate.** Run `"$H" run "$RUN" clean/inside -C projects/stale-app/node_modules -- clean --dry-run "$RUN/fixture/projects"`. Exit status `2`, stdout empty, stderr names that Candidate. `projects/stale-app/node_modules/sweep-plan.json` does not exist. `fixture-changes.txt` is empty.
- **Existing file.** Run `"$H" run "$RUN" clean/existing -- clean --dry-run --plan projects/tracked-app/package.json projects`. Exit status `2`, stdout empty, stderr says the file is not a Plan. `projects/tracked-app/package.json` is unchanged. `fixture-changes.txt` is empty.
- **Usage.** Run `"$H" run "$RUN" clean/usage -- clean`. Exit status `2`, stdout empty, stderr contains `--dry-run`. `sweep-plan.json` does not exist in the Fixture. `fixture-changes.txt` is empty.

## Gotchas

- The default Plan file is `sweep-plan.json` in the working directory. The default step passes `--plan` so that file is not created in the Fixture.
- The inside step's working directory is the Candidate, so its Root is the absolute `projects` directory. A relative `projects` would be resolved from inside the Candidate and would not be a Root.
- A Candidate is Stale when the newest modification of its Project's own files, outside the Candidate, is more than 30 days ago. In this Fixture that is only `projects/stale-app`.
