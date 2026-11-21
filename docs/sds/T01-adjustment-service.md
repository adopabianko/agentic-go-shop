# T01 — Use case penambahan stok

Status awal: **planned**, belum diimplementasikan. Sumber status aktual: `docs/sprint/tasks.json`.

## Tujuan dan kontrak

Baca `AGENTS.md`, `docs/architecture.md`, `docs/contracts/next.md`, `internal/contracts/next.go`, dan draft `api/next.openapi.json`. Kontrak baseline harus tetap kompatibel.

## Dependensi

- Mulai implementasi setelah: tidak ada; kontrak sudah tersedia di baseline.
- Integrasi setelah: T02.
- `start_after` membutuhkan commit implementasi pada base branch. `integrate_after` boleh belum tersedia saat coding terhadap interface/test double, tetapi harus tersedia sebelum klaim integrasi selesai.

## Ownership

- `internal/adjustments/*`
- `docs/handoffs/T01.md`

Jangan memperluas scope tanpa koordinasi. File shared, interface, manifest, dan routing hanya diubah pemiliknya.

## Acceptance criteria

1. Implementasikan New(repository contracts.AdjustmentRepository) dan Adjust sesuai contracts.AdjustmentUseCase.
2. Validasi key, SKU, quantity 1..1_000_000, dan reason 1..200 karakter ASCII printable tanpa mengubah reason.
3. Test double membuktikan input invalid tidak memanggil repository; success/replay/error diteruskan tanpa reservasi tambahan.
4. Tidak membuka transaksi atau melakukan HTTP sendiri; atomisitas dimiliki repository.

## Validasi

- `go test -race ./internal/adjustments/...`
- `python3 scripts/sprint.py check-scope T01 --base <base-branch>`
- Catat mana tes dengan fake, adapter asli, atau HTTP live. Tes yang belum dapat dijalankan dilaporkan sebagai pending, bukan pass.

## Penyerahan

Isi `docs/handoffs/T01.md` berdasarkan template; sertakan commit implementasi, perubahan kontrak (jika ada), bukti tes, dependensi belum tersedia, dan target MR. Koordinator saja yang memperbarui manifest.

