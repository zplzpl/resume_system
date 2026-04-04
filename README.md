# resume_system

Supabase Auth + RBAC skeleton for resume system backend.

## What Is Included

- Supabase-based session endpoints:
  - `POST /api/v1/auth/login`
  - `POST /api/v1/auth/refresh`
  - `POST /api/v1/auth/logout`
- JWT authentication middleware using `SUPABASE_JWT_SECRET`
- Role matrix:
  - `super_admin`
  - `hr`
  - `interviewer`
- Core protected APIs with permission checks:
  - `GET /api/v1/candidates`
  - `POST /api/v1/candidates`
  - `POST /api/v1/interviews`
  - `GET /api/v1/admin/users`
- Unified unauthorized response code (`UNAUTHORIZED`) for 401/403 cases

## Environment Variables

```bash
PORT=8080
SUPABASE_URL=https://<project>.supabase.co
SUPABASE_ANON_KEY=<anon-key>
SUPABASE_JWT_SECRET=<jwt-secret>
```

## Run

```bash
go run ./cmd/server
```

## Test

```bash
go test ./...
```
