CREATE TABLE IF NOT EXISTS user_google_calendar (
  user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  google_sub TEXT NOT NULL UNIQUE,
  email_hint TEXT NOT NULL DEFAULT '',
  refresh_token TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS user_google_calendar_google_sub_idx ON user_google_calendar (google_sub);
