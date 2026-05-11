-- Короткая заметка и хронотип для умных задач / советов
ALTER TABLE users ADD COLUMN IF NOT EXISTS personalization_note TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS chronotype TEXT NOT NULL DEFAULT '';
