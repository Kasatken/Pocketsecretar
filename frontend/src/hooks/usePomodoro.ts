import { useCallback, useEffect, useReducer } from 'react'

const WORK_SEC = 25 * 60
const SHORT_BREAK_SEC = 5 * 60
const LONG_BREAK_SEC = 15 * 60

/** Совпадает с <title> в index.html — для восстановления вкладки */
export const POMODORO_TAB_TITLE_BASE = 'Карманный секретарь'

export type PomodoroPhase = 'work' | 'shortBreak' | 'longBreak'

export function phaseDurationSeconds(phase: PomodoroPhase): number {
  switch (phase) {
    case 'work':
      return WORK_SEC
    case 'shortBreak':
      return SHORT_BREAK_SEC
    case 'longBreak':
      return LONG_BREAK_SEC
  }
}

type State = {
  phase: PomodoroPhase
  remaining: number
  running: boolean
  completedWork: number
  /** Пока идёт отсчёт — момент окончания текущей фазы (для устойчивости к throttling вкладки) */
  phaseEndsAtMs: number | null
}

type Action = { type: 'TICK' } | { type: 'TOGGLE' } | { type: 'RESET_PHASE' }

function transitionAfterWork(completedWork: number): { phase: PomodoroPhase; remaining: number } {
  const n = completedWork
  if (n % 4 === 0) {
    return { phase: 'longBreak', remaining: LONG_BREAK_SEC }
  }
  return { phase: 'shortBreak', remaining: SHORT_BREAK_SEC }
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
      }
    }
    case 'RESET_PHASE':
      return {
        ...state,
        running: false,
        remaining: phaseDurationSeconds(state.phase),
        phaseEndsAtMs: null,
      }
    case 'TICK': {
      if (!state.running || state.phaseEndsAtMs === null) return state

      let next: State = state
      let guard = 0
      while (
        next.running &&
        next.phaseEndsAtMs !== null &&
        Date.now() >= next.phaseEndsAtMs &&
        guard++ < 100
      ) {
        const now = Date.now()
        if (next.phase === 'work') {
          const completed = next.completedWork + 1
          const t = transitionAfterWork(completed)
          next = {
            ...next,
            phase: t.phase,
            remaining: t.remaining,
            completedWork: completed,
            phaseEndsAtMs: next.running ? now + t.remaining * 1000 : null,
          }
        } else {
          next = {
            ...next,
            phase: 'work',
            remaining: WORK_SEC,
            phaseEndsAtMs: next.running ? now + WORK_SEC * 1000 : null,
          }
        }
      }

      if (!next.running || next.phaseEndsAtMs === null) return next
      const rem = Math.max(0, Math.ceil((next.phaseEndsAtMs - Date.now()) / 1000))
      return rem === next.remaining ? next : { ...next, remaining: rem }
    }
    default:
      return state
  }
}

const initialState: State = {
  phase: 'work',
  remaining: WORK_SEC,
  running: false,
  completedWork: 0,
  phaseEndsAtMs: null,
}

export function usePomodoro() {
  const [state, dispatch] = useReducer(reducer, initialState)

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
    if (!state.running) {
      document.title = POMODORO_TAB_TITLE_BASE
      return
    }
    document.title = `${formatPomodoroClock(state.remaining)} · ${POMODORO_TAB_TITLE_BASE}`
    return () => {
      document.title = POMODORO_TAB_TITLE_BASE
    }
  }, [state.running, state.remaining])

  const toggle = useCallback(() => {
    dispatch({ type: 'TOGGLE' })
  }, [])

  const resetPhase = useCallback(() => {
    dispatch({ type: 'RESET_PHASE' })
  }, [])

  return {
    phase: state.phase,
    remaining: state.remaining,
    running: state.running,
    completedWork: state.completedWork,
    toggle,
    resetPhase,
  }
}

export function formatPomodoroClock(totalSeconds: number): string {
  const m = Math.floor(totalSeconds / 60)
  const s = totalSeconds % 60
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}
