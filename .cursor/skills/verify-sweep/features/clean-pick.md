# Pick by Kind, Ecosystem and age

`sweep clean --dry-run` picks every Stale Candidate. `--kind` and `--ecosystem` narrow that Pick, and each can be repeated. `--stale-days` replaces the age that counts as Stale for this run, including a threshold in the configuration file. `--all` picks every Candidate whatever its age. None of those four picks a Shared cache or Docker data. `--pick` still adds the Candidate it names. Each row's reason names the flag that decided it. Nothing is removed.

## Sub-features

- `kind` writes a Plan of the Stale Candidates of one Kind.
- `kinds` writes a Plan of every Candidate of two Kinds.
- `ecosystem` writes a Plan of one Ecosystem, whatever its age.
- `both` writes an empty Plan when the Kind and the Ecosystem do not meet.
- `age` replaces 30 days for one run.
- `age-flag` uses `--stale-days` instead of `stale_days` in the configuration file.
- `all` picks the seven Candidates beneath the given Roots, not the npm cache.
- `pick` adds a Candidate the Kind left out.
- `unknown` rejects an unknown Kind, an unknown Ecosystem and a bad age, and writes no Plan.
- `refused` does not carry out a Plan when one of the flags is given without `--dry-run`.

## How to get to it (user POV)

- Run `sweep clean --dry-run --plan <file> --kind Kind [Root...]`.
- Run `sweep clean --dry-run --kind Kind --kind Kind --all [Root...]`.
- Run `sweep clean --dry-run --ecosystem Name --all [Root...]`.
- Run `sweep clean --dry-run --kind Kind --ecosystem Name --all [Root...]`.
- Run `sweep clean --dry-run --stale-days n [Root...]`.
- Run `sweep clean --dry-run --stale-days n` after writing `stale_days` in the configuration file.
- Run `sweep clean --dry-run --all [Root...]`.
- Run `sweep clean --dry-run --kind Kind --pick <path> [Root...]`.
- Run `sweep clean --dry-run --kind <not a Kind>`, `--ecosystem <not an Ecosystem>` or `--stale-days -1`.
- Run `sweep clean --plan <file> --all`.

## Driving it with sweep-verify

Preconditions:

- Baseline preconditions from the [index](./README.md) hold.
- The configuration directory for `age-flag` is `$RUN/scratch/config`, so the file is not part of the Fixture. On Linux, set `XDG_CONFIG_HOME` to that directory.

