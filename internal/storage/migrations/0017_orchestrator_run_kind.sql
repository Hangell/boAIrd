ALTER TABLE runs ADD COLUMN kind TEXT NOT NULL DEFAULT 'brain' CHECK(kind IN ('brain', 'worker'));

UPDATE runs SET kind = 'worker' WHERE task_id IS NOT NULL AND task_id <> '';
