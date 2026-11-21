# T03 — Handler HTTP penambahan stok

Status awal: **planned**, belum diimplementasikan. Sumber status aktual: `docs/sprint/tasks.json`.

## Tujuan dan kontrak

Baca `AGENTS.md`, `docs/architecture.md`, `docs/contracts/next.md`, `internal/contracts/next.go`, dan draft `api/next.openapi.json`. Kontrak baseline harus tetap kompatibel.

## Dependensi

- Mulai implementasi setelah: tidak ada; kontrak sudah tersedia di baseline.
- Integrasi setelah: T01, T02.
- `start_after` membutuhkan commit implementasi pada base branch. `integrate_after` boleh belum tersedia saat coding terhadap interface/test double, tetapi harus tersedia sebelum klaim integrasi selesai.

## Ownership

- `internal/httpapi/adjustments.go`
- `internal/httpapi/adjustments_test.go`
- `docs/handoffs/T03.md`

Jangan memperluas scope tanpa koordinasi. File shared, interface, manifest, dan routing hanya diubah pemiliknya.

## Acceptance criteria

1. Sediakan AdjustmentHandler(service contracts.AdjustmentUseCase, logger *slog.Logger) http.HandlerFunc untuk didaftarkan oleh T09.
2. Ikuti draft OpenAPI: POST /v1/inventory/{sku}/adjustments, body quantity/reason, key wajib, 201 untuk create/replay.
3. Gunakan error envelope, strict JSON, batas body, dan media type baseline; handler berjalan di middleware request ID yang sama.
4. Test menggunakan fake use case; buktikan path SKU yang dikirim dan mapping error. Jangan ubah handler.go atau cmd/api.

## Validasi

- `go test -race ./internal/httpapi/...`
- `python3 scripts/sprint.py check-scope T03 --base <base-branch>`
- Catat mana tes dengan fake, adapter asli, atau HTTP live. Tes yang belum dapat dijalankan dilaporkan sebagai pending, bukan pass.

## Penyerahan

Isi `docs/handoffs/T03.md` berdasarkan template; sertakan commit implementasi, perubahan kontrak (jika ada), bukti tes, dependensi belum tersedia, dan target MR. Koordinator saja yang memperbarui manifest.

