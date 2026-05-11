CREATE TABLE IF NOT EXISTS user_daily_smart_tasks (
  user_id BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  day DATE NOT NULL,
  task_count INT NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, day)
);

CREATE INDEX IF NOT EXISTS user_daily_smart_tasks_day_idx ON user_daily_smart_tasks (day);
