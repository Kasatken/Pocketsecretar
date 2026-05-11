# База данных и миграции

СУБД: **PostgreSQL**. Подключение через `DATABASE_URL` (формат libpq).

## Механизм миграций

- Файлы: `backend/migrations/*.sql`, вшиты в бинарник через `//go:embed`.
- Таблица учёта: `schema_migrations` (создаётся в `runner.go`, если ещё нет).
- Миграции применяются **по имени файла в лексикографическом порядке** (`001_…`, `002_…`, …), каждая в транзакции.

При старте приложения вызывается `migrations.Run` из `main.go` — новый инстанс БД поднимается с актуальной схемой автоматически.

## Логическая схема (по миграциям)

### `users`

Ядро профиля:

- Идентификация: `email` (уникальный), `password_hash`.
- Профиль: `display_name`, `avatar_url`, `bio`.
- Геймификация: `experience_points`, `level`.
- Тариф: `plan` — `free` или `pro` (CHECK).
- Персонализация для LLM: `personalization_note`, `chronotype`.
- Служебные: `created_at`, `updated_at`, `last_login_at`.

Лишние поля профиля (`headline`, `location`, …) удалены миграцией `005_drop_profile_extras.sql`.

### `user_google_calendar`

Один ряд на пользователя: привязка Google (`google_sub`), подсказка email, **refresh token** Google для вызовов Calendar API.

### `user_daily_smart_tasks`

Счётчик smart-task на пользователя и календарный **день** (UTC): квота free-плана.

### `user_backlog_idea`

Стикеры бэклога: UUID, текст, `color_id`, `tag`, время создания.

### `user_daily_advice`

Кэш текста совета на `(user_id, day)` + `events_sig` для инвалидации при изменении набора событий.

### `calendar_event_completions`

Отметка выполнения события Google по `google_event_id` на пользователя; используется для XP и UI «выполнено».

## Индексы

Создаются в тех же SQL-файлах (email пользователя, даты для квот/советов, user_id для бэклога и completions — см. файлы миграций).

## Резервное копирование

Для бэкапа достаточно стандартных средств PostgreSQL (`pg_dump`). Отдельно учитывайте каталог загрузок аватаров на диске (`UPLOAD_DIR`), он не в БД.
