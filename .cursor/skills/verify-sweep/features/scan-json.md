# Scan as JSON

`sweep scan --json` prints the same Scan as one JSON document for scripts: `version` 1, the `roots`, the `candidates` with exact Size in bytes, and the paths that were `unreadable`. Every list is `[]` when empty, never `null`. A Size equals what `du -s` reports for the path, each hard-linked file counted once.

## Sub-features

- `json-shape` prints one document with `version`, `roots`, `candidates` and `unreadable`, and each Candidate has `path`, `size`, `kind`, `ecosystem` and `reason`.
- `json-key` lists every Candidate in the Answer key exactly once, with the Answer key's Kind.
- `json-size` gives each Candidate's Size as the exact bytes `du -s` gives, with a hard-linked file counted once.
- `json-empty` writes `"candidates": []` and `"unreadable": []` when there are none.
- `json-unchanged` leaves the Fixture exactly as it was.

## How to get to it (user POV)

- Run `sweep scan --json <Root...>`.
- Run `sweep scan <Root...> --json`: the flag may come after the Roots (see [Choosing Roots](./scan-roots.md)).

## Driving it with sweep-verify

Preconditions:

- Baseline preconditions from the [index](./README.md) hold.
- `$RUN/evidence/answer-key.json` is read for the Candidates and Kinds expected.

- **Match.** Scan the Fixture's Root. Run `"$H" run "$RUN" scan-json/match -- scan --json projects`. Exit status `0`, stderr empty, stdout one JSON document with `"version": 1` and `"roots": ["$RUN/fixture/projects"]`.
- **Answer key.** For each Candidate in the Answer key, exactly one entry in `candidates` has `path` equal to `$RUN/fixture/<its path>` and `kind` equal to its Kind. With jq: `jq -r '.candidates[] | "\(.path) \(.kind)"' "$RUN/evidence/scan-json/match/stdout"`. Each entry has a non-empty `ecosystem` (`Node`) and a `reason` ending in `.`.
- **Size.** Measure independently. Run `"$H" du "$RUN" "$RUN/fixture/projects/web-app/node_modules"`. The `size` of that Candidate equals the bytes `du` printed, exactly.
- **Hard link counted once.** `projects/web-app/node_modules/fake-alpha/LICENSE` and `fake-beta/LICENSE` are one file (about 64 KiB) under two names. The exact match with `du`, which counts it once, is the proof; a Size about 64 KiB above `du` means it was counted twice.
- **Empty lists.** Scan a directory with no Candidate. Run `"$H" run "$RUN" scan-json/empty -- scan --json projects/web-app/src`. Exit status `0`, and stdout contains `"candidates": []` and `"unreadable": []`, not `null`.
- **Unchanged.** For every step, `fixture-changes.txt` is empty.

## Gotchas

- `candidates` also holds `projects/tracked-app/node_modules`, a Trap, with Size equal to its `du`. That is correct today: the Git rule acts at Clean. The Answer key's Candidate list is what a Scan must find, not everything it may find.
- Measure with `"$H" du`, not a bare `du -sk` or `du -sb`: the helper asks for 512-byte blocks the way the end-to-end test does, and `-b` gives apparent size, which is not a Size.
- Measure after the Scan, never before launch: the file system may still be placing new blocks right after the Fixture is built. If `du` and `size` disagree, measure again; two equal `du` readings settle it.
- Paths in the JSON are absolute and include the run directory; replace it with `$RUN` before pasting into a pull request.
