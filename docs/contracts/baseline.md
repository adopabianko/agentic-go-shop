# Baseline behavior contract

Authoritative HTTP shape: `api/openapi.json`. Authoritative transaction interface: `internal/ports/store.go`. This document describes decisions that a signature cannot express.

## Data and transaction invariants

- Every item has a unique, case-sensitive SKU and integer quantity 1..1,000,000. Orders contain 1..100 items.
- Identifiers: ASCII letter/digit first, then letters/digits/`.`/`_`/`-`, maximum 64 characters.
- Seed inventory total is at most 1,000,000 per SKU. `available >= 0`, `reserved >= 0`; reserve/cancel preserve their sum.
- Status transition: `reserved -> cancelled`. Cancelled is terminal in the baseline.
- Reserve all items or none. Unknown SKU returns 404; insufficient quantity returns 409. When multiple items fail, the first failing SKU in canonical sorted order determines the error.
- Cancellation runs with stock release in the same transaction. Repeated cancellation preserves status, stock, and updated_at.
- Return values are isolated copies; callers cannot mutate stored order items.

## Idempotency

- `POST /v1/orders` requires exactly one `Idempotency-Key`: 8..128 ASCII characters following the documented pattern.
- Keys are scoped to order creation in one running demo process, not per customer. There is no authentication/tenant boundary in this example.
- Sort items before fingerprinting; reordered identical items are the same request. Customer and every quantity are part of the fingerprint.
- Persist successful records only, atomically with the order and reservation.
- A replay returns the **original 201 body**, Location, and `Idempotency-Replayed: true`; original request uses `false`.
- GET order shows the latest status. After cancellation, replaying create still shows the original `reserved` response. That is intentional response replay, not a new reservation.
- Same key with different canonical content returns 409. Failed requests can retry with the key; there is no TTL until restart.

## Transport

- JSON body limit: 16 KiB. Unknown struct fields and trailing JSON values rejected; duplicate JSON keys use Go decoder last-value-wins semantics, so callers must not send them. Go's decoder matches field names case-insensitively; clients should use the exact schema names.
- Content-Type for create must be `application/json`, optionally with parameters. Cancel accepts no body.
- Missing/invalid fields: 400; missing resource: 404; conflict/stock shortage: 409; body too large: 413; wrong media: 415; internal failure: 500; interrupted operation: 503.
- Errors: `{ "error": { "code", "message", "request_id" } }`; request ID is server-generated and also appears in `X-Request-ID`.
- Unsupported methods return 405 and Allow; unknown routes return 404. GET-only endpoints in this example reject HEAD with 405.
- No automatic server retry. Retry an interrupted create with the same key to resolve an uncertain outcome.

## Timing limits

WithinTx checks cancellation before lock, after lock, and before commit. It does not promise cancellation interrupts mutex acquisition. Keep callbacks short and never perform network I/O inside them. Cancellation after commit cannot undo an already completed operation.
