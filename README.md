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
  - `GET /api/v1/candidates` (supports combined filters: `keyword`/`skill`/`company`/`school` + `status_layer`)
  - `POST /api/v1/candidates`
  - `PATCH /api/v1/candidates/:id/status-layer`
  - `POST /api/v1/resumes/upload`
  - `POST /api/v1/resumes/upload/batch`
  - `GET /api/v1/resumes/:id`
  - `POST /api/v1/resumes/:id/retry`
  - `POST /api/v1/interviews`
  - `PATCH /api/v1/interviews/:id`
  - `GET /api/v1/interviews/calendar?view=day|week|month&date=YYYY-MM-DD`
  - `POST /api/v1/interviews/:id/evaluations`
  - `GET /api/v1/interviews/:id/evaluations`
  - `GET /api/v1/candidates/:id/evaluations/latest`
  - `GET /api/v1/admin/users`
- Unified unauthorized response code (`UNAUTHORIZED`) for 401/403 cases
- Resume upload/parse pipeline:
  - Supports `.pdf` / `.doc` / `.docx` upload
  - Stores file metadata (hash, content-type, storage path, size, uploader)
  - Parses core fields into candidate profile (`full_name`, `email`, `phone`, company/title, education, location, skills, experience months)
  - Tracks parse status (`pending`/`processing`/`success`/`failed`) with failure reason
  - Provides retry endpoint for failed files
- Candidate status layering:
  - Default status layer: `new`
  - Supported layers: `new` / `screening` / `interview` / `offer` / `hired` / `archived`
  - Status-layer filtering supports multiple values (e.g. `?status_layer=screening&status_layer=interview`)
- Interview scheduling:
  - Supports day/week/month calendar slices for interview events
  - Detects candidate/interviewer time conflicts when creating/updating schedules
  - Enqueues notification events for create/update operations (in-app + email)
  - Links candidate flow status to `interview` when schedule is created or changed
- Structured interview evaluation:
  - Uses fixed capability template (`technical_depth` / `problem_solving` / `communication` / `collaboration`)
  - Validates score range (1-5) and conclusion enum (`strong_hire` / `hire` / `hold` / `no_hire`)
  - Stores versioned evaluation archives per interviewer/interview and marks latest records
  - Provides candidate-level latest evaluation view grouped by round for report generation

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
