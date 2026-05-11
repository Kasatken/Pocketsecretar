import type { SmartTaskToday } from './calendarApi'

export function parseSmartTasksToday(payload: unknown): SmartTaskToday | null {
  if (!payload || typeof payload !== 'object') return null
  const o = payload as Record<string, unknown>
  const st = o.smart_tasks_today
  if (!st || typeof st !== 'object') return null
  const q = st as Record<string, unknown>
  const used = typeof q.used === 'number' ? q.used : Number(q.used)
  const limit = typeof q.limit === 'number' ? q.limit : Number(q.limit)
  const resets_at = typeof q.resets_at === 'string' ? q.resets_at.trim() : ''
  if (!Number.isFinite(used) || !Number.isFinite(limit) || resets_at === '') return null
  return { used, limit, resets_at }
}

/** Локальное время следующего сброса дневного лимита (полночь UTC). */
export function formatQuotaResetsAtHint(resetsAtISO: string): string {
  const t = Date.parse(resetsAtISO)
  if (!Number.isFinite(t)) return ''
  const when = new Date(t)
  const formatted = when.toLocaleString('ru-RU', {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
    hour: '2-digit',
    minute: '2-digit',
    timeZoneName: 'short',
  })
  return `Лимит обновится: ${formatted} (отсчёт дня по UTC).`
}
