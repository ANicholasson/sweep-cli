# Roots are the only boundary

A Scan looks only beneath its Roots, and sweep keeps no built-in list of known cache locations to check besides (decided 2026-11-15). A shared cache, such as a package manager's download cache or a model cache, is found only when a Root contains it. We chose this so the Root rule has no exceptions, and so a user can tell what a Scan will report from the Roots they gave it.

## Considered Options

- **A built-in list of known cache locations, checked on every Scan.** Rejected: sweep would then report and clean paths the user never pointed it at, and the Root rule would need a carve-out for them.

## Consequences

- To see shared caches, the user gives a Root that holds them, such as the home directory. A Scan of a projects directory alone will not show them.
- Docker data lives outside any Root, which is one reason a Clean never acts on it.
