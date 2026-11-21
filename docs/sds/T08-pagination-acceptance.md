# T08 — Integrasi query dan lifecycle order

Status awal: **planned**, belum diimplementasikan. Sumber status aktual: `docs/sprint/tasks.json`.

## Tujuan dan kontrak

Baca `AGENTS.md`, `docs/architecture.md`, `docs/contracts/next.md`, `internal/contracts/next.go`, dan draft `api/next.openapi.json`. Kontrak baseline harus tetap kompatibel.

## Dependensi

- Mulai implementasi setelah: T04, T05, T06.
- Integrasi setelah: T04, T05, T06.
- `start_after` membutuhkan commit implementasi pada base branch. `integrate_after` boleh belum tersedia saat coding terhadap interface/test double, tetapi harus tersedia sebelum klaim integrasi selesai.

## Ownership

- `tests/orderquery/*`
- `docs/handoffs/T08.md`

Jangan memperluas scope tanpa koordinasi. File shared, interface, manifest, dan routing hanya diubah pemiliknya.

## Acceptance criteria

1. Hubungkan HTTP list, use case asli, adapter asli, dan create/cancel order dalam fixture.
2. Uji >1 halaman, customer filter, empty page, status setelah cancel, cursor lintas filter ditolak.
3. Verifikasi tidak ada duplikasi ID antarhalaman tanpa concurrent writes; dokumentasikan semantics saat ada insert baru.
4. Verifikasi schema OrderPage pada draft OpenAPI; tidak bergantung pada urutan map Go.

## Validasi

- `go test -race ./tests/orderquery/...`
- `python3 scripts/sprint.py check-scope T08 --base <base-branch>`
- Catat mana tes dengan fake, adapter asli, atau HTTP live. Tes yang belum dapat dijalankan dilaporkan sebagai pending, bukan pass.

## Penyerahan

Isi `docs/handoffs/T08.md` berdasarkan template; sertakan commit implementasi, perubahan kontrak (jika ada), bukti tes, dependensi belum tersedia, dan target MR. Koordinator saja yang memperbarui manifest.

