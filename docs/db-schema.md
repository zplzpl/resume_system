# Resume System Core Data Model (ZPL-3)

## Scope
This document defines the V1 core entities required by issue `ZPL-3`:
- Candidate
- Resume
- Position
- Interview
- Interview Score
- Interview Report
- Audit Log

All SQL is implemented in `db/migrations/0001_core_schema.up.sql` and rollback in `db/migrations/0001_core_schema.down.sql`.

## ER Relationships

```text
users 1 --- n candidates(created_by/updated_by)
users 1 --- n resumes(uploaded_by)
users 1 --- n positions(created_by/updated_by)
users 1 --- n interviews(created_by/updated_by)
users 1 --- n interview_scores(interviewer_id)
users 1 --- n interview_reports(generated_user_id)
users 1 --- n audit_logs(actor_user_id)

candidates 1 --- n resumes
candidates 1 --- n interviews
positions  1 --- n interviews

interviews 1 --- n interview_scores
interviews 1 --- 1 interview_reports
interviews 1 --- n interview_panelists
users      1 --- n interview_panelists
```

## Table Definitions

### 1) `users`
| Field | Type | Null | Key | Description |
|---|---|---|---|---|
| id | BIGINT UNSIGNED AUTO_INCREMENT | NO | PK | Internal user id |
| auth_user_id | VARCHAR(64) | NO | UK | Supabase/Auth external id |
| name | VARCHAR(128) | NO | IDX | Display name |
| email | VARCHAR(255) | NO | UK | Login email |
| role | ENUM('super_admin','hr','interviewer','system') | NO | IDX | Platform role |
| status | ENUM('active','disabled') | NO | IDX | Account status |
| created_at | DATETIME(3) | NO |  | Creation time |
| updated_at | DATETIME(3) | NO |  | Update time |

### 2) `candidates`
| Field | Type | Null | Key | Description |
|---|---|---|---|---|
| id | BIGINT UNSIGNED AUTO_INCREMENT | NO | PK | Candidate id |
| candidate_no | VARCHAR(32) | NO | UK | Business identifier |
| full_name | VARCHAR(128) | NO | IDX | Candidate name |
| email | VARCHAR(255) | YES | IDX | Candidate email |
| phone | VARCHAR(32) | YES | IDX | Candidate phone |
| source_type | ENUM('manual','upload','job_board','referral') | NO | IDX | Source channel |
| current_company | VARCHAR(128) | YES |  | Current company |
| current_title | VARCHAR(128) | YES |  | Current title |
| total_experience_months | INT UNSIGNED | YES |  | Experience in months |
| highest_education | VARCHAR(64) | YES |  | Education level |
| location | VARCHAR(128) | YES |  | Current location |
| status | ENUM('new','screening','interviewing','offered','hired','rejected','talent_pool') | NO | IDX | Pipeline status |
| created_by | BIGINT UNSIGNED | YES | FK | Creator user id |
| updated_by | BIGINT UNSIGNED | YES | FK | Last updater user id |
| created_at | DATETIME(3) | NO | IDX | Creation time |
| updated_at | DATETIME(3) | NO |  | Update time |

### 3) `resumes`
| Field | Type | Null | Key | Description |
|---|---|---|---|---|
| id | BIGINT UNSIGNED AUTO_INCREMENT | NO | PK | Resume id |
| candidate_id | BIGINT UNSIGNED | NO | FK/IDX | Candidate reference |
| version_no | INT UNSIGNED | NO | UK(cand,version) | Resume version per candidate |
| is_primary | TINYINT(1) | NO | IDX | Whether current active version |
| file_name | VARCHAR(255) | NO |  | Uploaded file name |
| file_url | VARCHAR(1024) | NO |  | Storage URL |
| file_hash | CHAR(64) | YES | IDX | Dedup hash |
| parse_status | ENUM('pending','processing','success','failed') | NO | IDX | Parse state |
| parsed_payload | JSON | YES |  | Structured parse result |
| parsed_at | DATETIME(3) | YES |  | Parse completion time |
| uploaded_by | BIGINT UNSIGNED | YES | FK | Uploader user id |
| created_at | DATETIME(3) | NO | IDX | Creation time |
| updated_at | DATETIME(3) | NO |  | Update time |

### 4) `positions`
| Field | Type | Null | Key | Description |
|---|---|---|---|---|
| id | BIGINT UNSIGNED AUTO_INCREMENT | NO | PK | Position id |
| position_code | VARCHAR(32) | NO | UK | Business identifier |
| title | VARCHAR(128) | NO | IDX | Position title |
| department | VARCHAR(128) | YES | IDX | Department |
| location | VARCHAR(128) | YES |  | Work location |
| employment_type | ENUM('full_time','part_time','intern','contract') | NO | IDX | Employment type |
| jd_text | MEDIUMTEXT | YES |  | Job description |
| status | ENUM('draft','open','paused','closed') | NO | IDX | Recruiting status |
| headcount | INT UNSIGNED | NO |  | Planned headcount |
| created_by | BIGINT UNSIGNED | YES | FK | Creator user id |
| updated_by | BIGINT UNSIGNED | YES | FK | Last updater user id |
| created_at | DATETIME(3) | NO | IDX | Creation time |
| updated_at | DATETIME(3) | NO |  | Update time |