- **One Kind.** Run `"$H" run "$RUN" clean-pick/kind -- clean --dry-run --plan "$RUN/scratch/kind.json" --kind Dependencies projects`. Exit status `0`, stderr empty. The Plan's paths are the Answer key's Dependencies whose newest file outside the Candidate is more than 30 days old, and no other path. In this Fixture that is `projects/stale-app/node_modules` and not `projects/rust-app/target`. That Candidate's Fingerprint `size` equals `"$H" du "$RUN" "$RUN/fixture/projects/stale-app/node_modules"`. stdout has `picked: Stale` over the Stale Dependencies and `not picked: not named with --kind` over `projects/rust-app/target`. It does not name Docker data. `fixture-changes.txt` is empty.
- **Two Kinds.** Run `"$H" run "$RUN" clean-pick/kinds -- clean --dry-run --plan "$RUN/scratch/kinds.json" --kind Dependencies --kind "Build output" --all projects`. Exit status `0`, stderr empty. The Plan's paths are the seven Candidates beneath `projects`, and not the npm cache. `fixture-changes.txt` is empty.
- **One Ecosystem.** Run `"$H" run "$RUN" clean-pick/ecosystem -- clean --dry-run --plan "$RUN/scratch/ecosystem.json" --ecosystem Cargo --all projects`. Exit status `0`, stderr empty. The Plan's only path is `projects/rust-app/target`, whose parent holds `Cargo.toml`. stdout has `picked: --all` over that path and `not picked: not named with --ecosystem` over a `node_modules` Candidate. Its Fingerprint `size` equals `"$H" du "$RUN" "$RUN/fixture/projects/rust-app/target"`. `fixture-changes.txt` is empty.
- **Kind and Ecosystem.** Run `"$H" run "$RUN" clean-pick/both -- clean --dry-run --plan "$RUN/scratch/both.json" --kind "Build output" --ecosystem Node --all projects`. Exit status `0`, stderr empty. The Plan's `candidates` is empty. stdout has `not picked: not named with --kind` over a Dependencies Candidate and `not picked: not named with --ecosystem` over `projects/rust-app/target`. `fixture-changes.txt` is empty.
- **Age.** Run `"$H" run "$RUN" clean-pick/age-90 -- clean --dry-run --plan "$RUN/scratch/age-90.json" --stale-days 90 projects`. Exit status `0`, stderr empty. `projects/stale-app/node_modules` is absent from the Plan and printed under `not picked: newer than --stale-days 90`. The newest file in that Project outside `node_modules` is newer than 90 days before the run. `fixture-changes.txt` is empty. Run `"$H" run "$RUN" clean-pick/age-0 -- clean --dry-run --plan "$RUN/scratch/age-0.json" --stale-days 0 projects`. Exit status `0`. `projects/recent-app/node_modules` is in the Plan, under `picked: older than --stale-days 0`, and its Fingerprint `size` equals `"$H" du "$RUN" "$RUN/fixture/projects/recent-app/node_modules"`. `fixture-changes.txt` is empty.
- **Flag replaces the file.** Write `stale_days = 90` in `$RUN/scratch/config/sweep/config.toml`. With `XDG_CONFIG_HOME` set to `$RUN/scratch/config`, run `"$H" run "$RUN" clean-pick/age-flag -C "$RUN/scratch" -- clean --dry-run --plan "$RUN/scratch/age-flag.json" --stale-days 0 "$RUN/fixture/projects"`. Exit status `0`, stderr empty. `projects/stale-app/node_modules` is in the Plan. `fixture-changes.txt` is empty.
- **Any age.** Run `"$H" run "$RUN" clean-pick/all -- clean --dry-run --plan "$RUN/scratch/all.json" --all projects`. Exit status `0`, stderr empty. The Plan's paths are the seven Candidates beneath `projects`, and not the npm cache. No Trap that is not a Candidate is in it. stdout has `picked: --all` and does not have `picked: Stale` or Docker data. The npm cache is under `not picked: choose it yourself`. `fixture-changes.txt` is empty.
- **Pick outside the Kind.** Run `"$H" run "$RUN" clean-pick/pick -- clean --dry-run --plan "$RUN/scratch/pick.json" --kind "Build output" --pick projects/recent-app/node_modules projects`. Exit status `0`, stderr empty. The Plan holds `projects/recent-app/node_modules`. stdout has `picked: named with --pick` over that path. `projects/rust-app/target` is absent, because it was last used when the Fixture was built. `fixture-changes.txt` is empty.
- **Unknown.** Run `"$H" run "$RUN" clean-pick/unknown-kind -- clean --dry-run --plan "$RUN/scratch/unknown-kind.json" --kind dependencies projects`. Exit status `2`, stdout empty, stderr contains `unknown Kind`. The Plan file does not exist. Run `"$H" run "$RUN" clean-pick/unknown-ecosystem -- clean --dry-run --plan "$RUN/scratch/unknown-ecosystem.json" --ecosystem Python projects`. Exit status `2`, stdout empty, stderr contains `unknown Ecosystem`. The Plan file does not exist. Run `"$H" run "$RUN" clean-pick/bad-age -- clean --dry-run --plan "$RUN/scratch/bad-age.json" --stale-days -1 projects`. Exit status `2`, stdout empty, stderr contains `not a whole number of days`. The Plan file does not exist. Each `fixture-changes.txt` is empty.
- **Refused.** Run `"$H" run "$RUN" clean-pick/refused-plan -- clean --dry-run --plan "$RUN/scratch/refused.json" projects`, then `"$H" run "$RUN" clean-pick/refused -- clean --plan "$RUN/scratch/refused.json" --all`. The second exit status is `2`, stderr names `--all`, and `projects/stale-app/node_modules` is still in place. Both `fixture-changes.txt` files are empty.

## Gotchas

- `--kind` and `--ecosystem` name the glossary's words, so `dependencies` and `node` are rejected. `Build output` has a space.
- `--all` and `--stale-days` do not pick a Shared cache. The Fixture has the npm cache, and these Plans leave it out. `--pick-caches` picks it. `--pick` of its path does too.
- Docker data is not a Candidate and is not a row of this report.
- `--stale-days 0` marks a Candidate Stale when its Last used is before the run. It is not `--all`.
- For `age-flag`, set `XDG_CONFIG_HOME` to `$RUN/scratch/config`. Otherwise the dry run reads the machine's own file. On macOS the directory is `HOME`. On Windows it is `AppData`.
