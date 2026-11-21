# T07 — Integrasi adjustment dan order

Status awal: **planned**, belum diimplementasikan. Sumber status aktual: `docs/sprint/tasks.json`.

## Tujuan dan kontrak

Baca `AGENTS.md`, `docs/architecture.md`, `docs/contracts/next.md`, `internal/contracts/next.go`, dan draft `api/next.openapi.json`. Kontrak baseline harus tetap kompatibel.

## Dependensi

- Mulai implementasi setelah: T01, T02, T03.
- Integrasi setelah: T01, T02, T03.
- `start_after` membutuhkan commit implementasi pada base branch. `integrate_after` boleh belum tersedia saat coding terhadap interface/test double, tetapi harus tersedia sebelum klaim integrasi selesai.

## Ownership

- `tests/adjustments/*`
- `docs/handoffs/T07.md`

Jangan memperluas scope tanpa koordinasi. File shared, interface, manifest, dan routing hanya diubah pemiliknya.

## Acceptance criteria

1. Hubungkan handler adjustment, use case asli, memory adapter asli, dan order service dalam fixture test.
2. Uji create order -> adjust -> cancel; total stock harus sesuai stock awal + total adjustment unik.
3. Uji key bersamaan, payload conflict, batas total saat masih ada reservasi, serta adjustment bersamaan dengan order.
4. Gunakan listener lokal atau recorder; jangan menyatakan fake sebagai bukti integrasi.

## Validasi

- `go test -race ./tests/adjustments/...`
- `python3 scripts/sprint.py check-scope T07 --base <base-branch>`
- Catat mana tes dengan fake, adapter asli, atau HTTP live. Tes yang belum dapat dijalankan dilaporkan sebagai pending, bukan pass.

## Penyerahan

Isi `docs/handoffs/T07.md` berdasarkan template; sertakan commit implementasi, perubahan kontrak (jika ada), bukti tes, dependensi belum tersedia, dan target MR. Koordinator saja yang memperbarui manifest.

