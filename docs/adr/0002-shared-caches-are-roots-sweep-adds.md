# Shared caches are Roots that sweep adds itself

A Scan looks beneath the Roots the user gave it and beneath the Shared caches sweep knows for each Ecosystem, which it adds as Roots of its own (decided 2026-11-15). A Shared cache is found at its known location inside the user's home directory, is listed apart from the other Candidates, and is never picked for a Plan unless the user picks it. One setting turns the added Roots off.

We chose this because Shared caches are often the largest leftovers on a developer machine, and because a known location identifies one more reliably than its shape does.

## Considered Options

- **Only the Roots the user gave.** Rejected: a Scan of a projects directory would miss the largest leftovers, and a Rule would have to recognise a Shared cache by its shape wherever one appears, such as inside a backup of an old home directory.
- **Shared caches picked by default when Stale.** Rejected: Last used comes from modification times, so a cache that is read every day but never rewritten, such as a model, would look Stale.

## Consequences

- The Root rule keeps one meaning with no exceptions: every path sweep touches is beneath a Root, and the Roots sweep added are shown in the Scan's output and written in the Plan.
- Each Ecosystem carries its Shared cache locations per operating system, and the Fixture includes a home directory that holds them.
- A location outside the home directory is never added, which is one reason Docker data is reported and never cleaned.
