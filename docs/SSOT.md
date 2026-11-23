# SINGLE SOURCE OF TRUTH (SSOT)
## Multi-Workspace Task & Bug Tracker

**Version:** 1.0.0  
**Status:** Ready for MVP implementation  
**Date:** 2026-10-23  
**Primary language of product/UI:** Bahasa Indonesia  
**Tech stack:** Go API, Angular + TypeScript frontend, Bun for frontend dependency management/build commands, PostgreSQL database.

> **Normative rules:** `MUST` = wajib, `SHOULD` = dianjurkan, `MAY` = opsional. Semua yang ditandai **MUST** adalah bagian dari Definition of Done (DoD). Dokumen ini menjadi acuan requirement dan behavior. Skema fisik database diwujudkan melalui versioned migrations, sedangkan API harus terdokumentasi di OpenAPI dan konsisten dengan SSOT. Jika berbeda, koreksi implementasi/dokumen melalui perubahan SSOT yang tercatat; jangan diam-diam mengubah kontrak.

---

## 1. Product Vision and Constraints

Aplikasi web internal untuk mencatat, mengelola, dan memantau **Task** dan **Bug** pada beberapa **Workspace**, dengan status pekerjaan, prioritas, PIC (*person in charge*), dan bukti gambar. Targetnya sederhana, cepat, dan dapat di-deploy sendiri.

### Sasaran MVP
1. Login untuk akun yang dibuat administrator; **tidak tersedia public registration**.
2. **Super Admin** tersedia sejak instalasi pertama melalui bootstrap yang aman dan idempoten.
3. User dapat menjadi anggota lebih dari satu workspace; data tiap workspace terisolasi.
4. Task dan bug dapat dibuat, dilihat, diperbarui, dicari, difilter, dan (secara soft-delete) dihapus sesuai hak akses.
5. Satu item dapat memiliki PIC (opsional), status, prioritas, due date (opsional), dan hingga lima gambar.
6. Dashboard ringkas per workspace, daftar item, dan riwayat perubahan (*activity log*).
7. PostgreSQL menjadi source of truth data terstruktur, termasuk session.
8. Arsitektur minimal: satu Go API, satu Angular SPA, satu PostgreSQL, dan private upload directory yang persisten.

### Out of scope v1.0
Public sign-up, email invitation, social login, SSO/OAuth, 2FA, komentar/chat, real-time WebSocket, push notification, kanban drag-and-drop, sprint, milestone, time tracking, estimasi jam, subtask, recurring task, custom statuses, custom fields, multi-PIC, multi-organization billing, laporan XLSX/PDF, cloud-storage/S3 mandatory, Redis, RabbitMQ, event bus, dan full microservices. Jangan mengimplementasikan hal-hal ini secara diam-diam.

### Definisi istilah
- **User:** akun global dengan email unik.
- **Global role:** `SUPER_ADMIN` atau `USER`.
- **Workspace:** ruang isolasi data dan keanggotaan.
- **Workspace role:** `ADMIN`, `MEMBER`, atau `VIEWER`.
- **Work item:** satu entitas dengan tipe `TASK` atau `BUG`.
- **PIC:** satu user aktif yang masih menjadi anggota workspace dari item tersebut.

---

## 2. Architecture and Runtime Responsibility

