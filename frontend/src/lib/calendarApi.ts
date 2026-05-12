import type { User } from '../auth/AuthContext'
import type { NormalizedEvent } from './calendarUtils'

export type CalendarEventsResponse = {
  linked: boolean
  events: ApiCalendarEvent[]
}

export type ApiCalendarEvent = {
  id: string
  title: string
  start: string
  end: string
  all_day: boolean
  color_id?: string
  completed?: boolean
  is_break?: boolean
  /** Из Google Calendar private extendedProperties (события из предложения пространства). */
  space_proposal_id?: string
}

export function normalizeApiEvent(raw: ApiCalendarEvent): NormalizedEvent {
  const title = raw.title || 'Без названия'
  const isBreak =
    raw.is_break === true || /^\s*☕\s*️?/u.test(title)
  let spaceProposalId: number | undefined
  if (raw.space_proposal_id != null && String(raw.space_proposal_id).trim() !== '') {
    const n = parseInt(String(raw.space_proposal_id).trim(), 10)
    if (Number.isFinite(n) && n > 0) spaceProposalId = n
  }
  return {
    id: raw.id,
    title,
    start: new Date(raw.start),
    end: new Date(raw.end),
    allDay: raw.all_day,
    colorId: raw.color_id,
    completed: raw.completed === true,
    isSmartBreak: isBreak,
    spaceProposalId,
  }
}

export async function fetchCalendarEvents(
  accessToken: string,
  timeMin: Date,
  timeMax: Date,
): Promise<CalendarEventsResponse> {
  const params = new URLSearchParams({
    timeMin: timeMin.toISOString(),
    timeMax: timeMax.toISOString(),
  })
  const res = await fetch(`/api/calendar/events?${params}`, {
    headers: { Authorization: `Bearer ${accessToken}` },
    credentials: 'include',
  })

  if (res.status === 401) {
    const err = new Error('401')
    err.name = 'Unauthorized'
    throw err
  }

  if (res.status === 502) {
    const err = await res.json().catch(() => ({}))
    throw new Error(typeof err.error === 'string' ? err.error : 'Не удалось загрузить календарь')
  }

  if (!res.ok) {
    throw new Error('Запрос календаря не выполнен')
  }

  const data = (await res.json()) as CalendarEventsResponse
  return {
    linked: Boolean(data.linked),
    events: Array.isArray(data.events) ? data.events : [],
  }
}

export type SmartTaskToday = {
  used: number
  limit: number
  resets_at: string
}

export type SmartTaskResponse = {
  ok: boolean
  detail?: string
  error?: string
  daily_limit_truncated?: boolean
  smart_tasks_today?: SmartTaskToday
  tasks?: Array<{
    title: string
    date: string
    time: string
    end_time?: string
    priority?: string
    methodology?: string
  }>
}

export class SmartTaskError extends Error {
  status: number
  payload: unknown
  constructor(message: string, status: number, payload: unknown) {
    super(message)
    this.name = 'SmartTaskError'
    this.status = status
    this.payload = payload
  }
}

export type DailyAdviceResponse = {
  linked: boolean
  advice: string | null
  cached?: boolean
  events_count?: number
  detail?: string
  /** Сервер вернул сохранённый совет из-за сбоя генерации */
  stale?: boolean
  retry_suggest?: string
}

export async function fetchDailyAdvice(
  accessToken: string,
  opts?: { refresh?: boolean },
): Promise<DailyAdviceResponse> {
  const q = opts?.refresh ? '?refresh=1' : ''
  const res = await fetch(`/api/calendar/daily-advice${q}`, {
    headers: { Authorization: `Bearer ${accessToken}` },
    credentials: 'include',
  })

  if (res.status === 401) {
    const err = new Error('401')
    err.name = 'Unauthorized'
    throw err
  }

  const data = (await res.json().catch(() => ({}))) as DailyAdviceResponse & Record<string, unknown>

  if (res.status === 503) {
    return {
      linked: true,
      advice: null,
      detail: typeof data.detail === 'string' ? data.detail : 'Сервис советов временно недоступен',
    }
  }

  if (res.status === 502 || res.status === 500) {
    return {
      linked: true,
      advice: null,
      detail: typeof data.detail === 'string' ? data.detail : 'Не удалось получить совет',
    }
  }

  if (!res.ok) {
    return {
      linked: Boolean(data.linked),
      advice: null,
      detail: 'Запрос совета не выполнен',
    }
  }

  return {
    linked: Boolean(data.linked),
    advice:
      typeof data.advice === 'string' && data.advice.trim() !== '' ? data.advice.trim() : null,
    cached: data.cached === true,
    events_count: typeof data.events_count === 'number' ? data.events_count : undefined,
    stale: data.stale === true,
    retry_suggest: typeof data.retry_suggest === 'string' ? data.retry_suggest : undefined,
  }
}

