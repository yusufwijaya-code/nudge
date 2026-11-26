-- Geser otomatis ke selesai: the user's switch (Pengaturan > Umum). Off until the
-- user turns it on, and only works with a personal Jira API key.
ALTER TABLE ms_user
  ADD COLUMN autoDoneEnabled TINYINT(1) NOT NULL DEFAULT 0 COMMENT '1 = move a ticket to its board''s done status when its time is up';