```text
Browser
  |
  | HTTPS, same origin
  v
Reverse proxy / static frontend server
  |-- /             -> Angular SPA static assets
  `-- /api/v1/*     -> Go REST API (private port)
                           |-- PostgreSQL (users, workspace, items, sessions, metadata)
                           `-- private persistent volume (image binaries)
```

**Backend:** Go, modular monolith, REST/JSON, HTTP router `chi` (atau pilihan setara), driver PostgreSQL `pgx`, SQL eksplisit dengan parameter binding. Migrasi dengan tool standar yang dipilih sekali (`goose` atau `golang-migrate`). Hindari ORM berat hanya untuk CRUD sederhana. Gunakan transaksi untuk operasi lintas tabel.

**Frontend:** Angular standalone components, Angular Router, HttpClient, Signals untuk state UI lokal, Reactive Forms untuk form, CSS modern responsif. Bun untuk `bun install --frozen-lockfile`, `bun run start`, `bun run build`, `bun run test`. Bun bukan backend API kedua dan bukan pengganti Angular. Pastikan versi Angular/TypeScript dan Node/Bun yang dipin kompatibel pada CI; jika sebagian Angular CLI membutuhkan Node, sediakan Node LTS yang didukung di lingkungan build. **Bun digunakan sebagai package manager/runner; hasil akhir frontend adalah aset statis.**

**Database:** PostgreSQL versi stabil yang dipin dalam Docker image, semua timestamp `timestamptz` dalam UTC, seluruh operasi transaksi memakai konteks workspace. **File gambar tidak disimpan dalam PostgreSQL**; hanya metadatanya di DB.

**Deployment:** Docker Compose untuk baseline, satu Go API replica dan upload volume persisten. Reverse proxy menempatkan SPA dan `/api` pada **origin HTTPS yang sama**. Jangan mengklaim high availability pada arsitektur single-replica ini. Jika nanti ingin banyak API replica/HA, pindahkan lampiran ke S3-compatible shared object storage dan rancang ulang failover storage.

### Repo structure

```text
task-bug-tracker/
├── SSOT.md
├── README.md
├── .env.example
├── compose.yaml
├── backend/
│   ├── cmd/api/main.go
│   ├── cmd/bootstrap/main.go
│   ├── internal/config/
│   ├── internal/http/
│   │   ├── handlers/
│   │   └── middleware/
│   ├── internal/auth/
│   ├── internal/users/
│   ├── internal/workspaces/
│   ├── internal/items/
│   ├── internal/attachments/
│   ├── internal/audit/
│   ├── internal/storage/
│   └── internal/db/
│       ├── migrations/
│       └── queries/
├── frontend/
│   ├── bun.lock
│   ├── package.json
│   ├── angular.json
│   └── src/app/
│       ├── core/{auth,api,guards,interceptors,models}/
│       ├── shared/{components,pipes}/
│       └── features/{login,workspaces,dashboard,items,users,settings}/
├── api/openapi.yaml
└── docs/{architecture,security,testing}.md
```

Dilarang menggandakan business logic otorisasi di frontend dan backend; frontend menyembunyikan aksi sesuai role untuk UX, **Go API selalu mengotorisasi ulang**.

---

## 3. Roles, Permissions, and Workspace Boundary

### Global permissions
- **SUPER_ADMIN**: membuat/mengelola semua user dan workspace; akses seluruh workspace tanpa harus menjadi member; dapat memulihkan workspace yang diarsipkan; dapat memberi/mereset password pengguna. Tidak bisa dinonaktifkan oleh user biasa, dan aplikasi MUST mencegah penghilangan satu-satunya Super Admin aktif.
- **USER**: hanya dapat mengakses workspace yang memiliki keanggotaan aktif.
- **Tidak ada `POST /register`** atau menu daftar; endpoint yang tidak ada tidak boleh ditiru oleh frontend.

### Workspace permissions

| Aksi | SUPER_ADMIN | ADMIN | MEMBER | VIEWER |
|---|---:|---:|---:|---:|
| Lihat dashboard, item, aktivitas, gambar | Ya | Ya | Ya | Ya |
| Buat item | Ya | Ya | Ya | Tidak |
| Edit item/status/prioritas/PIC | Ya | Ya | Ya | Tidak |
| Upload/hapus gambar item | Ya | Ya | Ya | Tidak |
| Soft-delete item | Ya | Ya | Tidak | Tidak |
| Tambah/hapus member / ubah role | Ya | Ya | Tidak | Tidak |
| Ubah identitas workspace | Ya | Ya | Tidak | Tidak |
| Buat/arsip/pulihkan workspace | Ya | Tidak | Tidak | Tidak |
| Buat/nonaktifkan/reset password user global | Ya | Tidak | Tidak | Tidak |

**Rules:**
1. Every workspace-scoped endpoint MUST validate session, account active status, workspace existence/not archived for writes, and current workspace membership (unless SUPER_ADMIN).
2. **Jangan pernah memakai `workspace_id` dari body sebagai otoritas**; ambil dari path dan verifikasi semua relasi dalam konteks workspace yang sama. Tenant-sensitive SQL MUST filter `workspace_id`.
3. Item milik workspace A tidak boleh dapat dibaca/diubah dari path workspace B meskipun UUID item diketahui. Kembalikan `404` untuk resource yang tidak ada **atau tidak dapat diakses**, guna mengurangi kebocoran eksistensi.
4. PIC MUST merupakan **user aktif** dengan membership workspace tersebut. PIC boleh `null`. Super Admin tidak otomatis eligible sebagai PIC jika tidak tercatat sebagai member.
5. Saat user dikeluarkan dari workspace, semua item yang ditugaskan kepadanya pada workspace itu MUST di-unassign (`assignee_id = NULL`) dalam transaksi yang sama. Saat user dinonaktifkan, unassign semua item aktif yang menunjuk ke user tersebut.
6. Tidak boleh menghapus/menurunkan **ADMIN terakhir** dalam workspace aktif; semua mutasi membership mengunci baris workspace secara transaksional dan memeriksa kondisi ini. Workspace dibuat dengan pembuat sebagai member `ADMIN`, termasuk jika pembuat Super Admin.
7. Workspace yang diarsipkan dapat dibaca Super Admin, tetapi **read-only untuk semua role** hingga dipulihkan. User biasa tidak dapat mengakses workspace arsip (kecuali kebutuhan read-only diputuskan di versi baru; default: tidak bisa).
8. Mencabut user dari workspace, menonaktifkan user, atau mengubah privilege efektif pada request berikutnya; jangan cache role dalam session tanpa revalidasi DB.

---

## 4. Identity, Login, Session, Bootstrap

### Login-only authentication
- Login dengan **email + password**, email case-insensitive dan dinormalisasi menjadi huruf kecil di server.
- Password minimum 12 karakter, maksimum 128 karakter Unicode, hash memakai **Argon2id** beserta salt unik per akun; parameter minimal sesuai OWASP (`m >= 19 MiB`, `t >= 2`, `p >= 1`) dan benchmark pada host target.
- HTTP-only opaque server-side session: token CSPRNG 32 byte, berupa `{sessionId}.{verifier}`; server menyimpan **SHA-256 dari verifier acak**, bukan verifier mentah; gunakan perbandingan konstan-waktu.
- Session idle expiry 24 jam, absolute expiry 7 hari; tiap aktivitas dapat memperpanjang idle expiry tetapi **tidak** melebihi absolute expiry. Logout mencabut session di DB. Change/reset password membatalkan session lama dan menerbitkan session baru untuk user yang mengganti passwordnya sendiri.
- Production cookie: `__Host-sid`, `Secure`, `HttpOnly`, `SameSite=Lax`, `Path=/`, tanpa `Domain`. Local HTTP dev MAY menggunakan nama cookie berbeda dengan `Secure=false`; jangan pernah menurunkan pengaturan production.
- CSRF: backend mengeluarkan cookie `XSRF-TOKEN` (readable JS), Angular mengirim `X-XSRF-TOKEN` pada mutasi same-origin; server memvalidasi token yang diikat ke session menggunakan HMAC dengan server secret. Terapkan validasi `Origin` (dan fallback aman ketika header tidak ada) untuk semua mutating request. Login (belum punya session) wajib same-origin dan `Content-Type: application/json` dengan Origin check. Tidak boleh hanya mengandalkan SameSite.
- **No auth token in localStorage or sessionStorage**. Akun disabled, session revoked/expired, dan archived workspace harus ditolak oleh server.
- Login errors generik; rate limit percobaan login berdasarkan IP dan identitas tanpa membuka apakah email terdaftar; jangan logging password atau cookie.
- Setelah bootstrap dan setelah reset admin, login memaksa **ganti password** sebelum mengakses fitur selain `/auth/me`, `/auth/change-password`, dan `/auth/logout`.
- Provide forgot-password? **Tidak untuk MVP**. Super Admin dapat mereset user melalui admin panel; prosedur recovery Super Admin dijalankan di CLI pada server dengan kontrol akses operator.

### First-install Super Admin bootstrap
Bootstrap dijalankan **setelah database migration**, sebelum API dinyatakan ready:

```bash
# Example development only; never commit actual secrets
SUPERADMIN_EMAIL=admin@example.invalid
SUPERADMIN_NAME="System Administrator"
SUPERADMIN_PASSWORD_FILE=/run/secrets/superadmin_password

# Run on container/server with access to secrets and database:
./bootstrap-admin
```

Bootstrap MUST:
1. Memerlukan `SUPERADMIN_EMAIL` dan password secret dari **mounted file** (`SUPERADMIN_PASSWORD_FILE`, tidak ada default password hardcoded). Dev boleh memakai env password dengan warning, production MUST memakai secret file atau secret manager.
2. Gagal secara eksplisit bila secret hilang, password lemah, DB/migrations tidak siap, atau tidak memiliki hak DB yang diperlukan.
3. Memakai transaksi dan uniqueness constraint sehingga aman ketika dipanggil berulang/concurrently.
4. Jika email **belum ada**, buat user `SUPER_ADMIN`, `is_active=true`, `must_change_password=true`, dengan Argon2id.
5. Jika email sudah ada dengan role Super Admin, **jangan mengganti password** atau role; cukup log hasil idempotent tanpa secret.
6. Jika email sudah dipakai user biasa, **fail dan minta intervensi operator**, tidak otomatis menaikkan privilege.
7. Memeriksa tersedia minimal satu active SUPER_ADMIN; API readiness gagal jika tidak ada.
8. Menyediakan perintah recovery eksplisit `./bootstrap-admin --recover` yang memerlukan akses console + secret baru, memberi audit record, dan tidak dijalankan otomatis setiap restart.

Bootstrap tidak membuat user/member/workspace dummy di production. **Super Admin dibuat satu kali secara aman, bukan default credential seperti `admin/admin`.**

### Admin user lifecycle
- Super Admin membuat user dengan nama + email, kemudian sistem menghasilkan password sementara yang kuat (20+ karakter) yang ditampilkan **sekali saja** melalui koneksi HTTPS; operator harus menyerahkan secret itu dengan kanal aman.
- User baru `must_change_password=true`; jika disable/reset, seluruh session yang relevan harus dicabut.
- Admin workspace hanya dapat menambahkan **existing active user** berdasarkan email persis, bukan membuat user global. UI tidak membuka daftar seluruh akun global kepada workspace admin.
- Super Admin dapat membuat workspace, lalu menambah akun yang dibutuhkan.

---

## 5. Functional Specification

### 5.1 Workspaces
- Field: `id`, `name` (2–100), `slug` unik (3–50; lowercase alnum dan `-`), `description` (0–500), `created_by`, `created_at`, `updated_at`, `archived_at`.
- User login membuka **workspace switcher** yang menampilkan workspace accessible. Super Admin dapat melihat semua workspace.
- Selection workspace ditaruh pada route (UUID/slug yang tervalidasi). Setelah pindah workspace, semua state dan cache item harus di-reset.
- Dashboard dan daftar item selalu **di-scope pada satu workspace**, tidak digabung lintas workspace secara diam-diam.

### 5.2 Work items (Task/Bug unified entity)

| Field | Type | Required | Default / validation |
|---|---|---|---|
| `id` | UUID | generated | ID internal tidak berubah |
| `workspace_id` | UUID | yes | server derives from URL |
| `type` | `TASK \| BUG` | yes | `TASK` |
| `title` | string | yes | trim, 3–180 karakter |
| `description` | plain text | no | `""`, maksimal 20.000 karakter |
| `status` | enum | yes | `TODO` |
| `priority` | enum | yes | `MEDIUM` |
| `assignee_id` (PIC) | UUID/null | no | null, active member only |
| `due_date` | YYYY-MM-DD/null | no | null; calendar date, no timezone conversion |
| `steps_to_reproduce` | text | no | untuk `BUG` saja |
| `expected_result` | text | no | untuk `BUG` saja |
| `actual_result` | text | no | untuk `BUG` saja |
| `created_by`, `updated_by` | UUID | server | actor yang terautentikasi |
| `created_at`, `updated_at` | timestamptz | server | UTC |
| `version` | integer | server | mulai 1; optimistic locking |
| `deleted_at` | timestamptz/null | server | soft delete |

Enum **status**: `TODO`, `IN_PROGRESS`, `BLOCKED`, `IN_REVIEW`, `DONE`.  
Enum **priority**: `LOW`, `MEDIUM`, `HIGH`, `CRITICAL`.

**Behavior:**
- Semua status boleh ditransisikan ke status lain pada MVP; setiap perubahan tetap diaudit. Jangan menambahkan workflow approval tersembunyi.
- Deskripsi dan detail bug ditampilkan sebagai teks biasa dengan escaping. Jangan merender HTML mentah dari input user.
- Soft delete oleh workspace ADMIN/SUPER_ADMIN; item terhapus tidak muncul pada list, detail, dashboard, atau endpoint attachment normal. Tidak ada restore dari UI MVP, tetapi record dipertahankan untuk audit dan pemulihan operator.
- PATCH item MUST menyertakan `version` saat ini. Server mengupdate dengan `WHERE id=? AND workspace_id=? AND version=? AND deleted_at IS NULL`; bila tidak cocok dan item masih ada, kembalikan `409 VERSION_CONFLICT`, bukan overwrite diam-diam.
- Bila tipe berpindah `BUG` → `TASK`, server MUST membersihkan field spesifik bug (atau menolak transisi dengan pesan eksplisit; default keputusan: bersihkan) dan audit.

### 5.3 Filtering, sorting, pagination
- List default: sort `updated_at desc`, `id desc` untuk hasil stabil.
- Filter: `type`, `status`, `priority`, `assignee_id`, `unassigned=true`, `search` (title dan description), `due_before`, `due_after`.
- Pagination `page` mulai 1, `page_size` default 20 maksimal 100, backend mengembalikan `total` dan metadata halaman. Semua filter harus ikut berlaku pada `COUNT`.
- Pencarian case-insensitive, parameterized SQL; escape wildcard ketika hendak mencocokkan literal input. Minimum 2 karakter untuk search, maksimum 100.

### 5.4 Dashboard
- Summary cards: Total Open (semua kecuali DONE), TODO, IN_PROGRESS, BLOCKED, IN_REVIEW, DONE, Bugs, Tasks, Assigned to Me, dan Overdue (due_date < tanggal lokal terpilih dan belum DONE).
- Dashboard hanya menghitung non-deleted items di satu workspace. Tidak perlu analytics, grafik rumit, atau data warehouse.
- User dapat klik summary card untuk membuka list dengan filter yang sesuai.

### 5.5 Image attachments
- Attachments untuk item TASK maupun BUG, **maksimum 5 gambar per item**, satu POST mengirim satu file.
- Format diterima **JPEG / PNG / WebP** setelah inspeksi bytes nyata dan decode gambar, bukan hanya extension/MIME client.
- Maksimum input 5 MiB per gambar, maksimum dimensi 4096 × 4096 piksel, dan maksimum 16 megapiksel untuk mencegah decompression bomb; reject file rusak, SVG, GIF, PDF, script, polyglot tak valid.
- Server MUST mendekode gambar dan menormalkan/re-encode memakai image processor teruji untuk membuang metadata EXIF/lokasi dan menghilangkan konten tambahan; batasi ukuran hasil simpan dan lakukan sanitasi filename. Jika format input WebP, format output boleh berbeda (mis. PNG/JPEG); MIME dan ekstensi output mengikuti bytes aktual.
- Image original/normalized bytes berada di private persistent volume **di luar public web root**. `storage_key` UUID/random, jangan gunakan nama asli user sebagai path. File disajikan **hanya melalui authenticated and authorized image endpoint**; tidak ada direct static hosting dan tidak ada public access URL.
- Browser `<img>` menggunakan endpoint terlindungi pada same-origin sehingga cookie HttpOnly dikirim secara normal. Set `X-Content-Type-Options: nosniff`, MIME hasil nyata, CSP yang aman, `Cache-Control: private, no-store` untuk konten sensitif.
- Input multipart dibatasi melalui request body limit sebelum parsing. Proses streaming/temporary staging, validasi, re-encode, atomic move, metadata insert; failure MUST membersihkan file sementara. Jika DB insert gagal sesudah file dipindah, compensating cleanup. Penghapusan metadata harus disertai penghapusan file dan retry/garbage collection untuk orphan (task maintenance via Go, tidak memerlukan queue service).
- Saat item soft-delete, attachment tidak boleh bisa diakses melalui API biasa; file boleh dipertahankan sesuai retensi item. Attachment deletion oleh user berhak ditandai deleted lalu dibersihkan asynchronously/synchronous retry-safe.
- Frontend menampilkan preview thumbnail, lightbox/detail view, loading dan error state, progress upload, serta tombol hapus berdasarkan hak akses.

### 5.6 Audit trail
Simpan immutable event log untuk: login success/failure (tanpa secrets), logout, create/update/disable/reset user, create/update/archive workspace, add/remove/role-change membership, create/update/soft-delete item, perubahan status/prioritas/PIC, upload/delete attachment, dan bootstrap/recovery.

Event fields: `id`, `actor_user_id nullable` (system bootstrap), `workspace_id nullable`, `entity_type`, `entity_id nullable`, `action`, `details JSONB` (only safe field diffs), `created_at`, `request_id`. Tidak log password, cookies, secret, atau konten file. Tab **Aktivitas** pada detail item memakai event yang dipilih berdasarkan workspace + entity type + entity id.

---

## 6. PostgreSQL Model and Invariants

**Database migration files are authoritative for DDL.** Required tables and essential columns:

1. `users(id UUID PK, email TEXT, name TEXT, password_hash TEXT, global_role TEXT, is_active BOOLEAN, must_change_password BOOLEAN, created_at, updated_at, last_login_at)`; unique index on `lower(email)`.
2. `workspaces(id UUID PK, name, slug UNIQUE, description, created_by FK users, created_at, updated_at, archived_at)`.
3. `workspace_members(workspace_id FK, user_id FK, role, joined_at, added_by FK users nullable, PK(workspace_id,user_id))`.
4. `work_items(id UUID PK, workspace_id FK, type, title, description, status, priority, assignee_id FK users nullable, created_by FK users, updated_by FK users, due_date DATE, steps_to_reproduce, expected_result, actual_result, version, created_at, updated_at, deleted_at)` and unique `(workspace_id, id)` for composite referencing.
5. `item_attachments(id UUID PK, workspace_id UUID, item_id UUID, uploader_id FK users, original_filename, storage_key UNIQUE, mime_type, byte_size, width, height, created_at, deleted_at)` with **composite FK `(workspace_id,item_id)` → `work_items(workspace_id,id)`**.
6. `sessions(id UUID PK, user_id FK users, verifier_hash BYTEA, created_at, last_seen_at, idle_expires_at, absolute_expires_at, revoked_at, ip, user_agent)`.
7. `audit_events(id BIGSERIAL PK, actor_user_id FK users nullable, workspace_id FK workspaces nullable, entity_type, entity_id UUID nullable, action, details JSONB, request_id, created_at)`.

Additional required constraints:
- `CHECK` enums, `CHECK` lengths/positive byte sizes, `NOT NULL` required fields, consistent timestamps.
- `CREATE INDEX` on `(workspace_id, deleted_at, updated_at DESC, id DESC)`, `(workspace_id, status, deleted_at)`, `(workspace_id, priority, deleted_at)`, `(workspace_id, assignee_id, deleted_at)`, `(workspace_id, type, deleted_at)`, `(workspace_id, due_date, deleted_at)`.
- Index `item_attachments(workspace_id,item_id)` and `audit_events(workspace_id, entity_type, entity_id, created_at DESC)`.
- `sessions(verifier_hash)` MAY be unique; session lookup normally via UUID id, then constant-time verify hash. Index on `sessions(user_id, revoked_at)` for bulk revoke and scheduled cleanup.
- Database constraints plus server transaction MUST prevent cross-workspace attachment links. Cross-workspace PIC checks MUST be performed by application inside same DB transaction as write (user must be active and currently a member).
- Member revocation, last-admin check, unassign operations, item edits with version, and file metadata count limit MUST be transactional. For max-five uploads, obtain a database row lock on parent item **before** counting attachments to prevent parallel overrun.
- `id` is PostgreSQL-generated UUID v4 (`gen_random_uuid()`), unless project explicitly and consistently adopts UUIDv7 later.
- No hard delete cascades for audit-critical records; prefer account disable, workspace archive, item soft delete.
- Set application DB user with least-privilege access; migrations run using separate privileged migration account if practical.

### Example guarded SQL patterns

```sql
-- CRITICAL: never select a tenant item by :item_id alone.
SELECT * FROM work_items
WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL;

-- Version-safe update (illustrative; use parameter binding, no string concatenation).
UPDATE work_items
SET title = $1, updated_by = $2, updated_at = now(), version = version + 1
WHERE id = $3 AND workspace_id = $4 AND version = $5 AND deleted_at IS NULL
RETURNING *;

-- Eligible PIC lookup, executed in the same transaction as item mutation.
SELECT 1
FROM workspace_members wm
JOIN users u ON u.id = wm.user_id
WHERE wm.workspace_id = $1 AND wm.user_id = $2 AND u.is_active = true;
```

---

## 7. REST API Contract (v1)

API prefix `/api/v1`. JSON UTF-8 unless image endpoints. Internal identity from verified session, not user ID supplied by client. Method-level authz enforced uniformly. API documentation MUST be produced in `api/openapi.yaml` with request/response schemas and HTTP error examples.

### Auth

| Method | Path | Purpose | Access |
|---|---|---|---|
| `POST` | `/auth/login` | email/password → session + XSRF cookies | Public, same-origin only |
| `GET` | `/auth/me` | current user, global role, password-change flag | Authenticated |
| `POST` | `/auth/change-password` | current + new password; rotate session | Authenticated |
| `POST` | `/auth/logout` | revoke session, delete cookies | Authenticated |

### Global administration

| Method | Path | Purpose | Access |
|---|---|---|---|
| `GET` | `/admin/users` | pagination/search active users | Super Admin |
| `POST` | `/admin/users` | create user and return temporary password once | Super Admin |
| `PATCH` | `/admin/users/{userId}` | edit display name / activate / disable | Super Admin |
| `POST` | `/admin/users/{userId}/reset-password` | generate temporary password, revoke sessions | Super Admin |
| `GET` | `/workspaces` | accessible workspaces | Authenticated |
| `POST` | `/workspaces` | create workspace (creator becomes ADMIN) | Super Admin |
| `GET` | `/workspaces/{workspaceId}` | workspace details | Accessible member / SA |
| `PATCH` | `/workspaces/{workspaceId}` | edit name, slug, description | Workspace ADMIN / SA |
| `POST` | `/workspaces/{workspaceId}/archive` | archive workspace | Super Admin |
| `POST` | `/workspaces/{workspaceId}/restore` | restore workspace | Super Admin |

### Workspace members and summary

| Method | Path | Purpose | Access |
|---|---|---|---|
| `GET` | `/workspaces/{wid}/members` | members for PIC and membership UI | Accessible member / SA |
| `POST` | `/workspaces/{wid}/members` | add existing user by exact email, role | Workspace ADMIN / SA |
| `PATCH` | `/workspaces/{wid}/members/{userId}` | change role | Workspace ADMIN / SA |
| `DELETE` | `/workspaces/{wid}/members/{userId}` | remove member, unassign work items | Workspace ADMIN / SA |
| `GET` | `/workspaces/{wid}/dashboard` | status/type/priority/assignment summary | Accessible member / SA |

### Work items and attachments

| Method | Path | Purpose | Access |
|---|---|---|---|
| `GET` | `/workspaces/{wid}/items` | filters, pagination, sorting | Accessible member / SA |
| `POST` | `/workspaces/{wid}/items` | create task/bug | ADMIN/MEMBER/SA |
| `GET` | `/workspaces/{wid}/items/{id}` | detail | Accessible member / SA |
| `PATCH` | `/workspaces/{wid}/items/{id}` | edit, requires `version` | ADMIN/MEMBER/SA |
| `DELETE` | `/workspaces/{wid}/items/{id}` | soft-delete | ADMIN/SA |
| `GET` | `/workspaces/{wid}/items/{id}/activities` | activity timeline | Accessible member / SA |
| `POST` | `/workspaces/{wid}/items/{id}/attachments` | multipart image `image` | ADMIN/MEMBER/SA |
| `DELETE` | `/workspaces/{wid}/items/{id}/attachments/{aid}` | delete image metadata / blob cleanup | ADMIN/MEMBER/SA |
| `GET` | `/workspaces/{wid}/items/{id}/attachments/{aid}/content` | authorized image bytes | Accessible member / SA |

### Health endpoints
- `GET /health/live`: process alive, no DB required.
- `GET /health/ready`: PostgreSQL reachable, migrated schema, and at least one active Super Admin exist. Restrict deployment health exposure if sensitive.

### Envelope and errors

```json
// Success example: GET item
{
  "data": {
    "id": "UUID",
    "workspace_id": "UUID",
    "type": "BUG",
    "title": "Tombol login tidak merespons",
    "description": "Terjadi di browser mobile",
    "status": "IN_PROGRESS",
    "priority": "HIGH",
    "assignee_id": null,
    "due_date": null,
    "steps_to_reproduce": "Buka halaman login lalu klik masuk",
    "expected_result": "Redirect ke dashboard",
    "actual_result": "Tidak terjadi navigasi",
    "version": 3,
    "attachments": [],
    "created_at": "2026-10-23T02:00:00Z",
    "updated_at": "2026-10-23T02:05:00Z"
  }
}
```

```json
// Paginated list envelope
{
  "data": [],
  "meta": { "page": 1, "page_size": 20, "total": 0, "total_pages": 0 }
}
```

```json
// Error envelope
{
  "error": {
    "code": "VERSION_CONFLICT",
    "message": "Item telah diperbarui oleh pengguna lain. Muat ulang data.",
    "request_id": "opaque-request-id"
  }
}
```

HTTP response handling: `200` read/update, `201` create, `204` delete/logout where applicable, `400` malformed input, `401` not authenticated, `403` authenticated but forbidden (global-level/admin action), `404` unknown/inaccessible tenant-scoped resource, `409` duplicate/conflict/version mismatch, `413` oversized, `415` unsupported media type, `422` validated field failure, `429` rate limited, `500` server error. Avoid leaking SQL errors/stack traces.

### Representative request payloads

```json
// POST /workspaces/{wid}/items
{
  "type": "TASK",
  "title": "Perbaiki tampilan dashboard mobile",
  "description": "Rapikan sidebar dan navbar",
  "status": "TODO",
  "priority": "HIGH",
  "assignee_id": null,
  "due_date": "2026-10-30"
}
```

```json
// PATCH /workspaces/{wid}/items/{id}
{
  "version": 3,
  "status": "IN_REVIEW",
  "assignee_id": null
}
```

PATCH menggunakan **partial-update semantics**: field yang tidak dikirim tetap, field nullable yang eksplisit `null` mengosongkan field. `version` wajib. Update tidak boleh memperbolehkan client menulis `id`, `workspace_id`, `created_by`, atau timestamps.

---

## 8. UI/UX Information Architecture

### Route map

```text
/login
/change-password
/app/workspaces
/app/w/:workspaceId/dashboard
/app/w/:workspaceId/items
/app/w/:workspaceId/items/new
/app/w/:workspaceId/items/:itemId
/app/w/:workspaceId/members          (workspace ADMIN + SA)
/app/w/:workspaceId/settings         (workspace ADMIN + SA)
/app/admin/users                     (SUPER_ADMIN only)
/app/admin/workspaces                (SUPER_ADMIN only)
/404
```

**Global shell:** sidebar (Dashboard, Semua Item, Task, Bug, Members jika boleh, Workspace Settings jika boleh) yang dapat ditutup menjadi rail ikon 64px di desktop dan menjadi drawer di bawah 1024px. Info akun dan tombol logout berada di sidebar, bukan di topbar. Topbar berisi workspace switcher, chip peran, dan toggle tema.

**Login:** email/password, loading, show/hide password, error generik, tanpa link Register. Setelah login sukses: wajib ganti password bila flag aktif, jika tidak mendarat di workspace picker (`/app/workspaces`). Pemilih workspace yang menentukan workspace, bukan lompatan otomatis ke workspace terakhir, supaya pengguna tidak mendarat di workspace milik orang lain, workspace yang sudah diarsipkan, atau workspace yang tidak bisa diakses akunnya.

**Dashboard:** summary cards dengan skeleton loading, empty state jika workspace belum punya item; CTA tambah item hanya untuk role write.

**Item list:** tabel responsif di desktop dan compact cards di mobile. Kolom: Type, Title, Status, Priority, PIC, Due Date, Last Updated, Actions. Toolbar: search, filter type/status/priority/PIC, sort, reset filter, tombol Tambah Item. Query parameters disimpan di URL agar reload/bookmark stabil. Pagination server-side.

**Create/Edit:** form title, type, description, status, priority, PIC member selector, due date, dan `BUG` detail fields kondisional. Preview/unggah lampiran setelah item pertama kali berhasil dibuat. Validasi inline, optimistic UI **tidak** boleh menyembunyikan server errors. Saat konflik versi, minta reload dan jelaskan bahwa perubahan lain sudah tersimpan.

**Item detail:** metadata, tombol ubah status, assignment PIC, image grid, upload dropzone/file picker, preview gambar, tab Activity, edit, delete (bila authorized). Detail read-only untuk VIEWER. Confirmation dialog untuk delete.

**Admin pages:** daftar user, buat user, nonaktifkan user, reset password; workspace list/create/archive; anggota workspace tambah via email yang sudah ada, ubah role, keluarkan anggota. Temporary passwords ditampilkan sekali, dengan warning agar tidak dimasukkan ke logs/screenshots publik.

**Visual specification:** minimal dan profesional, status dan prioritas memiliki visual badge + label (jangan hanya warna), WCAG-conscious contrast/focus, keyboard access, toast yang tidak menutupi form, loading/empty/error states, no horizontal overflow di mobile. Gunakan Angular route guards untuk UX tetapi backend tetap final authority.

---

## 9. Security, Reliability, and Operations

### Mandatory security checklist
- HTTPS production, HSTS via reverse proxy, same-origin `/api`, production cookies secure.
- Validate session in Go for every protected request; no trusting Angular guards.
- Verify membership/role on **every** tenant endpoint including image fetches and dashboard aggregates.
- CSRF protections for cookie-auth mutating endpoints; strict Origin check; server-generated XSRF token.
- Parameterized SQL; no dynamic SQL string concatenation for user input, whitelisted sort columns.
- Password Argon2id, no plaintext password storage/logging, login throttling.
- Image magic-byte validation, decoding/re-encoding, pixel/file limits, private storage, authorization.
- Apply `Cache-Control: no-store` to auth and sensitive JSON, security headers, and avoid client-injected HTML.
- No default admin password, no seeded production sample data, no credentials in git.
- Rate limiting for login and password changes (single-instance memory limiter acceptable in MVP; document that it is **not** distributed and resets on restart).
- Audit administrative actions without recording secret values.
- Structured JSON application logs with request_id; sanitize error payloads.
- External direct access to PostgreSQL must be disabled in production.

### Operations
- `compose.yaml` defines `postgres`, `api`, `web` (Nginx or equivalent static server/reverse proxy). Use named volumes `postgres_data`, `attachments_data`. `web` is built using Bun + Angular, served statically.
- Startup sequence: PostgreSQL health → migration → bootstrap-admin → API ready → frontend/proxy. Bootstrap should be a one-shot operation, not a browser endpoint.
- Provide `.env.example` with placeholders and **never a usable production password**. Env includes: `DATABASE_URL`, `APP_ENV`, `HTTP_ADDR`, `APP_PUBLIC_URL`, `UPLOAD_DIR`, `UPLOAD_MAX_BYTES`, `CSRF_HMAC_SECRET_FILE`, `SUPERADMIN_EMAIL`, `SUPERADMIN_NAME`, `SUPERADMIN_PASSWORD_FILE`.
- Back up **PostgreSQL and attachments volume together** using documented coordinated/snapshot procedure, plus restore drill; DB-only backup cannot reconstruct attachments. Define retention and secure backup locations in deployment README.
- Apply automatic migration on deployment only if rollback strategy documented; take backup before potentially destructive migrations. Application version and DB schema version should be tracked.
- Readiness/healthcheck, graceful SIGTERM shutdown, DB connection pool sizing, query timeouts, HTTP read/write/body limits.
- Dependency versions pin in `go.mod` + `go.sum`, `bun.lock`, and Docker base images (fixed tag or digest). Use CI checks: go fmt/vet/test, frontend lint/test/build, DB migration and e2e smoke.
- No HA guarantee: local attachments on one node are a deliberate limitation; for multi-node use S3-compatible storage + managed/replicated PostgreSQL, with separate design change.

---

## 10. Testing Matrix and Acceptance Criteria

All tests below MUST exist, preferably with PostgreSQL integration tests and browser e2e for critical paths.

| ID | Test case | Expected result |
|---|---|---|
| AUTH-01 | Fresh DB migrated + bootstrap run | One active Super Admin exists; first login forces password change |
| AUTH-02 | Run bootstrap twice / concurrent | No duplicate; existing Super Admin password unchanged |
| AUTH-03 | Attempt registration URL/API | No registration UI/API; request rejected/404 |
| AUTH-04 | Login invalid password / inactive account | Generic failure; throttling applied |
| AUTH-05 | Logout then reuse old cookie | 401, session revoked server-side |
| AUTH-06 | Session past idle/absolute expiry | 401; no usable session |
| AUTH-07 | Reset user password | Old sessions revoked; temporary password change required |
| SEC-01 | User in workspace A requests B's item via known UUID | 404 and no leaked data |
| SEC-02 | User changes path wid but keeps item id | 404; no cross-tenant modification |
| SEC-03 | Member tries add/edit user globally | 403 |
| SEC-04 | Viewer tries create/edit/upload/delete | 403 |
| SEC-05 | CSRF-less mutation with valid session | Reject mutation |
| WS-01 | User belongs to two workspaces, switches workspace | Only chosen workspace data shown |
| WS-02 | Remove member assigned as PIC | PIC nullified in same workspace; other workspace unchanged |
| WS-03 | Remove/demote last workspace ADMIN | 409, transaction unchanged |
| WS-04 | Assign inactive/non-member user as PIC | 422; item unchanged |
| WS-05 | Workspace archived | Writes blocked; visible only to Super Admin |
| ITEM-01 | Create TASK, BUG, edit fields, change status | Correct persistence and audit events |
| ITEM-02 | Two concurrent clients edit same version | One success; second 409 |
| ITEM-03 | Search/filter/sort/page under workspace | Deterministic, workspace-scoped results |
| ITEM-04 | Soft delete item | Hidden from normal list/dashboard/detail |
| IMG-01 | Upload valid JPEG, PNG, WebP | Stored privately, normalized, accessible to authorized member |
| IMG-02 | Wrong magic bytes/oversized/decompression bomb | Rejected, no orphan file or metadata |
| IMG-03 | Upload six images to one item (including concurrent) | Sixth request rejected; at most five rows/files |
| IMG-04 | Fetch image using wrong workspace or inactive membership | 404; no binary leak |
| IMG-05 | Remove attachment | No longer retrievable; file eventually cleaned |
| DB-01 | Migration on clean database | All schema and indexes built |
| DB-02 | Roll forward with existing data | No data loss, migration idempotency appropriate |
| OPS-01 | Restart containers with persistent volumes | Users, tasks, sessions (until expiry), and images persist |
| OPS-02 | Backup + restore DB and image volume | Data/image integrity verified |
| UX-01 | Desktop/mobile end-to-end login → workspace → create bug → PIC → upload → DONE | Operates without fatal UI/API errors |
| UX-02 | Force expired session/403/409/413/429 | UI shows usable message and recovery path |

### Definition of Done (DoD)
- [ ] Semua MVP flows dapat dijalankan dari browser dan Docker Compose.
- [ ] Tidak ada register endpoint; Super Admin tersedia setelah bootstrap dan tidak memiliki hardcoded password.
- [ ] Semua matrix permissions dan workspace isolation tests lulus.
- [ ] Task/Bug full CRUD sesuai akses, status/priority/PIC, attachment, activity, filter, pagination berfungsi.
- [ ] Session/CSRF/password security serta upload validation lulus test negatif.
- [ ] PostgreSQL migrations berjalan pada DB kosong, dan tersimpan persisten saat restart.
- [ ] OpenAPI lengkap; README berisi setup dev, production, backup/restore, bootstrap/recovery dan troubleshooting.
- [ ] `go test ./...`, frontend build/lint/test, integration tests, e2e smoke lulus di CI.
- [ ] Tidak ada major/critical unresolved security findings yang diketahui.

---

## 11. Prioritized Implementation Sequence

**P0 — Foundation:** repository, Go API skeleton, Angular frontend skeleton, Bun lockfile, PostgreSQL Compose, migrations, health checks, shared error contract, auth/session/CSRF.

**P1 — Identity & isolation:** safe Super Admin bootstrap, forced change password, Super Admin user CRUD, workspace management, workspace membership and permission middleware, tenant-specific integration tests.

**P2 — Core productivity:** task/bug unified CRUD, PIC validation, statuses, priorities, due date, optimistic concurrency, filtering, pagination, dashboard, activity audit.

**P3 — Evidence & UX:** private image upload pipeline, authorization on image reads, image UI preview/delete, responsive polished interface, empty/loading/error states.

**P4 — Hardening & release:** all negative/security tests, CI, Docker Compose production configuration, backups/restore drills, OpenAPI, operator runbook, release smoke test.

**Non-negotiable gate:** Tidak boleh lanjut ke penambahan fitur di luar MVP sebelum P0–P4 dan semua kriteria DoD yang relevan terpenuhi.

---

## 12. Instruction for AI Coding Agent

Gunakan berkas SSOT ini sebagai spesifikasi utama. **Implementasikan seluruh MUST** dan jangan menambah produk di luar scope. Kerjakan bertahap dari P0 hingga P4; tiap tahap harus bisa dibangun dan diuji, bukan hanya pseudocode atau mock UI. Backend memakai Go, database PostgreSQL, frontend Angular, Bun sebagai frontend package manager/build runner. Auth memakai server-side opaque sessions dengan HttpOnly cookies, tidak ada registrasi; bootstrap Super Admin wajib idempotent dan aman, password berasal dari secret file. Semua query dan semua image endpoint harus menegakkan isolasi workspace di server. Sertakan Docker Compose, migrations, OpenAPI, README, unit/integration/e2e tests, dan `.env.example`. Gunakan data dummy hanya pada test fixture/dev (bukan production). Jika ada ketidakjelasan teknis, pilih implementasi paling sederhana yang mempertahankan seluruh security invariants; tulis keputusan di `docs/architecture.md` dan perbarui SSOT bila diperlukan. Laporkan implementasi final berdasarkan DoD dan hasil test yang benar-benar dijalankan; **jangan mengklaim semua fitur selesai sebelum terverifikasi**.

---

## 13. References for Security Choices

- OWASP Password Storage Cheat Sheet: https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html
- OWASP Session Management Cheat Sheet: https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html
- OWASP File Upload Cheat Sheet: https://cheatsheetseries.owasp.org/cheatsheets/File_Upload_Cheat_Sheet.html
- Angular XSRF protection: https://angular.dev/best-practices/security
- Angular CLI local setup (including Bun installation choice): https://angular.dev/tools/cli/setup-local

**End of SSOT v1.0.0.**
