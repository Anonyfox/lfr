-- Preserve existing health pauses; future pauses record their origin explicitly.
ALTER TABLE lists ADD COLUMN pause_auto INTEGER NOT NULL DEFAULT 0;
UPDATE lists SET pause_auto = 1
WHERE paused LIKE 'bounce rate %' OR paused LIKE 'complaint rate %';
