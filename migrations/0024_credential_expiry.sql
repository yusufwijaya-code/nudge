-- Personal Jira token expiry (the date Atlassian shows on the API tokens page).
-- The token string is opaque and Jira's API does not report it, so the user/admin
-- enters it. NULL = unknown: no expiry banner.
ALTER TABLE tr_usercredential
  ADD COLUMN expiresAt DATE NULL COMMENT 'Day the personal Jira API token expires (entered by the user)';
