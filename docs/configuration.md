# Конфигурация и запуск

## Требования

- **Go** 1.22+ (см. `backend/go.mod`).
- **Node.js** + npm для frontend.
- **PostgreSQL** и созданная БД; строка подключения в `DATABASE_URL`.

## Backend

Рабочий каталог: `backend`.

```bash
export DATABASE_URL='postgres://user:pass@localhost:5432/pocketsecret?sslmode=disable'
go run .
```

Сервер слушает **`127.0.0.1:$PORT`** (по умолчанию порт **8080**).

### Обязательные и важные переменные

| Переменная | Описание |
|------------|----------|
| `DATABASE_URL` | **Обязательно.** DSN PostgreSQL. Без неё процесс завершится с ошибкой. |
| `JWT_ACCESS_SECRET` | Секрет подписи access JWT (в продакшене задать явно). |
| `JWT_REFRESH_SECRET` | Секрет подписи refresh JWT и OAuth `state`. |
| `CORS_ALLOWED_ORIGIN` | Origin фронтенда, с которого разрешены credentialed-запросы (по умолчанию `http://localhost:5173`). |
| `COOKIE_SECURE` | `true`, если cookie только по HTTPS (продакшен). |

### Google Calendar / OAuth

| Переменная | Описание |
|------------|----------|
| `GOOGLE_CLIENT_ID` | OAuth client ID. |
| `GOOGLE_CLIENT_SECRET` | OAuth secret. |
| `GOOGLE_REDIRECT_URI` | Должен совпадать с настройкой в Google Cloud Console. |

Без этих трёх функции календаря и привязки Google недоступны (handlers возвращают 503/400 где уместно).

### OpenRouter (ИИ)

| Переменная | Описание |
|------------|----------|
| `OPENROUTER_API_KEY` | Ключ API. |
| `OPENROUTER_MODEL` | Модель для smart-task (по умолчанию `openai/gpt-4o-mini`). |
| `OPENROUTER_MODEL_ADVICE` | Отдельная модель для совета (опционально). |
| `OPENROUTER_REFERER` | Заголовок/Referer для OpenRouter. |
| `OPENROUTER_PROXY` | Опциональный HTTP-прокси для исходящих запросов. |
| `SMART_TASK_MAX_TOKENS` | Лимит токенов ответа для smart-task. |
| `ADVICE_MAX_TOKENS` | Лимит для совета. |
| `FREE_SMART_TASK_LIMIT` | Максимум smart-task в день для `plan=free`. |

### Прочее

| Переменная | Описание |
|------------|----------|
| `ACCESS_TTL_MIN` | TTL access JWT в минутах (по умолчанию 15). |
| `REFRESH_TTL_DAYS` | TTL refresh cookie/токена в днях (по умолчанию 14). |
| `UPLOAD_DIR` | Каталог для файлов (аватары в подпапке `avatars`). |
| `AVATAR_MAX_BYTES` | Максимальный размер файла аватара. |
| `PRO_PROMO_CODE` | Секретная строка; совпадение активирует Pro через `POST /api/auth/me/promo`. |

Дефолты для секретов JWT в коде предназначены только для разработки — **не использовать в продакшене**.

## Frontend

Рабочий каталог: `frontend`.

```bash
npm install
npm run dev
```

Dev-сервер проксирует `/api` → `http://127.0.0.1:8080`. Убедитесь, что backend запущен.

Сборка:

```bash
npm run build
```

Результат в `frontend/dist` — раздаётся статическим сервером; API должен быть на том же хосте по префиксу `/api` или настроен reverse proxy.

## Типичные проблемы

- **CORS** — origin браузера должен **точно** совпасть с `CORS_ALLOWED_ORIGIN` (включая схему и порт).
- **Долгие smart-task** — в dev таймаут прокси увеличен в Vite; за прокси nginx может понадобиться `proxy_read_timeout`.
- **Google redirect mismatch** — `GOOGLE_REDIRECT_URI` и консоль Google должны совпадать побайтово.
