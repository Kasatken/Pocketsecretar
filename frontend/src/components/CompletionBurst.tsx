import { useCallback, useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import type { User } from '../auth/AuthContext'
import s from './CompletionBurst.module.css'

type Burst = {
  key: number
  xp: number
  /** Новый уровень, если он вырос; null — обычное выполнение. */
  levelUp: number | null
}

/**
 * Празднование выполнения дела: всплывающий «+25 XP», при росте уровня — конфетти.
 * celebrate(prevUser, nextUser) — nextUser берите из ответа /complete (там свежие XP и уровень).
 */
export function useCompletionBurst() {
  const [burst, setBurst] = useState<Burst | null>(null)
  const timerRef = useRef<number | null>(null)

  useEffect(() => {
    return () => {
      if (timerRef.current !== null) window.clearTimeout(timerRef.current)
    }
  }, [])

  const celebrate = useCallback((prevUser: User | null, nextUser: User | null | undefined) => {
    const gained =
      prevUser && nextUser && nextUser.experience_points > prevUser.experience_points
        ? nextUser.experience_points - prevUser.experience_points
        : 25
    const levelUp = prevUser && nextUser && nextUser.level > prevUser.level ? nextUser.level : null
    setBurst({ key: Date.now(), xp: gained, levelUp })
    if (timerRef.current !== null) window.clearTimeout(timerRef.current)
    timerRef.current = window.setTimeout(() => setBurst(null), levelUp !== null ? 2600 : 1600)
  }, [])

  const burstNode = burst
    ? createPortal(
        <div key={burst.key} className={s.burstRoot} aria-hidden>
          {burst.levelUp !== null ? (
            <div className={s.levelWrap}>
              <div className={s.confetti}>
                {Array.from({ length: 12 }, (_, i) => (
                  <span key={i} className={s.confettiPiece} />
                ))}
              </div>
              <div className={s.levelPill}>Уровень {burst.levelUp}!</div>
              <div className={s.xpPillSmall}>+{burst.xp} XP</div>
            </div>
          ) : (
            <div className={s.xpPill}>+{burst.xp} XP</div>
          )}
        </div>,
        document.body,
      )
    : null

  return { burstNode, celebrate }
}
