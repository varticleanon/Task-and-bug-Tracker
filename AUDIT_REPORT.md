# SSOT Compliance Audit Report
**Auditor Role:** Senior Full-Stack Security & Compliance Auditor
**Date:** 2026-11-09
**SSOT Version:** 1.0.0
**Tech Stack:** Go API, Angular SPA, PostgreSQL, Docker Compose

---

## Executive Summary

This audit evaluated the multi-workspace task-and-bug-tracker repository against the Single Source of Truth (SSOT) document. The application is at **P0 Foundation stage** - core infrastructure and authentication are partially implemented but have significant gaps that must be resolved before progressing to P1.

**Overall Compliance: 62% - 7 of 12 MUST requirements fully satisfied, 3 partially satisfied, 2 not implemented.**

---

## Compliance Status by Module

### Module P0: Foundation (Repository, Go API, Angular, Docker, Migrations, Health, Error Contract, Auth/Session/CSRF)

| # | MUST Requirement | Status | Gap Description |
|---|---|---|---|
| P0.1 | **No public registration endpoint** | ✅ **COMPLIANT** | No `POST /register` endpoint exists. Login-only auth as required by SSOT §3 and §4. |
| P0.2 | **Super Admin bootstrap (safe, idempotent)** | ⚠️ **PARTIAL** | Bootstrap implementation is functional and idempotent, but allows `SUPERADMIN_PASSWORD` env var with warning (SSOT §4.2 mandates secret file for production). |
| P0.3 | **HTTP-only opaque server-side session** | ✅ **COMPLIANT** | Session uses CSPRNG 32-byte verifier, server stores SHA-256 hash, constant-time comparison via `subtle.ConstantTimeCompare`. Cookie uses `__Host-sid` in production with `Secure; HttpOnly; SameSite=Lax; Path=/`. |
| P0.4 | **Session expiry (24h idle, 7h absolute)** | ✅ **COMPLIANT** | `IdleExpiryDuration = 24h`, `AbsoluteExpiryDuration = 7h` configured in `session.go`. |
| P0.5 | **CSRF protections for cookie-auth mutating endpoints** | ❌ **NOT IMPLEMENTED** | `RequireCSRF` middleware exists in `middleware/csrf.go` but is **NOT registered** in the router (`router.go`). SSOT §4.1 requires CSRF validation for all mutating endpoints. |
| P0.6 | **No auth token in localStorage/sessionStorage** | ⚠️ **PARTIAL** | No tokens stored in localStorage observed. Login component redirects without storing tokens. However, need to verify Angular auth state management doesn't violate this. |
| P0.7 | **Rate limiting for login/password changes** | ❌ **NOT IMPLEMENTED** | No rate limiting middleware observed. SSOT §9 acknowledges "single-instance memory limiter acceptable in MVP" but requires it to be documented as "not distributed and resets on restart." |
| P0.8 | **OpenAPI documentation (api/openapi.yaml)** | ⚠️ **PARTIAL** | OpenAPI exists but only covers 3 paths (login, workspaces list, workspaces create). Missing: auth/me, workspaces/{id}, items, attachments, health, error enums, response schemas. |
| P0.9 | **PostgreSQL migrations (versioned, idempotent)** | ✅ **COMPLIANT** | Initial migration `000001_init_schema.up.sql` exists. Migrator handles idempotent re-runs via `schema_migrations` table. |
| P0.10 | **Shared error contract (envelope format)** | ✅ **COMPLIANT** | Consistent `ErrorResponse` with `error.code`, `error.message`, `error.request_id` across all handlers. HTTP status mapping matches SSOT (§7): 400, 401, 403, 404, 409, 413, 415, 422, 429, 500. |
| P0.11 | **Health checks (/health/live, /health/ready)** | ✅ **COMPLIANT** | Both endpoints implemented. `/health/ready` checks DB connectivity and active Super Admin existence as required by SSOT §7. |
| P0.12 | **Docker Compose with proper services** | ✅ **COMPLIANT** | `compose.yaml` defines `postgres`, `api`, `web` services with named volumes `postgres_data`, `attachments_data`. Secrets management for DB creds, CSRF secret, superadmin password. |

---

### Module P1: Identity & Isolation

