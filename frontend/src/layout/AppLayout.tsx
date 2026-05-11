import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, NavLink, Outlet } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'
import type { User } from '../auth/AuthContext'
import SiteFooter from '../components/SiteFooter'
import { APP_NAME, LOGO_URL } from '../lib/branding'
import { PENDING_PROPOSALS_CHANGED } from '../lib/pendingProposalsEvents'
import { fetchPendingProposalsForMe, type PendingProposalSpaceRow } from '../lib/spacesApi'
import s from './AppLayout.module.css'

function userDisplayName(user: User | null) {
  if (!user) return 'Профиль'
  const n = user.display_name?.trim()
  if (n) return n
  return user.email
}

function userInitial(user: User | null) {
  if (!user) return '?'
  const label = user.display_name?.trim() || user.email
  const ch = label.trim().charAt(0).toUpperCase()
  return ch || '?'
}

function pendingProposalWord(n: number) {
  const m = n % 100
  if (m >= 11 && m <= 14) return 'предложений'
  const k = n % 10
  if (k === 1) return 'предложение'
  if (k >= 2 && k <= 4) return 'предложения'
  return 'предложений'
}

function pendingSnapshotSig(total: number, rows: PendingProposalSpaceRow[]) {
  return `${total}|${rows.map((r) => `${r.space_id}:${r.pending_count}:${r.latest_created}`).join(';')}`
}

export default function AppLayout() {
  const { user, accessToken } = useAuth()
  const name = userDisplayName(user)
  const avatarUrl = user?.avatar_url?.trim()

  const [pendingTotal, setPendingTotal] = useState(0)
  const [pendingSpaces, setPendingSpaces] = useState<PendingProposalSpaceRow[]>([])
  const [suppressedBarSig, setSuppressedBarSig] = useState<string | null>(null)

  const pendingSig = useMemo(
    () => pendingSnapshotSig(pendingTotal, pendingSpaces),
    [pendingTotal, pendingSpaces],
  )

  const loadPending = useCallback(async () => {
    if (!accessToken) return
    try {
      const { total, spaces } = await fetchPendingProposalsForMe(accessToken)
      setPendingTotal(total)
      setPendingSpaces(spaces)
    } catch {
      /* тихо: сеть / 401 при истечении сессии */
    }
  }, [accessToken])

  useEffect(() => {
    void loadPending()
  }, [loadPending])

  useEffect(() => {
    if (!accessToken) return
    const t = window.setInterval(() => void loadPending(), 90_000)
    const onVis = () => {
      if (document.visibilityState === 'visible') void loadPending()
    }
    const onPendingChanged = () => void loadPending()
    window.addEventListener(PENDING_PROPOSALS_CHANGED, onPendingChanged)
    document.addEventListener('visibilitychange', onVis)
    return () => {
      window.clearInterval(t)
      window.removeEventListener(PENDING_PROPOSALS_CHANGED, onPendingChanged)
      document.removeEventListener('visibilitychange', onVis)
    }
  }, [accessToken, loadPending])

  const showPendingBar =
    pendingTotal > 0 && pendingSpaces.length > 0 && suppressedBarSig !== pendingSig

  const firstPendingSpace = pendingSpaces[0]

  const segmentClass = ({ isActive }: { isActive: boolean }) =>
    [s.segLink, isActive ? s.segLinkActive : ''].join(' ')

  return (
    <div className={s.layout}>
      <header className={s.topBar} role="banner">
        <div className={s.topBarInner}>
          <Link to="/app" className={s.brand} title={APP_NAME}>
            <img src={LOGO_URL} alt={APP_NAME} className={s.brandLogo} width={180} height={50} />
          </Link>

          <nav className={s.segmentWrap} aria-label="Разделы приложения">
            <div className={s.segment}>
              <NavLink className={segmentClass} to="/app" end>
                Сегодня
              </NavLink>
              <NavLink className={segmentClass} to="/calendar">
                Календарь
              </NavLink>
              <NavLink className={segmentClass} to="/spaces">
                <span className={s.segLinkInner}>
                  Пространства
                  {pendingTotal > 0 ? (
                    <span className={s.navBadge} aria-label={`Ожидают ответа: ${pendingTotal}`}>
                      {pendingTotal > 99 ? '99+' : pendingTotal}
                    </span>
                  ) : null}
                </span>
              </NavLink>
            </div>
          </nav>

          <NavLink
            to="/profile"
            className={({ isActive }: { isActive: boolean }) =>
              [s.profileBtn, isActive ? s.profileBtnActive : ''].join(' ')
            }
            aria-label={`Профиль: ${name}`}
          >
            <div className={s.profileAvatar} aria-hidden>
              {avatarUrl ? <img src={avatarUrl} alt="" /> : <span>{userInitial(user)}</span>}
            </div>
            <span className={s.profileName}>{name}</span>
          </NavLink>
        </div>
      </header>

      {showPendingBar && firstPendingSpace ? (
        <div className={s.pendingBarWrap}>
          <div className={s.pendingBar} role="status">
            <span>
              У вас {pendingTotal} {pendingProposalWord(pendingTotal)} задач в пространствах — ответьте, когда будет
              удобно.
            </span>
            <Link className={s.pendingBarLink} to={`/spaces/${firstPendingSpace.space_id}#proposals`}>
              Открыть предложения
            </Link>
            <button
              type="button"
              className={s.pendingBarDismiss}
              onClick={() => setSuppressedBarSig(pendingSig)}
              aria-label="Скрыть напоминание до новых предложений"
            >
              Скрыть
            </button>
          </div>
        </div>
      ) : null}

      <main className={s.main}>
        <Outlet />
      </main>

      <SiteFooter />
    </div>
  )
}
