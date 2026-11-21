# T10 — Validasi gabungan dan handoff sprint

Status awal: **planned**, belum diimplementasikan. Sumber status aktual: `docs/sprint/tasks.json`.

## Tujuan dan kontrak

Baca `AGENTS.md`, `docs/architecture.md`, `docs/contracts/next.md`, `internal/contracts/next.go`, dan draft `api/next.openapi.json`. Kontrak baseline harus tetap kompatibel.

## Dependensi

- Mulai implementasi setelah: T07, T08, T09.
- Integrasi setelah: T07, T08, T09.
- `start_after` membutuhkan commit implementasi pada base branch. `integrate_after` boleh belum tersedia saat coding terhadap interface/test double, tetapi harus tersedia sebelum klaim integrasi selesai.

## Ownership

- `docs/verification.md`
- `docs/sprint/release.md`
- `scripts/smoke.py`
- `docs/handoffs/T10.md`

Jangan memperluas scope tanpa koordinasi. File shared, interface, manifest, dan routing hanya diubah pemiliknya.

## Acceptance criteria

1. Perluas smoke script dengan adjustment dan list pada instance terisolasi; hindari menambah stok berulang dengan key acak tanpa catatan efek.
2. Jalankan make check dan smoke pada binary terbaru; rekam commit, perintah, exit code, tanggal, lingkungan, dan keterbatasan.
3. Rekonsiliasi status handoff tiap task dan buktikan tidak ada fitur yang hanya lulus dengan fake.
4. Tidak melakukan push, deploy, atau merge otomatis; laporkan readiness dengan bukti.

## Validasi

- `make check`
- `make build`
- `python3 scripts/smoke.py --url http://127.0.0.1:8099`
- `python3 scripts/sprint.py check-scope T10 --base <base-branch>`
- Catat mana tes dengan fake, adapter asli, atau HTTP live. Tes yang belum dapat dijalankan dilaporkan sebagai pending, bukan pass.

## Penyerahan

Isi `docs/handoffs/T10.md` berdasarkan template; sertakan commit implementasi, perubahan kontrak (jika ada), bukti tes, dependensi belum tersedia, dan target MR. Koordinator saja yang memperbarui manifest.