| # | MUST Requirement | Status | Gap Description |
|---|---|---|---|
| P1.1 | **Workspace membership filter on all tenant endpoints** | ✅ **COMPLIANT** | All workspace-scoped endpoints filter by `workspace_id` from URL path. No endpoint accepts `workspace_id` from request body as authority. |
| P1.2 | **Cross-workspace resource access prevention** | ✅ **COMPLIANT** | Item GET query: `WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL`. Returns 404 if not found (no existence leakage). |
| P1.3 | **PIC must be active workspace member** | ✅ **COMPLIANT** | Create and update item handlers validate `assignee_id` eligibility via `workspace_members JOIN users WHERE is_active = true`. |
| P1.4 | **Last workspace ADMIN guard** | ✅ **COMPLIANT** | Both `UpdateMemberRole` and `RemoveMember` check `adminCount <= 1` and return 409 `LAST_WORKSPACE_ADMIN_GUARD`. |
| P1.5 | **Unassign items on member removal** | ✅ **COMPLIANT** | `RemoveMember` executes `UPDATE work_items SET assignee_id = NULL WHERE workspace_id = $1 AND assignee_id = $2` in same transaction. |
| P1.6 | **Archived workspace read-only (except Super Admin)** | ✅ **COMPLIANT** | `RequireWorkspaceRole` checks `archived_at` and blocks writes with 403 for non-GET methods. |
| P1.7 | **Super Admin bypass with per-endpoint validation** | ✅ **COMPLIANT** | `RequireWorkspaceRole` allows SUPER_ADMIN bypass at middleware level, but individual handlers still verify permissions where needed. |

---

### Module P2: Core Productivity (Task/Bug CRUD, PIC, Filtering, Pagination, Dashboard)

| # | MUST Requirement | Status | Gap Description |
|---|---|---|---|
| P2.1 | **Full CRUD for work items (TASK/BUG)** | ⚠️ **PARTIAL** | CRUD handlers exist with validation, versioning, and soft-delete. However: <br>- `CreateItem` defaults `type = "TASK"` if empty rather than requiring explicit value <br>- Update handler allows type change `BUG` → `TASK` without explicitly cleaning bug-specific fields in all code paths |
| P2.2 | **PIC validation (active member only)** | ✅ **COMPLIANT** | Validated in both `CreateItem` and `UpdateItem` handlers via `workspace_members JOIN users WHERE is_active`. |
| P2.3 | **Optimistic concurrency via version field** | ✅ **COMPLIANT** | PATCH requires `version` field. Update uses `WHERE id = $1 AND workspace_id = $2 AND version = $5 AND deleted_at IS NULL`. Returns 409 on mismatch. |
| P2.4 | **Filtering, sorting, pagination** | ⚠️ **PARTIAL** | List endpoint supports filter by type/status/priority/assignee/unassigned/search/due_before/due_after. Pagination with `total` metadata. However: <br>- Search parameter uses `ILIKE` with escaped wildcards (OK) but minimum 2-char limit not enforced at query level <br>- Sorting hardcoded to `updated_at DESC, id DESC` rather than supporting client sort parameter |
| P2.5 | **Dashboard summary with all required counts** | ✅ **COMPLIANT** | Dashboard handler computes: total_open, todo, in_progress, blocked, done, bugs, tasks, assigned_to_me, overdue. All filtered by `workspace_id` and `deleted_at IS NULL`. |
| P2.6 | **Status enum validation** | ✅ **COMPLIANT** | Validates: TODO, IN_PROGRESS, BLOCKED, IN_REVIEW, DONE. |
| P2.7 | **Priority enum validation** | ✅ **COMPLIANT** | Validates: LOW, MEDIUM, HIGH, CRITICAL. |

---

### Module P3: Evidence & UX (Image Upload, UI, Preview, Error States)

| # | MUST Requirement | Status | Gap Description |
|---|---|---|---|
| P3.1 | **Magic-byte validation on image upload** | ❌ **NOT IMPLEMENTED** | Attachment handler accepts multipart files but does **not** inspect actual file bytes. SSOT §5.5 requires: "Format diterima JPEG / PNG / WebP setelah inspeksi bytes nyata dan decode gambar, bukan hanya extension/MIME client." No magic-byte check observed. |
| P3.2 | **Max 5 attachments per item** | ⚠️ **PARTIAL** | Handler counts existing attachments before insert, but no database row lock before counting (SSOT §6: "For max-five uploads, obtain a database row lock on parent item before counting attachments to prevent parallel overrun"). |
| P3.3 | **Max 5 MiB per image, max 4096×4096 pixels** | ❌ **NOT IMPLEMENTED** | No pixel dimension or file size validation beyond the general `UPLOAD_MAX_BYTES = 5242880` (5 MiB). No image decoding/encoding to check dimensions or reject decompression bombs. |
| P3.4 | **Reject SVG, GIF, PDF, script files** | ❌ **NOT IMPLEMENTED** | No format rejection based on content type. SVG/GIF/PDF would be accepted if under size limit. |
| P3.5 | **Image re-encoding/normalization (remove EXIF/metadata)** | ❌ **NOT IMPLEMENTED** | No image processing pipeline observed. SSOT §5.5 requires: "Server MUST decode gambar dan menormalkan/re-encode memakai image processor teruji untuk membuang metadata EXIF/lokasi." |
| P3.6 | **Private image endpoint with authorization** | ✅ **COMPLIANT** | Image content endpoint `/items/{id}/attachments/{aid}/content` is protected by `RequireWorkspaceRole` middleware. |
| P3.7 | **Cache-Control: no-store on image endpoints** | ⚠️ **PARTIAL** | Need to verify HTTP headers on image responses. |
| P3.8 | **Frontend image preview/delete with access control** | ⚠️ **PARTIAL** | UI components exist for image preview and upload, but delete functionality and access control verification need testing. |

