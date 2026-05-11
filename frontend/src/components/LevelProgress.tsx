import type { User } from '../auth/AuthContext'
import s from './LevelProgress.module.css'

const XP_PER_LEVEL_SEGMENT = 100

export function xpProgressPercent(xp: number): number {
  return xp % XP_PER_LEVEL_SEGMENT
}

/** XP still needed within the current level segment (100 XP per level). */
export function xpRemainingToNextLevel(xp: number): number {
  const m = xp % XP_PER_LEVEL_SEGMENT
  if (xp > 0 && m === 0) return XP_PER_LEVEL_SEGMENT
  return XP_PER_LEVEL_SEGMENT - m
}

type LevelProgressProps = {
  user: User | null
  variant?: 'card' | 'inline'
}

export function LevelProgress({ user, variant = 'card' }: LevelProgressProps) {
  const xp = user?.experience_points ?? 0
  const level = user?.level ?? 1
  const pct = xpProgressPercent(xp)
  const remaining = xpRemainingToNextLevel(xp)

  let hint: string
  if (xp === 0) {
    hint = 'Выполни задачу в календаре, чтобы получить XP'
  } else if (xp > 0 && xp % XP_PER_LEVEL_SEGMENT === 0) {
    hint = 'Уровень повышен — до следующего нужно 100 XP'
  } else {
    hint = `До следующего уровня: ${remaining} XP`
  }

  if (variant === 'inline') {
    return (
      <div className={s.wrapInline}>
        <div className={s.progressTrack}>
          <div className={s.progressFill} style={{ width: `${pct}%` }} />
        </div>
        <p className={s.progressHint}>
          {hint} · ур. {level}
        </p>
      </div>
    )
  }

  return (
    <div className={s.wrap}>
      <div className={s.levelRow}>
        <span className={s.levelLabel}>Уровень {level}</span>
        <span className={s.levelXp}>{xp.toLocaleString('ru-RU')} XP</span>
      </div>
      <div className={s.progressTrack}>
        <div className={s.progressFill} style={{ width: `${pct}%` }} />
      </div>
      <p className={s.progressHint}>{hint}</p>
    </div>
  )
}
