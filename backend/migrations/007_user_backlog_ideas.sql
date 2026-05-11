CREATE TABLE IF NOT EXISTS user_backlog_idea (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  body TEXT NOT NULL,
  color_id TEXT NOT NULL DEFAULT '1',
  tag TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT user_backlog_idea_body_len CHECK (char_length(body) BETWEEN 1 AND 2000),
  CONSTRAINT user_backlog_idea_tag_len CHECK (char_length(tag) <= 64)
);

CREATE INDEX IF NOT EXISTS user_backlog_idea_user_created_idx ON user_backlog_idea (user_id, created_at DESC);
