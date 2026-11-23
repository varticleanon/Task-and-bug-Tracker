# Task & Bug Tracker

Angular 19 frontend + Go backend for tracking tasks and bugs across workspaces.

## Project Structure

```
backend/          Go REST API (chi router, pgx, PostgreSQL)
  cmd/api/        Entry point
  internal/http/  Handlers, middleware, router
  internal/auth/  Sessions, password hashing
frontend/         Angular 19 standalone app
  src/app/core/   Services, models, guards, interceptors
  src/app/features/ Feature components (dashboard, items, workspaces, admin)
  src/app/shared/layout/ Shell components
```

## Commands

```bash
# Backend (port 8081)
cd backend && export $(grep -v '^#' ../.env | xargs) && HTTP_ADDR=:8081 go run ./cmd/api

# Frontend (port 4200, served under /absproxy/4200/)
cd frontend && bunx ng serve --host 0.0.0.0 --port 4200 --serve-path /absproxy/4200/ --disable-host-check

# Build frontend
cd frontend && bun run build

# Backend tests (need DATABASE_URL exported)
cd backend && export $(grep -v '^#' ../.env | xargs) && go test ./internal/http/...
```

Restart both services with `./scripts/restart-services.sh` (sets `ulimit -c 0` to
prevent multi-GB core dumps from filling the disk).

## Environment Notes

- Public frontend URL: `https://code-server.skyxserver.online/absproxy/4200/`
- API path: `/absproxy/8081/api/v1/...` (code-server proxies to localhost:8081)
- Port 8080 is taken by code-server, backend must use 8081.
- `<base href="/absproxy/4200/">` in `frontend/src/index.html` must stay as-is.
- Admin login: `admin@example.com` / `SuperAdminPassword123!`
- PostgreSQL: start with `sudo -n pg_ctlcluster 17 main start`
- Do not use Python servers. Avoid heavy processes, code-server is sluggish.

<!-- antislop:start -->
## antislop
For UI, copy, people, mobile layout, or code comments work, read `antislop.md` (core) and then the skill for the task:
- UI / visual: `skills/antislop-ui/SKILL.md`
- Copy & text: `skills/antislop-copywriting/SKILL.md`
- People: `skills/antislop-human/SKILL.md`
- Mobile / responsive: `skills/antislop-layoutmobile/SKILL.md`
- Code comments: `skills/antislop-code/SKILL.md`
Before starting, follow the core's "Two Usage Modes" section in strict order: explicit session instruction first, then global preference, then ask. A session instruction always wins. For a resolved mode, say `antislop active: <mode> (session override).` or `antislop active: <mode> (global preference).` once before presenting findings or making edits, using the actual mode and source. Acknowledging the user's request without naming the source does not replace this notice.
Only an explicit choice of antislop during or after selects a session mode. A request to review, audit, or avoid file edits does not select a mode; read the global preference in that case. Another skill's mode does not select antislop's mode.
If the mode is unresolved, ask during/after and end the response; wait for the answer before any UI review, planning, or concept. For read-only tasks, put the active-mode notice only at the start of the final answer, never in progress messages. For editing tasks, announce before the first edit and omit it from the final answer.
To update antislop later: download `antislop.md` again, or run `npx antislop-ai --update` if it was installed as skill folders.
<!-- antislop:end -->
