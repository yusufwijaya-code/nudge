-- Admin › Pengaturan Aplikasi: application-wide values an admin can change (one row
-- per setting). No row = the built-in default applies (deviation cap: 12%).
CREATE TABLE IF NOT EXISTS ms_app_setting (
  settingKey   VARCHAR(64)  NOT NULL COMMENT 'e.g. deviation_cap_percent',
  settingValue VARCHAR(255) NOT NULL,
  updatedBy    VARCHAR(255) NOT NULL COMMENT 'Admin email',
  updatedDate  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (settingKey)
);
