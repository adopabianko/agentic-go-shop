# T05 — Adapter pagination order

Status awal: **planned**, belum diimplementasikan. Sumber status aktual: `docs/sprint/tasks.json`.

## Tujuan dan kontrak

Baca `AGENTS.md`, `docs/architecture.md`, `docs/contracts/next.md`, `internal/contracts/next.go`, dan draft `api/next.openapi.json`. Kontrak baseline harus tetap kompatibel.

## Dependensi

- Mulai implementasi setelah: tidak ada; kontrak sudah tersedia di baseline.
- Integrasi setelah: tidak ada task lain, tetapi checks dan review tetap wajib.
- `start_after` membutuhkan commit implementasi pada base branch. `integrate_after` boleh belum tersedia saat coding terhadap interface/test double, tetapi harus tersedia sebelum klaim integrasi selesai.

## Ownership

- `internal/adapters/memory/order_list.go`
- `internal/adapters/memory/order_list_test.go`
- `docs/handoffs/T05.md`

Jangan memperluas scope tanpa koordinasi. File shared, interface, manifest, dan routing hanya diubah pemiliknya.

## Acceptance criteria

1. Tambahkan (*Store).ReadOrders dengan keyset pagination ascending (created_at,id), filter customer_id, limit ternormalisasi.
2. Cursor base64url JSON menyimpan version=1, created_at, id, customer_id; validasi field dan filter binding; malformed -> ErrInvalid.
3. Baca snapshot yang konsisten di bawah mutex; kembalikan clone setiap Order dan items=[] untuk halaman kosong.
4. Cursor terakhir kosong jika tidak ada halaman berikut; insert baru dapat muncul di halaman berikut, tidak ada jaminan snapshot lintas request.
5. Uji timestamp sama, lebih dari satu halaman, filter, cursor invalid, mutasi nilai hasil, dan concurrent create.

## Validasi

- `go test -race ./internal/adapters/memory/...`
- `python3 scripts/sprint.py check-scope T05 --base <base-branch>`
- Catat mana tes dengan fake, adapter asli, atau HTTP live. Tes yang belum dapat dijalankan dilaporkan sebagai pending, bukan pass.

## Penyerahan

Isi `docs/handoffs/T05.md` berdasarkan template; sertakan commit implementasi, perubahan kontrak (jika ada), bukti tes, dependensi belum tersedia, dan target MR. Koordinator saja yang memperbarui manifest.

