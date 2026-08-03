import { createPortal } from 'react-dom'
import { useCallback, useEffect, useId, useMemo, useRef, useState, type DragEvent, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Check, Loader2, Trash2 } from 'lucide-react'
import { useAuth } from '../auth/AuthContext'
import { BacklogIdeasPanel } from '../components/BacklogIdeasPanel'
import AppSessionLoading from '../components/AppSessionLoading'
import { SpaceProposalCalendarBadge } from '../components/SpaceProposalCalendarBadge'
import { BACKLOG_DRAG_MIME, type ScheduleBacklogOptions } from '../lib/backlogApi'
import {
  parseBacklogDragPayload,
  scheduleBacklogViaSmartTask,
  BACKLOG_DRAG_PAYLOAD_MIME,
} from '../lib/backlogSmartSchedule'
import {
  completeCalendarEvent,
  deleteCalendarEvent,
  fetchCalendarEvents,
  normalizeApiEvent,
  postSmartTask,
  SmartTaskError,
} from '../lib/calendarApi'
import {
  addDays,
  addMonths,
  addWeeks,
  buildMonthGridCells,
  dayKey,
  eventsByDayKey,
  formatEventTimeRange,
  formatCalendarHeroMonthYear,
  formatWeekRange,
  getMonthGridRange,
  getWeekRange,
  sortDayEvents,
  startOfWeekMonday,
  type NormalizedEvent,
} from '../lib/calendarUtils'
import { isSpaceProposalGhostEvent, mergeAcceptedSpaceProposalsIntoEvents } from '../lib/mergeAcceptedSpaceProposals'
import { fetchAcceptedCalendarOverlays } from '../lib/spacesApi'
import {
  loadSmartTaskPresets,
  newSmartPresetId,
  saveSmartTaskPresets,
  type SmartTaskPreset,
} from '../lib/smartTaskPresets'
import { formatQuotaResetsAtHint, parseSmartTasksToday } from '../lib/smartTaskQuota'
import { useCompletionBurst } from '../components/CompletionBurst'
import s from './CalendarPage.module.css'

function smartTaskUserMessage(err: unknown): string {
  if (err instanceof SmartTaskError) {
    let m = err.message
    if (err.status === 403) {
      const st = parseSmartTasksToday(err.payload)
      const hint = st?.resets_at ? formatQuotaResetsAtHint(st.resets_at) : ''
      if (hint) m = `${m} ${hint}`
    }
    return m
  }
  return err instanceof Error ? err.message : 'Не удалось добавить через ИИ'
}

const SMART_PRESETS_MAX = 24

const WEEKDAY_LABELS = ['Пн', 'Вт', 'Ср', 'Чт', 'Пт', 'Сб', 'Вс']
const HOURS = Array.from({ length: 24 }, (_, i) => i)
const PX_PER_HOUR = 44

function colorTone(colorId?: string): string {
  if (!colorId) return s.eventDefault
  const n = String(colorId)
  if (n >= '1' && n <= '11' && s[`tone${n}` as keyof typeof s]) {
    return s[`tone${n}` as keyof typeof s] as string
  }
  return s.eventDefault
}

/** Пауза — отдельный мягкий стиль, не как цветная задача из Google. */
function eventSurfaceClass(ev: NormalizedEvent): string {
  if (ev.isSmartBreak) return s.breakSuggestion
  const base = colorTone(ev.colorId)
  if (ev.spaceProposal) return [base, s.eventFromSpace].filter(Boolean).join(' ')
  return base
}

function breakRangeHint(ev: NormalizedEvent): string {
  return `Пауза · по желанию · ${formatEventTimeRange(ev)}`
}

function weekdayIndexMonday(d: Date): number {
  const day = d.getDay()
  return day === 0 ? 6 : day - 1
}

function timedEventsForDay(day: Date, events: NormalizedEvent[]): NormalizedEvent[] {
  const key = dayKey(day)
  return events.filter((ev) => {
    if (ev.allDay) return false
    const startK = dayKey(ev.start)
    const endK = dayKey(ev.end)
    return key >= startK && key <= endK
  })
}

function layoutBlock(day: Date, ev: NormalizedEvent): { top: number; height: number } | null {
  const day0 = new Date(day.getFullYear(), day.getMonth(), day.getDate(), 0, 0, 0, 0)
  const day1 = new Date(day.getFullYear(), day.getMonth(), day.getDate() + 1, 0, 0, 0, 0)
  const t0 = Math.max(ev.start.getTime(), day0.getTime())
  const t1 = Math.min(ev.end.getTime(), day1.getTime())
  if (t1 <= t0) return null
  const startMin = (t0 - day0.getTime()) / 60000
  const durMin = (t1 - t0) / 60000
  const top = (startMin / 60) * PX_PER_HOUR
  const minPx = ev.isSmartBreak ? 14 : 18
  const height = Math.max((durMin / 60) * PX_PER_HOUR, minPx)
  return { top, height }
}

function allDayForDay(day: Date, events: NormalizedEvent[]): NormalizedEvent[] {
  const key = dayKey(day)
  const map = eventsByDayKey(events.filter((e) => e.allDay))
  return map.get(key) ?? []
}

function formatModalDayTitle(d: Date): string {
  const s = d.toLocaleDateString('ru-RU', {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
    year: 'numeric',
  })
  return s.charAt(0).toUpperCase() + s.slice(1)
}

