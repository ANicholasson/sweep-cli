# sweep

Finds the disk space that developer tools leave behind and reclaims it safely.

## Language

### What sweep finds

**Candidate**:
A single directory or file that sweep found and believes is safe to reclaim. It is always taken as a whole, so nothing inside a Candidate is a Candidate of its own.
_Avoid_: hit, item, junk, finding, target

**Root**:
A directory a Scan looks beneath: one the user points sweep at, or a Shared cache sweep adds itself. A Scan looks at nothing else, and a Clean never touches a path outside the Roots.
_Avoid_: scan root, base, target directory

**Shared cache**:
A cache an Ecosystem keeps once per user for every Project, such as a download cache or a model cache. sweep finds it at its known location and never picks it for a Plan unless the user does.
_Avoid_: global cache, system cache, well-known location

**Kind**:
What sort of leftover a Candidate is: Dependencies, Build output, Cache, Models or Worktree. Every Rule names one.
_Avoid_: type, category

**Worktree**:
A linked git worktree, never a repository's main one. It is a Candidate only when nothing in it is uncommitted or untracked and its commits exist on a branch or remote.
_Avoid_: checkout, working copy

**Link**:
Anything that makes a path lead somewhere else, such as a symlink or a Windows junction.
_Avoid_: symlink (when junctions are also meant), shortcut

**Size**:
The disk space a Candidate occupies on its own, with each hard-linked file counted once. It can be more than a Clean gets back when the same files are also held elsewhere.
_Avoid_: weight, footprint

**Ecosystem**:
A toolchain whose leftovers sweep knows, such as Node or Cargo.
_Avoid_: language, platform, plugin

**Rule**:
One pattern in an Ecosystem that recognises a Candidate.
_Avoid_: detector, matcher, plugin

**Reason**:
The sentence a Rule gives for why its Candidates are safe to reclaim.
_Avoid_: justification, explanation

**Project**:
A directory a developer works in, recognised by its Ecosystem's manifest, such as `package.json`.
_Avoid_: repo, workspace, package

**Last used**:
When a Candidate's Project was last worked on: the newest modification among the Project's own files, outside its Candidates. A Candidate with no Project takes the newest modification inside it.
_Avoid_: last accessed, age

**Stale**:
Said of a Candidate whose Last used is older than the threshold the user set. Staleness decides what is picked by default, never what is found.
_Avoid_: old, unused, dead

**Docker data**:
Space held by Docker images and build cache. A Scan reports it, but it is not a Candidate and a Clean never acts on it.

### What sweep does

**Scan**:
Looking for Candidates. A Scan changes nothing.
_Avoid_: search, analyse, sweep (as a verb)

**Ignore**:
Leaving a path out of every Scan because the user said so. Unlike a Skip, it is the user's choice, made before anything is found.
_Avoid_: exclude, hide

**Pick**:
Choosing a Candidate for a Plan. sweep picks every Stale Candidate beneath the Roots the user gave, and the user can pick or unpick any Candidate.
_Avoid_: select, mark, include

**Clean**:
Carrying out a Plan.
_Avoid_: purge, prune, wipe, sweep (as a verb)

**Trash**:
Moving a Candidate to the operating system's trash, where it can be restored.
_Avoid_: remove, recycle, soft delete

**Delete**:
Removing a Candidate permanently. Only on request.
_Avoid_: remove, purge, hard delete

**Reclaimed**:
Said of space that is free again. Trashed space is not reclaimed until the trash is emptied, which sweep never does.
_Avoid_: freed (for trashed space), saved

### Cleaning safely

**Safety rule**:
One of the three rules a Clean can never break and no setting can switch off: the Git rule, the Root rule and the Fingerprint rule.
_Avoid_: check, guard, policy

**Git rule**:
sweep never touches a file git tracks, or a Worktree with uncommitted changes.

**Root rule**:
sweep never follows a Link out of a Root and never touches a path outside the Roots.

**Fingerprint rule**:
A Candidate whose Fingerprint no longer matches the Plan is a Skip.

**Plan**:
The exact list of Candidates one Clean will act on, written before anything is touched.
_Avoid_: manifest, selection, queue

**Fingerprint**:
What sweep recorded about a Candidate when the Plan was written, compared again just before the Candidate is trashed or deleted.
_Avoid_: snapshot, checksum, hash

**Skip**:
Leaving a Candidate untouched during a Clean because a Safety rule said so, reported with that rule.
_Avoid_: failure, error

### Proving it works

**Fixture**:
The fake developer machine the fixture tool builds, holding known Candidates and Traps.
_Avoid_: test data, sandbox

**Trap**:
A path in the Fixture that looks like a Candidate but must survive a Clean.
_Avoid_: decoy, negative case
