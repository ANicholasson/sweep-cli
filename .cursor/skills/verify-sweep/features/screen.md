# Interactive screen

Plain `sweep`, when standard input and standard output are a terminal, opens a list of Candidates grouped by Project. Each row shows a tick, the Size, the Kind, Last used and the path. Stale Candidates beneath the Roots the user gave start ticked. Shared caches are in their own section and start unticked. Docker data is shown and cannot be ticked. The Reason for the highlighted Candidate is shown. A total of how many Candidates and how much Size are picked updates with every tick. `j` and `k` move, `t` ticks, `u` unticks and `q` quits. Quitting writes no Plan and removes nothing.

## Sub-features

- `list` shows the Fixture's Candidates, the default ticks, Docker data, the Reason, the total and the keys.
- `tick-cache` ticks the npm Shared cache and the total changes.
- `quit` leaves the Fixture as it was.

## How to get to it (user POV)

- From the Fixture, with `HOME` and `USERPROFILE` pointed at `$RUN/scratch/home`, run `sweep` with no arguments in a terminal.

## Driving it with sweep-verify

Preconditions:

- Baseline preconditions from the [index](./README.md) hold.
- `tmux` is on the PATH. The helper's `run` captures pipes, so it takes the no-terminal path and cannot draw this screen. Drive the binary `launch` built, in a tmux session.

- **List.** Start a detached tmux session 160 columns wide and 24 rows tall, working directory `$RUN/fixture`, command `$RUN/bin/sweep` plus the suffix in `$RUN/goexe`, with `HOME` and `USERPROFILE` set to `$RUN/scratch/home`. Capture the pane once it contains `q quit`. It shows a Project heading, a row with `[x]`, a `Shared caches` heading whose row contains `[ ]` and `_cacache`, a `Docker data` line with no `[`, a `Reason` line, a line ending in `picked`, and `j/k move    t tick    u untick    q quit`.
- **Tick the Shared cache.** Send `j` 30 times, then `k`, so the Shared cache row is highlighted and still contains `[ ]`. Capture the pane and keep the total line. Send `t`. Capture the pane again. That row contains `[x]` and the total line differs from the capture before `t`.
- **Quit.** Send `q` with no Enter. The session's command exits 0. `"$H" doctor "$RUN"` still ends with `doctor: worth driving`, so the Fixture is unchanged.

## Gotchas

- Plain `sweep` with a pipe is the no-terminal path in [Version and usage](./version-and-usage.md). It does not draw this list.
- Send `q` with no Enter. Enter is not a key the screen reads as quit.
- The npm cache is the last Candidate. Docker data is the row after it, and `t` there does not change the total.
