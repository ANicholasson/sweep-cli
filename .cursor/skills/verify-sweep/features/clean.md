# Clean carries out a Plan

`sweep clean --plan <file>` moves exactly the Candidates in that Plan to the trash. Immediately before each one it re-checks the Git rule, the Root rule and the Fingerprint rule. A Candidate that fails one is a Skip, and nothing inside it is touched. A Candidate the trash cannot take stays in place. The summary says the moved space was moved to trash. On Windows the trash is the Recycle Bin, unless `XDG_DATA_HOME` names a directory, in which case that directory is used.

## Sub-features

- `act` moves the default Plan and leaves every other path.
- `changed` skips one Candidate under the Fingerprint rule and moves the rest.
- `link` skips a Candidate whose parent was swapped for a Link.
- `malformed` refuses a Plan that is not version 1 and leaves the Fixture untouched.
- `stuck` reports a failure when the trash cannot be created and leaves the Candidates in place.

## How to get to it (user POV)

- Run `sweep clean --dry-run --plan <file> [Root...]`, then `sweep clean --plan <file>`.
- Change one Candidate between the two, then run the Clean.
- Swap a directory above a Candidate for a Link between the two, then run the Clean.
- Point `sweep clean --plan` at a file that is not a Plan.
- Point the trash at a file, so a move cannot create it, then run the Clean.

## Driving it with sweep-verify

Preconditions:

- Baseline preconditions from the [index](./README.md) hold.
- On Linux, `XDG_DATA_HOME` is `$RUN/fixture/trash-home` for every Clean step, so the trash is `$RUN/fixture/trash-home/Trash` and a moved Candidate is under `Trash/files`. `XDG_CONFIG_HOME` is `$RUN/scratch/config` and `XDG_CACHE_HOME` is `$RUN/scratch/cache`.
- On Windows, `XDG_DATA_HOME` is the same directory, so a moved Candidate is under `Trash/files`. Leaving it unset sends the Candidate to the Recycle Bin. Set `AppData` to `$RUN/scratch/config` and `LocalAppData` to `$RUN/scratch/cache`. sweep reads the configuration from `AppData`, not from `XDG_CONFIG_HOME`. The file is `$RUN/scratch/config/sweep/config.toml`. A Clean that reads the machine's own file can name a Root outside the Fixture.
- On macOS, create `$RUN/fixture/trash-home` before the Clean and point `HOME` at it. It is not in the Fixture. The trash is `$RUN/fixture/trash-home/.Trash` and a moved Candidate is in that directory. The configuration file for a step that writes one is `$RUN/fixture/trash-home/Library/Application Support/sweep/config.toml`, because config and the trash both live under `HOME`.
- A Clean step is run on Linux, macOS and Windows.

