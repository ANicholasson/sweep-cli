# Scan as a table

`sweep scan` without `--json` prints, for a person, the Roots it looked beneath, then a table with one row per Candidate giving its Size in human units, its Kind, its path and its Reason. A Scan that finds nothing prints `No Candidates.` instead of a table. It changes nothing on disk.

## Sub-features

- `table-rows` lists every Candidate beneath the Root with Size, Kind, path and Reason.
- `table-roots` names each Root under a `Roots` heading before the table.
- `table-empty` prints `No Candidates.` when the Root holds none.
- `table-unchanged` leaves the Fixture exactly as it was.

## How to get to it (user POV)

- Run `sweep scan <Root...>` in a terminal.
- Run `sweep scan` with no Root inside a directory (see [Choosing Roots](./scan-roots.md) for the default Root).

## Driving it with sweep-verify

Preconditions:

- Baseline preconditions from the [index](./README.md) hold.
- `"$H" du "$RUN" "$RUN/fixture/projects/web-app/node_modules"` has given the Candidate's Size in bytes.

- **Rows.** Scan the Fixture's Root. Run `"$H" run "$RUN" scan-table/rows -- scan projects`. Exit status `0`, stderr empty. stdout starts with `Roots`, then `  $RUN/fixture/projects`, a blank line, and the header `SIZE  KIND  PATH  REASON` (columns padded by two or more spaces). One row has path `$RUN/fixture/projects/web-app/node_modules`, Kind `Dependencies`, and a Reason ending in `.`.
- **Size in the row.** Compare the row's Size with `du`. The row's value, such as `92.0 KiB`, is within 5% of one unit of the `du` bytes divided by 1024 (sizes under 1 KiB print as exact bytes, such as `512 B`).
- **Nothing outside the Root.** The table has no row under `$RUN/fixture/outside`.
- **Empty.** Scan a directory with no Candidate. Run `"$H" run "$RUN" scan-table/empty -- scan projects/web-app/src`. Exit status `0`. stdout is `Roots`, the Root, a blank line and `No Candidates.`; there is no `SIZE` header.
- **Unchanged.** For both steps, `fixture-changes.txt` is empty and the helper printed `the Fixture is unchanged`.

## Gotchas

- The table also lists `projects/tracked-app/node_modules`, a Trap. That is correct today: the Git rule acts at Clean, and a Scan does not consult git. Do not count it as a failure, and do not count it as a Candidate the Answer key requires.
- Rows come Root by Root, and within a directory in order of name; don't assert row positions across Roots.
- Human Sizes round to one decimal, so compare against `du` with the tolerance above, and use [Scan as JSON](./scan-json.md) for exact bytes.
- The `Not read, so the list may be short and a Size may be low` section appears only when something is unreadable; drive it through [A Scan that cannot read everything](./scan-unreadable.md).
