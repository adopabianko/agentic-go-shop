---
name: parallel-sprint
description: Coordinate dependent backend tasks in this Go shop repository using its sprint manifest, frozen contracts, isolated Git worktrees, and evidence-backed handoffs. Use for executing or preparing this repository's multi-task sprint, not a single unrelated code edit.
---

# Parallel sprint

Use the project root containing `docs/sprint/tasks.json`. Read [the working agreement](../../../AGENTS.md) and [workflow](../../../docs/workflow.md). Do not install global tooling or publish workspace artifacts merely to run this workflow.

## Decide readiness

Run `python3 scripts/sprint.py validate` and `plan`. Read the SDS only for candidate tasks and their actual dependencies. `start_after` requires implementation commits in the base; `integrate_after` permits coding against frozen interfaces before the real dependency is available. A status in JSON is a declaration to verify against handoff evidence, not a completed test.

The baseline already supplies `internal/contracts/next.go` and `docs/contracts/next.md`. T01/T02/T03 can start together; service/handler use test doubles and the adapter implements the real storage behavior. T07 proves their combination. Apply the same reasoning to T04/T05/T06 and T08.

## Isolate workers

Use `python3 scripts/sprint.py worktree Txx --base <ref>` from a clean coordinator checkout. The helper checks dependency ancestry and refuses to overwrite existing destinations. It does not launch AI. If the runtime supports independently rooted agents and the user's task authorizes parallel execution, assign one worker per returned directory. Otherwise produce brief/folder assignments for separate sessions. Never send concurrent writers into one checkout.

Default to the manifest's max_parallel unless the user requests another capacity. Check overlap in `owns` before assigning tasks; serialize or resolve ownership when files overlap. Distinct ports matter only if workers run servers; each process owns its memory state.

## Coordinate contracts and completion

Provide each worker its SDS, ownership, base, checks, and [handoff format](../../../docs/handoffs/TEMPLATE.md). Use `brief Txx` for a compact assignment. Workers do not update the coordinator manifest. For shared-contract changes, identify affected tasks, record the decision, and synchronize affected workers before dependent implementation continues.

Review code and evidence before setting implemented/integrated. Store a full implementation commit SHA; after squash integration use the corresponding integrated SHA for new dependent branches. Prepare a base containing all code prerequisites rather than assuming one parent branch contains them.

Run scope checks in each worker and relevant combined tests in integration. Finish with `make check` and `make live-check` when runtime behavior changed. Report actual fake vs real-adapter vs live-HTTP evidence and unverified items. Keep task implementation and successful integration distinct.

Make local results reviewable. Push, remote merge, deployment, destructive cleanup, and unrelated repository changes require authorization from the user's task; the skill grants none. Do not retry a failing command without inspecting its failure. Preserve completed work and continue independent tasks when one dependency is blocked.
