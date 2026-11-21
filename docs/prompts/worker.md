# Prompt worker

Ganti TASK_ID dan BASE_BRANCH, lalu jalankan dalam worktree yang ditugaskan.

```text
Kerjakan TASK_ID dalam worktree dan branch yang telah disiapkan.

1. Verifikasi cwd, branch, dan base commit. Baca AGENTS.md dan SDS task
   dari docs/sprint/tasks.json. Gunakan kontrak baseline dan next yang
   sudah disepakati.
2. Implementasikan acceptance criteria hanya pada file milik task.
   Gunakan test double untuk dependency yang belum tersedia; jangan
   membuat implementasi lain dari task tetangga.
3. Jika membutuhkan perubahan shared contract atau file di luar scope,
   laporkan kasus konkret dan task terdampak kepada koordinator.
   Lanjutkan bagian yang tidak terhalang keputusan tersebut.
4. Jalankan checks SDS dan:
   python3 scripts/sprint.py check-scope TASK_ID --base BASE_BRANCH
5. Siapkan perubahan untuk review dan handoff docs/handoffs/TASK_ID.md.
   Catat perintah, exit code, fake vs adapter nyata, serta checks pending.
   Jika diberi otorisasi commit lokal, buat commit terfokus dan laporkan SHA.

Jangan memperbarui manifest koordinator, mengedit worktree lain,
melakukan push/merge, atau menyebut task terintegrasi sebelum ada buktinya.
```
