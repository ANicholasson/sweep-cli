# Dry run writes a Plan

`sweep clean --dry-run` reads the same Configuration a Scan reads. Roots come from the command line, then the file, then the current directory. The file's Stale threshold decides what is Stale, and a path on the Ignore list is left out of the Plan. It prints the file it read, or `none`, picks every Stale Candidate beneath those Roots, prints each Candidate as picked or not picked with the reason, and writes a Plan. Nothing is removed. With no `--plan`, the Plan is `sweep/plan.json` in the operating system's per-user cache directory, and the dry run prints that full path. `--plan` names another file. A path inside a Project the Scan found is refused, and nothing is written.

## Sub-features

- `default` writes a Plan of the Stale Candidates.
- `pick` adds a Candidate that is not Stale.
- `inside` refuses a Plan path inside a Candidate, including one reached through a Link above the file's parent.
- `in-project` refuses a Plan path inside a Project.
- `twice` runs twice from inside a Stale Project and writes the same Plan outside it.
- `existing` refuses a file that already exists and is not a Plan.
- `usage` rejects `sweep clean` without `--dry-run`.
- `config-roots` uses the file's Roots, in that order, and names the file.
- `config-cli` lets Roots on the command line replace the file's Roots, in the order given.
- `config-cwd` uses the current directory when the file names no Roots.
- `config-stale` marks Stale from `stale_days` instead of 30.
- `config-ignore` leaves an Ignore path, and everything beneath it, out of the Plan, and refuses `--pick` of it.
- `config-delete` rejects a key that would make Delete the default, and writes no Plan.
- `config-hold` leaves out a Candidate that holds an Ignore path, and refuses `--pick` of it.
- `config-plan-ignore` refuses a Plan path inside an Ignore path.

## How to get to it (user POV)

- Run `sweep clean --dry-run --plan <file> [Root...]`.
- Run `sweep clean --dry-run --pick <path> [Root...]`.
- Run `sweep clean --dry-run` with no `--plan`. The Plan goes to the per-user cache.
- Run `sweep clean --dry-run` from inside a Project.
- Run `sweep clean --dry-run --plan <path>` with a path inside a Project.
- Run `sweep clean --dry-run --plan <file>` from inside a Candidate.
- Run `sweep clean` with no `--dry-run`.
- Run `sweep clean --dry-run` with no Root after writing Roots, `stale_days` or `ignore` in the configuration file.

## Driving it with sweep-verify

Preconditions:

- Baseline preconditions from the [index](./README.md) hold.
- `$RUN/scratch` exists. Plan files for the steps that should leave the Fixture unchanged go there.
- The cache directory for a step with no `--plan` is `$RUN/scratch/cache`, which is not the real per-user directory. On Linux, set `XDG_CACHE_HOME` to that directory. The Plan is `$RUN/scratch/cache/sweep/plan.json`. The helper sets `HOME` to `$RUN/fixture/home`, so on macOS the Plan is under that home's `Library/Caches/sweep/plan.json`. On Windows it is `LocalAppData`.
- The configuration directory is `$RUN/scratch/config`. On Linux, `XDG_CONFIG_HOME` is that directory for every step. It is not the real per-user directory. The file is `$RUN/scratch/config/sweep/config.toml`. Until a step below writes it, the directory has no file, and a report that is printed starts with `Configuration` and `none`.

