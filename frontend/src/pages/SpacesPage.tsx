import { useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
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
  const { accessToken } = useAuth()
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

  const load = async () => {
    if (!accessToken) return
    setErr(null)
    setLoading(true)
    try {
      const rows = await fetchSpaces(accessToken)
      setSpaces(rows)
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Не удалось загрузить')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [accessToken])

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
      setErr(e instanceof Error ? e.message : 'Не удалось вступить')
    } finally {
      setJoinBusy(false)
    }
  }

  return (
    <div className={a.page}>
      <div className={a.shell}>
        <header className={a.hero}>
          <p className={a.eyebrow}>Пространства</p>
          <h1 className={a.h1}>
            Совместные <span className={a.accent}>задачи</span>
          </h1>
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
  )
}
