import { createPortal } from 'react-dom'
import { useEffect, useId, useState, type ChangeEvent, type FormEvent } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'
import type { User } from '../auth/AuthContext'
import { CHRONOTYPE_OPTIONS, chronotypeLabel } from '../lib/chronotypes'
import { formatQuotaResetsAtHint } from '../lib/smartTaskQuota'
import { LevelProgress } from '../components/LevelProgress'
import s from './ProfilePage.module.css'

function initialLetter(user: User | null) {
  if (!user) return '?'
  const label = user.display_name?.trim() || user.email
  const ch = label.trim().charAt(0).toUpperCase()
  return ch || '?'
}

function planLabel(plan: string) {
  if (plan === 'pro') return 'Pro'
  return 'Free'
}

type GoogleStatus = {
  configured: boolean
  linked: boolean
  google_email?: string
}

type XpRankResponse = {
  rank: number
  total_users: number
}

export default function ProfilePage() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { user, accessToken, updateProfile, uploadAvatar, redeemPromo, logout } = useAuth()
  const [google, setGoogle] = useState<GoogleStatus | null>(null)
  const [flash, setFlash] = useState<{ type: 'ok' | 'err'; text: string } | null>(null)
  const [saving, setSaving] = useState(false)
  const [editingProfile, setEditingProfile] = useState(false)
  const [promoCode, setPromoCode] = useState('')
  const [promoBusy, setPromoBusy] = useState(false)
  const [promoErr, setPromoErr] = useState<string | null>(null)

  const [displayName, setDisplayName] = useState('')
  const [bio, setBio] = useState('')
  const [personalizationOpen, setPersonalizationOpen] = useState(false)
  const [persNote, setPersNote] = useState('')
  const [persChrono, setPersChrono] = useState('')
  const [savingPers, setSavingPers] = useState(false)
  const [xpRank, setXpRank] = useState<XpRankResponse | null>(null)
  const [statsHelpOpen, setStatsHelpOpen] = useState(false)
  const statsHelpTitleId = useId()

  type UserStats = {
    smart_tasks_week: number
    completed_week: number
    total_completed: number
    streak: number
    hours_saved: number
  }
  const [stats, setStats] = useState<UserStats | null>(null)

  const quota = user?.smart_tasks_today
  const quotaExhausted = quota != null && quota.used >= quota.limit
  const quotaLow =
    quota != null &&
    quota.limit - quota.used < Math.max(2, Math.ceil(quota.limit * 0.25))

  useEffect(() => {
    if (!user) return
    setDisplayName(user.display_name?.trim() ?? '')
    setBio(user.bio ?? '')
  }, [user])

  useEffect(() => {
    const g = searchParams.get('google')
    const reason = searchParams.get('reason')
    if (g === 'connected') {
      setFlash({ type: 'ok', text: 'Google Calendar подключён.' })
      navigate('/profile', { replace: true })
    } else if (g === 'error') {
      setFlash({
        type: 'err',
        text:
          reason === 'access_denied'
            ? 'Доступ отменён в окне Google.'
            : `Не удалось подключить (${reason ?? 'ошибка'}).`,
      })
      navigate('/profile', { replace: true })
    }
  }, [searchParams, navigate])

  useEffect(() => {
    if (!accessToken) return
    let cancelled = false
    ;(async () => {
      try {
        const res = await fetch('/api/auth/google/status', {
          headers: { Authorization: `Bearer ${accessToken}` },
          credentials: 'include',
        })
        if (!res.ok || cancelled) return
        const data = (await res.json()) as GoogleStatus
        if (!cancelled) setGoogle(data)
      } catch {
        if (!cancelled) setGoogle({ configured: false, linked: false })
      }
    })()
    return () => {
      cancelled = true
    }
  }, [accessToken])

  useEffect(() => {
    if (!accessToken) return
    let cancelled = false
    ;(async () => {
      try {
        const res = await fetch('/api/auth/me/stats', {
          headers: { Authorization: `Bearer ${accessToken}` },
          credentials: 'include',
        })
        if (!res.ok || cancelled) return
        const data = (await res.json()) as UserStats
        if (!cancelled) setStats(data)
      } catch {
        /* статистика необязательна */
      }
    })()
    return () => { cancelled = true }
  }, [accessToken])

  useEffect(() => {
    if (!accessToken) return
    let cancelled = false
    ;(async () => {
      try {
        const res = await fetch('/api/auth/me/xp-rank', {
          headers: { Authorization: `Bearer ${accessToken}` },
          credentials: 'include',
        })
        if (!res.ok || cancelled) return
        const data = (await res.json()) as XpRankResponse
        if (
          !cancelled &&
          typeof data.rank === 'number' &&
          typeof data.total_users === 'number' &&
          data.total_users > 0
        ) {
          setXpRank(data)
        }
      } catch {
        /* рейтинг необязателен для страницы */
      }
    })()
    return () => {
      cancelled = true
    }
  }, [accessToken, user?.experience_points])

  const cancelEdit = () => {
    if (!user) return
    setDisplayName(user.display_name?.trim() ?? '')
    setBio(user.bio ?? '')
    setEditingProfile(false)
  }

  const handleSave = async (e: FormEvent) => {
    e.preventDefault()
    if (!user) return
    setSaving(true)
    setFlash(null)
    try {
      await updateProfile({
        display_name: displayName.trim(),
        bio: bio.trim(),
      })
      setFlash({ type: 'ok', text: 'Сохранено.' })
      setEditingProfile(false)
    } catch (err) {
      setFlash({
        type: 'err',
        text: err instanceof Error ? err.message : 'Не удалось сохранить.',
      })
    } finally {
      setSaving(false)
    }
  }

  const onAvatarFile = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (!file) return
    setFlash(null)
    try {
      await uploadAvatar(file)
      setFlash({ type: 'ok', text: 'Фото обновлено.' })
    } catch (err) {
      setFlash({
        type: 'err',
        text: err instanceof Error ? err.message : 'Не удалось загрузить файл.',
      })
    }
  }

  const handlePromo = async (e: FormEvent) => {
    e.preventDefault()
    const code = promoCode.trim()
    if (!code) {
      setPromoErr('Введите промокод')
      return
    }
    setPromoBusy(true)
    setPromoErr(null)
    setFlash(null)
    try {
      const { alreadyPro } = await redeemPromo(code)
      setPromoCode('')
      if (alreadyPro) {
        setFlash({ type: 'ok', text: 'У вас уже активирован тариф Pro.' })
      } else {
        setFlash({ type: 'ok', text: 'Промокод активирован — у вас тариф Pro.' })
      }
    } catch (err) {
      setPromoErr(err instanceof Error ? err.message : 'Не удалось применить промокод')
    } finally {
      setPromoBusy(false)
    }
  }

  const handleLogout = async () => {
    try {
      await logout()
    } catch {
      // Очищаем сессию на клиенте даже если запрос упал (офлайн, таймаут).
    }
    navigate('/', { replace: true })
  }

  const handleSavePersonalization = async () => {
    setSavingPers(true)
    setFlash(null)
    try {
      await updateProfile({
        personalization_note: persNote.trim(),
        chronotype: persChrono.trim(),
      })
      setFlash({ type: 'ok', text: 'Персонализация сохранена.' })
      setPersonalizationOpen(false)
    } catch (err) {
      setFlash({
        type: 'err',
        text: err instanceof Error ? err.message : 'Не удалось сохранить персонализацию.',
      })
    } finally {
      setSavingPers(false)
    }
  }

  const shownName = user?.display_name?.trim()
  const avatarUrl = user?.avatar_url?.trim()
  const bioPreview = user?.bio?.trim()
  const hasPersonalization =
    Boolean(user?.personalization_note?.trim()) || Boolean(user?.chronotype?.trim())

  const personalizationSummary = (() => {
    const parts: string[] = []
    const cl = chronotypeLabel(user?.chronotype)
    if (cl && (user?.chronotype ?? '').trim()) parts.push(cl)
    const n = user?.personalization_note?.trim()
    if (n) parts.push(n.length > 72 ? `${n.slice(0, 72)}…` : n)
    return parts.join(' · ')
  })()

  return (
    <div className={s.page}>
      <h1 className={s.visuallyHidden}>Настройки</h1>

      <header className={s.pageIntro}>
        <span className={s.introBadge}>
          <span className={s.introPulse} aria-hidden />
          Старт
        </span>
        <p className={s.pageLead}>
          Имя и фото — чтобы вас узнавали. Календарь — чтобы день подтягивал встречи без ручного ввода.
        </p>
      </header>

      {flash ? (
        <div className={flash.type === 'ok' ? s.flashOk : s.flashErr} role="status">
          <span className={s.flashIcon} aria-hidden>
            <i
              className={`fa-solid ${flash.type === 'ok' ? 'fa-check' : 'fa-circle-exclamation'} ${s.flashFa}`}
            />
          </span>
          <span className={s.flashText}>{flash.text}</span>
        </div>
      ) : null}

      <div className={s.sheet}>
        <section className={`${s.card} ${s.cardProfile}`} aria-labelledby="profile-heading">
          <div className={s.cardHeader}>
            <span className={s.cardIcon} aria-hidden>
              <i className={`fa-solid fa-user ${s.faCardIcon}`} />
            </span>
            <div className={s.cardHeaderText}>
              <p className={s.cardEyebrow}>Профиль</p>
              <h2 id="profile-heading" className={s.cardTitle}>
                Имя и аватар
              </h2>
            </div>
            <div className={s.cardHeaderActions}>
              {!editingProfile ? (
                <button
                  type="button"
                  className={s.editToggleBtn}
                  onClick={() => setEditingProfile(true)}
                  aria-expanded={editingProfile}
                  aria-controls="profile-edit-panel"
                  title="Редактировать"
                >
                  <i className={`fa-solid fa-pen-to-square ${s.faEditIcon}`} aria-hidden />
                  <span className={s.visuallyHidden}>Редактировать</span>
                </button>
              ) : (
                <button type="button" className={s.editCancelBtn} onClick={cancelEdit}>
                  Отмена
                </button>
              )}
              <button
                type="button"
                className={s.logoutIconBtn}
                onClick={handleLogout}
                title="Выйти из аккаунта"
              >
                <i className={`fa-solid fa-right-from-bracket ${s.faEditIcon}`} aria-hidden />
                <span className={s.visuallyHidden}>Выйти из аккаунта</span>
              </button>
            </div>
          </div>

          <div className={s.profileBlock}>
            <div className={s.avatarWrap}>
              <div className={s.avatarRing} aria-hidden />
              <div className={s.avatar}>
                {avatarUrl ? (
                  <img className={s.avatarImg} src={avatarUrl} alt="" width={88} height={88} />
                ) : (
                  (user ? initialLetter(user) : '?')
                )}
              </div>
              {editingProfile ? (
                <label className={s.avatarUploadBtn}>
                  <input
                    type="file"
                    accept="image/jpeg,image/png,image/webp"
                    className={s.visuallyHidden}
                    onChange={onAvatarFile}
                  />
                  Загрузить фото
                </label>
              ) : null}
            </div>
            <div className={s.meta}>
              <div className={s.displayName}>{shownName || user?.email || '—'}</div>
              {shownName ? <p className={s.emailSecondary}>{user?.email}</p> : null}
              {!editingProfile && bioPreview ? <p className={s.bioPreview}>{bioPreview}</p> : null}
              <p className={s.statsCompact} aria-label="Тариф, уровень и опыт">
                <span className={user?.plan === 'pro' ? s.statAccent : ''}>{planLabel(user?.plan ?? 'free')}</span>
                <span className={s.statsSep}>·</span>
                <span>ур. {user?.level ?? '—'}</span>
                <span className={s.statsSep}>·</span>
                <span>{user?.experience_points ?? 0} XP</span>
                {xpRank ? (
                  <span
                    className={s.xpRankPill}
                    title="Место в рейтинге по суммарному опыту"
                    aria-label={`Место в рейтинге опыта: ${xpRank.rank} из ${xpRank.total_users}`}
                  >
                    <span className={s.xpRankNum}>{xpRank.rank.toLocaleString('ru-RU')}</span>
                    <span className={s.xpRankSlash} aria-hidden>
                      /
                    </span>
                    <span className={s.xpRankDen}>{xpRank.total_users.toLocaleString('ru-RU')}</span>
                  </span>
                ) : null}
              </p>
              <div className={s.levelProgressSlot}>
                <LevelProgress user={user} variant="inline" />
              </div>
            </div>
          </div>

          {editingProfile ? (
            <form id="profile-edit-panel" className={s.profileForm} onSubmit={handleSave}>
              <label className={s.field}>
                <span className={s.fieldLabel}>Имя</span>
                <input
                  className={s.fieldInput}
                  value={displayName}
                  onChange={(e) => setDisplayName(e.target.value)}
                  autoComplete="nickname"
                  required
                  minLength={2}
                  maxLength={80}
                />
              </label>
              <label className={s.field}>
                <span className={s.fieldLabel}>О себе</span>
                <textarea
                  className={s.fieldTextarea}
                  value={bio}
                  onChange={(e) => setBio(e.target.value)}
                  rows={3}
                  maxLength={2000}
                  placeholder="По желанию — пару слов о себе."
                />
              </label>
              <div className={s.formActions}>
                <button type="submit" className={s.saveBtn} disabled={saving}>
                  {saving ? 'Сохранение…' : 'Сохранить'}
                </button>
              </div>
            </form>
          ) : null}
        </section>

        <section className={`${s.card} ${s.cardGoogle}`} aria-labelledby="google-heading">
          <div className={s.cardHeader}>
            <span className={s.cardIcon} aria-hidden>
              <i className={`fa-solid fa-calendar-days ${s.faCardIcon}`} />
            </span>
            <div className={s.cardHeaderText}>
              <p className={s.cardEyebrow}>Синхронизация</p>
              <h2 id="google-heading" className={s.cardTitle}>
                Календарь Google
              </h2>
            </div>
          </div>

          <div className={s.googleMain}>
            {google?.configured === false ? (
              <p className={s.hintMuted}>Календарь на сервере пока не настроен.</p>
            ) : google?.linked ? (
              <div className={s.googleBody}>
                <p className={s.googleConnected}>
                  Подключено: {google.google_email ? <strong>{google.google_email}</strong> : 'Google'}
                </p>
                <a className={s.googleBtn} href="/api/auth/google/start">
                  Обновить доступ
                </a>
              </div>
            ) : (
              <div className={s.googleBody}>
                <p className={s.hintMuted}>Читать события календаря и показывать их в дне.</p>
                <a className={s.googleBtn} href="/api/auth/google/start">
                  Подключить календарь
                </a>
              </div>
            )}
          </div>
        </section>

        <section
          className={`${s.card} ${s.cardPersonalization} ${s.sheetFullRow}`}
          aria-labelledby="personalization-heading"
        >
          <div className={s.cardHeader}>
            <span className={s.cardIcon} aria-hidden>
              <i className={`fa-solid fa-wand-magic-sparkles ${s.faCardIcon}`} />
            </span>
            <div className={s.cardHeaderText}>
              <p className={s.cardEyebrow}>Умные задачи и совет дня</p>
              <h2 id="personalization-heading" className={s.cardTitle}>
                Персонализация
              </h2>
            </div>
          </div>

          <div className={s.personalization}>
            {!personalizationOpen ? (
              <div className={s.personalizationCollapsed}>
                <div className={s.personalizationCollapsedMain}>
                  {hasPersonalization ? (
                    <p className={s.personalizationSummary}>{personalizationSummary}</p>
                  ) : (
                    <p className={s.personalizationTeaser}>
                      Короткая заметка о себе и пик бодрости — чтобы ИИ лучше подбирал время задач в календаре.
                    </p>
                  )}
                </div>
                <button
                  type="button"
                  className={s.personalizationToggle}
                  onClick={() => {
                    setPersNote(user?.personalization_note?.trim() ?? '')
                    setPersChrono((user?.chronotype ?? '').trim().toLowerCase())
                    setPersonalizationOpen(true)
                  }}
                >
                  {hasPersonalization ? 'Изменить' : 'Добавить персонализацию'}
                </button>
              </div>
            ) : (
              <div className={s.personalizationPanel} id="personalization-panel">
                <p className={s.personalizationLead}>
                  Учитывается при создании умной задачи в календаре и при генерации совета дня.
                </p>
                <label className={s.field}>
                  <span className={s.fieldLabel}>Коротко о себе</span>
                  <textarea
                    className={s.fieldTextarea}
                    value={persNote}
                    onChange={(e) => setPersNote(e.target.value)}
                    rows={2}
                    maxLength={280}
                    placeholder="Например: удалёнка, важны утренние окна без созвонов."
                  />
                  <span className={s.charCounter} aria-live="polite">
                    {persNote.length}/280
                  </span>
                </label>
                <div className={s.field}>
                  <span className={s.fieldLabel}>Когда вы лучше соображаете</span>
                  <div className={s.chronotypeGrid} role="group" aria-label="Пик бодрости">
                    {CHRONOTYPE_OPTIONS.map((opt) => (
                      <button
                        key={opt.value === '' ? '_none' : opt.value}
                        type="button"
                        className={`${s.chronotypeBtn} ${persChrono === opt.value ? s.chronotypeBtnActive : ''}`}
                        onClick={() => setPersChrono(opt.value)}
                      >
                        {opt.label}
                      </button>
                    ))}
                  </div>
                </div>
                <div className={s.personalizationActions}>
                  <button
                    type="button"
                    className={s.saveBtn}
                    disabled={savingPers}
                    onClick={() => void handleSavePersonalization()}
                  >
                    {savingPers ? 'Сохранение…' : 'Сохранить'}
                  </button>
                  <button
                    type="button"
                    className={s.editCancelBtn}
                    disabled={savingPers}
                    onClick={() => setPersonalizationOpen(false)}
                  >
                    Свернуть
                  </button>
                </div>
              </div>
            )}
          </div>
        </section>

        {statsHelpOpen && createPortal(
          <div className={s.modalRoot}>
            <button type="button" className={s.modalBackdrop} aria-label="Закрыть" onClick={() => setStatsHelpOpen(false)} />
            <div
              className={s.modalPanel}
              role="dialog"
              aria-modal="true"
              aria-labelledby={statsHelpTitleId}
              lang="ru"
              onClick={(e) => e.stopPropagation()}
            >
              <header className={s.modalHeader}>
                <h2 id={statsHelpTitleId} className={s.modalTitle}>Как считается статистика</h2>
                <button type="button" className={s.modalClose} onClick={() => setStatsHelpOpen(false)} aria-label="Закрыть">×</button>
              </header>
              <div className={s.helpBody}>
                <ul className={s.helpSteps}>
                  <li className={s.helpStep}>
                    <span className={s.helpStepMark} aria-hidden><i className="fa-solid fa-wand-magic-sparkles" /></span>
                    <div>
                      <p className={s.helpStepTitle}>Задач создано за неделю</p>
                      <p className={s.helpStepText}>Количество умных задач, добавленных в Google Календарь за последние 7 дней через ИИ-планировщик.</p>
                    </div>
                  </li>
                  <li className={s.helpStep}>
                    <span className={s.helpStepMark} aria-hidden><i className="fa-solid fa-circle-check" /></span>
                    <div>
                      <p className={s.helpStepTitle}>Выполнено за неделю</p>
                      <p className={s.helpStepText}>События, отмеченные как выполненные за последние 7 дней. За каждое начисляется 25 XP и повышается уровень.</p>
                    </div>
                  </li>
                  <li className={s.helpStep}>
                    <span className={s.helpStepMark} aria-hidden><i className="fa-solid fa-fire" /></span>
                    <div>
                      <p className={s.helpStepTitle}>Дней подряд (стрик)</p>
                      <p className={s.helpStepText}>Непрерывная серия дней, в которые вы выполнили хотя бы одно событие. Если пропустить день — стрик обнуляется.</p>
                    </div>
                  </li>
                  <li className={s.helpStep}>
                    <span className={s.helpStepMark} aria-hidden><i className="fa-solid fa-clock" /></span>
                    <div>
                      <p className={s.helpStepTitle}>Часов сэкономлено</p>
                      <p className={s.helpStepText}>Приблизительная оценка: каждое выполненное событие засчитывается как 0,75 сохранённого часа. Показывает суммарный эффект от планирования.</p>
                    </div>
                  </li>
                </ul>
              </div>
            </div>
          </div>,
          document.body,
        )}

        {stats !== null && (
          <section
            className={`${s.card} ${s.cardStats} ${s.sheetFullRow}`}
            aria-labelledby="stats-heading"
          >
            <div className={s.cardHeader}>
              <span className={s.cardIcon} aria-hidden>
                <i className={`fa-solid fa-chart-simple ${s.faCardIcon}`} />
              </span>
              <div className={s.cardHeaderText}>
                <p className={s.cardEyebrow}>Прогресс</p>
                <h2 id="stats-heading" className={s.cardTitle}>
                  Статистика
                </h2>
              </div>
              <button
                type="button"
                className={s.helpBtn}
                aria-label="Как считается статистика"
                title="Подсказка"
                onClick={() => setStatsHelpOpen(true)}
              >
                <i className="fa-regular fa-circle-question" aria-hidden />
              </button>
            </div>
            <div className={s.statsGrid}>
              <div className={s.statCell}>
                <span className={s.statValue}>{stats.smart_tasks_week}</span>
                <span className={s.statLabel}>задач создано за неделю</span>
              </div>
              <div className={s.statCell}>
                <span className={s.statValue}>{stats.completed_week}</span>
                <span className={s.statLabel}>выполнено за неделю</span>
              </div>
              <div className={s.statCell}>
                <span className={`${s.statValue} ${stats.streak > 0 ? s.statStreak : ''}`}>
                  {stats.streak > 0 ? `🔥 ${stats.streak}` : stats.streak}
                </span>
                <span className={s.statLabel}>дней подряд</span>
              </div>
              <div className={s.statCell}>
                <span className={s.statValue}>~{stats.hours_saved.toFixed(1)}</span>
                <span className={s.statLabel}>часов сэкономлено</span>
              </div>
            </div>
          </section>
        )}

        <section
          className={`${s.card} ${s.cardPromo} ${s.promoFullRow}`}
          aria-labelledby="promo-heading"
        >
          <div className={s.cardHeader}>
            <span className={s.cardIcon} aria-hidden>
              <i className={`fa-solid fa-ticket ${s.faCardIcon}`} />
            </span>
            <div className={s.cardHeaderText}>
              <p className={s.cardEyebrow}>Тариф</p>
              <h2 id="promo-heading" className={s.cardTitle}>
                Промокод Pro
              </h2>
            </div>
          </div>

          <div className={s.promoBody}>
            {user?.plan === 'pro' ? (
              <p className={s.promoProBadge}>
                <i className="fa-solid fa-circle-check" aria-hidden />
                У вас тариф Pro — 30 умных задач в день.
                {quotaLow && quota ? ` Осталось: ${quota.limit - quota.used}.` : ''}
              </p>
            ) : (
              <>
                <p className={s.promoLead}>
                  По умолчанию доступен тариф Free (3 умных задачи в день). Введите промокод, чтобы получить Pro (30/день).
                </p>
                {quotaLow && quota && !quotaExhausted ? (
                  <p className={s.promoQuotaExhausted} role="status">
                    Осталось запросов сегодня: {quota.limit - quota.used} из {quota.limit}.
                  </p>
                ) : null}
                {quotaExhausted && quota?.resets_at ? (
                  <p className={s.promoQuotaExhausted} role="status">
                    {formatQuotaResetsAtHint(quota.resets_at)}
                  </p>
                ) : null}
                <form className={s.promoForm} onSubmit={handlePromo}>
                  <div className={s.promoRow}>
                    <label htmlFor="profile-promo-code" className={s.visuallyHidden}>
                      Промокод
                    </label>
                    <input
                      id="profile-promo-code"
                      className={s.promoInput}
                      value={promoCode}
                      onChange={(e) => {
                        setPromoCode(e.target.value)
                        setPromoErr(null)
                      }}
                      autoComplete="off"
                      spellCheck={false}
                      placeholder="Промокод"
                      disabled={promoBusy}
                      maxLength={128}
                    />
                    <button type="submit" className={s.promoSubmit} disabled={promoBusy}>
                      {promoBusy ? 'Проверка…' : 'Активировать'}
                    </button>
                  </div>
                  {promoErr ? (
                    <p className={s.promoFieldErr} role="alert">
                      {promoErr}
                    </p>
                  ) : null}
                </form>
              </>
            )}
          </div>
        </section>
      </div>
    </div>
  )
}
