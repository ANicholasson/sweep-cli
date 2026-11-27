# Delete on request

`sweep clean --plan <file> --delete` removes exactly the Candidates in that Plan permanently. The output says the removal is permanent before anything is removed. Immediately before each Delete it re-checks the Git rule, the Root rule and the Fingerprint rule. A Candidate that fails one is a Skip, and nothing inside it is touched. The summary lists what was deleted and calls that space reclaimed. Deleted Candidates are gone from the tree and are not in the trash. No configuration key can make Delete the default. `--delete` on a dry run is a usage error.

## Sub-features

- `act` deletes the default Plan, leaves every Trap, and leaves a Link target outside the Candidate.
- `changed` skips one Candidate under the Fingerprint rule and deletes the rest.
- `link` skips a Candidate whose parent was swapped for a Link, and leaves the directory that Link names.
- `readonly` deletes a Candidate that holds a read-only file.
- `dry` refuses `--delete` on a dry run and removes nothing.

## How to get to it (user POV)

- Run `sweep clean --dry-run --plan <file> [Root...]`, then `sweep clean --delete --plan <file>`.
- Change one Candidate between the two, then run the Clean with `--delete`.
- Swap a directory above a Candidate for a Link between the two, then run the Clean with `--delete`.
- After the Plan, take the write bit off a file inside a Candidate, then run the Clean with `--delete`.
- Run `sweep clean --delete --dry-run --plan <file>`.

## Driving it with sweep-verify

Preconditions:

- Baseline preconditions from the [index](./README.md) hold.
- On Linux, `XDG_DATA_HOME` is `$RUN/fixture/trash-home` for every Clean step, so a moved Candidate would be under `Trash/files`. `XDG_CONFIG_HOME` is `$RUN/scratch/config` and `XDG_CACHE_HOME` is `$RUN/scratch/cache`.
- On Windows, `XDG_DATA_HOME` is the same directory. Set `AppData` to `$RUN/scratch/config` and `LocalAppData` to `$RUN/scratch/cache`.
- On macOS, create `$RUN/fixture/trash-home` before the Clean and point `HOME` at it. The trash would be `$RUN/fixture/trash-home/.Trash`.
- A Clean step is run on Linux, macOS and Windows.

- **Default Plan.** Before the dry run, put a directory outside the Fixture and a Link to it at `projects/stale-app/node_modules/escape`, with a file `marker.txt` in that outside directory. Run `"$H" run "$RUN" clean-delete/dry -- clean --dry-run --plan "$RUN/scratch/plan.json" projects`. Exit status `0`, stderr empty. The Plan's only path is `projects/stale-app/node_modules`. Its Fingerprint `size` equals `"$H" du "$RUN" "$RUN/fixture/projects/stale-app/node_modules"`. Run `"$H" run "$RUN" clean-delete/delete -- clean --delete --plan "$RUN/scratch/plan.json"`. Exit status `0`, stderr empty. stdout starts `The removal is permanent.` and contains `Deleted` and `Reclaimed:` with the Size from `du`, and it does not contain `Moved to trash`. `projects/stale-app/node_modules` is gone. It is not under the trash. `marker.txt` still has the same bytes. Every other Candidate is still in place. Every Trap is still in place. `projects/stale-app/package.json` is still in place.
- **Changed Candidate.** Write `roots = ['$RUN/fixture/projects']` and `stale_days = 0` into `$RUN/scratch/config/sweep/config.toml`. On Linux, `XDG_CONFIG_HOME` is `$RUN/scratch/config`. On Windows, `AppData` is `$RUN/scratch/config`. On macOS the file is under `HOME`. Run `"$H" run "$RUN" clean-delete/changed-dry -- clean --dry-run --plan "$RUN/scratch/changed.json"` with no Root. Exit status `0`. Every path in the Plan is inside `$RUN/fixture`. Add a file inside `projects/shifted-app/node_modules`. Run `"$H" run "$RUN" clean-delete/changed -- clean --delete --plan "$RUN/scratch/changed.json"`. Exit status `1`, stderr empty. stdout starts `The removal is permanent.`, contains `Fingerprint rule` and that Candidate's path, and `Reclaimed:` for the others. It does not contain `Moved to trash`. The added file is still there. The other Candidates from the Plan are gone and are not under the trash. Every Trap is still in place.
- **Link above a Candidate.** On a fresh Fixture, dry-run the default Plan, rename `projects/stale-app` aside, and put a Link at that path whose target is `outside/stash-app`. Run `"$H" run "$RUN" clean-delete/link -- clean --delete --plan "$RUN/scratch/plan.json"`. Exit status `1`. stdout starts `The removal is permanent.`, contains `Root rule` and `projects/stale-app/node_modules`, and does not contain `Reclaimed: 1`. `outside/stash-app/node_modules/fake-theta/index.js` is unchanged, and the Candidate that was renamed aside is unchanged. Nothing was written under the trash.
- **Read-only file.** On a fresh Fixture, dry-run the default Plan. Take the write bit off `projects/stale-app/node_modules/fake-old/note.txt`. Run `"$H" run "$RUN" clean-delete/readonly -- clean --delete --plan "$RUN/scratch/plan.json"`. Exit status `0`, stderr empty. stdout starts `The removal is permanent.` and contains `Reclaimed:`. `projects/stale-app/node_modules` is gone, including that file. It is not under the trash. Every Trap is still in place.
- **Dry run.** Run `"$H" run "$RUN" clean-delete/refused -- clean --delete --dry-run --plan "$RUN/scratch/refused.json" projects`. Exit status `2`, stdout empty, stderr is the clean usage. `fixture-changes.txt` is empty. The Plan file was not written.

## Gotchas

- Point Linux and Windows `XDG_DATA_HOME` at `$RUN/fixture/trash-home`, Windows `AppData` at `$RUN/scratch/config`, and macOS `HOME` at `$RUN/fixture/trash-home`, the same as a Clean that moves. A Delete must not write that trash.
- A Link added inside a Candidate before the dry run is part of the Fingerprint. Adding it after the dry run makes the Fingerprint rule skip that Candidate.
- `delete = true` in the configuration file is an unknown key. That refusal is driven with [Configuration](./scan-configuration.md), not here.
- Changing a Candidate, or adding a Link, changes the Fixture on purpose. The proof is that step's listing, not a later doctor.
