# Docker data

A Scan prints one line of Docker data: the space Docker images and build cache hold, marked `reported only`. It is not a Candidate. `--json` carries it in `dockerData`, never in `candidates`. When Docker is missing or not running, the line says so and the Scan still exits zero. Nothing in the line can be picked.

## Sub-features

- `docker-line` prints one `Docker data` line, with a size and `reported only` when Docker answers, or the reason it does not.
- `docker-json` puts that same fact in `dockerData` and not in `candidates`.
- `docker-exit` stays zero when Docker is missing or not running and the Roots could be read.
- `docker-unchanged` leaves the Fixture exactly as it was.

## How to get to it (user POV)

- Run `sweep scan <Root...>`.
- Run `sweep scan --json <Root...>`.

## Driving it with sweep-verify

Preconditions:

- Baseline preconditions from the [index](./README.md) hold.
- `docker system df --format '{{json .}}'` is the independent source for the size. When that command exits non-zero, Docker is missing or not running, and the line must say so. There is no flag to turn the line off.

- **Table.** Run `"$H" run "$RUN" scan-docker/table -- scan projects`. Exit status `0`, stderr empty. stdout has exactly one line that starts with `Docker data`. A blank line separates it from the Candidate rows. When `docker system df` exits zero, the line contains a size and `reported only`. When it does not, the line says docker is missing, docker is not running, or docker did not answer, and it has no size.
- **JSON.** Run `"$H" run "$RUN" scan-docker/json -- scan --json projects`. Exit status `0`. When `docker system df` exits zero, `dockerData.mark` is `reported only` and `dockerData.size` equals the Images total plus the Build Cache total from that command, in bytes. Otherwise `dockerData` has no `size` and `mark` says why. No `candidates` entry has kind `Docker data`.
- **Usage.** Run `"$H" run "$RUN" scan-docker/usage -- scan -h`. Exit status `2`, stdout empty. stderr names `-json` and does not name Docker data or a Pick.
- **Unchanged.** For the table and JSON steps, `fixture-changes.txt` is empty.

## Gotchas

- The size is the Images total plus the Build Cache total, not containers or volumes. `docker system df` prints those totals in decimal units (`1.5kB` is 1500 bytes, `2MB` is 2000000), and the table prints sweep's usual binary units, so compare bytes in the JSON.
- Docker data follows the machine, not the Fixture. Two runs on different machines can both be right and still print different lines.
- The line is information only. `sweep scan -h` offers no flag that includes it in a Pick.
