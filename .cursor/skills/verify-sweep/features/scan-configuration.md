# Configuration

A per-user configuration file sets default Roots, the Stale threshold and the Ignore list. The file is `config.toml` in the `sweep` directory of the operating system's configuration directory. Roots come from the command line first, then the file, then the current directory. `stale_days` replaces the 30-day threshold. A path on the Ignore list, and everything beneath it, is never reported. A missing file means the defaults. A malformed file or an unknown key is an error, and nothing is scanned. The output names the file that was read, or `none`.

## Sub-features

- `config-default` uses the current directory when no file and no Root are given, and prints `none`.
- `config-roots` uses the file's Roots when the command line gives none, including from another directory, and names the file.
- `config-cli` lets a Root on the command line replace the file's Roots.
- `config-stale` marks Stale from `stale_days` instead of 30.
- `config-ignore` leaves out an ignored path and everything beneath it.
- `config-ignore-age` leaves Last used on the newest Project file when that file is ignored.
- `config-bad` rejects a malformed file, a bad value and a key that would switch off a Safety rule or make Delete the default.

## How to get to it (user POV)

- Run `sweep scan` with no Root, from any directory, after writing the file.
- Run `sweep scan <Root...>` to override the file's Roots. The threshold and the Ignore list still apply.
- Read the `Configuration` line, or the JSON `configuration` field.

## Driving it with sweep-verify

Preconditions:

- Baseline preconditions from the [index](./README.md) hold.
- The configuration directory for these steps is `$RUN/scratch/config`, which is not the real per-user directory. On Linux, set `XDG_CONFIG_HOME` to that directory for the steps below. The file is `$RUN/scratch/config/sweep/config.toml`. Build it in `scratch`, never in the Fixture.
- `$RUN/evidence/answer-key.json` names the Candidates. `projects/stale-app` was last used 60 days before the Fixture was built, and `projects/recent-app` one day before.

- **Missing file.** With no file in the configuration directory, run `"$H" run "$RUN" scan-configuration/missing -C projects -- scan --json`. Exit status `0`. `configuration` is `""` and `roots` is `["$RUN/fixture/projects"]`.
- **Roots from the file.** Write `roots = ['$RUN/fixture/projects']` into the file. From `$RUN/scratch`, run `"$H" run "$RUN" scan-configuration/roots -C "$RUN/scratch" -- scan --json`. Exit status `0`. `roots` is `["$RUN/fixture/projects"]` and `configuration` is the file's path. The table form, `"$H" run "$RUN" scan-configuration/roots-table -C "$RUN/scratch" -- scan`, contains a `Configuration` line with that path.
- **Command line wins.** With the same file, run `"$H" run "$RUN" scan-configuration/cli -- scan --json projects/recent-app`. Exit status `0`. `roots` is `["$RUN/fixture/projects/recent-app"]`.
- **Threshold.** Write `roots` as above and `stale_days = 90`. Run `"$H" run "$RUN" scan-configuration/stale -C "$RUN/scratch" -- scan --json`. Exit status `0`. `projects/stale-app/node_modules` has `stale` false. Take its Last used from the modification time of a file in that Project outside `node_modules`, and check it is not 90 days before the Scan.
- **Ignore.** Write `ignore` containing `$RUN/fixture/projects/web-app/node_modules` and `$RUN/fixture/projects/stale-app`, and the same `roots`. Run `"$H" run "$RUN" scan-configuration/ignore -- scan --json projects`. Exit status `0`. Those two Candidates are absent. `projects/recent-app/node_modules` is listed once.
- **Ignore inside a Project.** In `$RUN/scratch/age`, make `app/package.json` 100 days old, `app/src/new.js` one hour old, and `app/node_modules/a/index.js`. Point the configuration file's `ignore` at `$RUN/scratch/age/app/src` and give no `roots`. Run `"$H" run "$RUN" scan-configuration/ignore-age -C "$RUN/scratch/age" -- scan --json .`. Exit status `0`. The Candidate's `last_used` equals the modification time of `app/src/new.js`, and `stale` is false. `src` is not a reported path.
- **Rejected keys.** Write `delete = true`. Run `"$H" run "$RUN" scan-configuration/delete -C "$RUN/scratch" -- scan`. Exit status `2`, stdout empty, stderr names the file and `delete`. The same for `git = false`, `root = false` and `fingerprint = false`, each naming its key.
- **Unchanged.** Every step's `fixture-changes.txt` is empty.

## Gotchas

- The steps read the real configuration file unless the configuration directory is the scratch one named above. A Root outside `$RUN` means the real file was read. On macOS the directory is `HOME`, and the file is under `Library/Application Support/sweep`. On Windows it is `AppData`.
- `stale_days` is a number of days. `90` makes the Fixture's 60-day Project not Stale. The default `30` would mark it Stale.
- A Root on the command line replaces the file's Roots and does not turn Ignore off.
- Paths in the file are absolute. A relative path is an error and nothing is scanned.
