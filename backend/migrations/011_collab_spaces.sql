CREATE TABLE IF NOT EXISTS spaces (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  space_type TEXT NOT NULL CHECK (space_type IN ('work', 'personal', 'custom')),
  policy TEXT NOT NULL DEFAULT 'proposals_only' CHECK (policy IN ('proposals_only', 'direct_ok')),
  created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_spaces_created_by ON spaces (created_by);

CREATE TABLE IF NOT EXISTS space_members (
  space_id BIGINT NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK (role IN ('coordinator', 'member', 'observer')),
  share_access TEXT NOT NULL CHECK (share_access IN ('full', 'partial', 'view_only')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (space_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_space_members_user_id ON space_members (user_id);

CREATE TABLE IF NOT EXISTS space_invites (
  id BIGSERIAL PRIMARY KEY,
  space_id BIGINT NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  token TEXT NOT NULL UNIQUE,
  invited_email TEXT NOT NULL DEFAULT '',
  role TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('coordinator', 'member', 'observer')),
  share_access TEXT NOT NULL DEFAULT 'partial' CHECK (share_access IN ('full', 'partial', 'view_only')),
  created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_space_invites_space_id ON space_invites (space_id);

CREATE TABLE IF NOT EXISTS task_proposals (
  id BIGSERIAL PRIMARY KEY,
  space_id BIGINT NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  author_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  assignee_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  proposed_start TIMESTAMPTZ,
  proposed_end TIMESTAMPTZ,
  payload JSONB NOT NULL DEFAULT '{}',
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'declined', 'superseded')),
  assignee_note TEXT NOT NULL DEFAULT '',
  google_event_id TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_task_proposals_space_id ON task_proposals (space_id);
CREATE INDEX IF NOT EXISTS idx_task_proposals_assignee_pending ON task_proposals (assignee_id) WHERE status = 'pending';
