# A Scan that cannot read everything

When a directory beneath a Root cannot be read, the Scan still lists every Candidate it could reach, reports the unreadable path, and exits with status 1, so a script knows the list may be short and a Size may be low. The table adds a `Not read, so the list may be short and a Size may be low` section; the JSON fills `unreadable`.

## Sub-features

- `unreadable-json` puts the locked path in `unreadable` with an error, keeps the readable Candidate, and exits 1.
- `unreadable-table` adds the `Not read` section naming the locked path, and exits 1.

## How to get to it (user POV)

- Run `sweep scan [--json] <Root>` where some directory beneath the Root denies the user read access.

## Driving it with sweep-verify

Preconditions:

- Baseline preconditions from the [index](./README.md) hold.
- Linux or macOS, as an account that is not root (`id -u` is not `0`).
- The Fixture has no unreadable directory, so build a scratch tree outside it, as `cmd/sweep/scan_test.go` does:

```bash
S="$RUN/scratch/unreadable"
mkdir -p "$S/open/node_modules/a" "$S/locked"
printf '{}\n' > "$S/open/package.json"
printf '{}\n' > "$S/open/node_modules/a/index.js"
printf '{}\n' > "$S/locked/package.json"
chmod 0 "$S/locked"
```

- **JSON.** Run `"$H" run "$RUN" scan-unreadable/json -- scan --json "$S"`. Exit status `1`. `unreadable` has one entry whose `path` is `$S/locked` and whose `error` says `permission denied`. `candidates` has one entry, `$S/open/node_modules`.
- **Table.** Run `"$H" run "$RUN" scan-unreadable/table -- scan "$S"`. Exit status `1`. stdout has the row for `$S/open/node_modules`, then a blank line, one `Docker data` line, then a blank line, `Not read, so the list may be short and a Size may be low`, and `  $S/locked: open $S/locked: permission denied`.
- **Fixture unchanged.** Both steps' `fixture-changes.txt` are empty.

## Gotchas

- As root, or on Windows, mode `0` does not refuse a directory, so this journey cannot be reached. Report it as unreachable with that reason; don't count a Scan that exits `0` as a pass.
- Never lock a directory inside the Fixture: the Fixture is the fixture tool's alone, and the doctor would fail every later step.
- `"$H" cleanup "$RUN"` restores the scratch tree's modes before removing it; a plain `rm -rf` on `$S` fails on the locked directory.