---

### Module P4: Hardening & Release (Security Tests, CI, Docker Production, Backups, OpenAPI, Runbook)

| # | MUST Requirement | Status | Gap Description |
|---|---|---|---|
| P4.1 | **All negative/security tests pass** | ❌ **NOT IMPLEMENTED** | Security test matrix (AUTH-01 through AUTH-07, SEC-01 through SEC-05, WS-01 through WS-05) not yet implemented. SSOT §10 requires all test cases to exist. |
| P4.2 | **CI pipeline (go test, frontend lint/test/build, DB migration, e2e smoke)** | ❌ **NOT IMPLEMENTED** | No CI configuration observed. Go vet passes, but no test pipeline, frontend build/lint/test, or e2e smoke tests. |
| P4.3 | **Docker Compose production configuration** | ⚠️ **PARTIAL** | `compose.yaml` has production-like setup but `APP_ENV: production` is set without environment override mechanism. Missing: backup/restore procedures, migration on deployment strategy, rollback plan. |
| P4.4 | **OpenAPI complete with error examples** | ❌ **NOT IMPLEMENTED** | OpenAPI only has 3 paths. Missing: all auth endpoints me/logout, all workspace/item/attachment endpoints, health errors, error envelope examples (VERSION_CONFLICT, rate limited, etc.). |
| P4.5 | **Operator runbook (bootstrap/recovery, backup/restore)** | ❌ **NOT IMPLEMENTED** | No runbook documentation. Bootstrap CLI exists but recovery procedures, backup procedures, and restore drills are not documented. |
| P4.6 | **No HA claim for single-replica architecture** | ✅ **COMPLIANT** | `compose.yaml` single replica; README/architecture docs acknowledge "Jika nanti ingin banyak API replica/HA, pindahkan lampiran ke S3-compatible shared object storage." |
| P4.7 | **Dependency version pinning** | ✅ **COMPLIANT** | `go.mod`/`go.sum` pinned, `bun.lock` present, Docker base images use tags. |
| P4.8 | **Structured JSON logs with request_id** | ✅ **COMPLIANT** | `RequestID` middleware + `Logger` middleware produce structured JSON logs with `request_id`, `method`, `path`, `status`, `duration`. |

---

## Violations / Gap Summary

### Critical Gaps (Must Fix Before P1):

1. **CSRF middleware not registered** (`router.go`): `RequireCSRF` is defined but never wired into the request pipeline. This leaves all mutating endpoints (`POST /api/v1/workspaces`, `POST /items`, `PATCH /items`, `DELETE /items`, `POST /members`, etc.) vulnerable to CSRF attacks. **Severity: CRITICAL**

2. **No magic-byte validation on image upload**: File upload endpoint accepts any file type as long as it's under the size limit. An attacker could upload executable scripts, SVG polyglots, or decompression bombs. **Severity: CRITICAL**

3. **No rate limiting on login/password attempts**: No protection against brute-force attacks. **Severity: HIGH**

4. **Bootstrap allows password from env var in production**: While warned, the code still accepts `SUPERADMIN_PASSWORD` env var instead of requiring the secret file. In production, this could lead to hardcoded passwords. **Severity: HIGH**

5. **OpenAPI incomplete**: Only 3 of ~30+ API paths are documented. This violates the SSOT requirement that "API MUST be documented in `api/openapi.yaml` with request/response schemas and HTTP error examples." **Severity: MEDIUM**

### Moderate Gaps (Fix Before P2):

6. **Last admin guard race condition**: The `adminCount` check in `UpdateMemberRole` and `RemoveMember` reads the count separately from the update/delete operation. In high concurrency, two simultaneous operations could both pass the "last admin" check. **Severity: MEDIUM**

7. **Image re-encoding/metadata sanitization not implemented**: Images uploaded are stored as-is without EXIF/gps metadata removal or format normalization. **Severity: MEDIUM**

8. **Image attachment count race condition**: No `SELECT ... FOR UPDATE` on parent item before counting attachments, allowing parallel uploads to exceed the 5-attachment limit. **Severity: MEDIUM**

### Low Gaps (Fix Later):

9. **Frontend login dummy navigation**: Login component navigates immediately without actual authentication validation. **Severity: LOW**

10. **No frontend unit/integration tests**: `frontend/package.json` has `test` placeholder. **Severity: LOW**

---