export async function postSmartTask(
  accessToken: string,
  text: string,
  today: string,
): Promise<SmartTaskResponse> {
  const meta = import.meta as unknown as { env?: { DEV?: boolean } }
  const debugTiming = meta.env?.DEV === true
  const t0 = performance.now()
  let postAcceptMs = 0

  const pollSmartTask = async (jobId: string): Promise<SmartTaskResponse> => {
    const pollUrl = `/api/calendar/smart-task?job=${encodeURIComponent(jobId)}`
    const maxPolls = 240
    const delayMs = 500
    const pollT0 = performance.now()
    for (let i = 0; i < maxPolls; i++) {
      const pr = await fetch(pollUrl, {
        headers: { Authorization: `Bearer ${accessToken}` },
        credentials: 'include',
      })
      if (pr.status === 401) {
        const err = new Error('401')
        err.name = 'Unauthorized'
        throw err
      }
      const p = (await pr.json().catch(() => ({}))) as Record<string, unknown>
      if (p.status === 'done') {
        const { status: _st, ...rest } = p
        if (debugTiming) {
          const pollMs = Math.round(performance.now() - pollT0)
          const totalMs = Math.round(performance.now() - t0)
          console.log('[smart-task timing]', {
            totalMs,
            postAcceptMs,
            pollMs,
            polls: i + 1,
          })
        }
        return rest as SmartTaskResponse
      }
      if (p.status === 'error') {
        const code = typeof p.http_status === 'number' ? p.http_status : 500
        const d =
          typeof p.detail === 'string'
            ? p.detail
            : typeof p.error === 'string'
              ? p.error
              : 'Ошибка'
        throw new SmartTaskError(d, code, p)
      }
      await new Promise((r) => setTimeout(r, delayMs))
    }
    throw new SmartTaskError('Превышено время ожидания ответа ИИ', 504, {})
  }

  const res = await fetch('/api/calendar/smart-task', {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${accessToken}`,
      'Content-Type': 'application/json',
    },
    credentials: 'include',
    body: JSON.stringify({
      text,
      today,
    }),
  })

  if (res.status === 401) {
    const err = new Error('401')
    err.name = 'Unauthorized'
    throw err
  }

  if (res.status === 202) {
    const start = (await res.json().catch(() => ({}))) as { job_id?: string }
    const jobId = typeof start.job_id === 'string' ? start.job_id : ''
    if (!jobId) {
      throw new SmartTaskError('Некорректный ответ сервера (нет job_id)', 500, start)
    }
    postAcceptMs = Math.round(performance.now() - t0)
    return pollSmartTask(jobId)
  }

  const data = (await res.json().catch(() => ({}))) as SmartTaskResponse & Record<string, unknown>
  postAcceptMs = Math.round(performance.now() - t0)

  if (res.status === 403 && data.error === 'daily_limit') {
    throw new SmartTaskError('Дневной лимит исчерпан', 403, data)
  }
  if (res.status === 400) {
    const d = typeof data.detail === 'string' ? data.detail : 'Некорректный запрос'
    throw new SmartTaskError(d, 400, data)
  }
  if (res.status === 503) {
    throw new SmartTaskError('Сервис умных задач недоступен', 503, data)
  }
  if (res.status === 502 || res.status === 500) {
    const d = typeof data.detail === 'string' ? data.detail : 'Ошибка сервера'
    throw new SmartTaskError(d, res.status, data)
  }
  if (!res.ok) {
    const d = typeof data.detail === 'string' ? data.detail : 'Запрос не выполнен'
    throw new SmartTaskError(d, res.status, data)
  }
  if (debugTiming) {
    console.log('[smart-task timing]', {
      totalMs: Math.round(performance.now() - t0),
      postAcceptMs,
      pollMs: undefined,
      polls: undefined,
      sync: true,
    })
  }
  return data
}

export async function deleteCalendarEvent(accessToken: string, eventId: string): Promise<void> {
  const res = await fetch(`/api/calendar/events/${encodeURIComponent(eventId)}`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${accessToken}` },
    credentials: 'include',
  })

  if (res.status === 401) {
    const err = new Error('401')
    err.name = 'Unauthorized'
    throw err
  }

  if (res.status === 404) {
    throw new Error('Событие не найдено или уже удалено')
  }

  if (res.status === 400) {
    const errBody = (await res.json().catch(() => ({}))) as { error?: string }
    throw new Error(typeof errBody.error === 'string' ? errBody.error : 'Некорректный запрос')
  }

  if (res.status === 502 || res.status === 500) {
    const errBody = (await res.json().catch(() => ({}))) as { error?: string }
    throw new Error(typeof errBody.error === 'string' ? errBody.error : 'Не удалось удалить событие')
  }

  if (!res.ok) {
    throw new Error('Запрос не выполнен')
  }
}

export type CompleteCalendarEventResponse = {
  ok: boolean
  already_completed: boolean
  user: User
}

export async function completeCalendarEvent(
  accessToken: string,
  eventId: string,
): Promise<CompleteCalendarEventResponse> {
  const res = await fetch(`/api/calendar/events/${encodeURIComponent(eventId)}/complete`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${accessToken}` },
    credentials: 'include',
  })

  if (res.status === 401) {
    const err = new Error('401')
    err.name = 'Unauthorized'
    throw err
  }

  if (res.status === 404) {
    throw new Error('Событие не найдено')
  }

  const data = (await res.json().catch(() => ({}))) as CompleteCalendarEventResponse & { error?: string }

  if (res.status === 400) {
    throw new Error(typeof data.error === 'string' ? data.error : 'Некорректный запрос')
  }

  if (res.status === 502 || res.status === 500) {
    throw new Error(typeof data.error === 'string' ? data.error : 'Не удалось отметить событие')
  }

  if (!res.ok || !data.user) {
    throw new Error('Запрос не выполнен')
  }

  return {
    ok: Boolean(data.ok),
    already_completed: Boolean(data.already_completed),
    user: data.user,
  }
}
