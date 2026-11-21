# Prompt koordinator

Gunakan dari checkout koordinator; sesuaikan task dan jumlah worker jika diperlukan.

```text
Kerjakan sprint latihan pada repository ini sebagai koordinator.

Baca AGENTS.md, docs/workflow.md, docs/contracts/next.md, dan
docs/sprint/tasks.json. Gunakan skill lokal
.agents/skills/parallel-sprint/SKILL.md.

Mulai dari tugas berstatus planned yang siap diimplementasikan.
Maksimal tiga worker aktif. Pastikan setiap worker mempunyai branch,
worktree, port, SDS, dan ownership yang berbeda. Jika alatmu mendukung
peluncuran agent dengan working directory terpisah, delegasikan task;
jika tidak, siapkan brief dan folder untuk sesi AI terpisah.

Jangan menunggu implementation dependency yang hanya perlu tersedia
saat integrasi: gunakan kontrak frozen dan test double untuk coding awal.
Hormati start_after untuk dependency kode yang benar-benar diperlukan.
Jangan mengklaim integrasi hanya dari tes yang memakai fake.

Kamu memiliki manifest, koordinasi kontrak, dan urutan integrasi.
Worker memiliki file yang tercantum pada owns dan handoff-nya.
Tangani pertanyaan kontrak secara terpusat, catat keputusan, dan beri
tahu seluruh worker yang terdampak. Lanjutkan pekerjaan independen.

Review handoff dan kode sebelum memperbarui status. Simpan SHA commit
implementasi dan pastikan base dependent task benar-benar memuatnya.
Jalankan checks gabungan dan live-check sebelum menyatakan sprint selesai.

Buat perubahan dan bukti lokal yang siap direview.
Jangan push, merge ke remote, atau deploy kecuali saya menginstruksikannya.
```
