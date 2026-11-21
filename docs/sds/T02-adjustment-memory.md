# T02 — Adapter penambahan stok atomik

Status awal: **planned**, belum diimplementasikan. Sumber status aktual: `docs/sprint/tasks.json`.

## Tujuan dan kontrak

Baca `AGENTS.md`, `docs/architecture.md`, `docs/contracts/next.md`, `internal/contracts/next.go`, dan draft `api/next.openapi.json`. Kontrak baseline harus tetap kompatibel.

## Dependensi

- Mulai implementasi setelah: tidak ada; kontrak sudah tersedia di baseline.
- Integrasi setelah: tidak ada task lain, tetapi checks dan review tetap wajib.
- `start_after` membutuhkan commit implementasi pada base branch. `integrate_after` boleh belum tersedia saat coding terhadap interface/test double, tetapi harus tersedia sebelum klaim integrasi selesai.

## Ownership

- `internal/adapters/memory/adjustments.go`
- `internal/adapters/memory/adjustments_test.go`
- `docs/handoffs/T02.md`

Jangan memperluas scope tanpa koordinasi. File shared, interface, manifest, dan routing hanya diubah pemiliknya.

## Acceptance criteria

1. Tambahkan (*Store).ApplyAdjustment memenuhi contracts.AdjustmentRepository; gunakan mutex dan state transaksi yang sama dengan reservasi order.
2. Gunakan state.adjustments yang sudah disediakan, namespace key terpisah; fingerprint atas SKU, quantity, reason.
3. SKU tidak ada -> ErrNotFound; quantity yang membuat available+reserved melebihi MaxQuantity -> ErrInvalid tanpa mutasi.
4. Key sama payload sama mengembalikan receipt asli; payload beda -> ErrIdempotencyConflict; kegagalan tidak menyimpan key.
5. Uji konkurensi adjustment/reserve/cancel, rollback, replay, dan context cancellation sebelum commit.

## Validasi

- `go test -race ./internal/adapters/memory/... ./internal/orders/...`
- `python3 scripts/sprint.py check-scope T02 --base <base-branch>`
- Catat mana tes dengan fake, adapter asli, atau HTTP live. Tes yang belum dapat dijalankan dilaporkan sebagai pending, bukan pass.

## Penyerahan

Isi `docs/handoffs/T02.md` berdasarkan template; sertakan commit implementasi, perubahan kontrak (jika ada), bukti tes, dependensi belum tersedia, dan target MR. Koordinator saja yang memperbarui manifest.

