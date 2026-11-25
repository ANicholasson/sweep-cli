# Version and usage

`sweep --version` prints the version the build recorded and exits 0. Anything sweep does not understand, such as no command, an unknown command or an unknown flag, prints usage on stderr, prints nothing on stdout, and exits 2.

## Sub-features

- `version` prints `sweep <version>` on stdout and exits 0.
- `usage-none` prints the usage and exits 2 when no command is given.
- `usage-unknown-command` prints the usage and exits 2 for a command sweep does not have.
- `usage-unknown-flag` names the flag, prints the scan usage and exits 2.

## How to get to it (user POV)

- Run `sweep --version`.
- Run `sweep`, `sweep recycle`, or `sweep scan --jsno`.

## Driving it with sweep-verify

Preconditions:

- Baseline preconditions from the [index](./README.md) hold.
- `$RUN/evidence/build.txt` gives the commit the binary was built from.

- **Version.** Run `"$H" run "$RUN" version/version -- --version`. Exit status `0`, stderr empty, stdout one line `sweep v0.0.0-<commit time, UTC, as yyyymmddhhmmss>-<first 12 hex digits of the commit>`, with `+dirty` appended when `build.txt` lists uncommitted changes. The 12 hex digits match the `checkout:` line of `build.txt`.
- **No command.** Run `"$H" run "$RUN" version/none --`. Exit status `2`, stdout empty, stderr starts `usage: sweep scan [--json] [Root...]` and lists `sweep --version`.
- **Unknown command.** Run `"$H" run "$RUN" version/unknown -- recycle`. Exit status `2`, stdout empty, stderr is the same usage.
- **Unknown flag.** Run `"$H" run "$RUN" version/bad-flag -- scan --jsno`. Exit status `2`, stdout empty, stderr starts `flag provided but not defined: -jsno` and shows `Usage of sweep scan:`.
- **Unchanged.** Every step's `fixture-changes.txt` is empty.

## Gotchas

- `sweep clean` is a command. The unknown command in this file is `recycle`.
- A binary built with `go run` or outside a git checkout prints `sweep (devel)`; always use the binary `launch` built.
- `--version` is a flag of `sweep` itself, so `sweep scan --version` is an unknown flag of `scan`, not the version.