function localDateISO(d: Date): string {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

function pluralEvents(n: number): string {
  const m10 = n % 10
  const m100 = n % 100
  if (m100 >= 11 && m100 <= 14) return 'событий'
  if (m10 === 1) return 'событие'
  if (m10 >= 2 && m10 <= 4) return 'события'
  return 'событий'
}

function breakMinutes(ev: NormalizedEvent): number {
  return Math.max(0, Math.round((ev.end.getTime() - ev.start.getTime()) / 60000))
}

export default function CalendarPage() {
  const navigate = useNavigate()
  const { accessToken, refreshSession, ready, user } = useAuth()
  const { burstNode, celebrate } = useCompletionBurst()
  const [view, setView] = useState<'month' | 'week'>('month')
  const [anchor, setAnchor] = useState(() => new Date())
  const [events, setEvents] = useState<NormalizedEvent[]>([])
  const [linked, setLinked] = useState(true)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [dayModal, setDayModal] = useState<Date | null>(null)
  const [smartModalOpen, setSmartModalOpen] = useState(false)
  const [smartText, setSmartText] = useState('')
  const [smartBusy, setSmartBusy] = useState(false)
  const [smartError, setSmartError] = useState<string | null>(null)
  const [smartOk, setSmartOk] = useState<string | null>(null)
  const [smartPresetsOpen, setSmartPresetsOpen] = useState(false)
  const [smartPresetsList, setSmartPresetsList] = useState<SmartTaskPreset[]>(() => loadSmartTaskPresets())
  const [newPresetLabel, setNewPresetLabel] = useState('')
  const [newPresetText, setNewPresetText] = useState('')
  const [smartPresetEditorOpen, setSmartPresetEditorOpen] = useState(false)
  const [calTick, setCalTick] = useState(0)
  const [deletingEventId, setDeletingEventId] = useState<string | null>(null)
  const [completingEventId, setCompletingEventId] = useState<string | null>(null)
  const [deleteEventError, setDeleteEventError] = useState<string | null>(null)
  const [backlogRefreshKey, setBacklogRefreshKey] = useState(0)
  const [backlogModalLock, setBacklogModalLock] = useState(false)
  const [dragOverDayKey, setDragOverDayKey] = useState<string | null>(null)
  const [backlogAiBusy, setBacklogAiBusy] = useState(false)
  const [backlogDropError, setBacklogDropError] = useState<string | null>(null)
  const dayModalTitleId = useId()
  const smartModalTitleId = useId()
  const presetsModalTitleId = useId()
  const modalCloseRef = useRef<HTMLButtonElement>(null)
  const smartModalCloseRef = useRef<HTMLButtonElement>(null)
  const presetsModalCloseRef = useRef<HTMLButtonElement>(null)

  const navigateToLogin = useCallback(() => navigate('/login', { replace: true }), [navigate])

  const { gridStart, gridEnd } = useMemo(() => {
    if (view === 'month') {
      return getMonthGridRange(anchor.getFullYear(), anchor.getMonth())
    }
    const { start, end } = getWeekRange(anchor)
    return { gridStart: start, gridEnd: end }
  }, [view, anchor])

  const weekTitleRange = useMemo(() => {
    const { start, end } = getWeekRange(anchor)
    return formatWeekRange(start, end)
  }, [anchor])

  useEffect(() => {
    if (!ready) return
    let cancelled = false
    ;(async () => {
      if (!accessToken) {
        const ok = await refreshSession()
        if (cancelled) return
        if (!ok) {
          navigate('/login', { replace: true })
        }
        return
      }
      setLoading(true)
      setError(null)
      let overlays: Awaited<ReturnType<typeof fetchAcceptedCalendarOverlays>> = []
      try {
        overlays = await fetchAcceptedCalendarOverlays(accessToken, gridStart, gridEnd)
      } catch {
        overlays = []
      }
      if (cancelled) return
      try {
        const { linked: isLinked, events: raw } = await fetchCalendarEvents(
          accessToken,
          gridStart,
          gridEnd,
        )
        if (cancelled) return
        setLinked(isLinked)
        setEvents(mergeAcceptedSpaceProposalsIntoEvents(raw.map(normalizeApiEvent), overlays))
      } catch (e) {
        if (cancelled) return
        if (e instanceof Error && e.message === '401') {
          const ok = await refreshSession()
          if (ok && !cancelled) return
          navigate('/login', { replace: true })
          return
        }
        setError(e instanceof Error ? e.message : 'Ошибка загрузки')
        setEvents(mergeAcceptedSpaceProposalsIntoEvents([], overlays))
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [ready, accessToken, refreshSession, navigate, gridStart, gridEnd, calTick])

  const monthCells = useMemo(() => {
    const y = anchor.getFullYear()
    const m = anchor.getMonth()
    return buildMonthGridCells(y, m)
  }, [anchor])

  const byDay = useMemo(() => eventsByDayKey(events), [events])

  const modalDayEvents = useMemo(() => {
    if (!dayModal) return []
    return sortDayEvents(byDay.get(dayKey(dayModal)) ?? [])
  }, [dayModal, byDay])

  useEffect(() => {
    if (!dayModal) return
    setDeleteEventError(null)
  }, [dayModal])

  useEffect(() => {
    const onDragEnd = () => setDragOverDayKey(null)
    window.addEventListener('dragend', onDragEnd)
    return () => window.removeEventListener('dragend', onDragEnd)
  }, [])

  useEffect(() => {
    if (!smartModalOpen) setSmartPresetsOpen(false)
  }, [smartModalOpen])

  useEffect(() => {
    if (!smartPresetsOpen) return
    setSmartPresetsList(loadSmartTaskPresets())
    setSmartPresetEditorOpen(false)
    setNewPresetLabel('')
    setNewPresetText('')
  }, [smartPresetsOpen])

  useEffect(() => {
    if (!smartPresetsOpen) return
    presetsModalCloseRef.current?.focus()
  }, [smartPresetsOpen])

  useEffect(() => {
    if (!smartPresetEditorOpen) return
    const id = window.setTimeout(() => {
      document.getElementById('preset-label-input')?.focus()
    }, 0)
    return () => clearTimeout(id)
  }, [smartPresetEditorOpen])

  useEffect(() => {
    const locked = dayModal != null || smartModalOpen || backlogModalLock
    if (!locked) return
    const prevOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.body.style.overflow = prevOverflow
    }
  }, [dayModal, smartModalOpen, backlogModalLock])

  useEffect(() => {
    if (!dayModal) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setDayModal(null)
    }
    window.addEventListener('keydown', onKey)
    modalCloseRef.current?.focus()
    return () => {
      window.removeEventListener('keydown', onKey)
    }
  }, [dayModal])

  useEffect(() => {
    if (!smartModalOpen) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return
      if (smartPresetsOpen) {
        setSmartPresetsOpen(false)
        return
      }
      setSmartModalOpen(false)
    }
    window.addEventListener('keydown', onKey)
    if (!smartPresetsOpen) {
      smartModalCloseRef.current?.focus()
    }
    return () => {
      window.removeEventListener('keydown', onKey)
    }
  }, [smartModalOpen, smartPresetsOpen])

  const weekDays = useMemo(() => {
    const start = startOfWeekMonday(anchor)
    return Array.from({ length: 7 }, (_, i) => addDays(start, i))
  }, [anchor])

  const quota = user?.smart_tasks_today
  const quotaExhausted = quota != null && quota.used >= quota.limit
  const quotaLow =
    quota != null &&
    quota.limit - quota.used < Math.max(2, Math.ceil(quota.limit * 0.25))

  const runSmartAddOnce = async (token: string, text: string) => {
    const today = localDateISO(new Date())
    const res = await postSmartTask(token, text, today)
    setSmartText('')
    const n = res.tasks?.length ?? 0
    const tail = res.daily_limit_truncated ? ' (часть не вошла в дневной лимит)' : ''
    setSmartOk(n > 0 ? `Добавлено в календарь: ${n} ${pluralEvents(n)}${tail}` : 'Готово')
    await refreshSession()
    setCalTick((k: number) => k + 1)
  }

  const runSmartAdd = async (e: FormEvent) => {
    e.preventDefault()
    if (smartBusy) return
    if (!accessToken) return
    const t = smartText.trim()
    if (!t) {
      setSmartError('Введите описание задач')
      return
    }
    if (!linked) {
      setSmartError('Сначала подключите Google Календарь в профиле')
      return
    }
    setSmartBusy(true)
    setSmartError(null)
    setSmartOk(null)
    try {
      await runSmartAddOnce(accessToken, t)
    } catch (err) {
      if (err instanceof Error && err.message === '401') {
        const newToken = await refreshSession()
        if (!newToken) {
          navigate('/login', { replace: true })
          return
        }
        try {
          await runSmartAddOnce(newToken, t)
        } catch (retryErr) {
          setSmartError(smartTaskUserMessage(retryErr))
          if (retryErr instanceof SmartTaskError) await refreshSession()
        }
        return
      }
      if (err instanceof SmartTaskError) {
        setSmartError(smartTaskUserMessage(err))
        await refreshSession()
        return
      }
      setSmartError(err instanceof Error ? err.message : 'Ошибка')
    } finally {
      setSmartBusy(false)
    }
  }

  const persistPresets = (next: SmartTaskPreset[]) => {
    saveSmartTaskPresets(next)
    setSmartPresetsList(next)
  }

  const addSmartPreset = () => {
    const label = newPresetLabel.trim()
    const text = newPresetText.trim()
    if (!label || !text) return
    if (smartPresetsList.length >= SMART_PRESETS_MAX) return
    persistPresets([...smartPresetsList, { id: newSmartPresetId(), label, text }])
    setNewPresetLabel('')
    setNewPresetText('')
    setSmartPresetEditorOpen(false)
  }

  const deleteSmartPreset = (id: string) => {
    persistPresets(smartPresetsList.filter((p) => p.id !== id))
  }

  const applySmartPreset = (preset: SmartTaskPreset) => {
    setSmartText(preset.text)
    setSmartError(null)
    setSmartOk(null)
    setSmartPresetsOpen(false)
    queueMicrotask(() => document.getElementById('smart-task-input')?.focus())
  }

  const removeEventFromState = (eventId: string) => {
    setEvents((prev) => prev.filter((e) => e.id !== eventId))
  }

  const onCalendarDragOver = (e: DragEvent, dayKeyStr: string) => {
    if (!linked) return
    const types = [...e.dataTransfer.types]
    if (
      !types.includes(BACKLOG_DRAG_PAYLOAD_MIME) &&
      !types.includes(BACKLOG_DRAG_MIME) &&
      !types.includes('text/plain')
    )
      return
    e.preventDefault()
    e.dataTransfer.dropEffect = 'copy'
    setDragOverDayKey(dayKeyStr)
  }

  const onCalendarDragLeave = (e: DragEvent, dayKeyStr: string) => {
    const related = e.relatedTarget as Node | null
    if (related && (e.currentTarget as Node).contains(related)) return
    setDragOverDayKey((prev) => (prev === dayKeyStr ? null : prev))
  }

  const dropOptsFromWeekHourGrid = (e: DragEvent): ScheduleBacklogOptions => {
    const grid = e.currentTarget as HTMLElement
    const y = e.clientY - grid.getBoundingClientRect().top
    const minuteOfDay = Math.min(
      23 * 60 + 59,
      Math.max(0, Math.floor((y / PX_PER_HOUR) * 60)),
    )
    const snapped = Math.round(minuteOfDay / 15) * 15
    const clamped = Math.min(23 * 60 + 45, Math.max(0, snapped))
    const h = Math.floor(clamped / 60)
    const m = clamped % 60
    return {
      start_time: `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`,
    }
  }

  const onCalendarDropOnDay = async (e: DragEvent, d: Date, opts?: ScheduleBacklogOptions) => {
    if (!linked || !accessToken) return
    e.preventDefault()
    e.stopPropagation()
    setDragOverDayKey(null)
    setBacklogDropError(null)
    const snapshot = parseBacklogDragPayload(e.dataTransfer)
    if (!snapshot) {
      setBacklogDropError(
        'Не удалось прочитать стикер. Перетащите его снова с панели «Бэклог идей».',
      )
      return
    }
    const dateStr = localDateISO(d)
    const execute = async (token: string) => {
      await scheduleBacklogViaSmartTask(token, snapshot, dateStr, opts)
      setBacklogRefreshKey((x) => x + 1)
      setCalTick((x) => x + 1)
      await refreshSession()
      setBacklogDropError(null)
    }
    setBacklogAiBusy(true)
    try {
      await execute(accessToken)
    } catch (err) {
      if (err instanceof Error && err.message === '401') {
        const newToken = await refreshSession()
        if (!newToken) {
          navigate('/login', { replace: true })
          return
        }
        try {
          await execute(newToken)
        } catch (e2) {
          setBacklogDropError(smartTaskUserMessage(e2))
        }
        return
      }
      setBacklogDropError(smartTaskUserMessage(err))
    } finally {
      setBacklogAiBusy(false)
    }
  }

  const markEventCompletedInState = (eventId: string) => {
    setEvents((prev) => prev.map((e) => (e.id === eventId ? { ...e, completed: true } : e)))
  }

  const handleDeleteCalendarEvent = async (ev: NormalizedEvent) => {
    if (!accessToken || !linked || deletingEventId || completingEventId) return
    if (isSpaceProposalGhostEvent(ev)) return
    if (!window.confirm(`Удалить «${ev.title}» из Google Календаря?`)) return
    setDeletingEventId(ev.id)
    setDeleteEventError(null)
    try {
      await deleteCalendarEvent(accessToken, ev.id)
      removeEventFromState(ev.id)
    } catch (err) {
      if (err instanceof Error && err.message === '401') {
        const newToken = await refreshSession()
        if (!newToken) {
          navigate('/login', { replace: true })
          return
        }
        try {
          await deleteCalendarEvent(newToken, ev.id)
          removeEventFromState(ev.id)
        } catch (retryErr) {
          setDeleteEventError(retryErr instanceof Error ? retryErr.message : 'Не удалось удалить')
        }
        return
      }
      setDeleteEventError(err instanceof Error ? err.message : 'Не удалось удалить')
    } finally {
      setDeletingEventId(null)
    }
  }

  const handleCompleteCalendarEvent = async (ev: NormalizedEvent) => {
    if (!accessToken || !linked || deletingEventId || completingEventId || ev.completed) return
    if (isSpaceProposalGhostEvent(ev)) return
    setCompletingEventId(ev.id)
    setDeleteEventError(null)
    const prevUser = user
    try {
      const res = await completeCalendarEvent(accessToken, ev.id)
      markEventCompletedInState(ev.id)
      if (!res.already_completed) celebrate(prevUser, res.user)
      await refreshSession()
    } catch (err) {
      if (err instanceof Error && err.message === '401') {
        const newToken = await refreshSession()
        if (!newToken) {
          navigate('/login', { replace: true })
          return
        }
        try {
          const res = await completeCalendarEvent(newToken, ev.id)
          markEventCompletedInState(ev.id)
          if (!res.already_completed) celebrate(prevUser, res.user)
          await refreshSession()
        } catch (retryErr) {
          setDeleteEventError(retryErr instanceof Error ? retryErr.message : 'Не удалось отметить выполненным')
        }
        return
      }
      setDeleteEventError(err instanceof Error ? err.message : 'Не удалось отметить выполненным')
    } finally {
      setCompletingEventId(null)
    }
  }

  const modalActionBusy = deletingEventId !== null || completingEventId !== null

  const goPrev = () => {
    if (view === 'month') setAnchor((a) => addMonths(a, -1))
    else setAnchor((a) => addWeeks(a, -1))
  }

  const goNext = () => {
    if (view === 'month') setAnchor((a) => addMonths(a, 1))
    else setAnchor((a) => addWeeks(a, 1))
  }

  const now = new Date()
  const isToday = (d: Date) =>
    d.getDate() === now.getDate() && d.getMonth() === now.getMonth() && d.getFullYear() === now.getFullYear()

  if (!ready) {
    return <AppSessionLoading label="Загрузка календаря…" />
  }

  const dayModalUi =
    dayModal &&
    createPortal(
      <div className={s.modalRoot}>
        <button
          type="button"
          className={s.modalBackdrop}
          aria-label="Закрыть"
          onClick={() => setDayModal(null)}
        />
        <div
          className={s.modalPanel}
          role="dialog"
          aria-modal="true"
          aria-labelledby={dayModalTitleId}
          lang="ru"
          onClick={(e) => e.stopPropagation()}
        >
          <div className={s.modalHeader}>
            <h2 id={dayModalTitleId} className={s.modalTitle}>
              {formatModalDayTitle(dayModal)}
            </h2>
            <button
              ref={modalCloseRef}
              type="button"
              className={s.modalClose}
              onClick={() => setDayModal(null)}
              aria-label="Закрыть окно"
            >
              ×
            </button>
          </div>
          <div className={s.modalBody}>
            {deleteEventError && (
              <p className={s.modalDeleteErr} role="alert">
                {deleteEventError}
              </p>
            )}
            {modalDayEvents.length === 0 ? (
              <p className={s.modalEmpty}>На этот день событий нет — можно выдохнуть.</p>
            ) : (
              <ul className={s.modalList}>
                {modalDayEvents.map((ev) => (
                  <li
                    key={ev.id}
                    className={[
                      s.modalItem,
                      eventSurfaceClass(ev),
                      ev.isSmartBreak ? s.modalItemBreak : '',
                      ev.spaceProposal ? s.modalItemFromSpace : '',
                    ]
                      .filter(Boolean)
                      .join(' ')}
                  >
                    <span className={s.modalTime}>{formatEventTimeRange(ev)}</span>
                    <span
                      className={[
                        s.modalEventName,
                        ev.completed ? s.modalEventNameDone : '',
                        ev.isSmartBreak ? s.modalEventNameBreak : '',
                      ]
                        .filter(Boolean)
                        .join(' ')}
                    >
                      {ev.isSmartBreak ? (
                        <>
                          <i className={`fa-solid fa-mug-hot ${s.modalBreakIcon}`} aria-hidden />
                          <span className={s.modalBreakWord}>Пауза</span>
                          <span className={s.modalBreakHint}>по желанию</span>
                        </>
                      ) : ev.spaceProposal ? (
                        <>
                          <span className={s.modalEventRow}>
                            <SpaceProposalCalendarBadge
                              displayName={ev.spaceProposal.authorDisplayName}
                              avatarUrl={ev.spaceProposal.authorAvatarUrl}
                              spaceName={ev.spaceProposal.spaceName}
                              size="md"
                            />
                            <span>{ev.title}</span>
                          </span>
                          {isSpaceProposalGhostEvent(ev) ? (
                            <p className={s.modalGhostHint}>
                              Ещё не в Google Календаре — слот из пространства.{' '}
                              <Link to={`/spaces/${ev.spaceProposal.spaceId}`}>Открыть пространство</Link>
                            </p>
                          ) : null}
                        </>
                      ) : (
                        ev.title
                      )}
                    </span>
                    {!isSpaceProposalGhostEvent(ev) && linked ? (
                      <div className={s.modalItemActions}>
                        <button
                          type="button"
                          className={s.modalCompleteBtn}
                          disabled={modalActionBusy || Boolean(ev.completed)}
                          aria-busy={completingEventId === ev.id}
                          aria-label={ev.completed ? 'Уже выполнено' : 'Отметить выполненным'}
                          title={ev.completed ? 'Уже выполнено' : 'Выполнено'}
                          onClick={() => void handleCompleteCalendarEvent(ev)}
                        >
                          <Check size={16} strokeWidth={2.5} aria-hidden />
                        </button>
                        <button
                          type="button"
                          className={s.modalDeleteBtn}
                          disabled={modalActionBusy}
                          aria-busy={deletingEventId === ev.id}
                          aria-label={
                            deletingEventId === ev.id ? 'Удаление…' : 'Удалить из Google Календаря'
                          }
                          title="Удалить"
                          onClick={() => void handleDeleteCalendarEvent(ev)}
                        >
                          {deletingEventId === ev.id ? (
                            <Loader2 className={s.modalActionIconSpin} size={16} strokeWidth={2.25} aria-hidden />
                          ) : (
                            <Trash2 size={16} strokeWidth={2.25} aria-hidden />
                          )}
                        </button>
                      </div>
                    ) : null}
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      </div>,
      document.body,
    )

  const smartModalUi =
    smartModalOpen &&
    createPortal(
      <div className={s.smartModalRoot}>
        <button
          type="button"
          className={s.smartModalBackdrop}
          aria-label="Закрыть"
          onClick={() => setSmartModalOpen(false)}
        />
        <div
          className={s.smartModalPanel}
          role="dialog"
          aria-modal="true"
          aria-labelledby={smartModalTitleId}
          onClick={(e) => e.stopPropagation()}
        >
          <header className={s.smartModalHeader}>
            <div className={s.smartModalBrand}>
              <button
                type="button"
                className={s.smartModalGearBtn}
                onClick={(e) => {
                  e.stopPropagation()
                  setSmartPresetsOpen(true)
                }}
                aria-haspopup="dialog"
                aria-expanded={smartPresetsOpen}
                aria-label="Шаблоны быстрого ввода"
                title="Шаблоны быстрого ввода"
              >
                <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden>
                  <path
                    fill="currentColor"
                    d="M19.43 12.98c.04-.32.07-.64.07-.98s-.03-.66-.07-.98l2.11-1.65c.19-.15.24-.42.12-.64l-2-3.46c-.12-.22-.39-.3-.61-.22l-2.49 1c-.52-.4-1.08-.73-1.69-.98l-.38-2.65A.488.488 0 0 0 14 2h-4c-.25 0-.46.18-.49.42l-.38 2.65c-.61.25-1.17.59-1.69.98l-2.49-1c-.23-.09-.49 0-.61.22l-2 3.46c-.13.22-.07.49.12.64l2.11 1.65c-.04.32-.07.65-.07.98s.03.66.07.98l-2.11 1.65c-.19.15-.24.42-.12.64l2 3.46c.12.22.39.3.61.22l2.49-1c.52.4 1.08.73 1.69.98l.38 2.65c.03.24.24.42.49.42h4c.25 0 .46-.18.49-.42l.38-2.65c.61-.25 1.17-.59 1.69-.98l2.49 1c.23.09.49 0 .61-.22l2-3.46c.12-.22.07-.49-.12-.64l-2.11-1.65zM12 15.5c-1.93 0-3.5-1.57-3.5-3.5s1.57-3.5 3.5-3.5 3.5 1.57 3.5 3.5-1.57 3.5-3.5 3.5z"
                  />
                </svg>
              </button>
              <span id={smartModalTitleId} className={s.smartModalBrandTitle}>
                Умный ввод
              </span>
            </div>
            <button
              ref={smartModalCloseRef}
              type="button"
              className={s.smartModalClose}
              onClick={() => setSmartModalOpen(false)}
              aria-label="Закрыть"
            >
              ×
            </button>
          </header>

          <div className={s.smartModalBody}>
            <p className={s.smartModalUserHint}>
              Опишите день своими словами
            </p>
            {smartBusy && (
              <div className={s.smartModalWait}>
                <Loader2 className={s.smartModalWaitIcon} size={18} strokeWidth={2.25} aria-hidden />
                <span>
                  ИИ разбирает запрос и расставляет события в Google Календаре. Обычно это занимает несколько
                  секунд, иногда до минуты — окно можно не закрывать.
                </span>
              </div>
            )}
            {quotaLow && quota && (
              <p className={s.smartModalQuotaLine}>
                Осталось запросов:{' '}
                <strong>
                  {quota.limit - quota.used} / {quota.limit}
                </strong>
                {quota.resets_at && (
                  <span className={s.smartModalQuotaReset}>
                    {' · '}{formatQuotaResetsAtHint(quota.resets_at)}
                  </span>
                )}
              </p>
            )}

            <form className={s.smartModalForm} onSubmit={runSmartAdd}>
              <div className={s.smartModalInputShell}>
                <textarea
                  id="smart-task-input"
                  className={s.smartModalTextarea}
                  value={smartText}
                  onChange={(e) => {
                    setSmartText(e.target.value)
                    setSmartError(null)
                    setSmartOk(null)
                  }}
                  placeholder="Например: завтра встреча до обеда, потом отчёт и вечером почитать…"
                  rows={5}
                  disabled={smartBusy || quotaExhausted}
                  aria-invalid={Boolean(smartError)}
                  aria-label="Опишите задачи своими словами"
                />
              </div>

              {!linked && (
                <p className={s.smartModalLinkedHint}>
                  Подключите{' '}
                  <Link className={s.smartModalInlineLink} to="/profile" onClick={() => setSmartModalOpen(false)}>
                    Google Календарь
                  </Link>
                  , чтобы сохранять события.
                </p>
              )}

              <button
                type="submit"
                className={s.smartModalSubmit}
                disabled={smartBusy || !linked || quotaExhausted}
              >
                {smartBusy ? 'Создаю…' : (
                  <>
                    Создать задачу <span className={s.smartModalSubmitArrow} aria-hidden>→</span>
                  </>
                )}
              </button>

              {quotaExhausted && (
                <>
                  <p className={s.smartModalWarn}>Лимит умных задач на сегодня исчерпан.</p>
                  {quota?.resets_at ? (
                    <p className={s.smartModalQuotaReset}>{formatQuotaResetsAtHint(quota.resets_at)}</p>
                  ) : null}
                </>
              )}
              {smartOk && <p className={s.smartOk}>{smartOk}</p>}
              {smartError && (
                <p className={s.smartErr} role="alert">
                  {smartError}
                </p>
              )}
            </form>
          </div>
        </div>
      </div>,
      document.body,
    )

  const presetsModalUi =
    smartPresetsOpen &&
    smartModalOpen &&
    createPortal(
      <div className={s.presetsModalRoot}>
        <button
          type="button"
          className={s.presetsModalBackdrop}
          aria-label="Закрыть шаблоны"
          onClick={() => setSmartPresetsOpen(false)}
        />
        <div
          className={s.presetsModalPanel}
          role="dialog"
          aria-modal="true"
          aria-labelledby={presetsModalTitleId}
          onClick={(e) => e.stopPropagation()}
        >
          <header className={s.presetsModalHeader}>
            <div className={s.presetsModalTitleBlock}>
              <h2 id={presetsModalTitleId} className={s.presetsModalTitle}>
                Быстрые шаблоны
              </h2>
              <p className={s.presetsModalSubtitle}>Локально в браузере. «Вставить» — в поле умного ввода.</p>
            </div>
            <button
              ref={presetsModalCloseRef}
              type="button"
              className={s.presetsModalClose}
              onClick={() => setSmartPresetsOpen(false)}
              aria-label="Закрыть"
            >
              ×
            </button>
          </header>
          <div className={s.presetsModalBody}>
            {smartPresetsList.length === 0 ? (
              <p className={s.presetsEmpty}>Пока нет шаблонов.</p>
            ) : (
              <ul className={s.presetsList}>
                {smartPresetsList.map((p) => (
                  <li key={p.id} className={s.presetsRow}>
                    <div className={s.presetsRowLabel}>{p.label}</div>
                    <div className={s.presetsRowPreview}>{p.text}</div>
                    <div className={s.presetsRowActions}>
                      <button
                        type="button"
                        className={s.presetsMiniBtn}
                        onClick={() => applySmartPreset(p)}
                      >
                        Вставить
                      </button>
                      <button
                        type="button"
                        className={`${s.presetsMiniBtn} ${s.presetsMiniBtnDanger} ${s.presetsTrashBtn}`}
                        onClick={() => deleteSmartPreset(p.id)}
                        aria-label="Удалить шаблон"
                        title="Удалить"
                      >
                        <i className="fa-solid fa-trash" aria-hidden />
                      </button>
                    </div>
                  </li>
                ))}
              </ul>
            )}

            {!smartPresetEditorOpen ? (
              <div className={s.presetsRevealWrap}>
                <button
                  type="button"
                  className={s.presetsRevealBtn}
                  onClick={() => setSmartPresetEditorOpen(true)}
                  disabled={smartPresetsList.length >= SMART_PRESETS_MAX}
                >
                  {smartPresetsList.length >= SMART_PRESETS_MAX
                    ? `Лимит шаблонов (${SMART_PRESETS_MAX})`
                    : 'Добавить шаблон'}
                </button>
              </div>
            ) : (
            <div className={s.presetsAddBlock}>
              <div>
                <label className={s.presetsFieldLabel} htmlFor="preset-label-input">
                  Название
                </label>
                <input
                  id="preset-label-input"
                  className={s.presetsInput}
                  value={newPresetLabel}
                  onChange={(e) => setNewPresetLabel(e.target.value)}
                  placeholder="Например: Рабочий понедельник"
                  maxLength={80}
                />
              </div>
              <div>
                <label className={s.presetsFieldLabel} htmlFor="preset-text-input">
                  Текст для ИИ
                </label>
                <textarea
                  id="preset-text-input"
                  className={`${s.presetsInput} ${s.presetsTextarea}`}
                  value={newPresetText}
                  onChange={(e) => setNewPresetText(e.target.value)}
                  placeholder="Опишите задачи так, как обычно пишете в умном вводе…"
                  rows={4}
                />
              </div>
              <div className={s.presetsFormActions}>
                <button type="button" className={s.presetsAddBtn} onClick={addSmartPreset}>
                  Сохранить
                </button>
                <button
                  type="button"
                  className={s.presetsCancelBtn}
                  onClick={() => {
                    setSmartPresetEditorOpen(false)
                    setNewPresetLabel('')
                    setNewPresetText('')
                  }}
                >
                  Отмена
                </button>
              </div>
            </div>
            )}
          </div>
        </div>
      </div>,
      document.body,
    )

  return (
    <>
      {dayModalUi}
      {smartModalUi}
      {presetsModalUi}
      {burstNode}
      <div className={s.page}>
      <div className={s.shell}>
        <div className={s.mainColumn}>
          {backlogDropError && (
            <div className={s.dropBanner} role="alert">
              <span>{backlogDropError}</span>
              <button
                type="button"
                className={s.dropBannerDismiss}
                aria-label="Скрыть сообщение"
                onClick={() => setBacklogDropError(null)}
              >
                ×
              </button>
            </div>
          )}
          <header className={s.calendarHeader}>
            <h1
              className={[s.heroTitle, view === 'week' ? s.heroTitleWeek : ''].filter(Boolean).join(' ')}
            >
              <button
                type="button"
                className={s.heroNavBtn}
                onClick={goPrev}
                aria-label={view === 'month' ? 'Предыдущий месяц' : 'Предыдущая неделя'}
              >
                ‹
              </button>
              <span className={s.heroTitleText}>
                {view === 'month' ? formatCalendarHeroMonthYear(anchor) : weekTitleRange}
              </span>
              <button
                type="button"
                className={s.heroNavBtn}
                onClick={goNext}
                aria-label={view === 'month' ? 'Следующий месяц' : 'Следующая неделя'}
              >
                ›
              </button>
            </h1>
            <div className={s.calendarHeaderControls}>
              <div className={s.segmented} role="group" aria-label="Вид календаря">
                <button
                  type="button"
                  className={view === 'month' ? s.segActive : s.segBtn}
                  onClick={() => setView('month')}
                >
                  мес.
                </button>
                <button
                  type="button"
                  className={view === 'week' ? s.segActive : s.segBtn}
                  onClick={() => setView('week')}
                >
                  нед.
                </button>
              </div>
              <button
                type="button"
                className={s.headerAddBtn}
                onClick={() => {
                  setSmartModalOpen(true)
                  setSmartError(null)
                }}
                aria-haspopup="dialog"
                aria-expanded={smartModalOpen}
              >
                Добавить задачу
              </button>
            </div>
          </header>

          {backlogAiBusy && (
            <div className={s.backlogAiBanner} role="status" aria-live="polite">
              Добавляем через ИИ… это может занять несколько секунд.
            </div>
          )}

          {!linked && !loading && (
            <div className={s.banner}>
              <p>Google Календарь не подключён — события не отображаются.</p>
              <Link className={s.bannerLink} to="/profile">
                Подключить в профиле
              </Link>
            </div>
          )}

          {error && (
            <div className={s.bannerErr} role="alert">
              {error}
            </div>
          )}

          {loading ? (
            <div className={s.loading}>Загрузка…</div>
          ) : view === 'month' ? (
            <div className={s.monthWrap}>
              <div className={s.weekdayRow}>
                {WEEKDAY_LABELS.map((w) => (
                  <div key={w} className={s.weekdayHead}>
                    {w}
                  </div>
                ))}
              </div>
              <div className={s.monthGrid}>
                {monthCells.map((d) => {
                  const k = dayKey(d)
                  const dayEvents = byDay.get(k) ?? []
                  const inMonth = d.getMonth() === anchor.getMonth()
                  const dots = dayEvents.slice(0, 5)
                  const rest = Math.max(0, dayEvents.length - 1)
                  const shown = dayEvents.slice(0, 1)
                  return (
                    <button
                      key={k}
                      type="button"
                      className={[
                        s.dayCell,
                        s.dayCellBtn,
                        !inMonth ? s.dayMuted : '',
                        isToday(d) ? s.dayToday : '',
                        linked && dragOverDayKey === k ? s.dropHighlight : '',
                      ]
                        .filter(Boolean)
                        .join(' ')}
                      onClick={() => setDayModal(d)}
                      onDragOver={(e) => onCalendarDragOver(e, k)}
                      onDragLeave={(e) => onCalendarDragLeave(e, k)}
                      onDrop={(e) => void onCalendarDropOnDay(e, d)}
                      aria-label={`${d.getDate()}, открыть список дел (${dayEvents.length})`}
                    >
                      <div className={s.dayNum}>{d.getDate()}</div>
                      {dots.length > 0 && (
                        <div className={s.dots} aria-hidden>
                          {dots.map((ev) => (
                            <span
                              key={`${ev.id}-${k}-dot`}
                              className={[s.dot, ev.isSmartBreak ? s.breakDot : eventSurfaceClass(ev)].join(' ')}
                            />
                          ))}
                        </div>
                      )}
                      <div className={s.dayEvents}>
                        {shown.map((ev) => (
                          <div
                            key={`${ev.id}-${k}`}
                            className={[
                              s.dayEventLine,
                              eventSurfaceClass(ev),
                              ev.isSmartBreak ? s.dayEventLineBreak : '',
                            ]
                              .filter(Boolean)
                              .join(' ')}
                          >
                            {ev.isSmartBreak ? (
                              <>
                                <i className={`fa-solid fa-mug-hot ${s.dayEventBreakFa}`} aria-hidden />
                                <span className={s.dayEventTitle}>{breakMinutes(ev)} мин</span>
                              </>
                            ) : ev.spaceProposal ? (
                              <div className={s.eventRowFromSpace}>
                                <SpaceProposalCalendarBadge
                                  displayName={ev.spaceProposal.authorDisplayName}
                                  avatarUrl={ev.spaceProposal.authorAvatarUrl}
                                  spaceName={ev.spaceProposal.spaceName}
                                />
                                <span className={s.dayEventTitle}>{ev.title}</span>
                              </div>
                            ) : (
                              <span className={s.dayEventTitle}>{ev.title}</span>
                            )}
                          </div>
                        ))}
                        {rest > 0 && (
                          <div className={s.more} aria-hidden>
                            ещё +{rest}
                          </div>
                        )}
                      </div>
                    </button>
                  )
                })}
              </div>
            </div>
          ) : (
            <div className={s.weekWrap}>
              <div className={s.weekHeader}>
                <div className={s.gutterCorner} />
                {weekDays.map((d) => {
                  const wk = dayKey(d)
                  return (
                  <button
                    key={wk}
                    type="button"
                    className={[
                      s.weekHeadCell,
                      s.weekHeadBtn,
                      isToday(d) ? s.weekHeadToday : '',
                      linked && dragOverDayKey === wk ? s.dropHighlight : '',
                    ].join(' ')}
                    onClick={() => setDayModal(d)}
                    onDragOver={(e) => onCalendarDragOver(e, wk)}
                    onDragLeave={(e) => onCalendarDragLeave(e, wk)}
                    onDrop={(e) => void onCalendarDropOnDay(e, d)}
                    aria-label={`События на ${d.toLocaleDateString('ru-RU')}`}
                  >
                    <span className={s.weekDow}>{WEEKDAY_LABELS[weekdayIndexMonday(d)]}</span>
                    <span className={s.weekDom}>{d.getDate()}</span>
                  </button>
                  )
                })}
              </div>
              <div className={s.allDayRow}>
                <div className={s.allDayGutter}>Весь день</div>
                <div className={s.allDayCols}>
                  {weekDays.map((d) => {
                    const ads = allDayForDay(d, events)
                    const wk = dayKey(d)
                    return (
                      <div
                        key={wk}
                        className={[
                          s.allDayCell,
                          isToday(d) ? s.colToday : '',
                          linked && dragOverDayKey === wk ? s.dropHighlight : '',
                        ]
                          .filter(Boolean)
                          .join(' ')}
                        onClick={() => setDayModal(d)}
                        onDragOver={(e) => onCalendarDragOver(e, wk)}
                        onDragLeave={(e) => onCalendarDragLeave(e, wk)}
                        onDrop={(e) => void onCalendarDropOnDay(e, d)}
                      >
                        {ads.map((ev) => (
                          <div key={ev.id} className={[s.allDayPill, eventSurfaceClass(ev)].join(' ')}>
                            {ev.title}
                          </div>
                        ))}
                      </div>
                    )
                  })}
                </div>
              </div>
              <div className={s.weekScroll}>
                <div className={s.timeCol}>
                  {HOURS.map((h) => (
                    <div key={h} className={s.timeLabel}>
                      {String(h).padStart(2, '0')}:00
                    </div>
                  ))}
                </div>
                <div className={s.weekCols}>
                  {weekDays.map((d) => {
                    const timed = timedEventsForDay(d, events)
                    const nowLine = isToday(d)
                      ? ((now.getHours() * 60 + now.getMinutes()) / 60) * PX_PER_HOUR
                      : null
                    const wk = dayKey(d)
                    return (
                      <div
                        key={wk}
                        className={[
                          s.weekCol,
                          s.weekColClick,
                          isToday(d) ? s.colToday : '',
                          linked && dragOverDayKey === wk ? s.dropHighlight : '',
                        ]
                          .filter(Boolean)
                          .join(' ')}
                        onClick={() => setDayModal(d)}
                        onDragOver={(e) => onCalendarDragOver(e, wk)}
                        onDragLeave={(e) => onCalendarDragLeave(e, wk)}
                      >
                        <div
                          className={s.hourGrid}
                          style={{ height: `${24 * PX_PER_HOUR}px` }}
                          onDragOver={(e) => onCalendarDragOver(e, wk)}
                          onDrop={(e) => void onCalendarDropOnDay(e, d, dropOptsFromWeekHourGrid(e))}
                        >
                          {HOURS.map((h) => (
                            <div key={h} className={s.hourLine} />
                          ))}
                          {nowLine !== null && (
                            <div className={s.nowLine} style={{ top: `${nowLine}px` }} aria-hidden />
                          )}
                          {timed.map((ev) => {
                            const layout = layoutBlock(d, ev)
                            if (!layout) return null
                            return (
                              <div
                                key={`${ev.id}-${dayKey(d)}`}
                                className={[
                                  s.weekEvent,
                                  eventSurfaceClass(ev),
                                  ev.isSmartBreak ? s.weekEventBreak : '',
                                ]
                                  .filter(Boolean)
                                  .join(' ')}
                                style={{ top: `${layout.top}px`, height: `${layout.height}px` }}
                                title={
                                  ev.isSmartBreak
                                    ? breakRangeHint(ev)
                                    : `${ev.title} (${ev.start.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })} – ${ev.end.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })})`
                                }
                                onClick={(e) => e.stopPropagation()}
                              >
                                {ev.isSmartBreak ? (
                                  <>
                                    <span className={s.weekEventBreakIcon} aria-hidden>
                                      <i className="fa-solid fa-mug-hot" />
                                    </span>
                                    <span className={s.weekEventBreakMinutes}>{breakMinutes(ev)} мин</span>
                                  </>
                                ) : ev.spaceProposal ? (
                                  <div className={s.weekEventRow}>
                                    <SpaceProposalCalendarBadge
                                      displayName={ev.spaceProposal.authorDisplayName}
                                      avatarUrl={ev.spaceProposal.authorAvatarUrl}
                                      spaceName={ev.spaceProposal.spaceName}
                                    />
                                    <span className={s.weekEventTitle}>{ev.title}</span>
                                  </div>
                                ) : (
                                  <span className={s.weekEventTitle}>{ev.title}</span>
                                )}
                              </div>
                            )
                          })}
                        </div>
                      </div>
                    )
                  })}
                </div>
              </div>
            </div>
          )}
        </div>

        <aside className={s.backlogAside} aria-label="Бэклог идей">
          <BacklogIdeasPanel
            accessToken={accessToken}
            linked={linked}
            refreshKey={backlogRefreshKey}
            onRefreshCalendar={() => setCalTick((x) => x + 1)}
            onModalLockChange={setBacklogModalLock}
            refreshSession={refreshSession}
            navigateLogin={navigateToLogin}
            smartTasksToday={quota ?? null}
          />
        </aside>
      </div>
    </div>
    </>
  )
}
