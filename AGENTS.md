# sweep

These are the rules for changing sweep. They hold for everyone who changes it, a person or an agent.

## What sweep is

sweep finds the disk space that developer tools leave behind and reclaims it safely, on Windows, Linux and macOS. [README.md](README.md) has the detail.

## Standing rules

- Keep every Safety rule: the Git rule, the Root rule and the Fingerprint rule. [README.md](README.md#safety-rules) states them and [GLOSSARY.md](GLOSSARY.md) defines them.
- Run a Clean against the Fixture only. A Clean runs against a real directory only once trash by default works on all three operating systems and the formal check of the Fingerprint rule passes.
- Make Fixture data only with the fixture tool. It is synthetic, and none of it comes from a real machine.
- Write in project terms, because the repo is public. Code, docs, commit messages and pull request titles and bodies name no person, AI tool, skill or workflow, and no real machine path, user folder or project name.
- Leave attribution and co-author trailers off commits and pull requests.
- Deliver work as a branch and a pull request, and leave it unmerged. The maintainer merges.

## Code

- Name things with the words in [GLOSSARY.md](GLOSSARY.md). Add a new concept to the glossary before naming code after it.
- Follow the package layout in [ADR-0001](docs/adr/0001-hexagonal-packages-with-feature-folders.md).
- Build deep modules, each a small interface over the work it hides.
- Build what the current change needs and nothing for later.

## Checks

The gate is the one check:

    scripts/verify.sh

It runs on Linux, Windows and macOS for every pull request. [The gate](README.md#the-gate) in the README lists its steps.

A change is done when the gate passes and the pull request shows the evidence.

## Changes

- Keep each pull request to one underlying problem.
- Write commit subjects as Conventional Commits, `type(scope): subject`, such as `feat(gate): fail on Go files gofmt would change`.
- Say in the pull request body what was verified and how.
- When a change alters a pattern documented in this file, in the glossary or in an ADR, update that document in the same change.

## Agent skills

### Issue tracker

Issues and specs live in this repo's GitHub Issues, driven with the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

The five default triage labels: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `GLOSSARY.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.
