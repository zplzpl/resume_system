-- Resume System core schema rollback (ZPL-3)

SET FOREIGN_KEY_CHECKS = 0;

DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS interview_reports;
DROP TABLE IF EXISTS interview_scores;
DROP TABLE IF EXISTS interview_panelists;
DROP TABLE IF EXISTS interviews;
DROP TABLE IF EXISTS positions;
DROP TABLE IF EXISTS resumes;
DROP TABLE IF EXISTS candidates;
DROP TABLE IF EXISTS users;

SET FOREIGN_KEY_CHECKS = 1;
