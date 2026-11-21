# Agent working agreement

## Read in this order

1. `README.md` for delivered scope and runtime limits.
2. `docs/architecture.md` and `docs/adr/0001-transaction-boundary.md` for ownership of transactions.
3. `docs/contracts/baseline.md`; for sprint tasks also `docs/contracts/next.md` and `internal/contracts/next.go`.
4. Your task entry in `docs/sprint/tasks.json` and its SDS. Do not read all SDS files unless coordinating dependencies.

## Invariants

- Order, stock, and successful idempotency record commit in one serializable transaction.
- No overselling; no partial reservation; repeated cancellation cannot add stock twice.
- Same key + canonical payload returns the original creation response, even after cancellation. GET returns current state.
- Invalid/failed requests never consume a key. Item ordering is canonicalized; duplicate SKU is invalid.
- Domain/application packages must not import transport or concrete adapters. `make check` enforces import direction.
- Store callbacks run exactly once. No nested transaction, network I/O, or retaining Tx beyond callback scope.
- Memory is deliberately ephemeral. Do not describe tests against memory as proof of durable storage.

## Parallel work

- Each worker gets a task, branch, and worktree. Never switch the branch of another session's checkout or edit its files.
- The coordinator owns task status/commit metadata, dependency decisions, shared contracts, and integration sequencing.
- `start_after` is a real code dependency. The helper checks that implementation commits are in the selected base. `integrate_after` permits earlier coding with test doubles, but blocks an integration-complete claim.
- Workers edit only `owns` paths plus their own handoff. Check committed, staged, unstaged, and untracked paths with `make scope TASK=Txx BASE=<actual-base>`.
- Scope checks are conventions, not OS permission boundaries. Stop the affected portion and request a coordinated scope/contract update if it needs shared-file changes; continue independent authorized work.
- Do not update the central manifest from every worker; report to the coordinator. A stale manifest copied into an older worktree is not the coordinator's current state.
- Do not launch workers from one shared cwd. If the AI runtime cannot bind independent working directories, use separate sessions.
- No push, merge, remote workspace publication, or deployment follows merely from passing tests; follow the user's actual authorization.

## Commands and evidence

- `make check`: formatting, vet, full race tests including real local HTTP, manifest and helper checks.
- `make build`, `make run`, `make smoke` for a real process journey.
- `make plan`, `make brief TASK=T01`, `make worktree TASK=T01 BASE=main`.
- Focused checks are in each SDS; finish with relevant integration checks after dependencies are available.
- Record exact command, result, commit, fake vs real adapter, and pending checks in `docs/handoffs/Txx.md`.
- Keep the active contract `api/openapi.json` aligned with delivered routes. `api/next.openapi.json` remains a draft until T09 promotes it.
- Do not add dependencies for tasks the Go/Python standard libraries already handle. New dependencies need a concrete technical reason and a committed lock/sum as applicable.
