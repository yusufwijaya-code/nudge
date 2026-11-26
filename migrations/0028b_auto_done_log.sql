-- Geser otomatis ke selesai: one row per ticket run (a stretch in a tracked status)
-- Nudge tried to move to done. The unique key makes the attempt happen once per
-- run, also across restarts and instances; the row doubles as an audit log.
CREATE TABLE IF NOT EXISTS tr_autodone_log (
  autoDoneID    BIGINT       NOT NULL AUTO_INCREMENT,
  issueKey      VARCHAR(32)  NOT NULL,
  runStarted    DATETIME     NOT NULL COMMENT 'when the tracked run began (epoch if unknown)',
  assigneeEmail VARCHAR(255) NOT NULL,
  targetStatus  VARCHAR(128) NOT NULL COMMENT 'the project''s done status',
  outcome       VARCHAR(16)  NOT NULL DEFAULT 'pending' COMMENT 'pending | moved | failed',
  message       VARCHAR(500) NULL,
  createdDate   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (autoDoneID),
  UNIQUE KEY uq_autodone_run (issueKey, runStarted)
);