- **Default Plan.** Run `"$H" run "$RUN" clean-act/dry -- clean --dry-run --plan "$RUN/scratch/plan.json" projects`. Exit status `0`, stderr empty. The Plan's only path is `projects/stale-app/node_modules`. Its Fingerprint `size` equals `"$H" du "$RUN" "$RUN/fixture/projects/stale-app/node_modules"`. `fixture-changes.txt` is empty. Run `"$H" run "$RUN" clean-act/move -- clean --plan "$RUN/scratch/plan.json"`. Exit status `0`, stderr empty. stdout contains `Moved to trash:` and the Size from `du`, and it does not contain `reclaimed`. `projects/stale-app/node_modules` is gone. On Linux and Windows the same directory is under `$RUN/fixture/trash-home/Trash/files`, and on macOS it is under `$RUN/fixture/trash-home/.Trash`. `fake-old/note.txt` there has the same bytes as before the Clean. Every other Candidate is still in place. Every Trap is still in place, including `projects/shifted-app/node_modules`. `projects/stale-app/package.json` is still in place.
- **Changed Candidate.** Write `roots = ['$RUN/fixture/projects']` and `stale_days = 0` into `$RUN/scratch/config/sweep/config.toml`. On Linux, `XDG_CONFIG_HOME` is `$RUN/scratch/config`. On Windows, `AppData` is `$RUN/scratch/config`. On macOS the file is under `HOME`. Run `"$H" run "$RUN" clean-changed/dry -- clean --dry-run --plan "$RUN/scratch/changed.json"` with no Root. Exit status `0`. Every path in the Plan is inside `$RUN/fixture`, including `projects/shifted-app/node_modules`. Add a file inside that Candidate. Run `"$H" run "$RUN" clean-changed/move -- clean --plan "$RUN/scratch/changed.json"`. Exit status `1`, stderr empty. stdout contains `Fingerprint rule` and that Candidate's path, and `Moved to trash:` for the others. The added file is still there. The other Candidates from the Plan are under the trash and gone from where they were. Every Trap is still in place.
- **Link above a Candidate.** On a fresh Fixture, dry-run the default Plan, rename `projects/stale-app` aside, and put a Link at that path whose target is `outside/stash-app`. Run the Clean. Exit status `1`. stdout contains `Root rule` and `projects/stale-app/node_modules`. The file `outside/stash-app/node_modules/fake-theta/index.js` is unchanged, and the Candidate that was renamed aside is unchanged.
- **Malformed Plan.** Run `"$H" run "$RUN" clean-malformed/bad -- clean --plan "$RUN/scratch/not-a-plan.json"` with that file holding `not a plan`. Exit status `2`, stdout empty, stderr says the Plan is malformed. `fixture-changes.txt` is empty. Run `"$H" run "$RUN" clean-malformed/trailing -- clean --plan "$RUN/scratch/trailing.json"` with that file holding a version 1 Plan followed by `}garbage`. Exit status `2`, stdout empty, stderr says the Plan is malformed. `fixture-changes.txt` is empty. Run `"$H" run "$RUN" clean-malformed/dots -- clean --plan "$RUN/scratch/dots.json"` with a Root that is `$RUN/fixture/projects` and a Candidate that is that Root plus `/a/../tracked-app/node_modules`, where `projects/a` is a Link. Exit status `2`, stdout empty, stderr names that Candidate path. `fixture-changes.txt` shows only the Link that was added before the Clean. The tracked files under `projects/tracked-app/node_modules` are still there.
- **Trash behind a Link.** On Linux and Windows, point `XDG_DATA_HOME` at a Link to a real directory. On Windows, `AppData` stays `$RUN/scratch/config`. On macOS, point `HOME` at that Link. Dry-run the default Plan, then Clean it. Exit status `0`. stdout does not contain `not a directory`. `projects/stale-app/node_modules` is gone from where it was and is under the real directory the Link leads to, and a file from it can be put back.
- **Link above a Root.** `ln -s "$RUN/fixture" "$RUN/scratch/fx"`. From `$RUN/scratch/fx/projects/web-app`, with `PWD` set to that directory, run `"$H" run "$RUN" clean-root-link/dry -C "$RUN/scratch/fx/projects/web-app" -- clean --dry-run --plan "$RUN/scratch/root-link.json" --pick node_modules`. Exit status `0`. The Plan root is that spelled directory, not `$RUN/fixture/projects/web-app`. Run `"$H" run "$RUN" clean-root-link/move -C "$RUN/scratch/fx/projects/web-app" -- clean --plan "$RUN/scratch/root-link.json"`. Exit status `0`. stdout contains `Moved to trash:` and does not contain `not a directory` or `reclaimed`. `projects/web-app/node_modules` is under the trash and can be put back, and `fake-alpha/index.js` is the same file.
- **Trash refused.** On Linux and Windows, point `XDG_DATA_HOME` at a file inside the Fixture. On Windows, `AppData` stays `$RUN/scratch/config`. On macOS, `HOME` is a directory and `$HOME/.Trash` is a file. Dry-run a Plan, then Clean it. Exit status `1`. stdout contains `Failed` and does not contain `reclaimed`. Every Candidate in the Plan is still in place. Every Trap is still in place.

## Gotchas

- On Linux the trash follows `XDG_DATA_HOME`. On Windows the private trash does too, and the configuration follows `AppData`, not `XDG_CONFIG_HOME`. On macOS both follow `HOME` and the directory is `.Trash`. The helper keeps a `HOME` already set inside the run, and otherwise points it at `$RUN/scratch/home`. Point Linux and Windows `XDG_DATA_HOME` at `$RUN/fixture/trash-home`, Windows `AppData` at `$RUN/scratch/config`, and macOS `HOME` at `$RUN/fixture/trash-home`. Leave the Linux or Windows data directory unset and the Clean writes to the machine's trash.
- A Clean of the default Plan moves only `projects/stale-app/node_modules`. `projects/shifted-app/node_modules` is a Candidate and a Trap. It stays until a Plan includes it and it has not changed.
- Changing a Candidate, or swapping a directory for a Link, changes the Fixture on purpose. The proof is that step's listing, not a later doctor, which then reports the Fixture changed.
- A path the Windows Recycle Bin cannot take, including a long path or a path on a drive with no Recycle Bin, is a failure and stays in place. sweep does not delete it.
