# Документация Pocketsecret

Веб-приложение для планирования дня с интеграцией **Google Calendar**, ИИ-функциями (**OpenRouter**) и геймификацией (опыт, уровень). Репозиторий разделён на **backend** (Go, PostgreSQL, REST API) и **frontend** (React, TypeScript, Vite).

## Оглавление

| Документ | Содержание |
|----------|------------|
| [architecture.md](./architecture.md) | Общая архитектура, потоки данных, внешние сервисы |
| [backend.md](./backend.md) | Структура Go-кода, слои, маршруты, безопасность |
| [frontend.md](./frontend.md) | React-приложение, маршрутизация, состояние, UI |
| [database.md](./database.md) | Схема БД, миграции |
| [configuration.md](./configuration.md) | Переменные окружения и локальный запуск |
| [api.md](./api.md) | Сводка HTTP API для быстрой ориентации |
| [mechanics-and-security.md](./mechanics-and-security.md) | **Механики** (XP, Pro, smart-task, совет, помидоро) и **безопасность** (токены, CORS, лимиты, файлы) |
| [spaces-and-day-gauge.md](./spaces-and-day-gauge.md) | **Пространства** (collaboration, предложения, календарь, уведомления) и **«Ход дня»** на экране «Сегодня» |

## Быстрый старт для нового разработчика

1. Прочитайте [architecture.md](./architecture.md) — картина целиком.
2. Ознакомьтесь с [mechanics-and-security.md](./mechanics-and-security.md) — правила игры и угрозная модель.
3. При работе с **пространствами** и виджетом **«Ход дня»** откройте [spaces-and-day-gauge.md](./spaces-and-day-gauge.md).
4. Поднимите PostgreSQL и задайте `DATABASE_URL`; см. [configuration.md](./configuration.md).
5. Запустите backend из каталога `backend`, frontend из `frontend` (прокси `/api` на бэкенд).
6. Для календаря и умных задач настройте Google OAuth и при необходимости OpenRouter.

## Термины

- **Access token** — JWT в заголовке `Authorization: Bearer …`, короткоживущий.
- **Refresh token** — JWT в HttpOnly-cookie `refresh_token`, обновляет сессию.
- **Smart task** — запрос к LLM с последующим созданием событий в Google Calendar (с квотой для free-плана).
- **Daily advice** — ежедневный совет на основе календаря, кэшируется по дню.
