import { useCallback, useEffect, useReducer } from 'react'

/** Совпадает с <title> в index.html — для восстановления вкладки */
export const POMODORO_TAB_TITLE_BASE = 'Карманный секретарь'

/** Переживает переключение вкладок навигации, разгрузку фоновой вкладки и перезагрузку страницы */
const STORAGE_KEY = 'pomodoro-timer-v1'

export type PomodoroPhase = 'work' | 'shortBreak' | 'longBreak'

export type PomodoroSettings = {
  workSec: number
  shortBreakSec: number
  longBreakSec: number
  /** Сколько рабочих блоков подряд до длинного перерыва */
  longBreakInterval: number
}

export const DEFAULT_POMODORO_SETTINGS: PomodoroSettings = {
  workSec: 25 * 60,
  shortBreakSec: 5 * 60,
  longBreakSec: 15 * 60,
  longBreakInterval: 4,
}

export const POMODORO_PRESETS: { id: string; label: string; settings: PomodoroSettings }[] = [
  { id: 'classic', label: 'Классика 25/5/15', settings: DEFAULT_POMODORO_SETTINGS },
  {
    id: 'deep-focus',
    label: 'Глубокий фокус 50/10/30',
    settings: { workSec: 50 * 60, shortBreakSec: 10 * 60, longBreakSec: 30 * 60, longBreakInterval: 2 },
  },
]

function clampInt(value: number, min: number, max: number, fallback: number): number {
  if (!Number.isFinite(value)) return fallback
  return Math.min(max, Math.max(min, Math.round(value)))
}

export function sanitizeSettings(raw: Partial<PomodoroSettings> | null | undefined): PomodoroSettings {
  const d = DEFAULT_POMODORO_SETTINGS
  return {
    workSec: clampInt(raw?.workSec ?? d.workSec, 60, 180 * 60, d.workSec),
    shortBreakSec: clampInt(raw?.shortBreakSec ?? d.shortBreakSec, 60, 60 * 60, d.shortBreakSec),
    longBreakSec: clampInt(raw?.longBreakSec ?? d.longBreakSec, 60, 120 * 60, d.longBreakSec),
    longBreakInterval: clampInt(raw?.longBreakInterval ?? d.longBreakInterval, 1, 12, d.longBreakInterval),
  }
}

export function phaseDurationSeconds(phase: PomodoroPhase, settings: PomodoroSettings): number {
  switch (phase) {
    case 'work':
      return settings.workSec
    case 'shortBreak':
      return settings.shortBreakSec
    case 'longBreak':
      return settings.longBreakSec
  }
}

type State = {
  settings: PomodoroSettings
  phase: PomodoroPhase
  remaining: number
  running: boolean
  completedWork: number
  /** Пока идёт отсчёт — момент окончания текущей фазы (для устойчивости к throttling вкладки) */
  phaseEndsAtMs: number | null
  /** Фаза завершилась сама — ждём, когда пользователь подтвердит старт следующей */
  awaitingConfirmation: boolean
  /** Звук/уведомление о завершении уже показаны для текущего awaitingConfirmation */
  notified: boolean
}

type Action =
  | { type: 'TICK' }
  | { type: 'TOGGLE' }
  | { type: 'RESET_SESSION' }
  | { type: 'NOTIFIED' }
  | { type: 'UPDATE_SETTINGS'; settings: PomodoroSettings }

function transitionAfterWork(
  completedWork: number,
  settings: PomodoroSettings,
): { phase: PomodoroPhase; remaining: number } {
  if (completedWork % settings.longBreakInterval === 0) {
    return { phase: 'longBreak', remaining: settings.longBreakSec }
  }
  return { phase: 'shortBreak', remaining: settings.shortBreakSec }
}

