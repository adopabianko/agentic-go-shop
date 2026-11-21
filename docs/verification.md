# Verification evidence

Verified locally on **2026-11-21**, Linux, Go **1.26.11**, Python **3.14**. The generated project was not committed or pushed; this evidence applies to the delivered source snapshot. No remote service or database was used.

## Commands actually run

| Command | Result | Evidence scope |
|---|---|---|
| `make check GOCACHE=/tmp/agentic-go-shop-cache` | PASS, exit 0 in the combined check/live run | gofmt, go vet, uncached race tests, manifest/DAG, import boundaries, Python helper tests |
| `make live-check GOCACHE=/tmp/agentic-go-shop-cache` | PASS, exit 0 in the same run | Rebuilt binary, isolated loopback process, real HTTP journey, graceful SIGTERM exit |
| `make plan` | PASS | 10 planned tasks; T01..T06 ready for implementation, remaining code dependencies reported |
| Skill creator `quick_validate.py .agents/skills/parallel-sprint` | PASS | Skill frontmatter/name validation; not proof of autonomous agent orchestration |

Actual combined command: `make check live-check GOCACHE=/tmp/agentic-go-shop-cache`. `GOCACHE` was overridden only to use a writable shared temporary build cache. The Makefile defaults to a project-local ignored cache.

The first HTTP test attempt was blocked by the execution sandbox's socket restriction. The final run used approved local execution and completed with real loopback sockets. An initial helper test found a null-commit validation bug; it was fixed and the full checks rerun successfully.

## What the Go tests establish

- Shared adapter contract: rollback of stock/order/key writes, defensive copies, canceled transactions, panic rollback/unlock, serializable concurrent updates.
- Order behavior: only one request can reserve the last stock; concurrent requests with the same key return one order; canonical item ordering preserves replay; changed payload conflicts.
- Cancellation: concurrent/repeated cancellation restores stock once; original creation response still replays after cancellation.
- Failure injection: errors after stock mutation roll back reservation, and errors after release roll back both stock and status. Failed keys remain usable.
- HTTP tests use the real memory, inventory, and order implementations behind a local HTTP server. Responses are checked against the schema subset used by the active OpenAPI document.
- Invalid input, unknown fields, trailing JSON, media type, body limits, missing resources, stock shortage, unknown route, and wrong method are exercised.

The HTTP schema checker rejects unsupported keywords rather than silently skipping them. Structural Python checks also resolve references in both OpenAPI documents, check operation IDs and path parameters. These are not a complete independent OpenAPI specification validator.

## What the workflow tests establish

Six Python test methods, including subcases, passed. They cover contract structure and helper behavior in temporary Git repositories: cyclic/missing dependencies, missing SDS, duplicate ports, missing commit evidence, path traversal, clean-checkout requirement, isolated worktree creation, refusal to overwrite, prerequisite commit ancestry, and scope violations including rename sources and untracked files.

No real project branches were created by these tests. Temporary repositories used a clearly labeled test author. Worktree creation does not launch AI and scope checks are not OS isolation.

## What remains deliberately unverified

- The 10 future sprint tasks: all remain planned; their endpoints are not exposed by the baseline.
- Actual multi-agent execution quality, model-specific skill discovery, and how an external AI app binds worker directories.
- Docker image build and GitLab CI execution. Files are provided but have not run on those platforms.
- PostgreSQL, durable persistence, distributed transactions, production load, auth/tenant isolation, external dependencies, deployment, or remote MR creation.

See README for runtime limits, and keep this evidence updated when future task implementations change the source.
