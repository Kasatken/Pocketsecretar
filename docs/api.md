# Сводка HTTP API

Базовый префикс: **`/api`**. Типы содержимого: JSON, кроме загрузки аватара (`multipart/form-data`).

Аутентификация для защищённых методов: заголовок **`Authorization: Bearer <access_token>`**. Обновление сессии: **`POST /api/auth/refresh`** с cookie **`credentials`** (без Bearer).

---

## Health

| Метод | Путь | Auth | Описание |
|-------|------|------|----------|
| GET | `/api/health` | Нет | Проверка доступности сервиса. |

---

## Auth и профиль

| Метод | Путь | Auth | Описание |
|-------|------|------|----------|
| POST | `/api/auth/register` | Нет | Регистрация; тело: email, password, display_name. Выдаёт access + cookie refresh. |
| POST | `/api/auth/login` | Нет | Вход; выдаёт access + cookie. Rate limit. |
| POST | `/api/auth/refresh` | Cookie | Новая пара токенов + user JSON. |
| POST | `/api/auth/logout` | — | Очистка refresh cookie. |
| GET | `/api/auth/me` | Bearer | Текущий пользователь (JSON). |
| PATCH | `/api/auth/me` | Bearer | Частичное обновление профиля (display_name, bio, personalization_note, chronotype). |
| POST | `/api/auth/me/avatar` | Bearer | Загрузка поля `avatar` (файл). |
| POST | `/api/auth/me/promo` | Bearer | Промокод Pro (`code` в JSON). Rate limit. |

Ответы успешного входа/refresh обычно включают поля `access_token` и `user`.

---

## Статика

| Метод | Путь | Auth | Описание |
|-------|------|------|----------|
| GET | `/api/static/avatars/*` | Нет | Файлы аватаров из `UPLOAD_DIR/avatars`. |

---

## Google OAuth

| Метод | Путь | Auth | Описание |
|-------|------|------|----------|
| GET | `/api/auth/google/start` | Cookie refresh | Редирект на Google OAuth (state подписан). |
| GET | `/api/auth/google/callback` | — | Callback OAuth; редирект обратно в приложение. |
| GET | `/api/auth/google/status` | Bearer | Статус подключения календаря. |

---

## Календарь

| Метод | Путь | Auth | Описание |
|-------|------|------|----------|
| GET | `/api/calendar/events` | Bearer | Query: `timeMin`, `timeMax` (ISO). События + `linked`. |
| DELETE | `/api/calendar/events/{id}` | Bearer | Удаление события в Google Calendar. |
| POST | `/api/calendar/events/{id}/complete` | Bearer | Отметить выполненным; возможно начисление XP; возвращает обновлённого `user`. |

---

## ИИ: умные задачи и совет

| Метод | Путь | Auth | Описание |
|-------|------|------|----------|
| POST | `/api/calendar/smart-task` | Bearer | Тело: текст задачи и контекст (например `today`). Может вернуть **202** + `job_id`. |
| GET | `/api/calendar/smart-task?job=…` | Bearer | Статус асинхронной job / результат. |
| GET | `/api/calendar/daily-advice` | Bearer | Query: опционально `refresh=1`. Кэшированный совет дня. |

---

## Бэклог идей

| Метод | Путь | Auth | Описание |
|-------|------|------|----------|
| GET | `/api/backlog/ideas` | Bearer | Список стикеров. |
| POST | `/api/backlog/ideas` | Bearer | Создание (body, color_id, tag). |
| DELETE | `/api/backlog/ideas/{id}` | Bearer | Удаление по UUID. |
| POST | `/api/backlog/ideas/{id}/schedule` | Bearer | Создать событие в календаре на дату (опционально время). |

---

## Пространства (collaboration)

Сводная таблица; подробная семантика, UI и виджет **«Ход дня»** — в **[spaces-and-day-gauge.md](./spaces-and-day-gauge.md)** (фронтенд-only части там же).

| Метод | Путь | Auth | Описание |
|-------|------|------|----------|
| GET | `/api/spaces/calendar-overlays` | Bearer | Query: `time_min`, `time_max` (RFC3339). Принятые предложения текущего пользователя как исполнителя в диапазоне. |
| GET | `/api/spaces/pending-for-me` | Bearer | Ожидающие ответа предложения, где пользователь — исполнитель (`total`, `spaces[]`). |
| GET | `/api/spaces` | Bearer | Список пространств пользователя. |
| POST | `/api/spaces` | Bearer | Создание: `name`, опционально `space_type`, `policy`. |
| POST | `/api/spaces/join` | Bearer | Вступление: `{ "token" }`. |
| GET | `/api/spaces/{id}` | Bearer | Детали пространства для участника. |
| GET | `/api/spaces/{id}/members` | Bearer | Участники. |
| POST | `/api/spaces/{id}/invites` | Bearer | Только **coordinator**: приглашение. |
| GET | `/api/spaces/{id}/proposals` | Bearer | Список предложений. |
| POST | `/api/spaces/{id}/proposals` | Bearer | Создать предложение (не **observer**). |
| POST | `/api/spaces/{id}/proposals/{proposalId}/respond` | Bearer | Ответ исполнителя: `accepted` / `declined`. |

---

## Коды ошибок (общая логика)

- **401** — нет или невалидный Bearer, нет refresh при refresh.
- **403** — например дневной лимит smart-task (free).
- **404** — не найден ресурс (событие, идея).
- **409** — конфликт (например email при регистрации).
- **429** — rate limit.
- **502/503** — проблемы Google API или OpenRouter / сервис не сконфигурирован.

Точные текстовые тела ошибок зависят от обработчика; фронт часто показывает `response.text()` или поля `detail` / `error` из JSON.
