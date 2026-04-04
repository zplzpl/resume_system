#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
UP_SQL="$ROOT_DIR/db/migrations/0001_core_schema.up.sql"
DOWN_SQL="$ROOT_DIR/db/migrations/0001_core_schema.down.sql"

if [[ ! -f "$UP_SQL" || ! -f "$DOWN_SQL" ]]; then
  echo "Missing migration files." >&2
  exit 1
fi

WORK_DIR="${TMPDIR:-/tmp}/resume-system-mysql-$$"
DATA_DIR="$WORK_DIR/data"
SOCKET="$WORK_DIR/mysql.sock"
PID_FILE="$WORK_DIR/mysql.pid"
LOG_FILE="$WORK_DIR/mysql.log"
DB_NAME="resume_system_test"

cleanup() {
  if [[ -f "$PID_FILE" ]]; then
    kill "$(cat "$PID_FILE")" >/dev/null 2>&1 || true
    for _ in {1..20}; do
      if [[ -S "$SOCKET" ]]; then
        sleep 0.2
      else
        break
      fi
    done
  fi
  rm -rf "$WORK_DIR"
}
trap cleanup EXIT

mkdir -p "$DATA_DIR"

mysqld --initialize-insecure --datadir="$DATA_DIR" --log-error="$LOG_FILE" >/dev/null 2>&1
mysqld \
  --datadir="$DATA_DIR" \
  --socket="$SOCKET" \
  --pid-file="$PID_FILE" \
  --skip-networking \
  --log-error="$LOG_FILE" \
  --daemonize

for _ in {1..40}; do
  if mysql --protocol=SOCKET --socket="$SOCKET" -uroot -Nse "SELECT 1" >/dev/null 2>&1; then
    break
  fi
  sleep 0.25
done

if ! mysql --protocol=SOCKET --socket="$SOCKET" -uroot -Nse "SELECT 1" >/dev/null 2>&1; then
  echo "MySQL temp instance failed to start." >&2
  cat "$LOG_FILE" >&2 || true
  exit 1
fi

mysql --protocol=SOCKET --socket="$SOCKET" -uroot -e "DROP DATABASE IF EXISTS ${DB_NAME}; CREATE DATABASE ${DB_NAME};"

# Apply twice to validate repeatability.
mysql --protocol=SOCKET --socket="$SOCKET" -uroot "$DB_NAME" < "$UP_SQL"
mysql --protocol=SOCKET --socket="$SOCKET" -uroot "$DB_NAME" < "$UP_SQL"

TABLE_COUNT="$(mysql --protocol=SOCKET --socket="$SOCKET" -uroot -Nse "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='${DB_NAME}' AND table_type='BASE TABLE';")"
if [[ "$TABLE_COUNT" != "9" ]]; then
  echo "Expected 9 tables after migration, got ${TABLE_COUNT}." >&2
  exit 1
fi

INDEX_COUNT="$(mysql --protocol=SOCKET --socket="$SOCKET" -uroot -Nse "
SELECT COUNT(DISTINCT CONCAT(table_name, ':', index_name))
FROM information_schema.statistics
WHERE table_schema='${DB_NAME}'
  AND (
    (table_name='users' AND index_name='uk_users_email') OR
    (table_name='candidates' AND index_name='idx_candidates_status_created_at') OR
    (table_name='resumes' AND index_name='uk_resumes_candidate_version') OR
    (table_name='positions' AND index_name='idx_positions_status_department') OR
    (table_name='interviews' AND index_name='idx_interviews_status_start') OR
    (table_name='interview_panelists' AND index_name='uk_interview_panelists_interview_user') OR
    (table_name='interview_scores' AND index_name='uk_interview_scores_interview_interviewer') OR
    (table_name='interview_reports' AND index_name='uk_interview_reports_interview_id') OR
    (table_name='audit_logs' AND index_name='idx_audit_logs_entity')
  );")"
if [[ "$INDEX_COUNT" != "9" ]]; then
  echo "Expected 9 critical indexes, got ${INDEX_COUNT}." >&2
  exit 1
fi

mysql --protocol=SOCKET --socket="$SOCKET" -uroot "$DB_NAME" < "$DOWN_SQL"

REMAINING_TABLES="$(mysql --protocol=SOCKET --socket="$SOCKET" -uroot -Nse "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='${DB_NAME}' AND table_type='BASE TABLE';")"
if [[ "$REMAINING_TABLES" != "0" ]]; then
  echo "Expected 0 tables after rollback, got ${REMAINING_TABLES}." >&2
  exit 1
fi

echo "Migration verification passed: apply(apply again)->rollback succeeded on temporary MySQL instance."
