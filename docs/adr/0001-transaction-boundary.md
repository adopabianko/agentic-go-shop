# ADR 0001 — One transaction for inventory, order, and idempotency

Status: accepted for this reference implementation.

## Context

Several agents will work on stock and order behaviors concurrently. A shared interface alone does not define whether failures leave partial state. The example needs one explicit, testable atomicity guarantee.

## Decision

The order use case owns `ports.Store.WithinTx`. Inventory participates through the provided `ports.Tx` and never opens its own transaction. Stock, order, and the original successful response are committed together. Adapters execute callbacks once and roll back on error, panic, or cancellation observed before commit.

The memory adapter uses serialized copy-on-write state. The test suite in `internal/ports/storetest` is reusable by future adapters; integration tests also validate business invariants under concurrency.

## Consequences

The demo is easy to run offline but is neither durable nor horizontally scalable. Replacing it with SQL requires serializable behavior or equivalent explicit locking and unique constraints. Callback retries must not silently repeat nontransactional effects.

If inventory and orders become separate network services, this ADR no longer supplies atomicity. Create a new design for reservation identity, expiration, compensation, idempotent retries, and transactional event publication. Do not translate the interface into HTTP and assume the same guarantees remain.
