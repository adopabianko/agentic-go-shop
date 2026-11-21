# T06 — Handler HTTP daftar order

Status awal: **planned**, belum diimplementasikan. Sumber status aktual: `docs/sprint/tasks.json`.

## Tujuan dan kontrak

Baca `AGENTS.md`, `docs/architecture.md`, `docs/contracts/next.md`, `internal/contracts/next.go`, dan draft `api/next.openapi.json`. Kontrak baseline harus tetap kompatibel.

## Dependensi

- Mulai implementasi setelah: tidak ada; kontrak sudah tersedia di baseline.
- Integrasi setelah: T04, T05.
- `start_after` membutuhkan commit implementasi pada base branch. `integrate_after` boleh belum tersedia saat coding terhadap interface/test double, tetapi harus tersedia sebelum klaim integrasi selesai.

## Ownership

- `internal/httpapi/order_list.go`
- `internal/httpapi/order_list_test.go`
- `docs/handoffs/T06.md`

Jangan memperluas scope tanpa koordinasi. File shared, interface, manifest, dan routing hanya diubah pemiliknya.

## Acceptance criteria

1. Sediakan OrderListHandler(service contracts.OrderListUseCase, logger *slog.Logger) http.HandlerFunc.
2. GET /v1/orders menerima customer_id, limit, cursor; query tidak dikenal atau duplikat ditolak 400.
3. Limit harus bilangan desimal 1..100 bila diberikan; parameter kosong eksplisit ditolak, parameter absen memakai default use case.
4. Gunakan fake use case untuk tes parse query, page kosong, error mapping, request ID. Wiring dimiliki T09.

## Validasi

- `go test -race ./internal/httpapi/...`
- `python3 scripts/sprint.py check-scope T06 --base <base-branch>`
- Catat mana tes dengan fake, adapter asli, atau HTTP live. Tes yang belum dapat dijalankan dilaporkan sebagai pending, bukan pass.

## Penyerahan

Isi `docs/handoffs/T06.md` berdasarkan template; sertakan commit implementasi, perubahan kontrak (jika ada), bukti tes, dependensi belum tersedia, dan target MR. Koordinator saja yang memperbarui manifest.

