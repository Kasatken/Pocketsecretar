# Frontend (React + TypeScript + Vite)

Пакет `frontend`, сборка через **Vite 5**, UI на **React 18** и **react-router-dom 6**.

## Структура `frontend/src`

| Путь | Назначение |
|------|------------|
| `main.tsx` | Точка входа: `AuthProvider`, `BrowserRouter`, `Router`, глобальные стили, favicon из брендинга. |
| `router.tsx` | Маршруты: лендинг, логин, юридические страницы, защищённая зона `/app`, `/calendar`, `/spaces`, `/profile`. |
| `auth/AuthContext.tsx` | Сессия: user, accessToken, login/register/logout, refresh, PATCH профиля, аватар, промокод. Все auth-запросы на `/api/auth/*`. |
| `layout/AppLayout.tsx` | Оболочка для авторизованных страниц: шапка с навигацией «Сегодня / Календарь / Пространства», бейдж ожидающих предложений, опциональная полоска, ссылка на профиль, футер. |
| `pages/` | Экраны: `Landing`, `AppAuthPage`, `AppPage` («Сегодня»), `CalendarPage`, `SpacesPage`, `SpacePage`, `ProfilePage`, `PrivacyPage`, `PersonalDataPage`. |
| `components/` | Переиспользуемые блоки: бэклог, прогресс уровня, футер сайта, бейджи событий из пространств и др. |
| `lib/` | API-клиенты (`calendarApi.ts`, `backlogApi.ts`, `spacesApi.ts`), утилиты календаря, слияние принятых предложений с событиями, пресеты smart-task, брендинг, хронотипы. |
| `hooks/` | Например `usePomodoro.ts`. |

Стили — CSS Modules рядом с компонентами (например `AppLayout.module.css`, `ProfilePage.module.css`).

## Маршрутизация и доступ

- **`RequireAuth`** — пока `ready === false`, ничего не рендерит; если пользователя нет — редирект на `/login`.
- **`LoginRoute`** — авторизованных отправляет на `/app`.
- Редиректы совместимости: `/app/profile` → `/profile`, `/app/calendar` → `/calendar`.
- Неизвестные пути → `/`.

## Работа с API

- В development запросы к **`/api`** проксируются на backend (`vite.config.ts`, таймаут 300 с для долгих smart-task).
- Защищённые запросы передают `Authorization: Bearer ${accessToken}` и `credentials: 'include'` для cookie refresh.
- Типичный паттерн: страница получает `accessToken` из `useAuth()`, вызывает функции из `lib/calendarApi.ts`, `lib/backlogApi.ts` или `lib/spacesApi.ts`.

Пространства и виджет **«Ход дня»** на «Сегодня» описаны в **[spaces-and-day-gauge.md](./spaces-and-day-gauge.md)**.

### Особенности smart-task на клиенте

`postSmartTask` в `calendarApi.ts`:

- Если ответ **202**, читает `job_id` и поллит `GET /api/calendar/smart-task?job=…` до статуса `done` или `error`.
- Ошибки завёрнуты в `SmartTaskError` с HTTP-статусом и телом.

### Ошибки 401

Многие функции API при 401 бросают ошибку с `name === 'Unauthorized'` — страницы могут инициировать повторный `refreshSession` или выход.

## Брендинг

`lib/branding.ts` — имя приложения, URL логотипа (используется в шапке и как favicon в `main.tsx`).

## Сборка

- `npm run dev` — dev-сервер Vite (по умолчанию порт 5173).
- `npm run build` — артефакты в `frontend/dist`.
- `publicDir: false` — статика не из каталога `public/`, а импортируется из кода.

В продакшене обычно отдают `dist` через nginx/CDN и проксируют `/api` на Go-сервер; origin для CORS должен совпадать с `CORS_ALLOWED_ORIGIN` на backend.