/** Однократный переход на следующую фазу с остановкой — ждём явного подтверждения пользователя */
function completePhase(state: State): State {
  if (state.phase === 'work') {
    const completed = state.completedWork + 1
    const t = transitionAfterWork(completed, state.settings)
    return {
      ...state,
      phase: t.phase,
      remaining: t.remaining,
      completedWork: completed,
      running: false,
      phaseEndsAtMs: null,
      awaitingConfirmation: true,
      notified: false,
    }
  }
  return {
    ...state,
    phase: 'work',
    remaining: state.settings.workSec,
    running: false,
    phaseEndsAtMs: null,
    awaitingConfirmation: true,
    notified: false,
  }
}

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case 'TOGGLE': {
      if (state.running) {
        const rem =
          state.phaseEndsAtMs != null
            ? Math.max(0, Math.ceil((state.phaseEndsAtMs - Date.now()) / 1000))
            : state.remaining
        return { ...state, running: false, remaining: rem, phaseEndsAtMs: null }
      }
      return {
        ...state,
        running: true,
        phaseEndsAtMs: Date.now() + state.remaining * 1000,
        awaitingConfirmation: false,
        notified: false,
      }
    }
    case 'RESET_SESSION':
      // Полный сброс сессии — назад к первому рабочему блоку, а не только к началу текущей фазы
      return {
        ...state,
        phase: 'work',
        completedWork: 0,
        running: false,
        remaining: state.settings.workSec,
        phaseEndsAtMs: null,
        awaitingConfirmation: false,
        notified: false,
      }
    case 'NOTIFIED':
      return state.notified ? state : { ...state, notified: true }
    case 'UPDATE_SETTINGS': {
      const settings = sanitizeSettings(action.settings)
      if (state.running) {
        // Не прерываем активную сессию — новые длительности вступят в силу со следующей фазы
        return { ...state, settings }
      }
      return { ...state, settings, remaining: phaseDurationSeconds(state.phase, settings) }
    }
    case 'TICK': {
      if (!state.running || state.phaseEndsAtMs === null) return state
      const now = Date.now()
      if (now >= state.phaseEndsAtMs) {
        return completePhase(state)
      }
      const rem = Math.max(0, Math.ceil((state.phaseEndsAtMs - now) / 1000))
      return rem === state.remaining ? state : { ...state, remaining: rem }
    }
    default:
      return state
  }
}

const initialState: State = {
  settings: DEFAULT_POMODORO_SETTINGS,
  phase: 'work',
  remaining: DEFAULT_POMODORO_SETTINGS.workSec,
  running: false,
  completedWork: 0,
  phaseEndsAtMs: null,
  awaitingConfirmation: false,
  notified: false,
}

function loadPersisted(): State {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (!raw) return initialState
    const saved = JSON.parse(raw) as Partial<State>
    if (saved.phase !== 'work' && saved.phase !== 'shortBreak' && saved.phase !== 'longBreak') {
      return initialState
    }
    const phase = saved.phase
    const settings = sanitizeSettings(saved.settings)
    const base: State = {
      settings,
      phase,
      remaining: typeof saved.remaining === 'number' ? saved.remaining : phaseDurationSeconds(phase, settings),
      running: Boolean(saved.running),
      completedWork: typeof saved.completedWork === 'number' ? saved.completedWork : 0,
      phaseEndsAtMs: typeof saved.phaseEndsAtMs === 'number' ? saved.phaseEndsAtMs : null,
      awaitingConfirmation: Boolean(saved.awaitingConfirmation),
      notified: Boolean(saved.notified),
    }
    if (!base.running || base.phaseEndsAtMs === null) {
      return { ...base, running: false, phaseEndsAtMs: null }
    }
    const now = Date.now()
    if (now >= base.phaseEndsAtMs) {
      return completePhase(base)
    }
    return { ...base, remaining: Math.max(0, Math.ceil((base.phaseEndsAtMs - now) / 1000)) }
  } catch {
    return initialState
  }
}

/**
 * Один AudioContext на вкладку, создаётся/разблокируется по реальному клику (см. unlockChimeAudio).
 * Звук, запущенный таймером в фоне, слышен только если контекст уже был разблокирован жестом —
 * так требует autoplay policy браузеров (особенно строго в Safari).
 */
let chimeAudioCtx: AudioContext | null = null

/** Вызывать прямо из обработчика клика — иначе браузер молча заглушит звук таймера */
function unlockChimeAudio() {
  try {
    if (typeof window.AudioContext === 'undefined') return
    if (!chimeAudioCtx) chimeAudioCtx = new window.AudioContext()
    if (chimeAudioCtx.state === 'suspended') void chimeAudioCtx.resume()
  } catch {
    /* аудио недоступно */
  }
}

