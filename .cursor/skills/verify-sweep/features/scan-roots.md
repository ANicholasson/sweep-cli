# Choosing Roots

A user tells `sweep scan` where to look by giving Roots. With none, the current directory is the Root. Several Roots, even overlapping ones, list each Candidate once and nothing inside a Candidate, whatever their order. A path outside every Root is never listed. If any Root is missing or is not a directory, nothing is scanned and the exit status is 2.

## Sub-features

- `roots-default` uses the current directory when no Root is given.
- `roots-overlap` lists each Candidate once, and nothing inside one, for overlapping Roots in any order.
- `roots-flag-after` accepts `--json` after the Roots.
- `roots-dashes` treats everything after `--` as a Root.
- `roots-root-rule` never lists the Trap outside the Root.
- `roots-bad` stops before scanning when a Root is missing or is a file, and names it.

## How to get to it (user POV)

- Run `sweep scan` inside a directory.
- Run `sweep scan <Root> [<Root>...]` with relative or absolute paths, with `--json` before or after them, and with `--` before Roots that begin with `-`.

## Driving it with sweep-verify

Preconditions:

- Baseline preconditions from the [index](./README.md) hold.

- **Default Root.** Scan from inside the Fixture's Root with no Root. Run `"$H" run "$RUN" scan-roots/default -C projects -- scan --json`. Exit status `0`, `roots` is `["$RUN/fixture/projects"]`, and `$RUN/fixture/projects/web-app/node_modules` is listed once.
- **Overlap, Project and Root.** Run `"$H" run "$RUN" scan-roots/project-first -- scan --json projects/web-app projects`, then `"$H" run "$RUN" scan-roots/root-first -- scan --json projects projects/web-app`. Each exits `0`, `roots` keeps the given order, and `web-app/node_modules` appears exactly once.
- **Overlap, Candidate as Root.** Run `"$H" run "$RUN" scan-roots/candidate-first -- scan --json projects/web-app/node_modules projects`. Exit status `0`. `web-app/node_modules` is listed once and nothing beneath it is listed: no `fake-alpha/node_modules`.
- **Flag after Roots.** Run `"$H" run "$RUN" scan-roots/flag-after -- scan projects --json`. Exit status `0` and stdout is JSON with `roots` `["$RUN/fixture/projects"]`.
- **Dashes.** Run `"$H" run "$RUN" scan-roots/dashes -- scan --json -- projects`. Exit status `0`, JSON as above. Then run `"$H" run "$RUN" scan-roots/dashes-flag -- scan -- --json`. Exit status `2`, stdout empty, stderr `sweep: --json cannot be a Root: ...`.
- **Root rule Trap.** In the evidence of `scan-roots/default`, no `path` starts with `$RUN/fixture/outside`, although `outside/stray-app/node_modules` sits beside a `package.json` (check with `ls "$RUN/fixture/outside/stray-app"`). No `path` contains `linked-app/through`. That Link leads to `outside/stash-app`, and `outside/stash-app/node_modules` is not listed either.
- **Missing Root.** Run `"$H" run "$RUN" scan-roots/missing -- scan --json projects gone`. Exit status `2`, stdout empty, stderr names `gone` and says `no such file or directory`; the good Root is not scanned either.
- **File as Root.** Run `"$H" run "$RUN" scan-roots/file -- scan --json projects answer-key.json`. Exit status `2`, stdout empty, stderr `sweep: answer-key.json cannot be a Root: it is not a directory`.
- **Unchanged.** Every step's `fixture-changes.txt` is empty.

## Gotchas

- A Candidate given alone as a Root is scanned inside: `scan --json projects/web-app/node_modules` lists `fake-alpha/node_modules` beneath it. Only together with a Root that holds the Candidate is the inner one dropped. Don't mistake the lone case for the overlap bug.
- stderr names a bad Root as typed, so a relative Root stays relative in the message, and the error names the absolute path.
- `-C` resolves against `$RUN/fixture`, so `-C projects` is the Fixture's Root, not the checkout's.
- The `tracked-app` Trap is inside the Root and is not listed. The `outside` Trap and the Link at `projects/linked-app/through` prove the Root rule. The ignored and untracked `node_modules` under `projects/tracked-app` are Candidates, so a Scan of `projects` lists them.
