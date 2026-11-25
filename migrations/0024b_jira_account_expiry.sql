-- Expiry date of the central Jira account's token (Admin > Akun Jira). One row
-- at most; it describes whichever token is in force (saved or from the env).
CREATE TABLE IF NOT EXISTS ms_jira_account_expiry (
  accountSlot TINYINT      NOT NULL DEFAULT 1 COMMENT 'always 1',
  expiresAt   DATE         NOT NULL,
  updatedBy   VARCHAR(255) NOT NULL,
  updatedDate DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (accountSlot),
  CONSTRAINT chk_ms_jira_account_expiry_single CHECK (accountSlot = 1)
);
