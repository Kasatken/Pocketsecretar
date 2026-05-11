# Backend (Go)

Модуль Go `pocketsecret` (см. `backend/go.mod`). Точка входа — `backend/main.go`.

## Структура каталогов

| Путь | Назначение |
|------|------------|
| `main.go` | Инициализация БД, репозиториев, HTTP-маршрутов, запуск сервера на `127.0.0.1:$PORT`. |
| `config/` | Загрузка конфигурации из переменных окружения (`config.Load()`). |
| `handlers/` | HTTP-обработчики по доменам: auth, Google OAuth, календарь, smart-task, советы, бэклог, health. |
| `middleware/` | `CORS`, `RequireAuth` (JWT access), rate limit для части auth-операций. |
| `repository/` | Доступ к PostgreSQL (пользователи, Google, бэклог, квоты, советы, completions). |
| `models/` | Доменные структуры (например `User`). |
| `security/` | Хеширование паролей, выпуск и проверка JWT (access, refresh, OAuth state). |
| `migrations/` | Встроенные SQL-миграции (`embed`), runner в `migrations/runner.go`. |

## Слой HTTP

Сервер использует `net/http` и `http.NewServeMux`. С Go 1.22+ для некоторых маршрутов заданы методы и параметры пути (`GET /api/calendar/events`, `DELETE /api/calendar/events/{id}`).

Обёртка цепочки:

1. `middleware.CORS(cfg.AllowedOrigin, mux)` — разрешённый origin и credentials.
2. Для защищённых маршрутов — `middleware.RequireAuth(tokens, handler)` — извлекает `UserID` из access JWT в контекст (`middleware.UserIDContextKey`).

## Аутентификация и профиль

- **Регистрация / логин**: пароль хешируется (`security`), пользователь создаётся/находится в `repository.UserRepository`, выдаётся пара токенов (`handlers.AuthHandler.respondWithSession`).
- **Refresh**: читает cookie `refresh_token`, валидирует как refresh JWT, перевыпускает оба токена.
- **Logout**: очищает cookie.
- **PATCH /api/auth/me**: обновление профиля (имя, bio, персонализация, хронотип) — см. `handlers/profile.go`, валидация хронотипа через `handlers/persona.go` (`ValidChronotype`, подсказки для LLM).
- **POST /api/auth/me/avatar**: загрузка файла в `cfg.UploadDir/avatars`, URL отдаётся в профиле.
- **POST /api/auth/me/promo**: активация Pro по секретному коду (`PRO_PROMO_CODE`), с rate limit.

Публичное представление пользователя в JSON формируется в `handlers/user_json.go` (в т.ч. `smart_tasks_today` для free).

## Google OAuth

`handlers/google.go`:

- `GET /api/auth/google/start` — требует cookie refresh, выпускает `state` (JWT), редирект на Google.
- `GET /api/auth/google/callback` — обмен code, сохранение refresh token Google в `user_google_calendar`.
- `GET /api/auth/google/status` (с Bearer) — статус привязки для UI.

Конфигурация OAuth: `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `GOOGLE_REDIRECT_URI`.

## Календарь и события

`handlers/calendar_events.go`:

- **GET /api/calendar/events** — события за интервал `timeMin`/`timeMax`, флаг `linked`, нормализация под фронт; объединение с локальными отметками «выполнено» из `calendar_event_completions`.
- **DELETE** — удаление события в Google Calendar.
- **POST …/complete** — проверка существования события в Google, запись completion, начисление XP (идемпотентно по `(user_id, google_event_id)`).

## Умные задачи (LLM + Google)

`handlers/smart_task.go` + `handlers/smart_task_jobs.go`:

- POST принимает текст запроса и дату; при длительной обработке отвечает **202** с `job_id`.
- Клиент опрашивает `GET /api/calendar/smart-task?job=…` (реализовано в том же `ServeHTTP` smart-task handler).
- Хранилище job — in-memory (`smartTaskJobStore`).
- Учёт дневной квоты: `repository.SmartTaskRepository`, для free-плана лимит из конфига.

Системный промпт задаёт строгий JSON-выход для парсера и правила создания событий в календаре.

## Ежедневный совет

`handlers/daily_advice.go`:

- Загружает события, строит сигнатуру для кэша, при необходимости вызывает OpenRouter.
- Кэш в `user_daily_advice`; параметр refresh на GET может форсировать пересчёт.

## Бэклог идей

`handlers/backlog_ideas.go`:

- CRUD стикеров в БД.
- **POST …/schedule** — создание события в Google Calendar на выбранную дату (опционально время), без LLM.

## Прочее

- **GET /api/health** — проверка живости.
- **Rate limiting**: см. `middleware/ratelimit.go`, используется в auth (и промо).

## Зависимости (go.mod)

- `github.com/lib/pq` — драйвер PostgreSQL.
- `github.com/golang-jwt/jwt/v5` — JWT.
- `golang.org/x/oauth2` и `google` — OAuth Google.
- `golang.org/x/crypto` — пароли.

Подробнее про переменные окружения — [configuration.md](./configuration.md).

Детали механик (квоты, XP) и модель безопасности — [mechanics-and-security.md](./mechanics-and-security.md).
