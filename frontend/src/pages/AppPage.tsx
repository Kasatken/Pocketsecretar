import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faArrowsRotate } from '@fortawesome/free-solid-svg-icons'
import { Link, useNavigate } from 'react-router-dom'
import { ClipboardList, Check, Focus, Pause, Play, Settings, Sparkles, Timer, Trash2, Trophy } from 'lucide-react'
import { useAuth } from '../auth/AuthContext'
import type { User } from '../auth/AuthContext'
import { formatPomodoroClock, phaseDurationSeconds, usePomodoro, type PomodoroPhase } from '../hooks/usePomodoro'
import { PomodoroSettingsModal } from '../components/PomodoroSettingsModal'
import { fetchCalendarEvents, fetchDailyAdvice, normalizeApiEvent, completeCalendarEvent, deleteCalendarEvent } from '../lib/calendarApi'
import {
  dayKey,
  endOfLocalDay,
  eventsByDayKey,
  formatEventTimeRange,
  sortDayEvents,
  startOfLocalDay,
  type NormalizedEvent,
} from '../lib/calendarUtils'
import { isSpaceProposalGhostEvent, mergeAcceptedSpaceProposalsIntoEvents } from '../lib/mergeAcceptedSpaceProposals'
import { fetchAcceptedCalendarOverlays } from '../lib/spacesApi'
import { getEffectiveSkyHour, previewSkyGradientAtHour } from '../lib/dayGaugeSkyGradient'
import { LevelProgress } from '../components/LevelProgress'
import { SpaceProposalCalendarBadge } from '../components/SpaceProposalCalendarBadge'
import s from './AppPage.module.css'

function displayLabel(user: User | null) {
  if (!user) return 'пользователь'
  const n = user.display_name?.trim()
  if (n) return n
  return user.email
}

function capitalizeRu(str: string) {
  return str.length ? str.charAt(0).toUpperCase() + str.slice(1) : str
}

function phaseLabelRu(phase: PomodoroPhase): string {
  switch (phase) {
    case 'work':
      return 'Работа'
    case 'shortBreak':
      return 'Короткий перерыв'
    case 'longBreak':
      return 'Длинный перерыв'
  }
}

function pomodoroDotsOn(phase: PomodoroPhase, completedWork: number, interval: number): number {
  if (phase === 'longBreak') return interval
  return completedWork % interval
}

function pomodoroStatusLine(phase: PomodoroPhase, completedWork: number, interval: number): string {
  switch (phase) {
    case 'work':
      return `Работа ${(completedWork % interval) + 1} из ${interval}`
    case 'shortBreak':
      return 'Передышка перед следующим блоком'
    case 'longBreak':
      return `После ${interval} блоков — отдых подольше`
  }
}

/** Текст в статусе, когда фаза сама завершилась и таймер ждёт подтверждения */
function pomodoroReadyLine(nextPhase: PomodoroPhase): string {
  return nextPhase === 'work' ? 'Перерыв закончился — пора поработать' : 'Работа завершена — время отдохнуть'
}

/** Короткая подпись кнопки при ожидании подтверждения — без громоздкого названия фазы */
function pomodoroConfirmLabel(nextPhase: PomodoroPhase): string {
  return nextPhase === 'work' ? 'Начать работу' : 'Начать перерыв'
}

/** Фазы суток — как теги состояния в UI Metaphor */
function dayPhaseRu(d: Date): string {
  const h = d.getHours()
  if (h >= 5 && h < 12) return 'Утро'
  if (h >= 12 && h < 17) return 'День'
  if (h >= 17 && h < 22) return 'Вечер'
  return 'Ночь'
}

function eventsWordRu(n: number): string {
  const m10 = n % 10
  const m100 = n % 100
  if (m100 >= 11 && m100 <= 14) return 'событий'
  if (m10 === 1) return 'событие'
  if (m10 >= 2 && m10 <= 4) return 'события'
  return 'событий'
}

