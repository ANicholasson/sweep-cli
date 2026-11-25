# A Scan of a tree built to mislead it

A Scan keeps its promises when the tree works against it: a Link that leads out of the Root, a Root that is itself a Link, a directory it cannot read, a repository hidden inside a Candidate, and a `docker` that never answers. Each case here once produced a wrong Scan that the Fixture did not catch, so drive this file whenever a change touches how a Scan walks, resolves a Root, reads Last used, applies the Git rule or asks Docker.

## Sub-features

- `hostile-link-out` never lists what lies behind a Link beneath a Root: a Link to a Project outside, a Link to the directory that holds it, a Link to nothing, and a Link to another `node_modules`.
- `hostile-link-root` resolves a Root that is a Link reached through another Link to the directory `realpath` gives, not to a sibling of the same name.
- `hostile-unreadable-project` lists a directory it cannot read inside a Project once, and does not mark that Project's Candidate Stale.
- `hostile-link-last-used` does not count a file behind a Link as work in the Project, so the Candidate stays Stale.
- `hostile-nested-repo` leaves out a Candidate that holds a repository with a tracked file, and lists one whose nested repository tracks nothing.
- `hostile-docker-stall` gives up on a `docker` that never answers, says so on the Docker data line, and still exits zero.

## How to get to it (user POV)

- Run `sweep scan [--json] <Root...>` on a tree that holds Links, unreadable directories or nested repositories.
- Run `sweep scan [--json] <Root>` where `<Root>` is a Link.
- Run `sweep scan` on a machine whose `docker` hangs.

## Driving it with sweep-verify

Preconditions:

- Baseline preconditions from the [index](./README.md) hold.
- Linux or macOS, as an account that is not root (`id -u` is not `0`). Making a Link on Windows needs a privilege most accounts lack.
- Every tree is built in `$RUN/scratch/hostile`, never in the Fixture:

```bash
T="$RUN/scratch/hostile"

L="$T/links"
mkdir -p "$L/root/app/node_modules/a" "$L/outside/stray/node_modules/b"
printf '{}\n' > "$L/root/app/package.json"
printf '{}\n' > "$L/outside/stray/package.json"
printf 'x\n' > "$L/root/app/node_modules/a/index.js"
printf 'x\n' > "$L/outside/stray/node_modules/b/index.js"
ln -s ../outside/stray "$L/root/escape"
ln -s . "$L/root/loop"
ln -s nowhere "$L/root/dangling"
ln -s ../../outside/stray/node_modules "$L/root/app/vendor"

R="$T/rootlink"
mkdir -p "$R/deep/dir" "$R/deep/target/node_modules/right" "$R/target/node_modules/wrong"
printf '{}\n' > "$R/deep/target/package.json"
printf '{}\n' > "$R/target/package.json"
ln -s deep/dir "$R/alias"
ln -s ../target "$R/deep/dir/rootlink"

P="$T/stale"
mkdir -p "$P/locked/secret" "$P/locked/node_modules/a" "$P/linked/node_modules/a" "$P/linked/src" "$P/plain/node_modules/a" "$P/plain/src" "$P/elsewhere"
for d in locked linked plain; do
	printf '{}\n' > "$P/$d/package.json"
	printf 'x\n' > "$P/$d/node_modules/a/index.js"
done
printf 'x\n' > "$P/linked/src/a.js"
printf 'x\n' > "$P/plain/src/a.js"
ln -s ../elsewhere "$P/linked/jump"
find "$P/locked" "$P/linked" "$P/plain" -exec touch -h -t 202001010000 {} +
printf 'new\n' > "$P/elsewhere/fresh.js"
chmod 0 "$P/locked/secret"

N="$T/nested"
mkdir -p "$N/tracked/node_modules/pkg" "$N/empty/node_modules/pkg"
printf '{}\n' > "$N/tracked/package.json"
printf '{}\n' > "$N/empty/package.json"
repo() { git -C "$1" -c user.name=Fixture -c user.email=fixture@example.invalid -c init.defaultBranch=main "${@:2}"; }
repo "$N/tracked/node_modules/pkg" init -q
printf 't\n' > "$N/tracked/node_modules/pkg/t.txt"
repo "$N/tracked/node_modules/pkg" add t.txt
repo "$N/tracked/node_modules/pkg" commit -qm tracked
repo "$N/empty/node_modules/pkg" init -q
printf 'u\n' > "$N/empty/node_modules/pkg/u.txt"

D="$T/docker"
mkdir -p "$D/bin" "$D/empty"
printf '#!/bin/sh\nsleep 30\n' > "$D/bin/docker"
chmod +x "$D/bin/docker"
```