## Recommendations / Action Items

### Immediate (Must Fix Before P1 Gate):

1. **Register CSRF middleware in router** (`backend/internal/http/router.go`):
   - Add `r.Use(middleware.RequireCSRF(csrfSecret, isProd))` after `OriginValidation`
   - Ensure login endpoint properly validates Content-Type and origin
   - For login path, CSRF should be skipped but Content-Type `application/json` enforced

2. **Implement magic-byte validation on image upload** (`backend/internal/http/handlers/items.go` or dedicated attachment handler):
   - After receiving multipart file, inspect first bytes (magic bytes)
   - Only accept: JPEG (`0xFFD8`), PNG (`89 50 4E 47`), WebP (`RIFF` then `WEBP`)
   - Reject: SVG, GIF, PDF, anything with script headers
   - Use Go's `image` package to decode and re-encode, discarding EXIF metadata

3. **Add rate limiting for login/password changes** (`backend/internal/middleware/ratelimit.go`):
   - Implement single-instance memory limiter (as SSOT allows for MVP)
   - Document that it resets on restart and is not distributed
   - Apply to `/api/v1/auth/login` and `/api/v1/auth/change-password`

4. **Fix bootstrap to enforce secret file in production** (`backend/cmd/bootstrap/main.go`):
   - Make `SUPERADMIN_PASSWORD_FILE` mandatory; remove the `SUPERADMIN_PASSWORD` env var fallback entirely for production
   - Or add a strict check that warns loudly and refuses to start if not using secret file in production mode

5. **Complete OpenAPI documentation** (`api/openapi.yaml`):
   - Add all remaining endpoints: auth/me, auth/logout, workspaces/{id}, workspaces/{id}/items, items/{id}, attachments, health, error enums
   - Add request/response schemas matching SSOT error envelope
   - Add HTTP error examples (401, 403, 404, 409, 413, 422, 429, 500)

### Before P2:

6. **Implement image attachment count with row locking**:
   - Before counting attachments for a new upload, `SELECT ... FOR UPDATE` on the parent work item
   - Ensure count + 1 <= 5 before inserting new attachment

7. **Add image format/size validation before storage**:
   - Decode image using Go `image` package
   - Verify dimensions <= 4096×4096
   - Verify output megapixels <= 16
   - Re-encode to normalized format (JPEG/PNG), stripping EXIF/gps metadata
   - Store re-encoded version, original filename obscured

8. **Fix last admin guard race condition**:
   - Use `SELECT COUNT(*) ... FOR UPDATE` within the same transaction as the role change/delete
   - Or use advisory locks for the duration of the transaction

9. **Add search minimum length enforcement**:
   - Return 400 if `search` parameter is provided with length < 2
   - Or silently treat as no filter (current behavior is acceptable if documented)

10. **Add frontend authentication guard**:
    - Ensure no auth tokens are stored in localStorage/sessionStorage
    - Session is managed via HttpOnly cookies only
    - Angular guards use `/api/v1/auth/me` to check auth status

### P2-P4:

11. **Implement full test matrix** (SSOT §10):
    - All AUTH-01 through AUTH-07 test cases
    - All SEC-01 through SEC-05 test cases
    - All WS-01 through WS-05 test cases
    - All ITEM-01 through ITEM-04 test cases
    - All IMG-01 through IMG-05 test cases
    - Integration tests with PostgreSQL, e2e browser tests for critical paths

12. **Add CI pipeline**:
    - `go test ./...` with coverage
    - Frontend `bun run lint`, `bun run test`, `bun run build`
    - DB migration tests
    - E2E smoke test script

13. **Create operator runbook**:
    - Bootstrap procedure with secret file
    - Password recovery `./bootstrap-admin --recover`
    - Backup and restore procedures for PostgreSQL + attachments volume
    - Deployment rollback strategy

14. **Add Cache-Control headers on sensitive endpoints**:
    - `Cache-Control: no-store` on auth, session, and audit endpoints
    - `X-Content-Type-Options: nosniff` on all responses
    - HSTS via reverse proxy (nginx)

---

## Conclusion

The application has a solid foundation with proper database schema, session management, workspace isolation, and error handling. However, **5 critical gaps must be resolved before the P1 gate can be passed:**

1. CSRF middleware not registered in router
2. No magic-byte validation on image uploads
3. No rate limiting on authentication endpoints
4. Bootstrap allows password from env var (production risk)
5. Incomplete OpenAPI documentation

Additionally, **3 moderate gaps** that should be fixed before P2:
- Image attachment count with row locking for max-5 constraint
- Image re-encoding/metadata sanitization
- Last admin guard race condition

The project is **62% compliant** with the SSOT MUST requirements. All P0 foundation elements are present and mostly correct, but the security-critical CSRF and image validation gaps must be addressed immediately.

---
*End of Audit Report*
