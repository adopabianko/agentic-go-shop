# T04 — Use case daftar order

Status awal: **planned**, belum diimplementasikan. Sumber status aktual: `docs/sprint/tasks.json`.

## Tujuan dan kontrak

Baca `AGENTS.md`, `docs/architecture.md`, `docs/contracts/next.md`, `internal/contracts/next.go`, dan draft `api/next.openapi.json`. Kontrak baseline harus tetap kompatibel.

## Dependensi

- Mulai implementasi setelah: tidak ada; kontrak sudah tersedia di baseline.
- Integrasi setelah: T05.
- `start_after` membutuhkan commit implementasi pada base branch. `integrate_after` boleh belum tersedia saat coding terhadap interface/test double, tetapi harus tersedia sebelum klaim integrasi selesai.

## Ownership

- `internal/orderquery/*`
- `docs/handoffs/T04.md`

Jangan memperluas scope tanpa koordinasi. File shared, interface, manifest, dan routing hanya diubah pemiliknya.

## Acceptance criteria

1. Implementasikan New(reader contracts.OrderReader) dan List sesuai OrderListUseCase.
2. Limit 0 default 50; nilai negatif atau >100 invalid; customer_id kosong berarti semua atau ID valid; cursor <=2048 byte.
3. Jangan mengurutkan atau memfilter kembali halaman adapter; pagination harus terjadi di repository.
4. Test double memverifikasi query ternormalisasi dan propagasi error.

## Validasi

- `go test -race ./internal/orderquery/...`
- `python3 scripts/sprint.py check-scope T04 --base <base-branch>`
- Catat mana tes dengan fake, adapter asli, atau HTTP live. Tes yang belum dapat dijalankan dilaporkan sebagai pending, bukan pass.

## Penyerahan

Isi `docs/handoffs/T04.md` berdasarkan template; sertakan commit implementasi, perubahan kontrak (jika ada), bukti tes, dependensi belum tersedia, dan target MR. Koordinator saja yang memperbarui manifest.

