CREATE TABLE IF NOT EXISTS user_daily_advice (
  user_id BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  day DATE NOT NULL,
  advice TEXT NOT NULL,
  events_sig TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (user_id, day)
);

CREATE INDEX IF NOT EXISTS user_daily_advice_day_idx ON user_daily_advice (day);
