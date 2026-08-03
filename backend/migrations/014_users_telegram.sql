-- Вход через Telegram. email/password_hash остаются NOT NULL: пользователям, пришедшим
-- из бота, пишутся синтетические значения (tg<id>@telegram.local и пустой хеш), поэтому
-- вход по паролю для таких аккаунтов невозможен, а существующие запросы не меняются.
ALTER TABLE users ADD COLUMN IF NOT EXISTS telegram_id BIGINT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS telegram_username TEXT NOT NULL DEFAULT '';

-- invited_by и join_source существуют ради единственной метрики запуска — сколько новых
-- людей приводит один активный пользователь. Без них проверить петлю роста нечем.
ALTER TABLE users ADD COLUMN IF NOT EXISTS invited_by BIGINT REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS join_source TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS users_telegram_id_idx ON users (telegram_id) WHERE telegram_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS users_invited_by_idx ON users (invited_by) WHERE invited_by IS NOT NULL;