### 5) `interviews`
| Field | Type | Null | Key | Description |
|---|---|---|---|---|
| id | BIGINT UNSIGNED AUTO_INCREMENT | NO | PK | Interview id |
| interview_no | VARCHAR(32) | NO | UK | Business identifier |
| candidate_id | BIGINT UNSIGNED | NO | FK/IDX | Candidate reference |
| position_id | BIGINT UNSIGNED | NO | FK/IDX | Position reference |
| round_no | INT UNSIGNED | NO | IDX | Interview round |
| interview_type | ENUM('phone','onsite','video','technical','hr') | NO | IDX | Interview type |
| scheduled_start_at | DATETIME(3) | NO | IDX | Start time |
| scheduled_end_at | DATETIME(3) | NO | IDX | End time |
| timezone | VARCHAR(64) | NO |  | TZ identifier |
| status | ENUM('scheduled','confirmed','completed','cancelled','rescheduled','no_show') | NO | IDX | Schedule status |
| meeting_provider | VARCHAR(32) | YES |  | Zoom/Tencent/etc |
| meeting_url | VARCHAR(1024) | YES |  | Meeting link |
| transcript_status | ENUM('not_started','processing','ready','failed') | NO | IDX | Transcript pipeline state |
| transcript_ref | VARCHAR(255) | YES |  | Transcript object reference |
| created_by | BIGINT UNSIGNED | YES | FK | Creator user id |
| updated_by | BIGINT UNSIGNED | YES | FK | Last updater user id |
| created_at | DATETIME(3) | NO | IDX | Creation time |
| updated_at | DATETIME(3) | NO |  | Update time |

### 6) `interview_panelists`
| Field | Type | Null | Key | Description |
|---|---|---|---|---|
| id | BIGINT UNSIGNED AUTO_INCREMENT | NO | PK | Row id |
| interview_id | BIGINT UNSIGNED | NO | FK/IDX | Interview reference |
| user_id | BIGINT UNSIGNED | NO | FK/IDX | Panelist user id |
| panel_role | ENUM('owner','co_interviewer','observer') | NO | IDX | Role in this interview |
| created_at | DATETIME(3) | NO |  | Creation time |

### 7) `interview_scores`
| Field | Type | Null | Key | Description |
|---|---|---|---|---|
| id | BIGINT UNSIGNED AUTO_INCREMENT | NO | PK | Score id |
| interview_id | BIGINT UNSIGNED | NO | FK/IDX | Interview reference |
| interviewer_id | BIGINT UNSIGNED | NO | FK/IDX | Interviewer user id |
| overall_score | DECIMAL(5,2) | NO | IDX | Total score |
| recommendation | ENUM('strong_hire','hire','hold','no_hire','strong_no_hire') | NO | IDX | Hiring recommendation |
| scoring_payload | JSON | YES |  | Dimension scores |
| comment_text | TEXT | YES |  | Free-text notes |
| created_at | DATETIME(3) | NO | IDX | Creation time |
| updated_at | DATETIME(3) | NO |  | Update time |

### 8) `interview_reports`
| Field | Type | Null | Key | Description |
|---|---|---|---|---|
| id | BIGINT UNSIGNED AUTO_INCREMENT | NO | PK | Report id |
| interview_id | BIGINT UNSIGNED | NO | UK/FK | One report per interview |
| summary_text | MEDIUMTEXT | YES |  | Final summary |
| strengths | TEXT | YES |  | Strength points |
| weaknesses | TEXT | YES |  | Weakness points |
| risk_notes | TEXT | YES |  | Hiring risk notes |
| hire_recommendation | ENUM('strong_hire','hire','hold','no_hire','strong_no_hire') | YES | IDX | Consolidated recommendation |
| report_payload | JSON | YES |  | Structured report |
| generated_by | ENUM('ai','human') | NO | IDX | Source of report |
| generated_user_id | BIGINT UNSIGNED | YES | FK | Human generator id |
| generated_at | DATETIME(3) | YES | IDX | Generation time |
| created_at | DATETIME(3) | NO |  | Creation time |
| updated_at | DATETIME(3) | NO |  | Update time |

### 9) `audit_logs`
| Field | Type | Null | Key | Description |
|---|---|---|---|---|
| id | BIGINT UNSIGNED AUTO_INCREMENT | NO | PK | Audit id |
| actor_user_id | BIGINT UNSIGNED | YES | FK/IDX | Operator id |
| entity_type | ENUM('candidate','resume','position','interview','score','report','user','system') | NO | IDX | Domain entity type |
| entity_id | BIGINT UNSIGNED | YES | IDX | Domain entity id |
| action | VARCHAR(64) | NO | IDX | Action name |
| before_data | JSON | YES |  | Before snapshot |
| after_data | JSON | YES |  | After snapshot |
| ip_address | VARCHAR(45) | YES |  | Source IP |
| user_agent | VARCHAR(512) | YES |  | UA |
| trace_id | VARCHAR(64) | YES | IDX | Request trace id |
| created_at | DATETIME(3) | NO | IDX | Event time |

## Key Query Indexes
1. Candidate funnel and recent activity: `(status, created_at)` on `candidates`.
2. Candidate lookup: `email`, `phone`, and `candidate_no` unique index.
3. Resume retrieval/dedup: `(candidate_id, created_at)`, `file_hash`, and `(candidate_id, version_no)` unique.
4. Position management: `(status, department)` and `position_code` unique.
5. Interview schedules: `(candidate_id, scheduled_start_at)`, `(position_id, status)`, and `(status, scheduled_start_at)`.
6. Panel conflict checks: `(user_id, interview_id)` unique + `interview_id` index on `interview_panelists`.
7. Score aggregation: `(interview_id, interviewer_id)` unique, plus `recommendation` and `created_at`.
8. Report retrieval: unique `interview_id` and `generated_at` index.
9. Audit traceability: `(entity_type, entity_id, created_at)`, `(actor_user_id, created_at)`, `trace_id`.
