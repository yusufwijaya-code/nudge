-- Admin › Akun Jira: the central Jira account (email + API token) the bot acts
-- as. One row at most (accountSlot is always 1); the token is stored encrypted
-- (AES-256-GCM, CREDENTIAL_ENCRYPTION_KEY). No row = the JIRA_EMAIL /
-- JIRA_API_TOKEN pair from the environment is used.
CREATE TABLE IF NOT EXISTS ms_jira_account (
  accountSlot           TINYINT       NOT NULL DEFAULT 1 COMMENT 'always 1: a single central account',
  jiraEmail             VARCHAR(255)  NOT NULL           COMMENT 'Atlassian account email',
  jiraAPITokenEncrypted VARBINARY(1024) NOT NULL         COMMENT 'nonce|ciphertext|tag',
  updatedBy             VARCHAR(255)  NOT NULL           COMMENT 'Admin email',
  updatedDate           DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (accountSlot),
  CONSTRAINT chk_ms_jira_account_single CHECK (accountSlot = 1)
);