- **Default Pick.** Run `"$H" run "$RUN" clean/default -- clean --dry-run --plan "$RUN/scratch/plan.json" projects`. Exit status `0`, stderr empty. stdout has `picked: Stale` and, under it, `projects/stale-app/node_modules`. It has `not picked: used recently` and, under it, the other six Candidates beneath `projects`. It has `not picked: choose it yourself` and, under it, the npm cache. It ends with `1 Candidate in the Plan`, a Size, `Written to $RUN/scratch/plan.json`, `Left out 6 Candidates`, `used recently`, `Left out 1 Candidate`, `choose it yourself`, and `To add one, pass --pick and its path.` The Plan file is JSON `version` 1. Its `roots` are the `projects` directory and then the npm cache path. Its `candidates` paths are exactly `projects/stale-app/node_modules` and no Trap and not the cache. That Candidate's Fingerprint `size` equals `"$H" du "$RUN" "$RUN/fixture/projects/stale-app/node_modules"`. `fixture-changes.txt` is empty.
- **Pick.** Run `"$H" run "$RUN" clean/pick -- clean --dry-run --plan "$RUN/scratch/picked.json" --pick projects/recent-app/node_modules projects`. Exit status `0`, stderr empty. The Plan's paths are `projects/stale-app/node_modules` and `projects/recent-app/node_modules`. stdout has `picked: named with --pick` and, under it, the recent Candidate. `fixture-changes.txt` is empty.
- **Inside a Candidate.** Run `"$H" run "$RUN" clean/inside -C projects/stale-app/node_modules -- clean --dry-run --plan sweep-plan.json "$RUN/fixture/projects"`. Exit status `2`, stdout empty, stderr names that Candidate. `projects/stale-app/node_modules/sweep-plan.json` does not exist. `fixture-changes.txt` is empty.
- **Inside a Project.** Run `"$H" run "$RUN" clean/in-project -- clean --dry-run --plan projects/stale-app/plan.json projects`. Exit status `2`, stdout empty, stderr names `projects/stale-app/plan.json` and the Project `projects/stale-app`. That file does not exist. `fixture-changes.txt` is empty.
- **Twice from a Stale Project.** With the cache directory pointed at `$RUN/scratch/cache`, run `"$H" run "$RUN" clean/twice -C projects/stale-app -- clean --dry-run .` and the same command again as `clean/twice-again`. Both exit status `0`, stderr empty, and both print `1 Candidate in the Plan` and `Written to` the full Plan path under the cache directory. The Plan's bytes after the second run equal its bytes after the first. No new file appears under `projects/stale-app`. Both `fixture-changes.txt` files are empty.
- **Existing file.** Run `"$H" run "$RUN" clean/existing -- clean --dry-run --plan "$RUN/fixture/answer-key.json" projects`. Exit status `2`, stdout empty, stderr says the file is not a Plan. `answer-key.json` is unchanged. `fixture-changes.txt` is empty.
- **Usage.** Run `"$H" run "$RUN" clean/usage -- clean`. Exit status `2`, stdout empty, stderr contains `--dry-run`. `sweep-plan.json` does not exist in the Fixture. `fixture-changes.txt` is empty.
- **Roots from the file.** Write `roots = ['$RUN/fixture/projects/recent-app', '$RUN/fixture/projects/web-app']`. From `$RUN/scratch`, run `"$H" run "$RUN" clean/config-roots -C "$RUN/scratch" -- clean --dry-run --plan "$RUN/scratch/config-roots.json"`. Exit status `0`, stderr empty. stdout contains `Configuration` and `$RUN/scratch/config/sweep/config.toml`. The Plan's `roots` are `$RUN/fixture/projects/recent-app`, then `$RUN/fixture/projects/web-app`, then the npm cache path. `fixture-changes.txt` is empty.
- **Command line wins.** Write `roots = ['$RUN/fixture/projects/web-app', '$RUN/fixture/projects/recent-app']`. Run `"$H" run "$RUN" clean/config-cli -- clean --dry-run --plan "$RUN/scratch/config-cli.json" projects/stale-app projects/recent-app`. Exit status `0`, stderr empty. The Plan's `roots` are `$RUN/fixture/projects/stale-app`, then `$RUN/fixture/projects/recent-app`, then the npm cache path. `fixture-changes.txt` is empty.
- **Current directory.** Write `stale_days = 30` and no `roots`. Run `"$H" run "$RUN" clean/config-cwd -C projects -- clean --dry-run --plan "$RUN/scratch/config-cwd.json"`. Exit status `0`, stderr empty. The Plan's `roots` are `$RUN/fixture/projects` and then the npm cache path. stdout names the configuration file. `fixture-changes.txt` is empty.
- **Threshold.** Write `roots = ['$RUN/fixture/projects']` and `stale_days = 90`. Run `"$H" run "$RUN" clean/config-stale -C "$RUN/scratch" -- clean --dry-run --plan "$RUN/scratch/config-stale.json"`. Exit status `0`, stderr empty. `projects/stale-app/node_modules` is absent from the Plan and printed under `not picked: used recently`. The newest file in that Project outside `node_modules` is newer than 90 days before the run. `fixture-changes.txt` is empty. Write `stale_days = 0` with the same `roots` and run `"$H" run "$RUN" clean/config-stale-zero -C "$RUN/scratch" -- clean --dry-run --plan "$RUN/scratch/config-stale-zero.json"`. Exit status `0`. `projects/recent-app/node_modules` is in the Plan, and its Fingerprint `size` equals `"$H" du "$RUN" "$RUN/fixture/projects/recent-app/node_modules"`. `fixture-changes.txt` is empty.
- **Ignore.** Write `roots = ['$RUN/fixture/projects']` and `ignore` containing `$RUN/fixture/projects/stale-app` and `$RUN/fixture/projects/web-app/node_modules`. Run `"$H" run "$RUN" clean/config-ignore -- clean --dry-run --plan "$RUN/scratch/config-ignore.json" projects`. Exit status `0`, stderr empty. Neither ignored path, nor `projects/stale-app/node_modules`, is in the Plan or stdout. `projects/recent-app/node_modules` is in stdout. `fixture-changes.txt` is empty. Run `"$H" run "$RUN" clean/config-ignore-scan -- scan --json projects`. Exit status `0`. Those Candidates are absent from `candidates`, and `fixture-changes.txt` is empty. Run `"$H" run "$RUN" clean/config-ignore-pick -- clean --dry-run --plan "$RUN/scratch/config-ignore-pick.json" --pick "$RUN/fixture/projects/stale-app/node_modules" projects`. Exit status `2`, stdout empty, stderr names that path. `$RUN/scratch/config-ignore-pick.json` does not exist. `fixture-changes.txt` is empty.
- **Rejected key.** Write `delete = true`. Run `"$H" run "$RUN" clean/config-delete -C "$RUN/scratch" -- clean --dry-run --plan "$RUN/scratch/config-delete.json"`. Exit status `2`, stdout empty, stderr names the file and `delete`. `$RUN/scratch/config-delete.json` does not exist. `fixture-changes.txt` is empty.
- **Ignore inside a Candidate.** In `$RUN/scratch/hold`, make `app/package.json`, `app/src/a.js` and `app/node_modules/keep/precious.txt`, each last modified in 2020, and the same for `other/package.json` and `other/node_modules/a.js`. Write `ignore` as `$RUN/scratch/hold/app/node_modules/keep`. Run `"$H" run "$RUN" clean/config-hold -- clean --dry-run --plan "$RUN/scratch/hold-plan.json" "$RUN/scratch/hold"`. Exit status `0`. The Plan does not hold `app/node_modules`, and stdout does not name it. `other/node_modules` is in the Plan. `precious.txt` is still there. Run `"$H" run "$RUN" clean/config-hold-pick -- clean --dry-run --plan "$RUN/scratch/hold-pick.json" --pick "$RUN/scratch/hold/app/node_modules" "$RUN/scratch/hold"`. Exit status `2`, stdout empty, stderr names that path. The Plan file does not exist. `fixture-changes.txt` is empty for both.
- **Plan inside a dropped Candidate.** With the hold tree and `ignore` of `app/node_modules/keep`, run `"$H" run "$RUN" clean/config-hold-project -- clean --dry-run --plan "$RUN/scratch/hold/app/plan.json" "$RUN/scratch/hold"`. Exit status `2`, stdout empty, stderr names that path and the Project. The file does not exist. Run `"$H" run "$RUN" clean/config-hold-candidate -- clean --dry-run --plan "$RUN/scratch/hold/app/node_modules/plan.json" "$RUN/scratch/hold"`. Exit status `2`, stdout empty, stderr names that Candidate. The file does not exist. Both `fixture-changes.txt` files are empty.
- **Plan inside an Ignore path.** With the same tree, write `ignore` as `$RUN/scratch/hold/app`. Run `"$H" run "$RUN" clean/config-plan-project -- clean --dry-run --plan "$RUN/scratch/hold/app/plan.json" "$RUN/scratch/hold"`. Exit status `2`, stdout empty, stderr names that Plan path and `app`. The file does not exist. Write `ignore` as `$RUN/scratch/hold/other/node_modules`. Run `"$H" run "$RUN" clean/config-plan-candidate -- clean --dry-run --plan "$RUN/scratch/hold/other/node_modules/plan.json" "$RUN/scratch/hold"`. Exit status `2`, stdout empty, stderr names that Plan path. The file does not exist. Both `fixture-changes.txt` files are empty.

## Gotchas

- With no `--plan`, the Plan is `sweep/plan.json` in the per-user cache directory, not `sweep-plan.json` in the working directory. Point that directory at `$RUN/scratch/cache` or the dry run writes outside the run.
- The inside step passes `--plan sweep-plan.json` because the working directory is the Candidate. Its Root is the absolute `projects` directory. A relative `projects` would be resolved from inside the Candidate and would not be a Root.
- The existing step uses `answer-key.json`, which sits outside every Project. A path inside a Project is refused first, so it never reaches the check that the file is already a Plan.
- A Candidate is Stale when the newest modification of its Project's own files, outside the Candidate, is more than 30 days ago. In this Fixture that is only `projects/stale-app`. `stale_days` of `90` makes that Project not Stale. `stale_days` of `0` marks a Candidate Stale when its Last used is before the run.
- These steps read the real configuration file unless the configuration directory is the scratch one named above. A Root outside `$RUN` means the real file was read.
