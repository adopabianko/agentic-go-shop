# T09 — Wiring fitur dan promosi kontrak

Status awal: **planned**, belum diimplementasikan. Sumber status aktual: `docs/sprint/tasks.json`.

## Tujuan dan kontrak

Baca `AGENTS.md`, `docs/architecture.md`, `docs/contracts/next.md`, `internal/contracts/next.go`, dan draft `api/next.openapi.json`. Kontrak baseline harus tetap kompatibel.

## Dependensi

- Mulai implementasi setelah: T01, T02, T03, T04, T05, T06.
- Integrasi setelah: T07, T08.
- `start_after` membutuhkan commit implementasi pada base branch. `integrate_after` boleh belum tersedia saat coding terhadap interface/test double, tetapi harus tersedia sebelum klaim integrasi selesai.

## Ownership

- `internal/httpapi/handler.go`
- `internal/httpapi/handler_test.go`
- `cmd/api/main.go`
- `api/openapi.json`
- `docs/handoffs/T09.md`

Jangan memperluas scope tanpa koordinasi. File shared, interface, manifest, dan routing hanya diubah pemiliknya.

## Acceptance criteria

1. Hubungkan use case dan adapter baru di composition root; daftarkan kedua handler tanpa mengubah kontrak baseline.
2. Pertahankan httpapi.New yang dipakai tes baseline; tambahkan constructor/options bila dibutuhkan.
3. Promosikan endpoint tambahan dari next.openapi.json ke openapi.json dan tambahkan validasi respons di contract tests.
4. Method dispatch /v1/orders melayani GET dan POST; Allow yang benar untuk metode lain.
5. Baseline journey, original idempotent replay setelah cancel, unknown route, dan semua status sebelumnya tetap lolos.

## Validasi

- `go test -race ./...`
- `go vet ./...`
- `python3 scripts/sprint.py check-scope T09 --base <base-branch>`
- Catat mana tes dengan fake, adapter asli, atau HTTP live. Tes yang belum dapat dijalankan dilaporkan sebagai pending, bukan pass.

## Penyerahan

Isi `docs/handoffs/T09.md` berdasarkan template; sertakan commit implementasi, perubahan kontrak (jika ada), bukti tes, dependensi belum tersedia, dan target MR. Koordinator saja yang memperbarui manifest.

