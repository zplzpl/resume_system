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
  - `POST /api/v1/resumes/upload`
  - `POST /api/v1/resumes/upload/batch`
  - `GET /api/v1/resumes/:id`
  - `POST /api/v1/resumes/:id/retry`
  - `POST /api/v1/interviews`
  - `GET /api/v1/admin/users`
- Unified unauthorized response code (`UNAUTHORIZED`) for 401/403 cases
- Resume upload/parse pipeline:
  - Supports `.pdf` / `.doc` / `.docx` upload
  - Stores file metadata (hash, content-type, storage path, size, uploader)
  - Parses core fields into candidate profile (`full_name`, `email`, `phone`, company/title, education, location, skills, experience months)
  - Tracks parse status (`pending`/`processing`/`success`/`failed`) with failure reason
  - Provides retry endpoint for failed files

## Environment Variables

```bash
PORT=8080
SUPABASE_URL=https://<project>.supabase.co
SUPABASE_ANON_KEY=<anon-key>
SUPABASE_JWT_SECRET=<jwt-secret>
RESUME_STORAGE_DIR=./data/resumes
```

## Run

```bash
go run ./cmd/server
```

## Test

```bash
go test ./...
```
