CREATE TABLE IF NOT EXISTS calendar_event_completions (
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  google_event_id TEXT NOT NULL,
  completed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (user_id, google_event_id)
);

CREATE INDEX IF NOT EXISTS idx_calendar_event_completions_user_id
  ON calendar_event_completions (user_id);
