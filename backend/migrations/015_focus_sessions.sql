-- Совместная фокус-сессия — синхронное событие, а не запись в календаре: у неё есть
-- момент старта, участники и общий итог. Существующие таблицы событий для этого не подходят,
-- события пользователя живут в Google Calendar.
CREATE TABLE IF NOT EXISTS focus_sessions (
  id BIGSERIAL PRIMARY KEY,
  starter_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  goal TEXT NOT NULL DEFAULT '',
  starts_at TIMESTAMPTZ,
  duration_min INT NOT NULL DEFAULT 50,
  -- draft: цель названа, время ещё не выбрано (черновик живёт между двумя сообщениями бота).
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'scheduled', 'running', 'finished', 'cancelled')),
  share_token TEXT NOT NULL UNIQUE,
  -- notified_soon: напоминание за 5 минут отправлено (тикер не должен слать его дважды).
  notified_soon BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_focus_sessions_due ON focus_sessions (starts_at)
  WHERE status IN ('scheduled', 'running');

CREATE TABLE IF NOT EXISTS focus_participants (
  session_id BIGINT NOT NULL REFERENCES focus_sessions(id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  -- done: NULL — на вопрос «получилось?» ещё не ответили.
  done BOOLEAN,
  joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (session_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_focus_participants_user ON focus_participants (user_id);
