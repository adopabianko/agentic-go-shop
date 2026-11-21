# Agentic Go Shop

Contoh backend Go untuk belajar mengerjakan task yang saling bergantung dengan beberapa agent dan Git worktree. Project ini sudah memiliki fitur bisnis yang berjalan, kontrak yang dapat dibaca mesin, tes concurrency, serta sprint latihan 10 task.

**Baseline sudah berjalan; 10 task sprint adalah pekerjaan lanjutan yang sengaja belum diimplementasikan.** Tidak ada agent yang dijalankan otomatis oleh project ini.

## Jalankan dalam 2 terminal

Prasyarat: Go **1.26+**, Python **3.10+**, Git, Make; race detector memerlukan C toolchain. Tidak ada dependency Go eksternal, database, API key, atau akun SaaS.

```bash
# Terminal 1, dari folder project
make check
make run
```

```bash
# Terminal 2
make smoke
```

Default server: `127.0.0.1:8080`. Untuk instance terpisah:

```bash
HTTP_ADDR=127.0.0.1:8101 make run
make smoke URL=http://127.0.0.1:8101
```

Atau jalankan `make live-check` untuk otomatis membangun binary, menjalankan server pada port loopback acak, menjalankan smoke, lalu memverifikasi shutdown. Perintah ini tidak meninggalkan server berjalan.

Seed: `SKU-001` tersedia 10 dan `SKU-002` tersedia 5. Semua state berada di memory dan hilang saat proses berhenti. Smoke membuat satu order lalu membatalkannya; order dan key replay tetap ada sampai restart. Jalankan smoke pada instance demo terisolasi.

## Fitur yang sudah diimplementasikan

| Endpoint | Perilaku |
|---|---|
| `GET /healthz` | Liveness proses |
| `GET /v1/inventory/{sku}` | Stok tersedia dan terreservasi |
| `POST /v1/orders` | Buat order dan reservasi seluruh item secara atomik |
| `GET /v1/orders/{id}` | Status order saat ini |
| `POST /v1/orders/{id}/cancel` | Batalkan dan kembalikan stok tepat satu kali |

```bash
curl -i http://127.0.0.1:8080/v1/orders \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: example-order-001' \
  -d '{"customer_id":"customer-001","items":[{"sku":"SKU-001","quantity":2}]}'
```

Kirim request yang sama dengan key sama untuk memperoleh response awal yang sama tanpa mengurangi stok lagi. Jika payload berbeda, server mengembalikan `409 idempotency_conflict`. Gunakan `id` response untuk GET atau POST cancel; cancel tidak menerima body.

Kontrak aktif: [api/openapi.json](api/openapi.json). Kontrak fitur **belum aktif**: [api/next.openapi.json](api/next.openapi.json). OpenAPI menggunakan JSON agar dapat dibaca tanpa YAML dependency.

## Apa yang membuat project ini mudah dikerjakan agent?

| Artefak | Kegunaan |
|---|---|
| [AGENTS.md](AGENTS.md) | Peta baca, invariant, aturan ownership, perintah verifikasi |
| [Arsitektur](docs/architecture.md) dan [ADR](docs/adr/0001-transaction-boundary.md) | Alasan keputusan dan batas yang tidak boleh ditebak ulang |
| [Kontrak baseline](docs/contracts/baseline.md) dan [kontrak sprint](docs/contracts/next.md) | Perilaku error, transaksi, retry, dan pagination |
| [Interface Go sprint](internal/contracts/next.go) | Agent bisa compile terhadap kontrak sebelum dependensi selesai |
| [Manifest sprint](docs/sprint/tasks.json) dan `docs/sds/` | 10 task dengan dependensi, ownership file, port, dan acceptance criteria |
| [Helper sprint](scripts/sprint.py) | Plan, task brief, worktree terisolasi, cek ancestry dan scope |
| [Skill parallel-sprint](.agents/skills/parallel-sprint/SKILL.md) | Prosedur koordinasi yang dapat dipakai ulang |
| `docs/prompts/` dan `docs/handoffs/` | Prompt koordinator/worker dan format serah-terima bukti |
| `make check` | Format, vet, race tests, contract tests, import boundaries, manifest, helper tests |
| [Template MR](.gitlab/merge_request_templates/Default.md) | Memisahkan bukti fake, integrasi asli, dan pekerjaan pending |

## Coba workflow parallel

1. Baca [panduan workflow](docs/workflow.md) dan jalankan `make plan`.
2. Jika folder belum menjadi repository sendiri, buat commit awal. Gunakan identitas Git kamu sendiri:

   ```bash
   git init -b main
   git add .
   git commit -m "Bootstrap agentic Go shop"
   ```

3. Dari checkout koordinator yang bersih, siapkan tiga worker:

   ```bash
   make worktree TASK=T01 BASE=main
   make worktree TASK=T02 BASE=main
   make worktree TASK=T03 BASE=main
   ```

   Folder dibuat di sibling `agentic-go-shop-worktrees/t01`, `t02`, dan `t03`. Semua task tersebut bisa mulai sekarang: kontrak sudah tersedia walaupun implementasi saling membutuhkan.

4. Buka satu sesi AI per folder. Gunakan `make brief TASK=T01` atau [prompt worker](docs/prompts/worker.md). Brief tidak meluncurkan AI. Pastikan sesi benar-benar membuka worktree yang ditugaskan.
5. Worker menjalankan tes, cek scope, dan membuat handoff. Koordinator mereview bukti, memperbarui manifest, mengintegrasikan dependensi, lalu menguji gabungan hasil.

AI yang mendukung skill dapat diminta membaca `.agents/skills/parallel-sprint/SKILL.md` secara eksplisit; jika aplikasi menemukan skill repository, gunakan `$parallel-sprint`. Ketersediaan fitur peluncuran multi-agent tetap bergantung pada aplikasi kamu. Alur manual dengan beberapa sesi tetap dapat digunakan.

## Batas contoh

- Single process, data ephemeral, tanpa autentikasi/otorisasi. Bind loopback secara default; ini bukan layanan yang siap diekspos ke internet.
- Adapter memory memakai satu mutex dan copy-on-write seluruh state: mudah diuji untuk serializability, bukan desain throughput tinggi. Records/order tidak dieviction.
- Membatalkan context saat menunggu mutex tidak langsung menghentikan penantian; context dicek setelah lock dan sebelum commit. Callback transaksi harus singkat tanpa I/O.
- Tidak ada payment, shipment, harga, atau tenant. Stock tidak pernah "dikonsumsi permanen": baseline hanya reserve/cancel.
- `make check` memerlukan kemampuan membuka port loopback untuk tes HTTP. Ini adalah tes pada adapter memory asli, bukan bukti untuk PostgreSQL.
- Dockerfile dan GitLab CI disediakan; lihat [bukti verifikasi](docs/verification.md) untuk pemeriksaan yang benar-benar dijalankan.

Untuk mengembangkan persistence, pertahankan kontrak `ports.Store` dan jalankan suite `internal/ports/storetest` terhadap adapter baru. Implementasi PostgreSQL perlu desain locking, transaksi, constraint unik key, serta tes database asli; mengganti nama adapter saja tidak cukup.

## Referensi desain

- [Git worktree](https://git-scm.com/docs/git-worktree)
- [GitLab stacked merge requests](https://docs.gitlab.com/user/project/merge_requests/reviews/stacked_merge_requests/)
- [Go HTTP server dan graceful shutdown](https://pkg.go.dev/net/http#Server.Shutdown)
- [OpenAPI 3.1](https://spec.openapis.org/oas/v3.1.0.html)
