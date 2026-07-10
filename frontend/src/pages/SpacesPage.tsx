import { createPortal } from 'react-dom'
import { useEffect, useId, useRef, useState, type FormEvent } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'
import { extractInviteToken } from '../lib/inviteLink'
import { createSpace, fetchSpaces, joinSpaceWithToken, type Space } from '../lib/spacesApi'
import a from './AppPage.module.css'
import s from './spacesPages.module.css'

function typeLabel(t: string) {
  if (t === 'work') return 'Работа'
  if (t === 'personal') return 'Личное'
  return 'Другое'
}

export default function SpacesPage() {
  const { accessToken, refreshSession } = useAuth()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const didAutoJoin = useRef(false)
  const [helpOpen, setHelpOpen] = useState(false)
  const helpModalTitleId = useId()
  const [spaces, setSpaces] = useState<Space[]>([])
  const [loading, setLoading] = useState(true)
  const [err, setErr] = useState<string | null>(null)
  const [name, setName] = useState('')
  const [spaceType, setSpaceType] = useState('work')
  const [creating, setCreating] = useState(false)
  const [joinLink, setJoinLink] = useState('')
  const [joinBusy, setJoinBusy] = useState(false)
  const [joinOk, setJoinOk] = useState<string | null>(null)
  const [createOpen, setCreateOpen] = useState(false)

  useEffect(() => {
    if (!joinOk) return
    const tid = setTimeout(() => setJoinOk(null), 4000)
    return () => clearTimeout(tid)
  }, [joinOk])

  const load = async () => {
    if (!accessToken) return
    setErr(null)
    setLoading(true)
    try {
      const rows = await fetchSpaces(accessToken)
      setSpaces(rows)
    } catch (e) {
      if (e instanceof Error && (e.message === '401' || e.message.toLowerCase().includes('unauthorized'))) {
        const ok = await refreshSession()
        if (!ok) navigate('/login', { replace: true })
        return
      }
      setErr(e instanceof Error ? e.message : 'Не удалось загрузить')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [accessToken])

  // Авто-вступление если в URL есть ?token=...
  useEffect(() => {
    if (didAutoJoin.current || !accessToken) return
    const urlToken = searchParams.get('token')
    if (!urlToken) return
    didAutoJoin.current = true
    setSearchParams({}, { replace: true })
    const token = extractInviteToken(urlToken)
    if (!token.trim()) {
      setErr('Неверный токен в ссылке')
      return
    }
    setJoinBusy(true)
    setErr(null)
    setJoinOk(null)
    joinSpaceWithToken(accessToken, token)
      .then(async (sp) => {
        setJoinOk(`Вы вступили в пространство «${sp.name}».`)
        await load()
      })
      .catch((e) => {
        if (e instanceof Error && (e.message === '401' || e.message.toLowerCase().includes('unauthorized'))) {
          void refreshSession().then((ok) => {
            if (!ok) navigate('/login', { replace: true })
          })
          return
        }
        setErr(e instanceof Error ? e.message : 'Не удалось вступить')
      })
      .finally(() => setJoinBusy(false))
  }, [accessToken]) // eslint-disable-line react-hooks/exhaustive-deps

  const onCreate = async (e: FormEvent) => {
    e.preventDefault()
    if (!accessToken) return
    setCreating(true)
    setErr(null)
    try {
      await createSpace(accessToken, { name, space_type: spaceType })
      setName('')
      setCreateOpen(false)
      await load()
    } catch (e) {
      if (e instanceof Error && e.message === '401') {
        const ok = await refreshSession()
        if (!ok) navigate('/login', { replace: true })
        return
      }
      setErr(e instanceof Error ? e.message : 'Не удалось создать')
    } finally {
      setCreating(false)
    }
  }

  const onJoin = async (e: FormEvent) => {
    e.preventDefault()
    if (!accessToken) return
    const token = extractInviteToken(joinLink)
    if (!token.trim()) {
      setErr('Вставьте ссылку или токен')
      return
    }
    setJoinBusy(true)
    setErr(null)
    setJoinOk(null)
    try {
      const sp = await joinSpaceWithToken(accessToken, token)
      setJoinLink('')
      setJoinOk(`Вы в пространстве «${sp.name}».`)
      await load()
    } catch (e) {
      if (e instanceof Error && e.message === '401') {
        const ok = await refreshSession()
        if (!ok) navigate('/login', { replace: true })
        return
      }
      setErr(e instanceof Error ? e.message : 'Не удалось вступить')
    } finally {
      setJoinBusy(false)
    }
  }

  const helpModal =
    helpOpen &&
    createPortal(
      <div className={s.modalRoot}>
        <button type="button" className={s.modalBackdrop} aria-label="Закрыть" onClick={() => setHelpOpen(false)} />
        <div
          className={s.modalPanel}
          role="dialog"
          aria-modal="true"
          aria-labelledby={helpModalTitleId}
          lang="ru"
          onClick={(e) => e.stopPropagation()}
        >
          <header className={s.modalHeader}>
            <h2 id={helpModalTitleId} className={s.modalTitle}>Как работают пространства</h2>
            <button type="button" className={s.modalClose} onClick={() => setHelpOpen(false)} aria-label="Закрыть">×</button>
          </header>
          <div className={s.helpBody}>
            <ul className={s.helpSteps}>
              <li className={s.helpStep}>
                <span className={s.helpStepMark} aria-hidden><i className="fa-solid fa-plus" /></span>
                <div>
                  <p className={s.helpStepTitle}>Создайте пространство</p>
                  <p className={s.helpStepText}>Нажмите «Добавить», укажите название и тип (работа, личное или другое). Пространство — отдельная комната для совместных задач, не связанная с вашим личным календарём.</p>
                </div>
              </li>
              <li className={s.helpStep}>
                <span className={s.helpStepMark} aria-hidden><i className="fa-solid fa-link" /></span>
                <div>
                  <p className={s.helpStepTitle}>Пригласите участников</p>
                  <p className={s.helpStepText}>Внутри пространства скопируйте ссылку-приглашение и отправьте её нужному человеку. Вступить можно и по одному токену без полной ссылки.</p>
                </div>
              </li>
              <li className={s.helpStep}>
                <span className={s.helpStepMark} aria-hidden><i className="fa-solid fa-lightbulb" /></span>
                <div>
                  <p className={s.helpStepTitle}>Предлагайте задачи</p>
                  <p className={s.helpStepText}>Участники создают предложения задач с заголовком, описанием и датами. Все участники видят список предложений и могут реагировать на них.</p>
                </div>
              </li>
              <li className={s.helpStep}>
                <span className={s.helpStepMark} aria-hidden><i className="fa-solid fa-calendar-check" /></span>
                <div>
                  <p className={s.helpStepTitle}>Перенесите в календарь</p>
                  <p className={s.helpStepText}>Принятое предложение можно добавить в свой Google Календарь одним нажатием — оно появится в расписании как обычное событие.</p>
                </div>
              </li>
              <li className={s.helpStep}>
                <span className={s.helpStepMark} aria-hidden><i className="fa-solid fa-crown" /></span>
                <div>
                  <p className={s.helpStepTitle}>Роли участников</p>
                  <p className={s.helpStepText}><strong>Coordinator</strong> управляет пространством и участниками. <strong>Member</strong> создаёт и видит предложения задач. Роль назначается при вступлении.</p>
                </div>
              </li>
            </ul>
          </div>
        </div>
      </div>,
      document.body,
    )

  return (
    <>
      {helpModal}
    <div className={a.page}>
      <div className={a.shell}>
        <header className={a.hero}>
          <p className={a.eyebrow}>Пространства</p>
          <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: '0.5rem' }}>
            <h1 className={a.h1}>
              Совместные <span className={a.accent}>задачи</span>
            </h1>
            <button
              type="button"
              className={s.helpBtn}
              aria-label="Как работают пространства"
              title="Подсказка"
              onClick={() => setHelpOpen(true)}
              style={{ marginTop: '0.25rem' }}
            >
              <i className="fa-regular fa-circle-question" aria-hidden />
            </button>
          </div>
          <p className={a.lead}>
            Общий список задач с близкими или коллегами: накидываете идеи и согласуете их здесь. В календарь ничего не попадает, пока вы сами не перенесёте.
          </p>
        </header>

        <div className={a.kineticRule} aria-hidden />

        {err && <p className={a.warn}>{err}</p>}
        {joinOk && <p className={s.flashOk}>{joinOk}</p>}

        <div className={s.spacesTopGrid}>
          <section className={`${a.card} ${s.joinColumn}`} aria-labelledby="join-h">
            <div className={a.cardHeader}>
              <h2 className={a.cardTitle} id="join-h">
                Вступить
              </h2>
            </div>
            <p className={s.collapsibleHint}>Вставьте ссылку приглашения или только токен — поле поймёт оба варианта.</p>
            <form className={s.joinForm} onSubmit={onJoin}>
              <div className={s.field}>
                <label className={s.fieldLabel} htmlFor="join-link">
                  Ссылка или токен
                </label>
                <input
                  id="join-link"
                  className={s.fieldInput}
                  value={joinLink}
                  onChange={(e) => setJoinLink(e.target.value)}
                  autoComplete="off"
                  placeholder="https://…?token=…"
                />
              </div>
              <div className={s.topCardFoot}>
                <button type="submit" className={a.addBtn} disabled={joinBusy || !joinLink.trim()}>
                  {joinBusy ? 'Вступаем…' : 'Вступить'}
                </button>
              </div>
            </form>
          </section>

          <section className={a.card} aria-labelledby="create-h">
            <div className={a.cardHeader}>
              <h2 className={a.cardTitle} id="create-h">
                Новое пространство
              </h2>
            </div>
            {!createOpen ? (
              <>
                <p className={s.collapsibleHint}>
                  Отдельная «комната» для предложений задач: удобно, если не хотите светить личным календарём целиком.
                </p>
                <div className={s.topCardGrow} aria-hidden />
                <div className={s.topCardFoot}>
                  <button type="button" className={a.addBtn} onClick={() => setCreateOpen(true)}>
                    Добавить
                  </button>
                </div>
              </>
            ) : (
              <>
                <div className={s.createExpandedBar}>
                  <p className={s.collapsibleHint}>Укажите название и тип — затем «Создать».</p>
                  <button type="button" className={s.btnSmall} onClick={() => setCreateOpen(false)}>
                    Свернуть
                  </button>
                </div>
                <form onSubmit={onCreate}>
                  <div className={s.field}>
                    <label className={s.fieldLabel} htmlFor="sp-name">
                      Название
                    </label>
                    <input
                      id="sp-name"
                      className={s.fieldInput}
                      value={name}
                      onChange={(e) => setName(e.target.value)}
                      required
                      maxLength={200}
                    />
                  </div>
                  <div className={s.field}>
                    <label className={s.fieldLabel} htmlFor="sp-type">
                      Тип
                    </label>
                    <select
                      id="sp-type"
                      className={s.fieldSelect}
                      value={spaceType}
                      onChange={(e) => setSpaceType(e.target.value)}
                    >
                      <option value="work">Работа</option>
                      <option value="personal">Личное</option>
                      <option value="custom">Другое</option>
                    </select>
                  </div>
                  <button type="submit" className={a.addBtn} disabled={creating || !name.trim()}>
                    {creating ? 'Создаём…' : 'Создать'}
                  </button>
                </form>
              </>
            )}
          </section>
        </div>

        <section className={`${a.card} ${a.cardWide}`} aria-labelledby="list-h">
          <div className={a.cardHeader}>
            <h2 className={a.cardTitle} id="list-h">
              Ваши пространства
            </h2>
          </div>
          {loading ? (
            <p className={a.muted}>Загрузка…</p>
          ) : spaces.length === 0 ? (
            <div className={a.empty}>
              <p className={a.emptyText}>Пока пусто — создайте или вступите по ссылке.</p>
            </div>
          ) : (
            <ul className={s.spaceList}>
              {spaces.map((sp) => (
                <li key={sp.id} className={s.spaceListItem}>
                  <h3 className={s.spaceTitle}>
                    <Link to={`/spaces/${sp.id}`}>{sp.name}</Link>
                  </h3>
                  <p className={s.spaceMeta}>
                    {typeLabel(sp.space_type)}
                    {sp.policy === 'proposals_only' ? ' · только предложения' : ''}
                    {' · '}
                    вы: {sp.my_role}
                  </p>
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>
    </div>
    </>
  )
}
