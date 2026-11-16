# Each Ecosystem is one Go file that adds itself

An Ecosystem is written in Go, as one file in `internal/domain/ecosystems` that holds only an `init` (decided 2026-11-16). The `init` passes a `domain.Ecosystem` literal to `add`, and `ecosystems.All()` returns every Ecosystem that was added, so no list names them. The model lives one package up, in `internal/domain`.

- `Kind` is a struct with one unexported field. Another package can name only the five Kinds in the glossary, or the zero value.
- A `Rule` has a `Kind`, a `Reason` and a `Where`. `Where` is an `InProject{Manifest, Candidate}` or an `InSharedCache{Linux, MacOS, Windows, Split}`, and nothing else compiles. `InSharedCache` holds one slash-separated path inside the home directory per operating system, as [ADR-0002](0002-shared-caches-are-roots-sweep-adds.md) requires.
- `domain.Validate` takes every Ecosystem at once and refuses each break of the model. That includes the breaks that show only across files that are each right on their own: two Ecosystems with one Name in any case, two Rules that find the same Candidate, and a Shared cache at or beneath another. A test in the gate runs it over `ecosystems.All()`.
- A test parses each Ecosystem file and fails one that declares anything beyond its `init`, so two files added side by side cannot declare the same name.

We built both shapes that issue #5 named. Each started with Node dependencies and then added Cargo build output as a trial. These are the files each Ecosystem added:

- Go files. Node added `internal/domain/ecosystems/node.go`. Cargo added `internal/domain/ecosystems/cargo.go`.
- Data files. Node added `internal/domain/ecosystems/rules/node.json` and `internal/domain/ecosystems/node_test.go`. Cargo added `internal/domain/ecosystems/rules/cargo.json` and `internal/domain/ecosystems/cargo_test.go`.

In both shapes no Ecosystem touched a file that another touched. The shared code landed with Node, before the trial. This is `git diff --stat HEAD~1` of the Go trial:

```
 internal/domain/ecosystems/cargo.go | 25 +++++++++++++++++++++++++
 1 file changed, 25 insertions(+)
```

This is `git diff --stat HEAD~1` of the data-file trial:

```
 internal/domain/ecosystems/cargo_test.go    | 34 +++++++++++++++++++++++++++++
 internal/domain/ecosystems/rules/cargo.json | 22 +++++++++++++++++++
 2 files changed, 56 insertions(+)
```

Both trials add only new files, so isolation does not decide between the shapes. The shared code does. The Go shape shares 186 lines, counted with `wc -l internal/domain/kind.go internal/domain/rule.go internal/domain/validate.go internal/domain/ecosystems/ecosystems.go`. The data-file shape shared 404 lines of Go, and 355 of them were the loader and the parser in its `ecosystems.go` and `rule.go`.

We chose Go files because they keep each Ecosystem to one new file with less than half the shared code. The compiler also refuses a sixth Kind. A Worktree and an Ollama model, which data cannot describe, will each be a Rule in a Go file like every other.

## Considered Options

- **Data files, one JSON file per Ecosystem, embedded with `embed`.** Its strongest case is this. A Rules file can only name things, so a reviewer reads twenty lines of JSON and knows that the Rule runs no code and reaches nothing outside a Project or the home directory. The loader is the one boundary, and no other package can set a Rule's fields. The file name is the Ecosystem's Name, so two changes that claim one Name conflict in git rather than in a test. It needs no `init`, because the embedded directory is the registry. Rejected for these reasons:
  - A Rules file can use only the patterns that the shared loader knows. A Rule can say "this directory beside this manifest" or "this path inside the home directory". By our reading of each tool's conventions, about seven of the v1 Ecosystems need another pattern: no manifest at all (`__pycache__`), a marker inside the directory (`.venv`), a manifest glob (`bin` beside `*.csproj`), or a name glob (`bazel-*`). The first Ecosystem to need each pattern edits the loader. So the data files edit shared code for a new pattern just as the Go files do, and keeping Rules to data gains no isolation that the Go files lack.
  - A Worktree needs git state, and an Ollama model is a manifest over blobs that other models share. Neither can be data, so each would need a Rule built in Go, which is a second way to write a Rule.
  - Its errors come from a test, placed by file and Rule number with no line. In the Go files, a misspelled field or a Kind outside the list is a compile error at its line.
  - Its shared code is larger. Besides the 404 lines, it exports seven functions and three accessors on Rule, against `Validate` and `All` here. Some of that, such as `ParseKind` and `Recognise`, is work that a later ticket adds in either shape. The rest exists because only the loader can build a Rule. JSON also has no comments, matches keys without regard to case, and keeps the last of two repeated keys with no error.
- **One Go package per Ecosystem.** Rejected: Go builds a package into sweep only when another package imports it. That needs a list of blank imports that every Ecosystem edits, or a generated file that every Ecosystem regenerates, and both make adding an Ecosystem touch a shared file.
- **`type Kind string` with constants.** Rejected: `Kind: "Logs"` would compile in any package, so only a test could refuse a sixth Kind.

## Consequences

- Adding an Ecosystem adds one file in `internal/domain/ecosystems` and edits none. Issue #11 adds Cargo again as the first test of that claim.
- Go is the configuration language. A contributor writes Go to add an Ecosystem, and only a new build changes a Rule, so a user cannot add a Rule without building sweep.
- The compiler holds only part of the model. An empty Reason, the zero Kind, a missing Where, a location outside the home directory, and every break across Ecosystems all compile, and only `Validate` refuses them. It runs in the gate and not at start-up, so a binary built without the gate could carry a bad Rule.
- The registry is a package-level slice that each `init` appends to. Only package initialisation writes it, and `All` returns a copy.
- An Ecosystem file declares no helper and no constant, so some literals repeat, such as `.npm/_cacache` for Linux and for macOS.
- A new kind of `Where` edits `rule.go` and `validate.go` once. `__pycache__`, `.venv`, `bin` beside `*.csproj`, `bazel-*` and git Worktrees each need one. After that, each Ecosystem that uses it adds one file.
- The data-file prototype is not merged.