- **Links beneath a Root.** Run `"$H" run "$RUN" scan-hostile/link-out -- scan --json "$L/root"`. Exit status `0`. `candidates` has one entry, `$L/root/app/node_modules`, and `unreadable` is empty. `ls "$L/root/escape"` prints `node_modules` and `package.json`, so a Scan that followed the Link would have listed a second Candidate.
- **A Root that is a Link.** Run `"$H" run "$RUN" scan-hostile/link-root -C "$R" -- scan --json alias/rootlink`. Exit status `0`. `roots` is the one path `(cd "$R" && realpath alias/rootlink)` prints, which ends in `deep/target`. `candidates` has one entry, `$R/deep/target/node_modules`, and nothing under `$R/target`.
- **Unreadable, and a Link in an old Project.** Run `"$H" run "$RUN" scan-hostile/stale -- scan --json "$P/locked" "$P/linked" "$P/plain"`. Exit status `1`. `unreadable` has exactly one entry, `$P/locked/secret`. The Candidate `$P/locked/node_modules` has `"stale": false`, because recent work could be in what was not read. `$P/linked/node_modules` and `$P/plain/node_modules` have `"stale": true`: every file in those Projects was set to the start of 2020, and `$P/elsewhere/fresh.js` is new but only behind the Link `jump`.
- **Nested repositories.** Run `"$H" run "$RUN" scan-hostile/nested -- scan --json "$N"`. Exit status `0`. `candidates` has one entry, `$N/empty/node_modules`. `git -C "$N/tracked/node_modules/pkg" --no-optional-locks ls-files` prints `t.txt`, and the same command in `$N/empty/node_modules/pkg` prints nothing.
- **A docker that never answers.** Run `PATH="$D/bin:$PATH" timeout 20 "$H" run "$RUN" scan-hostile/docker-stall -C "$D/empty" -- scan --json .`. The helper returns in about 5 seconds with exit status `0`, not `124` from `timeout`. The step's `status` is `0` and `dockerData` is `{"mark": "docker did not answer"}` with no `size`.
- **Fixture unchanged.** Every step's `fixture-changes.txt` is empty.

## Gotchas

- As root, mode `0` does not refuse a directory, so the unreadable step cannot be reached. Report it as unreachable with that reason.
- Set the old times with `touch -h -t` and a fixed date. `-h` ages the Link `jump` itself, where plain `touch` follows it and ages `elsewhere` instead. `-t` with a fixed date is read the same by GNU and BSD `touch`; a relative date such as `-d '90 days ago'` is GNU only.
- Build `elsewhere/fresh.js` after the `find ... touch` line, or it is aged with the rest and the step proves nothing.
- The stand-in `docker` is a script that leaves a child process holding its output. A stand-in that only `exec`s `sleep` passes even when that harder case is broken.
- Keep the stand-in on the `PATH` for that one step only. Every other step should reach the machine's own `docker`, or none.
- A Scan does not close one case, and no step here covers it: a directory above the one being listed can be swapped for a Link while the Scan runs. A Clean must refuse that.
- `"$H" cleanup "$RUN"` restores the scratch tree's modes before removing it; a plain `rm -rf` fails on `$P/locked/secret`.
