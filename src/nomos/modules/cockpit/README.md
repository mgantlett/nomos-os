# Cockpit Package

The `cockpit` package provides the Open Core REST API server and WebSocket event bus for Nomos.

## Cross-Module Export Architecture

This package exports several symbols specifically for extension by the Sovereign Edition workspace (`github.com/mgantlett/nomos-cockpit`). These exports allow Sovereign to embed the Open Core base routes and WebSocket contexts without duplicating handler logic (DRY architecture).

### Key Exported Symbols

- `NewBaseHandler`: A factory method returning the initialized base HTTP handler with all default routes. The Sovereign `server.go` mounts this as a fallback router to inherit base endpoints.
- `UpdateRepoRoot`: Thread-safe mutator for the WebSocket manager's active workspace context. Used by Sovereign when switching multi-repo projects.
- `GetRepoRoot`: Thread-safe accessor for the current workspace context. Consumed by Sovereign's backlog queue endpoints.

These symbols are intentionally exposed and should not be considered dead code, even though they have zero internal references within the `nomos-commons` repository itself.