const DAY_GAUGE_SEGMENTS = 12

/** Тихий опрос: совет строится по всей неделе на сервере; на главной грузим только «сегодня», поэтому подтягиваем совет периодически без перезагрузки страницы. */
const ADVICE_POLL_MS = 4 * 60 * 1000

export default function AppPage() {
  const navigate = useNavigate()
  const { user, accessToken, refreshSession, ready } = useAuth()
  const [events, setEvents] = useState<NormalizedEvent[]>([])
  const [linked, setLinked] = useState(true)
  const [loadingCal, setLoadingCal] = useState(true)
  const [calError, setCalError] = useState<string | null>(null)
  const [taskDeletingId, setTaskDeletingId] = useState<string | null>(null)
  const [taskCompletingId, setTaskCompletingId] = useState<string | null>(null)
  const [taskActionError, setTaskActionError] = useState<string | null>(null)
  const [adviceText, setAdviceText] = useState<string | null>(null)
  const [adviceLoading, setAdviceLoading] = useState(true)
  const [adviceError, setAdviceError] = useState<string | null>(null)
  /** Подпись при откате к сохранённому совету (сеть / ИИ временно недоступны). */
  const [adviceFootnote, setAdviceFootnote] = useState<string | null>(null)
  const [adviceLinked, setAdviceLinked] = useState(true)
  /** После первого ответа /daily-advice — чтобы не опрашивать API, пока не знаем, связан ли календарь. */
  const [adviceHydrated, setAdviceHydrated] = useState(false)
  const [nowTick, setNowTick] = useState(0)
  /** Чаще, чем nowTick: плавное движение градиента неба по реальному времени */
  const [skyTick, setSkyTick] = useState(0)
  const [calSyncGen, setCalSyncGen] = useState(0)
  const pomodoro = usePomodoro()
  const [pomodoroSettingsOpen, setPomodoroSettingsOpen] = useState(false)

  const pomodoroPhaseTotalSec = phaseDurationSeconds(pomodoro.phase, pomodoro.settings)
  const pomodoroRingProgress =
    pomodoroPhaseTotalSec > 0 ? pomodoro.remaining / pomodoroPhaseTotalSec : 1
  const pomodoroDotsFilled = pomodoroDotsOn(pomodoro.phase, pomodoro.completedWork, pomodoro.settings.longBreakInterval)
  const pomodoroPhaseClass =
    pomodoro.phase === 'work'
      ? s.timerPhaseWork
      : pomodoro.phase === 'shortBreak'
        ? s.timerPhaseShort
        : s.timerPhaseLong
  /** Активная рабочая фаза — приглушаем остальной дашборд, чтобы не отвлекать */
  const pomodoroFocusActive = pomodoro.running && pomodoro.phase === 'work'
  const focusDimCls = pomodoroFocusActive ? s.focusDimmed : ''

  /** Длина окружности SVG-кольца (viewBox 100×100, r=44). */
  const pomodoroRingLen = 2 * Math.PI * 44

  /** Последний «отпечаток» событий сегодня, для которого уже подтянули совет (без лишних запросов при том же календаре). */
  const lastAdviceTodaySigRef = useRef<string | null>(null)
  const prevTodayKeyRef = useRef<string | null>(null)

  useEffect(() => {
    const id = window.setInterval(() => setNowTick((t: number) => t + 1), 30_000)
    return () => window.clearInterval(id)
  }, [])

  useEffect(() => {
    const id = window.setInterval(() => setSkyTick((t: number) => t + 1), 60_000)
    return () => window.clearInterval(id)
  }, [])

  useEffect(() => {
    const onVis = () => {
      if (document.visibilityState === 'visible') {
        setCalSyncGen((n) => n + 1)
      }
    }
    document.addEventListener('visibilitychange', onVis)
    return () => document.removeEventListener('visibilitychange', onVis)
  }, [])

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
      const today = new Date()
      const timeMin = startOfLocalDay(today)
      const timeMax = endOfLocalDay(today)
      setLoadingCal(true)
      setCalError(null)
      let overlays: Awaited<ReturnType<typeof fetchAcceptedCalendarOverlays>> = []
      try {
        overlays = await fetchAcceptedCalendarOverlays(accessToken, timeMin, timeMax)
      } catch {
        overlays = []
      }
      if (cancelled) return
      try {
        const { linked: isLinked, events: raw } = await fetchCalendarEvents(accessToken, timeMin, timeMax)
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
        setCalError(e instanceof Error ? e.message : 'Ошибка загрузки')
        setEvents(mergeAcceptedSpaceProposalsIntoEvents([], overlays))
      } finally {
        if (!cancelled) setLoadingCal(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [ready, accessToken, refreshSession, navigate, calSyncGen])

  const todayKey = useMemo(() => dayKey(new Date()), [nowTick])

  const todayEvents = useMemo(() => {
    const map = eventsByDayKey(events)
    return sortDayEvents(map.get(todayKey) ?? [])
  }, [events, todayKey])

  /** Совпадает с логикой сигнатуры на сервере для «сегодня» в локальном дне пользователя: новое/изменённое дело → новый запрос совета. */
  const todayEventsSig = useMemo(() => {
    return todayEvents
      .map((e) => `${e.id}\t${e.start.toISOString()}\t${e.end.toISOString()}`)
      .sort()
      .join('\n')
  }, [todayEvents])

  const markTaskCompleted = useCallback((eventId: string) => {
    setEvents((prev) => prev.map((e) => (e.id === eventId ? { ...e, completed: true } : e)))
  }, [])

  const removeTaskFromState = useCallback((eventId: string) => {
    setEvents((prev) => prev.filter((e) => e.id !== eventId))
  }, [])

  const handleCompleteTask = useCallback(
    async (ev: NormalizedEvent) => {
      if (!accessToken || !linked || taskDeletingId !== null || taskCompletingId !== null || ev.completed) return
      if (isSpaceProposalGhostEvent(ev)) return
      setTaskCompletingId(ev.id)
      setTaskActionError(null)
      try {
        await completeCalendarEvent(accessToken, ev.id)
        markTaskCompleted(ev.id)
        await refreshSession()
      } catch (err) {
        if (err instanceof Error && err.message === '401') {
          const newToken = await refreshSession()
          if (!newToken) {
            navigate('/login', { replace: true })
            return
          }
          try {
            await completeCalendarEvent(newToken, ev.id)
            markTaskCompleted(ev.id)
            await refreshSession()
          } catch (retryErr) {
            setTaskActionError(retryErr instanceof Error ? retryErr.message : 'Не удалось отметить выполненным')
          }
          return
        }
        setTaskActionError(err instanceof Error ? err.message : 'Не удалось отметить выполненным')
      } finally {
        setTaskCompletingId(null)
      }
    },
    [accessToken, linked, taskDeletingId, taskCompletingId, markTaskCompleted, refreshSession, navigate],
  )

  const handleDeleteTask = useCallback(
    async (ev: NormalizedEvent) => {
      if (!accessToken || !linked || taskDeletingId !== null || taskCompletingId !== null) return
      if (isSpaceProposalGhostEvent(ev)) return
      if (!window.confirm(`Удалить «${ev.title}» из Google Календаря?`)) return
      setTaskDeletingId(ev.id)
      setTaskActionError(null)
      try {
        await deleteCalendarEvent(accessToken, ev.id)
        removeTaskFromState(ev.id)
      } catch (err) {
        if (err instanceof Error && err.message === '401') {
          const newToken = await refreshSession()
          if (!newToken) {
            navigate('/login', { replace: true })
            return
          }
          try {
            await deleteCalendarEvent(newToken, ev.id)
            removeTaskFromState(ev.id)
          } catch (retryErr) {
            setTaskActionError(retryErr instanceof Error ? retryErr.message : 'Не удалось удалить')
          }
          return
        }
        setTaskActionError(err instanceof Error ? err.message : 'Не удалось удалить')
      } finally {
        setTaskDeletingId(null)
      }
    },
    [accessToken, linked, taskDeletingId, taskCompletingId, removeTaskFromState, refreshSession, navigate],
  )

  const taskActionBusy = taskDeletingId !== null || taskCompletingId !== null

  useEffect(() => {
    if (prevTodayKeyRef.current !== todayKey) {
      prevTodayKeyRef.current = todayKey
      lastAdviceTodaySigRef.current = null
    }
  }, [todayKey])

  const applyAdviceResponse = useCallback(
    (res: Awaited<ReturnType<typeof fetchDailyAdvice>>) => {
      setAdviceLinked(res.linked)
      if (res.stale && res.retry_suggest?.trim()) {
        setAdviceFootnote(res.retry_suggest.trim())
      } else {
        setAdviceFootnote(null)
      }
      if (res.detail) {
        setAdviceError(res.detail)
        setAdviceText(null)
      } else {
        setAdviceError(null)
        setAdviceText(res.advice)
      }
    },
    [],
  )

  const loadAdvice = useCallback(
    async (token: string, opts?: { refresh?: boolean; showSpinner?: boolean }) => {
      const show = opts?.showSpinner ?? true
      if (show) {
        setAdviceLoading(true)
        setAdviceError(null)
      }
      try {
        const res = await fetchDailyAdvice(token, { refresh: opts?.refresh })
        applyAdviceResponse(res)
      } catch (e) {
        if (e instanceof Error && e.message === '401') {
          throw e
        }
        setAdviceError(e instanceof Error ? e.message : 'Не удалось загрузить совет')
        setAdviceText(null)
      } finally {
        if (show) setAdviceLoading(false)
        setAdviceHydrated(true)
      }
    },
    [applyAdviceResponse],
  )

  useEffect(() => {
    if (!ready) return
    let cancelled = false
    ;(async () => {
      let token = accessToken
      if (!token) {
        const ok = await refreshSession()
        if (cancelled) return
        if (!ok) {
          navigate('/login', { replace: true })
          return
        }
        return
      }
      if (loadingCal) return
      if (lastAdviceTodaySigRef.current === todayEventsSig) {
        setAdviceHydrated(true)
        return
      }
      try {
        await loadAdvice(token, { showSpinner: true })
        if (cancelled) return
        lastAdviceTodaySigRef.current = todayEventsSig
      } catch (e) {
        if (cancelled) return
        if (e instanceof Error && e.message === '401') {
          const ok = await refreshSession()
          if (ok && !cancelled) return
          navigate('/login', { replace: true })
          return
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [
    ready,
    accessToken,
    refreshSession,
    navigate,
    loadingCal,
    todayEventsSig,
    loadAdvice,
  ])

  useEffect(() => {
    if (!ready || !accessToken || !adviceLinked || !adviceHydrated || loadingCal) return
    const id = window.setInterval(() => {
      void (async () => {
        try {
          const res = await fetchDailyAdvice(accessToken)
          applyAdviceResponse(res)
        } catch {
          /* тихо: сеть или 401 обработаем при следующем явном запросе */
        }
      })()
    }, ADVICE_POLL_MS)
    return () => window.clearInterval(id)
  }, [ready, accessToken, adviceLinked, adviceHydrated, loadingCal, applyAdviceResponse])

  const nextEvent = useMemo(() => {
    const now = Date.now()
    for (const ev of todayEvents) {
      if (ev.completed) continue
      if (ev.start.getTime() > now) return ev
    }
    return null
  }, [todayEvents, nowTick])

  const dateLine = useMemo(() => {
    const d = new Date()
    return capitalizeRu(
      d.toLocaleDateString('ru-RU', {
        weekday: 'long',
        day: 'numeric',
        month: 'long',
      }),
    )
  }, [nowTick])

  const phaseTag = useMemo(() => dayPhaseRu(new Date()), [nowTick])

  const journalLine = useMemo(() => {
    if (loadingCal) return 'Обновление журнала…'
    if (calError) return 'Журнал недоступен'
    if (!linked) return 'Календарь не связан'
    const n = todayEvents.length
    return `${n} ${eventsWordRu(n)} в записи`
  }, [loadingCal, calError, linked, todayEvents.length])

  const dayGaugeFilled = useMemo(() => {
    const now = Date.now()
    const start = startOfLocalDay(new Date(now)).getTime()
    const end = endOfLocalDay(new Date(now)).getTime()
    if (end <= start) return 0
    const p = (now - start) / (end - start)
    const raw = p * DAY_GAUGE_SEGMENTS
    if (raw <= 0) return 0
    // floor от доли дня — без завышения (ceil давал 6/12 уже к 11:00). Если сутки уже идут,
    // но доля < 1/12 — показываем один сегмент, чтобы ночью полоска не была полностью пустой.
    let n = Math.floor(raw)
    if (n === 0) n = 1
    return Math.min(DAY_GAUGE_SEGMENTS, n)
  }, [nowTick])

  /** Прямой градиент на сегменты: надёжнее, чем var() + @property (в части браузеров «включённые» ячейки оставались серыми). */
  const dayGaugeSky = useMemo(
    () => previewSkyGradientAtHour(getEffectiveSkyHour(new Date())),
    [nowTick, skyTick],
  )

  if (!ready) {
    return null
  }

  return (
    <div className={s.page}>
      <div className={s.shell}>
        <div className={s.todayLayout}>
          <div className={s.todayLeft}>
            <header className={`${s.hero} ${focusDimCls}`}>
              <p className={s.eyebrow}>Сегодня</p>
              <h1 className={s.h1}>
                Привет, <span className={s.accent}>{displayLabel(user)}</span>
              </h1>
              <p className={s.lead}>{dateLine}</p>
              <div className={s.heroMeta}>
                <span className={s.phaseTag}>{phaseTag}</span>
                <span className={s.metaSlash} aria-hidden>
                  /
                </span>
                <span className={s.metaJournal}>{journalLine}</span>
              </div>
            </header>

            {nextEvent ? (
              <section className={s.nextStrip} aria-live="polite">
                <span className={s.nextStripDot} aria-hidden />
                <div className={s.nextStripInner}>
                  {nextEvent.spaceProposal ? (
                    <SpaceProposalCalendarBadge
                      displayName={nextEvent.spaceProposal.authorDisplayName}
                      avatarUrl={nextEvent.spaceProposal.authorAvatarUrl}
                      spaceName={nextEvent.spaceProposal.spaceName}
                      size="md"
                    />
                  ) : null}
                  <span className={s.nextStripLabel}>Дальше</span>
                  <span className={s.nextStripTitle}>{nextEvent.title}</span>
                  <span className={s.nextStripTime}>{formatEventTimeRange(nextEvent)}</span>
                </div>
              </section>
            ) : null}

            <div className={s.leftMidRow}>
              <div className={s.leftTimerProgressCol}>
                <section className={`${s.card} ${s.timerCard}`}>
                  <div className={s.cardHeader}>
                    <span className={s.cardIcon} aria-hidden>
                      <Timer size={18} strokeWidth={2.25} />
                    </span>
                    <h2 className={s.cardTitle}>Pomodoro</h2>
                    {pomodoroFocusActive ? (
                      <span className={s.focusBadge}>
                        <Focus size={13} strokeWidth={2.5} aria-hidden />
                        Фокус
                      </span>
                    ) : null}
                    <button
                      type="button"
                      className={s.pomodoroSettingsBtn}
                      aria-label="Настройки таймера"
                      title="Настройки таймера"
                      onClick={() => setPomodoroSettingsOpen(true)}
                    >
                      <Settings size={16} strokeWidth={2.25} aria-hidden />
                    </button>
                  </div>
                  <div
                    className={`${s.timer} ${pomodoroPhaseClass}`}
                    data-running={pomodoro.running ? 'true' : undefined}
                    data-awaiting={pomodoro.awaitingConfirmation ? 'true' : undefined}
                  >
                    <div className={s.timerBody}>
                      <div className={s.timerRingWrap}>
                        <svg className={s.timerRingSvg} viewBox="0 0 100 100" aria-hidden>
                          <circle className={s.timerRingTrack} r="44" cx="50" cy="50" fill="none" />
                          <circle
                            className={s.timerRingArc}
                            r="44"
                            cx="50"
                            cy="50"
                            fill="none"
                            strokeLinecap="round"
                            transform="rotate(-90 50 50)"
                            strokeDasharray={pomodoroRingLen}
                            strokeDashoffset={pomodoroRingLen * (1 - pomodoroRingProgress)}
                          />
                        </svg>
                        <div className={s.timerRingInner}>
                          <div className={s.timerDisplay}>{formatPomodoroClock(pomodoro.remaining)}</div>
                          <p className={s.timerSub}>{phaseLabelRu(pomodoro.phase)}</p>
                        </div>
                      </div>
                      <div className={s.timerSide}>
                        <div className={s.timerDots} aria-hidden>
                          {Array.from({ length: pomodoro.settings.longBreakInterval }, (_, i) => (
                            <span
                              key={i}
                              className={i < pomodoroDotsFilled ? s.timerDotOn : s.timerDotOff}
                            />
                          ))}
                        </div>
                        <p className={s.timerStatus}>
                          {pomodoro.awaitingConfirmation
                            ? pomodoroReadyLine(pomodoro.phase)
                            : pomodoroStatusLine(pomodoro.phase, pomodoro.completedWork, pomodoro.settings.longBreakInterval)}
                        </p>
                        <div className={s.timerActions}>
                          <button type="button" className={s.timerBtn} onClick={pomodoro.toggle}>
                            {pomodoro.running ? (
                              <Pause size={18} strokeWidth={2.5} aria-hidden />
                            ) : (
                              <Play size={18} strokeWidth={2.5} aria-hidden />
                            )}
                            <span>
                              {pomodoro.running
                                ? 'Пауза'
                                : pomodoro.awaitingConfirmation
                                  ? pomodoroConfirmLabel(pomodoro.phase)
                                  : 'Старт'}
                            </span>
                          </button>
                          <button
                            type="button"
                            className={s.timerBtnSecondary}
                            title="Начать сессию заново с первого рабочего блока"
                            onClick={pomodoro.resetSession}
                          >
                            Сброс
                          </button>
                        </div>
                      </div>
                    </div>
                  </div>
                </section>

                {pomodoroSettingsOpen ? (
                  <PomodoroSettingsModal
                    settings={pomodoro.settings}
                    onSave={pomodoro.updateSettings}
                    onClose={() => setPomodoroSettingsOpen(false)}
                  />
                ) : null}

                <section className={`${s.card} ${s.progressCard} ${focusDimCls}`}>
                  <div className={s.cardHeader}>
                    <span className={s.cardIcon} aria-hidden>
                      <Trophy size={18} strokeWidth={2.25} />
                    </span>
                    <h2 className={s.cardTitle}>Прогресс</h2>
                  </div>
                  <div className={`${s.progress} ${s.progressThemed}`}>
                    <LevelProgress user={user} />
                  </div>
                </section>
              </div>

              <section className={`${s.card} ${s.aiAdviceCard} ${focusDimCls}`}>
                <div className={`${s.cardHeader} ${s.aiAdviceHeader}`}>
                  <div className={s.aiAdviceHeaderLeft}>
                    <span className={s.cardIcon} aria-hidden>
                      <Sparkles size={18} strokeWidth={2.25} />
                    </span>
                    <h2 className={s.cardTitle}>AI-совет дня</h2>
                  </div>
                  {adviceLinked && accessToken ? (
                    <button
                      type="button"
                      className={s.aiAdviceRefreshBtn}
                      disabled={adviceLoading}
                      aria-label="Обновить совет"
                      title="Обновить совет"
                      onClick={async () => {
                        if (!accessToken) return
                        try {
                          await loadAdvice(accessToken, { refresh: true, showSpinner: true })
                          lastAdviceTodaySigRef.current = todayEventsSig
                        } catch (e) {
                          setAdviceError(e instanceof Error ? e.message : 'Ошибка')
                        }
                      }}
                    >
                      <FontAwesomeIcon icon={faArrowsRotate} spin={adviceLoading} />
                    </button>
                  ) : null}
                </div>
                <div className={s.aiAdviceBody}>
                  <div className={s.aiHint}>
                    {adviceLoading ? (
                      <p className={s.muted}>Загружаю совет…</p>
                    ) : adviceError ? (
                      <p className={s.warn}>{adviceError}</p>
                    ) : !adviceLinked ? (
                      <>
                        <p className={s.aiText}>
                          <span className={s.aiGuillemet} aria-hidden>
                            «
                          </span>
                          Подключи Google Календарь — и я разберу твои события на сегодня и ближайшие дни, чтобы
                          подсказать, как лучше распределить время.
                          <span className={s.aiGuillemetEnd} aria-hidden>
                            »
                          </span>
                        </p>
                        <Link className={s.textLink} to="/profile">
                          Подключить в профиле
                        </Link>
                      </>
                    ) : adviceText ? (
                      <>
                        <p className={s.aiText}>
                          <span className={s.aiGuillemet} aria-hidden>
                            «
                          </span>
                          {adviceText}
                          <span className={s.aiGuillemetEnd} aria-hidden>
                            »
                          </span>
                        </p>
                        {adviceFootnote ? <p className={s.muted}>{adviceFootnote}</p> : null}
                      </>
                    ) : (
                      <>
                        <p className={s.muted}>Совет не сгенерировался — попробуй обновить позже.</p>
                        <Link className={s.textLink} to="/calendar">
                          Открыть календарь
                        </Link>
                      </>
                    )}
                  </div>
                </div>
              </section>
            </div>

            <section
              className={`${s.dayGauge} ${focusDimCls}`}
              aria-label={`День прошёл примерно на ${dayGaugeFilled} из ${DAY_GAUGE_SEGMENTS} долей`}
            >
              <span className={s.dayGaugeCaption}>Ход дня</span>
              <div className={s.dayGaugeTrack} role="presentation">
                {Array.from({ length: DAY_GAUGE_SEGMENTS }, (_, i) => {
                  const on = i < dayGaugeFilled
                  return (
                    <span
                      key={i}
                      className={[s.dayGaugeSeg, on ? s.dayGaugeSegOn : ''].filter(Boolean).join(' ')}
                      style={
                        on
                          ? {
                              background: `linear-gradient(180deg, ${dayGaugeSky.start} 0%, ${dayGaugeSky.end} 100%)`,
                              opacity: 1,
                            }
                          : undefined
                      }
                    />
                  )
                })}
              </div>
            </section>
          </div>

          <aside className={`${s.todayRight} ${focusDimCls}`} aria-label="Задачи на сегодня">
            <section className={`${s.card} ${s.tasksCard}`}>
              <div className={s.cardHeader}>
                <span className={s.cardIcon} aria-hidden>
                  <ClipboardList size={18} strokeWidth={2.25} />
                </span>
                <h2 className={s.cardTitle}>Задачи на сегодня</h2>
              </div>
              <div className={s.tasksScroll}>
                {loadingCal ? (
                  <p className={s.muted}>Загрузка…</p>
                ) : calError ? (
                  <p className={s.warn}>{calError}</p>
                ) : !linked ? (
                  <div className={s.stack}>
                    <p className={s.muted}>Календарь Google не подключён — события недоступны.</p>
                    <Link className={s.textLink} to="/profile">
                      Подключить в профиле
                    </Link>
                  </div>
                ) : todayEvents.length === 0 ? (
                  <div className={s.empty}>
                    <p className={s.emptyText}>На сегодня событий нет</p>
                    <Link className={s.addBtn} to="/calendar">
                      Открыть календарь
                    </Link>
                  </div>
                ) : (
                  <>
                    {taskActionError ? (
                      <p className={s.warn} role="alert">
                        {taskActionError}
                      </p>
                    ) : null}
                    <ul className={s.taskList}>
                      {todayEvents.map((ev, i) => (
                        <li
                          key={ev.id}
                          className={[s.taskRow, ev.spaceProposal ? s.taskRowSpace : ''].filter(Boolean).join(' ')}
                          aria-label={`Задача ${i + 1}: ${ev.title}`}
                        >
                          <button
                            type="button"
                            className={`${s.taskIconBtn} ${s.taskCheckboxBtn}`}
                            disabled={
                              taskActionBusy ||
                              Boolean(ev.completed) ||
                              isSpaceProposalGhostEvent(ev)
                            }
                            aria-busy={taskCompletingId === ev.id}
                            aria-pressed={ev.completed ? true : false}
                            aria-label={ev.completed ? 'Уже выполнено' : 'Отметить выполненным'}
                            title={
                              isSpaceProposalGhostEvent(ev)
                                ? 'Сначала появится в Google или измените в пространстве'
                                : ev.completed
                                  ? 'Уже выполнено'
                                  : 'Выполнено'
                            }
                            onClick={() => void handleCompleteTask(ev)}
                          >
                            <Check
                              size={17}
                              strokeWidth={2.5}
                              aria-hidden
                              className={ev.completed ? s.checkDone : s.checkHover}
                            />
                          </button>
                          <div className={s.taskMain}>
                            <div className={s.taskTitleRow}>
                              {ev.spaceProposal ? (
                                <SpaceProposalCalendarBadge
                                  displayName={ev.spaceProposal.authorDisplayName}
                                  avatarUrl={ev.spaceProposal.authorAvatarUrl}
                                  spaceName={ev.spaceProposal.spaceName}
                                />
                              ) : null}
                              <span
                                className={[s.taskTitle, ev.completed ? s.taskTitleDone : ''].filter(Boolean).join(' ')}
                              >
                                {ev.title}
                              </span>
                            </div>
                            <span className={s.taskTime}>{formatEventTimeRange(ev)}</span>
                            {isSpaceProposalGhostEvent(ev) ? (
                              <span className={s.taskSpaceGhostHint}>
                                Из пространства «{ev.spaceProposal?.spaceName}» — пока нет записи в Google.{' '}
                                <Link className={s.textLink} to={`/spaces/${ev.spaceProposal?.spaceId ?? ''}`}>
                                  Открыть
                                </Link>
                              </span>
                            ) : null}
                          </div>
                          <div className={s.taskActions}>
                            <button
                              type="button"
                              className={s.taskIconBtn}
                              disabled={taskActionBusy || isSpaceProposalGhostEvent(ev)}
                              aria-busy={taskDeletingId === ev.id}
                              aria-label="Удалить из календаря"
                              title={
                                isSpaceProposalGhostEvent(ev)
                                  ? 'Удаление только из пространства'
                                  : 'Удалить'
                              }
                              onClick={() => void handleDeleteTask(ev)}
                            >
                              <Trash2 size={16} strokeWidth={2.25} aria-hidden />
                            </button>
                          </div>
                        </li>
                      ))}
                    </ul>
                  </>
                )}
              </div>
              {!loadingCal && !calError && linked && todayEvents.length > 0 ? (
                <div className={s.cardFooter}>
                  <Link className={s.textLink} to="/calendar">
                    Открыть календарь
                  </Link>
                </div>
              ) : null}
            </section>
          </aside>
        </div>
      </div>
    </div>
  )
}
