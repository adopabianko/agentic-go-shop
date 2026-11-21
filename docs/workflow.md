# Workflow sprint dengan agent dan worktree

## Tiga komponen berbeda

- **Kontrak** membuat dependent task dapat dikerjakan sebelum implementasi dependensinya selesai.
- **Worktree** memisahkan file dan staging setiap branch; tidak menjalankan AI, menyinkronkan kode, atau mengisolasi resource runtime.
- **Agent** adalah sesi AI yang membaca SDS dan mengerjakan satu task. Koordinator mengelola hubungan antar-task dan bukti integrasi.

Satu agent per worktree; satu proses server per port. Pada adapter memory, proses server berbeda juga berarti database berbeda. Jangan memakai instance server yang sama untuk smoke paralel.

## Sprint latihan yang disediakan

Semua 10 task mulai berstatus `planned`. Baseline core dan kontrak extension sudah dibuat sehingga T01..T06 langsung siap untuk coding.

| Task | Peran | Mulai setelah kode | Integrasi setelah |
|---|---|---|---|
| T01 | Adjustment use case | Kontrak baseline tersedia | T02 |
| T02 | Adjustment memory adapter | Kontrak baseline tersedia | — |
| T03 | Adjustment HTTP handler | Kontrak baseline tersedia | T01, T02 |
| T04 | Order list use case | Kontrak baseline tersedia | T05 |
| T05 | Order list memory adapter | Kontrak baseline tersedia | — |
| T06 | Order list HTTP handler | Kontrak baseline tersedia | T04, T05 |
| T07 | Adjustment acceptance | T01, T02, T03 | T01, T02, T03 |
| T08 | Pagination acceptance | T04, T05, T06 | T04, T05, T06 |
| T09 | Composition dan routes | T01..T06 | T07, T08 |
| T10 | Bukti release gabungan | T07, T08, T09 | T07, T08, T09 |

Mulai dengan T01/T02/T03 paralel. T01 memakai fake repository, T03 memakai fake use case; T02 menguji adapter nyata. Begitu ada slot kosong, ambil T04/T05/T06. T07 dan T08 menguji gabungan fitur saat kode tersedia. T09 boleh mulai wiring sebelum acceptance selesai, tetapi belum siap diintegrasikan sebelum T07/T08 lulus.

Ini bukan kewajiban menyelesaikan satu gelombang penuh sebelum gelombang lain. `max_parallel=3` adalah rekomendasi kapasitas review; helper tidak menghitung sesi AI aktif.

## Persiapan repository

Project harus menjadi repository Git sendiri dengan commit awal. Helper menolak nested directory di repository lain dan checkout koordinator yang dirty. README memiliki perintah bootstrap. Tidak ada remote yang diperlukan untuk mencoba workflow.

```bash
make plan
make worktree TASK=T01 BASE=main
make worktree TASK=T02 BASE=main
make worktree TASK=T03 BASE=main
```

Helper memvalidasi DAG, SDS, branch dan port unik, lalu membuat branch dari commit base yang di-resolve sekali. Untuk `start_after`, commit implementasi prerequisite harus benar-benar ancestor dari base. Ia tidak otomatis melakukan merge, rebase, cherry-pick, atau push.

## Menjalankan worker

Buka sesi AI pada masing-masing folder yang dicetak helper. Gunakan `docs/prompts/worker.md` atau output `make brief TASK=T01`. Saat ada beberapa agent native dalam satu aplikasi, koordinator tetap harus memastikan cwd/working directory berbeda sebelum worker mulai menulis.

Worker membaca SDS, mengimplementasikan file yang dimiliki, menjalankan checks, lalu menulis handoff. Untuk T01, misalnya:

```bash
go test -race ./internal/adjustments/...
make scope TASK=T01 BASE=main
```

Paket task tersebut baru ada setelah implementasinya dibuat; perintah fokus belum dapat dijalankan pada baseline. Gunakan `make check` untuk memeriksa baseline.

## Status dan bukti

Hanya koordinator memperbarui `docs/sprint/tasks.json`, lalu commit perubahan metadata di checkout koordinator:

- `planned`: belum dikerjakan.
- `in_progress`: ada worker aktif.
- `implemented`: acceptance implementasi dan tes fokus selesai; isi `commit` dengan SHA penuh dari commit kode. Bisa masih memakai test double dan belum terintegrasi.
- `integrated`: implementasi sudah berada pada branch integrasi/utama yang disepakati, prerequisite integrasi terpenuhi, dan pemeriksaan gabungan relevan lulus. Isi `commit` dengan commit yang benar-benar berada pada garis history tersebut; squash bisa menghasilkan SHA baru.

Status adalah metadata yang perlu direview, bukan bukti otomatis. Handoff memuat perintah, exit code, jenis dependensi, pending checks, dan SHA. `make plan` melaporkan metadata; helper worktree juga memverifikasi ancestry commit prerequisite.

Jika worker dibentuk dari base yang lebih tua, salinan manifest di dalam worktree itu mungkin tertinggal. Gunakan manifest checkout koordinator sebagai sumber penjadwalan; worker tidak perlu memperbaruinya sendiri.

## Base branch untuk dependent task

Pilihan termudah: integrasikan prerequisite ke `main`, baru buat branch T07. Jika review belum selesai, koordinator dapat menyiapkan branch integrasi lokal yang berisi commit T01/T02/T03 dan membuat T07 dari sana. Branch integrasi tidak otomatis berarti siap untuk main.

```bash
make worktree TASK=T07 BASE=integration/adjustments
```

Untuk satu induk, stacked branch/MR bisa menargetkan branch task induk. Untuk beberapa induk, gunakan base integrasi eksplisit; jangan menganggap satu branch sudah memiliki seluruh dependency.

Pembaruan branch induk tidak otomatis masuk ke branch worker. Koordinasikan merge atau rebase dalam worktree worker, hentikan penulisan aktif sebelum rewrite history, dan jalankan ulang tes yang terdampak. Setelah parent di-squash, sesuaikan basis agar commit parent lama tidak ikut muncul dalam MR anak.

## Merge dan selesai

Pertahankan MR per task. Adapter dapat masuk lebih dahulu, kemudian service dan handler yang masih belum diroute. T07/T08 membuktikan interaksi nyata; T09 mengaktifkan route dan memperbarui kontrak aktif; T10 mencatat verifikasi keseluruhan.

Sebelum mengklaim selesai:

1. Scope dan acceptance criteria sesuai SDS.
2. `make check` pada gabungan perubahan lulus.
3. `make live-check` pada binary terbaru lulus.
4. Handoff membedakan fake, adapter nyata, HTTP nyata, dan hal yang belum diuji.
5. MR target/dependensi jelas; push/merge mengikuti otorisasi pengguna.

Helper tidak menghapus worktree. Setelah perubahan tersimpan dan tidak ada worker aktif, gunakan `git worktree remove <path>` secara manual. Hindari force removal terhadap pekerjaan yang belum disimpan.
