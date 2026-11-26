-- Admin › Pengaturan Aplikasi › Kredensial & token: the application credentials that
-- used to live only in the server environment. A text credential (Gemini API key,
-- Google Chat webhook) is stored encrypted (AES-256-GCM, CREDENTIAL_ENCRYPTION_KEY);
-- every credential, also the ones that are only monitored (OAuth tokens, service
-- account key), can carry the day it expires, which drives the admin banner.
CREATE TABLE IF NOT EXISTS ms_app_credential (
  credKey        VARCHAR(64)     NOT NULL COMMENT 'e.g. gemini_api_key',
  valueEncrypted VARBINARY(2048) NULL     COMMENT 'nonce|ciphertext|tag; NULL = no value kept here (monitored only, or removed)',
  expiresAt      DATE            NULL     COMMENT 'the day it expires, entered by an admin; NULL = unknown',
  updatedBy      VARCHAR(255)    NOT NULL COMMENT 'Admin email, or "env" for a value carried over from the environment',
  updatedDate    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (credKey)
);
