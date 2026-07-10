import { useCallback, useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import {
  fetchCalendarEvents,
  normalizeApiEvent,
  completeCalendarEvent,
  deleteCalendarEvent,
  rescheduleCalendarEvent,
} from '../lib/calendarApi'
import type { NormalizedEvent } from '../lib/calendarUtils'
import s from './MissedTasksReview.module.css'

type Props = {
  accessToken: string | null
  /** Дёргается после любого действия, чтобы главная страница обновила «сегодня». */
  onChanged?: () => void
}

const MAX_SHOWN = 6
const QUARTER_MS = 15 * 60 * 1000

function startOfYesterday(): Date {
  const d = new Date()
  d.setHours(0, 0, 0, 0)
  d.setDate(d.getDate() - 1)
  return d
}

/** Завтра в то же время дня, что и оригинал (предсказуемо и без наложений разных дел). */
function tomorrowSameTime(original: Date): Date {
  const start = new Date()
  start.setDate(start.getDate() + 1)
  start.setHours(original.getHours(), original.getMinutes(), 0, 0)
  return start
}

function whenLabel(start: Date): string {
  const time = start.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })
  const startDay = new Date(start)
  startDay.setHours(0, 0, 0, 0)
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  const diffDays = Math.round((today.getTime() - startDay.getTime()) / (24 * 60 * 60 * 1000))
  if (diffDays <= 0) return `сегодня, ${time}`
  if (diffDays === 1) return `вчера, ${time}`
  return `${start.toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' })}, ${time}`
}

/** Мягкий разбор незакрытых дел (доводчик) — лёгкое модальное окно поверх. Любая ошибка → просто не показываем. */
export default function MissedTasksReview({ accessToken, onChanged }: Props) {
  const [items, setItems] = useState<NormalizedEvent[] | null>(null)
  const [dismissed, setDismissed] = useState(false)
  const [busyId, setBusyId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!accessToken) return
    const token = accessToken
    let cancelled = false
    ;(async () => {
      try {
        const now = new Date()
        const { events } = await fetchCalendarEvents(token, startOfYesterday(), now)
        if (cancelled) return
        const missed = events
          .map(normalizeApiEvent)
          .filter((e) => !e.completed && !e.allDay && !e.isSmartBreak && e.end.getTime() < now.getTime())
          .sort((a, b) => a.start.getTime() - b.start.getTime())
        setItems(missed)
      } catch {
        if (!cancelled) setItems([])
      }
    })()
    return () => {
      cancelled = true
    }
  }, [accessToken])

  const removeItem = useCallback(
    (id: string) => {
      setItems((prev) => (prev ? prev.filter((e) => e.id !== id) : prev))
      onChanged?.()
    },
    [onChanged],
  )

  const runAction = useCallback(
    async (id: string, fn: () => Promise<void>) => {
      if (!accessToken || busyId) return
      setBusyId(id)
      setError(null)
      try {
        await fn()
        removeItem(id)
      } catch (e) {
        setError(e instanceof Error ? e.message : 'Не получилось. Попробуй ещё раз.')
      } finally {
        setBusyId(null)
      }
    },
    [accessToken, busyId, removeItem],
  )

  const open = !(!accessToken || dismissed || items === null || items.length === 0)

  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setDismissed(true)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open])

  if (!open) return null

  const token = accessToken
  const shown = items.slice(0, MAX_SHOWN)
  const extra = items.length - shown.length

  return createPortal(
    <div className={s.overlay} role="dialog" aria-modal="true" aria-label="Незакрытые дела">
      <button type="button" className={s.backdrop} aria-label="Закрыть" onClick={() => setDismissed(true)} />
      <div className={s.panel}>
        <div className={s.head}>
          <div className={s.headText}>
            <h2 className={s.title}>Осталось незакрытым</h2>
            <p className={s.subtitle}>Бывает. Быстро разберём — и с плеч долой.</p>
          </div>
          <button type="button" className={s.close} onClick={() => setDismissed(true)} aria-label="Закрыть">
            ×
          </button>
        </div>

        <ul className={s.list}>
          {shown.map((ev) => {
            const busy = busyId === ev.id
            return (
              <li key={ev.id} className={s.row} data-busy={busy || undefined}>
                <div className={s.rowText}>
                  <span className={s.rowTitle}>{ev.title}</span>
                  <span className={s.rowWhen}>{whenLabel(ev.start)}</span>
                </div>
                <div className={s.actions}>
                  <button
                    type="button"
                    className={`${s.btn} ${s.btnDone}`}
                    disabled={busy}
                    onClick={() => runAction(ev.id, () => completeCalendarEvent(token, ev.id).then(() => undefined))}
                  >
                    Сделал
                  </button>
                  <button
                    type="button"
                    className={`${s.btn} ${s.btnMove}`}
                    disabled={busy}
                    onClick={() =>
                      runAction(ev.id, () => {
                        const start = tomorrowSameTime(ev.start)
                        const durationMs = Math.max(QUARTER_MS, ev.end.getTime() - ev.start.getTime())
                        const end = new Date(start.getTime() + durationMs)
                        return rescheduleCalendarEvent(token, ev.id, start, end)
                      })
                    }
                  >
                    Завтра
                  </button>
                  <button
                    type="button"
                    className={`${s.btn} ${s.btnDrop}`}
                    disabled={busy}
                    onClick={() => runAction(ev.id, () => deleteCalendarEvent(token, ev.id))}
                  >
                    Убрать
                  </button>
                </div>
              </li>
            )
          })}
        </ul>

        {extra > 0 && <p className={s.more}>…и ещё {extra}</p>}
        {error && (
          <p className={s.error} role="status">
            {error}
          </p>
        )}

        <button type="button" className={s.later} onClick={() => setDismissed(true)}>
          Позже
        </button>
      </div>
    </div>,
    document.body,
  )
}
