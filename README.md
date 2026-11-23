# Multi-Workspace Task & Bug Tracker (MVP v1.0.0)

Aplikasi internal berbasis web untuk mengelola Task dan Bug lintas Workspace secara terisolasi.

## 1. Arsitektur Utama
- **Backend**: Go (chi router, pgx)
- **Frontend**: Angular SPA (dikompilasi via Bun)
- **Database**: PostgreSQL 16
- **Proxy**: Nginx HTTPS Same-Origin

## 2. Cara Menjalankan Produksi
1. Siapkan rahasia lokal di folder `./secrets/`.
2. Jalankan perintah:
   docker compose up -d --build

## 3. Pemulihan Darurat Super Admin
./backend/cmd/bootstrap/bootstrap-admin --recover

## 4. Operational Backup & Restore
./scripts/backup-restore-drill.sh

## 5. Pengujian Otomatis
./scripts/ci-check.sh
./scripts/verify-production.sh
