-- Aturan Board > Jadwal yang tidak dihitung: the Daily Stand Up (per weekday) and
-- rituals (Sprint Planning / Grooming...) whose hours do not count toward a ticket's
-- time. {"dsu":[{weekday,start,minutes}],"rituals":[{name,enabled,weekday,start,
-- minutes,frequency,sprintWeek}]}. NULL = nothing excluded.
ALTER TABLE ms_tracking_rule
  ADD COLUMN excludedSchedule JSON NULL COMMENT 'DSU and rituals that are not counted toward ticket time';
