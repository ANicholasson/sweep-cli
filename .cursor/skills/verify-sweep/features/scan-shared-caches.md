# Shared caches

A Scan adds each known Shared cache as a Root inside the home directory, without being pointed at it. The npm cache is the first. It is a Candidate of Kind Cache, listed under `Shared caches`, apart from the Candidates beneath the Roots the user gave. The Root is marked `added by sweep` and written in the Plan. It is never in the default Pick. `--no-shared-caches` and `shared_caches = false` leave it out. `--pick-caches Node` picks it for one run.

## Sub-features

- `cache-found` lists the npm cache, marked as a Root sweep added, in the Shared caches group.
- `cache-off` leaves it out with `--no-shared-caches`, and the Roots are only the ones the user gave.
- `cache-setting` leaves it out when `shared_caches` is false.
- `cache-pick` leaves it out of the default Pick and adds it with `--pick-caches`.

## How to get to it (user POV)

- Run `sweep scan` anywhere. The npm cache is listed when it exists in the home directory.
- Run `sweep scan --no-shared-caches` to look only beneath the Roots you gave.
- Set `shared_caches = false` in the configuration file to leave Shared caches off.
- Run `sweep clean --dry-run --pick-caches Node` to put the npm cache in the Plan.

## Driving it with sweep-verify

Preconditions:

- Baseline preconditions from the [index](./README.md) hold.
- The npm cache path below is `$RUN/scratch/home/.npm/_cacache` on Linux and macOS, and `$RUN/scratch/home/AppData/Local/npm-cache/_cacache` on Windows. `"$H" du "$RUN"` of that path is its Size. The Fixture holds the same tree at `$RUN/fixture/home`.
- The configuration directory for the setting step is `$RUN/scratch/config`. On Linux, `XDG_CONFIG_HOME` is that directory.

- **Found.** Run `"$H" run "$RUN" scan-shared/found -- scan projects`. Exit status `0`, stderr empty. stdout has `Roots`, the `projects` Root, the npm cache path followed by `added by sweep`, a `Shared caches` heading, and a row for the cache with Kind `Cache`. `fixture-changes.txt` is empty.
- **Size.** Run `"$H" run "$RUN" scan-shared/json -- scan --json projects`. Exit status `0`. The cache's `kind` is `Cache`, `ecosystem` is `Node`, `project` is `""`, `stale` is true, and `size` equals the `du` bytes. `added` is that path only. `roots` ends with it.
- **Flag off.** Run `"$H" run "$RUN" scan-shared/off -- scan --json --no-shared-caches projects`. Exit status `0`. No `path` is the npm cache. `added` is `[]`. `roots` is `$RUN/fixture/projects` only. `fixture-changes.txt` is empty.
- **Setting off.** Write `shared_caches = false` into `$RUN/scratch/config/sweep/config.toml`. Run `"$H" run "$RUN" scan-shared/setting -- scan --json projects`. Exit status `0`. The npm cache is absent and `added` is `[]`. `fixture-changes.txt` is empty.
- **Not picked.** Run `"$H" run "$RUN" scan-shared/plan -- clean --dry-run --plan "$RUN/scratch/cache-plan.json" projects`. Exit status `0`, stderr empty. stdout has `not picked: choose it yourself` and, under it, the npm cache. The Plan's `roots` include the cache path. Its `candidates` do not. `fixture-changes.txt` is empty.
- **Pick by Ecosystem.** Run `"$H" run "$RUN" scan-shared/pick -- clean --dry-run --plan "$RUN/scratch/cache-pick.json" --pick-caches Node projects`. Exit status `0`. stdout has `picked: named with --pick-caches` and the cache path. The Plan's `candidates` include the cache, and its Fingerprint `size` equals the `du` bytes. `fixture-changes.txt` is empty.
- **Unknown Ecosystem.** Run `"$H" run "$RUN" scan-shared/unknown -- clean --dry-run --plan "$RUN/scratch/cache-unknown.json" --pick-caches Nope projects`. Exit status `2`, stdout empty, stderr names `Nope`. The Plan file does not exist.
- **Unchanged.** Every step's `fixture-changes.txt` is empty.

## Gotchas

- The helper sets `HOME` and `USERPROFILE` to `$RUN/scratch/home`. A Root outside `$RUN` means that did not happen. A dry run's default Plan is under that home, not in the Fixture.
- The cache is stale because its files are old, and it is still not in the default Pick.
- `--pick-caches` names an Ecosystem, `Node`, not a path. `--pick` of the cache path also puts it in the Plan.
- On macOS the setting step's configuration file is under `$RUN/scratch/home/Library/Application Support/sweep`, because the helper's `HOME` is that home directory.
