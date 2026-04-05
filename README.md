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
  - `GET /api/v1/candidates` (supports combined filters: `keyword`/`skill`/`company`/`school`/`location` + `status_layer`, `min_experience_years`, `max_experience_years`; also supports natural-language query via `natural_query`/`q` and returns parsed explainable conditions)
  - `POST /api/v1/candidates`
  - `PATCH /api/v1/candidates/:id/status-layer`
  - `POST /api/v1/resumes/upload`
  - `POST /api/v1/resumes/upload/batch`
  - `GET /api/v1/resumes/:id`
  - `DELETE /api/v1/resumes/:id`
  - `POST /api/v1/resumes/:id/retry`
  - `POST /api/v1/interviews`
  - `PATCH /api/v1/interviews/:id`
  - `GET /api/v1/interviews/calendar?view=day|week|month&date=YYYY-MM-DD`
  - `POST /api/v1/interviews/:id/transcriptions/sessions`
  - `POST /api/v1/interviews/:id/transcriptions`
  - `GET /api/v1/interviews/:id/transcriptions?session_id=<id>&since_seq=<n>`
  - `POST /api/v1/interviews/:id/transcriptions/sessions/:sessionID/interrupted`
  - `POST /api/v1/interviews/:id/transcriptions/sessions/:sessionID/reconnect`
  - `POST /api/v1/interviews/:id/evaluations`
  - `GET /api/v1/interviews/:id/evaluations`
  - `POST /api/v1/interviews/:id/question-recommendations`
  - `GET /api/v1/interviews/:id/question-recommendations`
  - `GET /api/v1/candidates/:id/evaluations/latest`
  - `POST /api/v1/candidates/:id/interview-report`
  - `GET /api/v1/interview-reports/:id/export?format=json|markdown`
  - `GET /api/v1/analytics/recruiting-dashboard`
  - `GET /api/v1/analytics/recruiting-dashboard/export.csv`
  - `GET /api/v1/audit-logs`
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
- Real-time interview transcription:
  - Supports interview-scoped transcription sessions to continuously append transcript segments
  - Persists transcript segments with ordered sequence cursor for incremental reads (`since_seq`)
  - Supports explicit speaker labels (`interviewer` / `candidate`) per transcript segment
  - Provides interruption status and reconnect endpoint for stream recovery/error prompting
- Structured interview evaluation:
  - Uses fixed capability template (`technical_depth` / `problem_solving` / `communication` / `collaboration`)
  - Validates score range (1-5) and conclusion enum (`strong_hire` / `hire` / `hold` / `no_hire`)
  - Stores versioned evaluation archives per interviewer/interview and marks latest records
  - Provides candidate-level latest evaluation view grouped by round for report generation
- Structured interview report generation:
  - One-click generation from candidate profile + latest evaluation archives
  - Deterministic report ID and stable score ordering on repeated generation
  - Includes candidate information, score details, interviewer comments, and hiring recommendation
  - Supports JSON and Markdown export formats with explicit failure messages
- Recruiting funnel and efficiency dashboard:
  - Returns stage candidate counts and stage-to-stage conversion rates for `new/screening/interview/offer/hired`
  - Returns interviewer workload (feedback count) and average feedback duration
  - Supports CSV export for downstream BI/reporting
  - Includes explicit metric definitions (formula/unit/aggregation)
- Audit logging:
  - Automatically records key operations (`auth.login`, `auth.logout`, `resume.delete`, `interview.evaluation.submit`, `interview.evaluation.modify`)
  - Includes operator, operation time, object type, object id, and operation metadata
  - Supports filtered query by action/operator/object/time window via `GET /api/v1/audit-logs`

## Recruiting Dashboard Metric Definitions

| Metric ID | Definition | Formula | Unit | Aggregation |
| --- | --- | --- | --- | --- |
| `stage_candidate_count` | Current candidates in each stage | `count(candidates where status_layer = stage)` | 人 | 按阶段 |
| `stage_conversion_rate` | Ratio of current stage to previous stage | `stage_count(current_stage) / stage_count(previous_stage)` | 比例 | 按阶段 |
| `interviewer_workload` | Feedback entries submitted by each interviewer | `count(evaluations where interviewer_id = x)` | 条反馈 | 按面试官 |
| `feedback_duration_hours` | Elapsed time from interview end to feedback submission | `evaluation.submitted_at - interview.ends_at` | 小时 | 平均值 |

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