/** Мягкий двухнотный «колокольчик» через WebAudio — без внешнего аудиофайла */
function playChime() {
  const ctx = chimeAudioCtx
  if (!ctx) return
  try {
    if (ctx.state === 'suspended') void ctx.resume()
    const now = ctx.currentTime
    const filter = ctx.createBiquadFilter()
    filter.type = 'lowpass'
    filter.frequency.value = 1600
    filter.connect(ctx.destination)
    // Две ноты внахлёст (терция) с медленной атакой и долгим затуханием — звучит как тихий звоночек, а не сигнал тревоги
    ;[
      { offset: 0, freq: 659.25 },
      { offset: 0.22, freq: 523.25 },
    ].forEach(({ offset, freq }) => {
      const osc = ctx.createOscillator()
      const gain = ctx.createGain()
      osc.type = 'sine'
      osc.frequency.value = freq
      gain.gain.setValueAtTime(0.0001, now + offset)
      gain.gain.exponentialRampToValueAtTime(0.14, now + offset + 0.08)
      gain.gain.exponentialRampToValueAtTime(0.0001, now + offset + 0.9)
      osc.connect(gain)
      gain.connect(filter)
      osc.start(now + offset)
      osc.stop(now + offset + 0.9)
    })
  } catch {
    /* аудио недоступно (например, автоплей заблокирован) */
  }
}

function notifyPhaseComplete(nextPhase: PomodoroPhase) {
  if (typeof Notification === 'undefined' || Notification.permission !== 'granted') return
  try {
    new Notification(POMODORO_TAB_TITLE_BASE, {
      body: nextPhase === 'work' ? 'Перерыв закончился — пора поработать' : 'Рабочий блок завершён — время на перерыв',
    })
  } catch {
    /* уведомления недоступны */
  }
}

export function usePomodoro() {
  const [state, dispatch] = useReducer(reducer, undefined, loadPersisted)

  useEffect(() => {
    try {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(state))
    } catch {
      /* приватный режим / квота хранилища */
    }
  }, [state])

  useEffect(() => {
    if (!state.running) return
    const tick = () => dispatch({ type: 'TICK' })
    const id = window.setInterval(tick, 1000)
    const onVis = () => {
      if (document.visibilityState === 'visible') tick()
    }
    document.addEventListener('visibilitychange', onVis)
    return () => {
      document.removeEventListener('visibilitychange', onVis)
      window.clearInterval(id)
    }
  }, [state.running])

  useEffect(() => {
    if (state.awaitingConfirmation && !state.notified) {
      playChime()
      notifyPhaseComplete(state.phase)
      dispatch({ type: 'NOTIFIED' })
    }
  }, [state.awaitingConfirmation, state.notified, state.phase])

  useEffect(() => {
    if (state.awaitingConfirmation) {
      const label = state.phase === 'work' ? 'Перерыв окончен, пора работать' : 'Работа окончена, время отдыха'
      document.title = `⏰ ${label} · ${POMODORO_TAB_TITLE_BASE}`
      return () => {
        document.title = POMODORO_TAB_TITLE_BASE
      }
    }
    if (!state.running) {
      document.title = POMODORO_TAB_TITLE_BASE
      return
    }
    document.title = `${formatPomodoroClock(state.remaining)} · ${POMODORO_TAB_TITLE_BASE}`
    return () => {
      document.title = POMODORO_TAB_TITLE_BASE
    }
  }, [state.running, state.remaining, state.awaitingConfirmation])

  const toggle = useCallback(() => {
    if (!state.running) {
      unlockChimeAudio()
      if (typeof Notification !== 'undefined' && Notification.permission === 'default') {
        void Notification.requestPermission()
      }
    }
    dispatch({ type: 'TOGGLE' })
  }, [state.running])

  const resetSession = useCallback(() => {
    dispatch({ type: 'RESET_SESSION' })
  }, [])

  const updateSettings = useCallback((settings: PomodoroSettings) => {
    dispatch({ type: 'UPDATE_SETTINGS', settings })
  }, [])

  return {
    phase: state.phase,
    remaining: state.remaining,
    running: state.running,
    completedWork: state.completedWork,
    awaitingConfirmation: state.awaitingConfirmation,
    settings: state.settings,
    toggle,
    resetSession,
    updateSettings,
  }
}

export function formatPomodoroClock(totalSeconds: number): string {
  const m = Math.floor(totalSeconds / 60)
  const s = totalSeconds % 60
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}
