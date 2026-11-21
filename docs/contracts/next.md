# Frozen contracts for the practice sprint

These decisions are ready for parallel implementation. The two new endpoints are **not active in the baseline**. Machine-readable HTTP draft: `api/next.openapi.json`; Go ports: `internal/contracts/next.go`.

## Positive inventory adjustment

`POST /v1/inventory/{sku}/adjustments` accepts `{quantity, reason}` and one Idempotency-Key. Quantity is integer 1..1,000,000; reason contains 1..200 printable ASCII characters (space is allowed), preserved exactly without trimming. No negative adjustments or creation of unknown SKUs.

- `AdjustmentUseCase.Adjust` validates input and calls `AdjustmentRepository.ApplyAdjustment` once.
- Repository atomically checks the key, checks SKU/total, adds available quantity, and records the receipt under the same serialization used by orders.
- Total `available + reserved + quantity` must remain <=1,000,000; violation is ErrInvalid/400. Unknown SKU is ErrNotFound/404.
- Key namespace is separate from order creation. Fingerprint includes SKU, quantity, and exact reason. Same payload replays the original receipt; different payload conflicts.
- Receipt: `id=adj_<32 lowercase hex>`, SKU, quantity, reason, UTC created_at. 201 for new/replay, `Idempotency-Replayed` header, no Location (there is no receipt retrieval endpoint).
- Reuse baseline error envelope, key validation, body size, media rules, and interruption semantics.
- Memory's `state.adjustments` ledger is predeclared in the foundation. T02 adds methods in its own files; no package-global or per-instance side map with different locking.

Expected constructor for T01: `adjustments.New(repository contracts.AdjustmentRepository)` returning a value implementing AdjustmentUseCase. T03 exports `httpapi.AdjustmentHandler(service contracts.AdjustmentUseCase, logger *slog.Logger) http.HandlerFunc`, to be wrapped in the normal middleware and routed by T09.

## Order listing

`GET /v1/orders?customer_id=...&limit=...&cursor=...` returns `{items: [...], next_cursor: "..."}`. No total count. Empty list is `[]`, never null; last page has empty next_cursor.

- Only the three declared parameters are accepted; reject duplicates and explicit empty values. Missing customer_id means all customers in this demo.
- `OrderListUseCase.List` normalizes limit=0 to 50; valid non-default limits are 1..100. HTTP explicit limit=0 is invalid. Missing/invalid cursor decoding belongs to the adapter; input length <=2048 bytes.
- `OrderReader.ReadOrders` receives a normalized query; filter by customer_id, sort ascending by `(created_at, id)`, then fetch limit+1 to determine whether another page exists.
- Cursor is base64url without padding of JSON `{version:1, created_at:<RFC3339Nano>, id:<order-id>, customer_id:<filter-or-empty>}`. Strictly validate the fields, version, timestamp, ID, and equality of filter. Malformed cursor is ErrInvalid/400, not a server error.
- Cursor is opaque to clients, but not a signed authorization token. The demo has no tenant isolation.
- Each page is a consistent snapshot; there is no snapshot guarantee across requests. New orders after the cursor may appear; cancellation may change status between pages. Ordering by immutable creation time + ID avoids unstable map iteration.
- Return clones of order items. Read under the same mutex used for writes; no direct unsynchronized iteration.

Expected constructor for T04: `orderquery.New(reader contracts.OrderReader)` returning OrderListUseCase. T06 exports `httpapi.OrderListHandler(service contracts.OrderListUseCase, logger *slog.Logger) http.HandlerFunc`.

## Contract changes

The coordinator owns these contracts. If implementation reveals an inconsistency, describe the concrete case and affected tasks, update contracts together, then notify affected workers. Do not let one worker silently reinterpret idempotency, transaction scope, or cursor semantics.
