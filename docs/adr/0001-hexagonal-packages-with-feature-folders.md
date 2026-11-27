# Hexagonal packages with feature folders inside

sweep is one Go module in a ports-and-adapters (hexagonal) shape, with code grouped by feature, not by kind, inside each package tree (decided 2026-11-15).

- `cmd/sweep` is the composition root. It wires the adapters to the use cases by hand, and it is the only package that imports both sides.
- `internal/domain` holds the model in the glossary's words and the use cases, one folder per feature: `scan`, `plan`, `safety`, `clean` and `ecosystems`. It does no IO: it imports only the standard library and itself, and never `os`, `os/exec` or `net`. Each feature declares the ports it needs.
- `internal/system` holds the adapters that implement those ports, named in the domain's words: `files`, `git`, `trash`, `docker` and `config`. `trash` is the only package that removes anything from disk. Each operating system that has a trash has its own file, and the link-safe rename is shared by those systems. `docker` only reads sizes, because Docker data is reported and never cleaned. `config` reads the per-user configuration file and does nothing else.
- `internal/report` prints what a Scan or a Clean found, and `internal/tui` is the interactive screen. The screen turns keys into the next drawing and does no IO. `cmd/sweep` reads the terminal. Both depend on the domain and never on `internal/system`.
- `internal/sweeptest` holds the fakes of the ports and the contract suites every adapter must pass.
- `tools/fixture` builds the Fixture.

Dependencies point inward, toward the domain. [ADR-0003](0003-each-ecosystem-is-one-go-file.md) records how Rules are written inside `ecosystems`.

## Considered Options

- **Packages per technical kind** (`models`, `services`, `helpers`). Rejected: it cuts every feature across several packages, so each change becomes a horizontal slice.
- **A dependency injection container.** Left out: at this size it would be clever rather than obvious. The composition root wires things by hand.

## Consequences

- The gate fails when the domain imports anything outside the standard library and itself, and when `os.Remove`, `os.RemoveAll`, `os.Rename` or `syscall.Renameat` is selected outside `internal/system/trash`.
- The gate fails when a Go file outside `cmd/sweep` imports a package beneath `internal/system`. So `internal/report` and `internal/tui` never import an adapter, and `cmd/sweep` is the only package that imports both the domain and an adapter. It also means an adapter never imports another adapter, and the test of an adapter is written in the adapter's own package, where it hands the adapter to its contract suite in `internal/sweeptest`.
- A Worktree is trashed like any other Candidate, so there is no second removal path through git.
- A new Ecosystem touches only `internal/domain/ecosystems`, so several can be added at once without colliding.
- A new operating system is a new file in `internal/system/trash` that passes the trash contract suite. It uses the shared link-safe rename, and the file for systems with no trash stops claiming that system.
