-- Admin roster: users the owner (ADMIN_EMAIL) promoted to admin. The owner is always an
-- admin and is not stored here; 1 = promoted admin.
ALTER TABLE ms_user
  ADD COLUMN isAdmin TINYINT(1) NOT NULL DEFAULT 0 COMMENT '1 = promoted to admin by the owner';
