import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { Link, useLocation, useNavigate, useParams } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'
import {
  createProposal,
  createSpaceInvite,
  deleteSpace,
  fetchProposals,
  fetchSpace,
  fetchSpaceMembers,
  respondToProposal,
  type Space,
  type SpaceMember,
  type TaskProposal,
} from '../lib/spacesApi'
import { notifyPendingProposalsChanged } from '../lib/pendingProposalsEvents'
import a from './AppPage.module.css'
import s from './spacesPages.module.css'

function localInputToISO(v: string): string | undefined {
  const t = v.trim()
  if (!t) return undefined
  const d = new Date(t)
  if (Number.isNaN(d.getTime())) return undefined
  return d.toISOString()
}

function statusPillClass(st: string) {
  if (st === 'accepted') return s.pillOk
  if (st === 'declined') return s.pillNo
  return s.pillPending
}

function roleRu(r: string) {
  if (r === 'coordinator') return 'координатор'
  if (r === 'observer') return 'наблюдатель'
  return 'участник'
}

function shareRu(x: string) {
  if (x === 'full') return 'полный'
  if (x === 'view_only') return 'просмотр'
  return 'частичный'
}

function recordWordRu(n: number) {
  const m = n % 100
  if (m >= 11 && m <= 14) return 'записей'
  const k = n % 10
  if (k === 1) return 'запись'
  if (k >= 2 && k <= 4) return 'записи'
  return 'записей'
}

function proposalsCollapsedHint(list: TaskProposal[]) {
  const n = list.length
  const pend = list.filter((p) => p.status === 'pending').length
  const acc = list.filter((p) => p.status === 'accepted').length
  const dec = list.filter((p) => p.status === 'declined').length
  const bits: string[] = []
  if (pend) bits.push(`${pend} ждут ответа`)
  if (acc) bits.push(`${acc} принято`)
  if (dec) bits.push(`${dec} отклонено`)
  const tail = bits.length ? ` · ${bits.join(' · ')}` : ''
  return `${n} ${recordWordRu(n)}${tail}`
}

export default function SpacePage() {
  const { id } = useParams()
  const location = useLocation()
  const navigate = useNavigate()
  const { accessToken, user, refreshSession } = useAuth()
  const myId = user?.id
  const spaceId = id ? parseInt(id, 10) : NaN
  const proposalsSectionRef = useRef<HTMLElement | null>(null)
  const copiedTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    return () => {
      if (copiedTimerRef.current !== null) clearTimeout(copiedTimerRef.current)
    }
  }, [])

  const [space, setSpace] = useState<Space | null>(null)
  const [members, setMembers] = useState<SpaceMember[]>([])
  const [proposals, setProposals] = useState<TaskProposal[]>([])
  const [loading, setLoading] = useState(true)
  const [err, setErr] = useState<string | null>(null)
  const [flash, setFlash] = useState<string | null>(null)

  const [inviteEmail, setInviteEmail] = useState('')
  const [inviteRole, setInviteRole] = useState('member')
  const [inviteShare, setInviteShare] = useState('partial')
  const [inviteDays, setInviteDays] = useState(7)
  const [inviteToken, setInviteToken] = useState<string | null>(null)
  const [inviteBusy, setInviteBusy] = useState(false)
  const [inviteOpen, setInviteOpen] = useState(false)
  const [inviteAdvanced, setInviteAdvanced] = useState(false)
  const [copied, setCopied] = useState(false)

  const [assigneeId, setAssigneeId] = useState<number>(0)
  const [pTitle, setPTitle] = useState('')
  const [pDesc, setPDesc] = useState('')
  const [pStart, setPStart] = useState('')
  const [pEnd, setPEnd] = useState('')
  const [propBusy, setPropBusy] = useState(false)
  const [proposeOpen, setProposeOpen] = useState(false)

  const [proposalsHistoryOpen, setProposalsHistoryOpen] = useState(true)

  const [respondNote, setRespondNote] = useState<Record<number, string>>({})
  const [deleteBusy, setDeleteBusy] = useState(false)

  const spaceIdRef = useRef(spaceId)
  spaceIdRef.current = spaceId

  useEffect(() => {
    if (!flash) return
    const tid = setTimeout(() => setFlash(null), 4000)
    return () => clearTimeout(tid)
  }, [flash])

  const loadAll = useCallback(async () => {
    const sid = spaceIdRef.current
    if (!Number.isFinite(sid) || sid < 1) {
      setLoading(false)
      return
    }
    if (!accessToken) return
    setErr(null)
    setLoading(true)
    try {
      const [sp, mem, pr] = await Promise.all([
        fetchSpace(accessToken, sid),
        fetchSpaceMembers(accessToken, sid),
        fetchProposals(accessToken, sid),
      ])
      if (spaceIdRef.current !== sid) return
      setSpace(sp)
      setMembers(mem)
      setProposals(pr)
    } catch (e) {
      if (spaceIdRef.current !== sid) return
      if (e instanceof Error && (e.message === '401' || e.message.toLowerCase().includes('unauthorized'))) {
        const ok = await refreshSession()
        if (!ok) navigate('/login', { replace: true })
        return
      }
      setErr(e instanceof Error ? e.message : 'Ошибка загрузки')
      setSpace(null)
    } finally {
      if (spaceIdRef.current === sid) setLoading(false)
    }
  }, [accessToken, refreshSession, navigate])

  useEffect(() => {
    void loadAll()
  }, [spaceId, loadAll])

  useEffect(() => {
    setInviteOpen(false)
    setInviteAdvanced(false)
    setCopied(false)
    setProposeOpen(false)
    setInviteToken(null)
    setAssigneeId(0)
    setProposalsHistoryOpen(true)
  }, [spaceId])

  useEffect(() => {
    if (proposals.length === 0) setProposalsHistoryOpen(true)
  }, [proposals.length])

  useEffect(() => {
    if (!members.length) return
    const valid = members.some((m) => m.user_id === assigneeId)
    if (!assigneeId || !valid) {
      const other = members.find((m) => m.user_id !== user?.id)
      setAssigneeId(other?.user_id ?? members[0].user_id)
    }
  }, [members, user?.id, assigneeId])

  useEffect(() => {
    if (location.hash !== '#proposals') return
    setProposalsHistoryOpen(true)
    if (loading) return
    const el = proposalsSectionRef.current
    if (!el) return
    const tid = window.setTimeout(() => {
      el.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }, 120)
    return () => window.clearTimeout(tid)
  }, [location.hash, loading, spaceId, proposals.length])

  const copyInviteLink = (token: string) => {
    const link = `${window.location.origin}/spaces?token=${encodeURIComponent(token)}`
    void navigator.clipboard.writeText(link).then(() => {
      setCopied(true)
      if (copiedTimerRef.current !== null) clearTimeout(copiedTimerRef.current)
      copiedTimerRef.current = setTimeout(() => setCopied(false), 2500)
    })
  }

  const onQuickInvite = async () => {
    if (!accessToken || !space) return
    setInviteBusy(true)
    setErr(null)
    setFlash(null)
    setInviteToken(null)
    try {
      const out = await createSpaceInvite(accessToken, space.id, {
        role: 'member',
        share_access: 'partial',
        expires_days: 7,
      })
      setInviteToken(out.token)
      copyInviteLink(out.token)
      setFlash('Ссылка скопирована — отправьте её участнику. Она действует 7 дней и сгорает после входа.')
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Не удалось создать приглашение')
    } finally {
      setInviteBusy(false)
    }
  }

  const onInvite = async (e: FormEvent) => {
    e.preventDefault()
    if (!accessToken || !space) return
    setInviteBusy(true)
    setErr(null)
    setFlash(null)
    setInviteToken(null)
    try {
      const out = await createSpaceInvite(accessToken, space.id, {
        invited_email: inviteEmail.trim() || undefined,
        role: inviteRole,
        share_access: inviteShare,
        expires_days: inviteDays,
      })
      setInviteToken(out.token)
      copyInviteLink(out.token)
      setFlash('Ссылка скопирована — отправьте её участнику. После входа ссылка сгорает.')
      setInviteOpen(false)
      setInviteAdvanced(false)
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Не удалось создать приглашение')
    } finally {
      setInviteBusy(false)
    }
  }

  const onCreateProposal = async (e: FormEvent) => {
    e.preventDefault()
    if (!accessToken || !space) return
    setPropBusy(true)
    setErr(null)
    setFlash(null)
    try {
      const ps = localInputToISO(pStart)
      const pe = localInputToISO(pEnd)
      await createProposal(accessToken, space.id, {
        assignee_id: assigneeId,
        title: pTitle,
        description: pDesc,
        proposed_start: ps,
        proposed_end: pe,
      })
      setPTitle('')
      setPDesc('')
      setPStart('')
      setPEnd('')
      setFlash('Предложение отправлено.')
      setProposeOpen(false)
      await loadAll()
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Не удалось создать')
    } finally {
      setPropBusy(false)
    }
  }

  const onRespond = async (proposalId: number, status: 'accepted' | 'declined') => {
    if (!accessToken || !space) return
    setErr(null)
    setFlash(null)
    try {
      const out = await respondToProposal(accessToken, space.id, proposalId, {
        status,
        assignee_note: respondNote[proposalId] ?? '',
      })
      if (out.calendar_warning) {
        setFlash(`${status === 'accepted' ? 'Принято' : 'Отклонено'}. ${out.calendar_warning}`)
      } else {
        setFlash(status === 'accepted' ? 'Принято.' : 'Отклонено.')
      }
      await loadAll()
      notifyPendingProposalsChanged()
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Не удалось ответить')
    }
  }

  const onDeleteSpace = async () => {
    if (!accessToken || !space || myId == null || space.created_by !== myId) return
    const ok = window.confirm(
      `Удалить пространство «${space.name}»? У участников оно пропадёт из списка, все предложения будут удалены. Это действие необратимо.`,
    )
    if (!ok) return
    setDeleteBusy(true)
    setErr(null)
    setFlash(null)
    try {
      await deleteSpace(accessToken, space.id)
      notifyPendingProposalsChanged()
      navigate('/spaces', { replace: true })
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Не удалось удалить')
    } finally {
      setDeleteBusy(false)
    }
  }

  if (!Number.isFinite(spaceId) || spaceId < 1) {
    return (
      <div className={a.page}>
        <div className={a.shell}>
          <p className={a.warn}>Некорректный адрес.</p>
          <Link className={a.textLink} to="/spaces">
            К списку пространств
          </Link>
        </div>
      </div>
    )
  }

  const isCoordinator = space?.my_role === 'coordinator'
  const canPropose = space && space.my_role !== 'observer'
  const isCreator = space != null && myId != null && space.created_by === myId

  const memberNameById = new Map(
    members.map((m) => [m.user_id, m.display_name?.trim() || m.email]),
  )

  return (
    <div className={a.page}>
      <div className={a.shell}>
        <p className={`${a.muted} ${s.backRow}`}>
          <Link className={a.textLink} to="/spaces">
            ← Все пространства
          </Link>
        </p>

        {loading ? (
          <p className={a.muted}>Загрузка…</p>
        ) : !space ? (
          <p className={a.warn}>{err ?? 'Не найдено'}</p>
        ) : (
          <>
            <header className={a.hero}>
              <p className={a.eyebrow}>Пространство</p>
              <h1 className={a.h1}>{space.name}</h1>
              <p className={a.lead}>
                Вы — <span className={a.accent}>{roleRu(space.my_role)}</span>, доступ: {shareRu(space.my_share)}.
                {space.policy === 'proposals_only' ? ' Задачи только через предложения.' : ''}
              </p>
            </header>

            <div className={a.kineticRule} aria-hidden />

            {flash && <p className={s.flashOk}>{flash}</p>}
            {err && <p className={a.warn}>{err}</p>}

            <section className={`${a.card} ${a.cardWide}`} aria-labelledby="mem-h">
              <div className={a.cardHeader}>
                <h2 className={a.cardTitle} id="mem-h">
                  Участники
                </h2>
              </div>
              {members.map((m) => (
                <div key={m.user_id} className={s.memberLine}>
                  <span className={s.memberName}>{m.display_name?.trim() || m.email}</span>
                  <span className={a.muted}>
                    {' '}
                    · {m.email} · {roleRu(m.role)} · {shareRu(m.share_access)}
                  </span>
                </div>
              ))}
            </section>

            {isCoordinator && (
              <section className={`${a.card} ${a.cardWide}`} aria-labelledby="inv-h">
                <div className={s.collapsibleHead}>
                  <div className={s.collapsibleTitles}>
                    <div
                      className={a.cardHeader}
                      style={{ marginBottom: 0, paddingBottom: 0, borderBottom: 'none' }}
                    >
                      <h2 className={a.cardTitle} id="inv-h">
                        Пригласить
                      </h2>
                    </div>
                    {!inviteOpen && (
                      <p className={s.collapsibleHint}>
                        Сгенерируйте ссылку и отправьте её участнику — он откроет её и сразу войдёт в пространство.
                      </p>
                    )}
                  </div>
                  <div className={s.btnToolbar}>
                    {!inviteOpen && (
                      <button
                        type="button"
                        className={a.addBtn}
                        disabled={inviteBusy}
                        onClick={() => void onQuickInvite()}
                      >
                        {inviteBusy ? 'Создаём…' : 'Скопировать ссылку'}
                      </button>
                    )}
                    <button
                      type="button"
                      className={s.btnSmall}
                      onClick={() => {
                        setInviteOpen((o) => !o)
                        setInviteAdvanced(false)
                      }}
                    >
                      {inviteOpen ? 'Свернуть' : 'Настроить'}
                    </button>
                  </div>
                </div>

                {inviteOpen && (
                  <div className={s.collapsibleBody}>
                    <p className={s.fieldHint}>
                      Email по желанию — ссылка сработает только для этого адреса.
                    </p>
                    <form onSubmit={onInvite}>
                      <div className={s.field}>
                        <label className={s.fieldLabel} htmlFor="inv-email">
                          Email (необязательно)
                        </label>
                        <input
                          id="inv-email"
                          type="email"
                          className={s.fieldInput}
                          value={inviteEmail}
                          onChange={(e) => setInviteEmail(e.target.value)}
                          autoComplete="off"
                          placeholder="user@example.com"
                        />
                      </div>

                      <button
                        type="button"
                        className={s.btnSmall}
                        style={{ marginBottom: 'var(--s3)' }}
                        onClick={() => setInviteAdvanced((o) => !o)}
                      >
                        {inviteAdvanced ? '▲ Скрыть роль и срок' : '▼ Роль и срок'}
                      </button>

                      {inviteAdvanced && (
                        <>
                          <div className={s.field}>
                            <label className={s.fieldLabel} htmlFor="inv-role">
                              Роль
                            </label>
                            <select
                              id="inv-role"
                              className={s.fieldSelect}
                              value={inviteRole}
                              onChange={(e) => setInviteRole(e.target.value)}
                            >
                              <option value="member">Участник</option>
                              <option value="observer">Наблюдатель</option>
                              <option value="coordinator">Координатор</option>
                            </select>
                          </div>
                          <div className={s.field}>
                            <label className={s.fieldLabel} htmlFor="inv-share">
                              Доступ
                            </label>
                            <select
                              id="inv-share"
                              className={s.fieldSelect}
                              value={inviteShare}
                              onChange={(e) => setInviteShare(e.target.value)}
                            >
                              <option value="partial">Частичный</option>
                              <option value="full">Полный</option>
                              <option value="view_only">Только просмотр</option>
                            </select>
                          </div>
                          <div className={s.field}>
                            <label className={s.fieldLabel} htmlFor="inv-days">
                              Срок (дней, макс. 30)
                            </label>
                            <input
                              id="inv-days"
                              type="number"
                              className={s.fieldInput}
                              min={1}
                              max={30}
                              value={inviteDays}
                              onChange={(e) => setInviteDays(parseInt(e.target.value, 10) || 7)}
                            />
                          </div>
                        </>
                      )}

                      <button type="submit" className={a.addBtn} disabled={inviteBusy}>
                        {inviteBusy ? 'Создаём…' : 'Сгенерировать и скопировать'}
                      </button>
                    </form>
                  </div>
                )}

                {inviteToken && (
                  <div className={s.tokenBox} role="status">
                    <div className={s.shareLinkRow}>
                      <span className={s.shareLink}>
                        {`${window.location.origin}/spaces?token=${encodeURIComponent(inviteToken)}`}
                      </span>
                      <button
                        type="button"
                        className={copied ? s.btnCopied : s.btnSmall}
                        onClick={() => copyInviteLink(inviteToken)}
                      >
                        {copied ? '✓ Скопировано' : 'Копировать'}
                      </button>
                    </div>
                    <p className={s.tokenHint}>
                      Ссылка одноразовая — сгорит после первого входа.
                    </p>
                  </div>
                )}
              </section>
            )}

            {canPropose && (
              <section className={`${a.card} ${a.cardWide}`} aria-labelledby="prop-new-h">
                <div className={s.collapsibleHead}>
                  <div className={s.collapsibleTitles}>
                    <div
                      className={a.cardHeader}
                      style={{ marginBottom: 0, paddingBottom: 0, borderBottom: 'none' }}
                    >
                      <h2 className={a.cardTitle} id="prop-new-h">
                        Предложить задачу
                      </h2>
                    </div>
                    {!proposeOpen && (
                      <p className={s.collapsibleHint}>
                        Исполнитель получит предложение и сможет принять или отклонить его.
                      </p>
                    )}
                  </div>
                  <div className={s.btnToolbar}>
                    <button
                      type="button"
                      className={proposeOpen ? s.btnSmall : a.addBtn}
                      onClick={() => setProposeOpen((o) => !o)}
                    >
                      {proposeOpen ? 'Свернуть' : 'Предложить'}
                    </button>
                  </div>
                </div>
                {proposeOpen && (
                  <div className={s.collapsibleBody}>
                    <form onSubmit={onCreateProposal}>
                      <div className={s.field}>
                        <label className={s.fieldLabel} htmlFor="asg">
                          Кому
                        </label>
                        <select
                          id="asg"
                          className={s.fieldSelect}
                          value={assigneeId || ''}
                          onChange={(e) => setAssigneeId(parseInt(e.target.value, 10))}
                        >
                          {members.map((m) => (
                            <option key={m.user_id} value={m.user_id}>
                              {m.display_name?.trim() || m.email}
                            </option>
                          ))}
                        </select>
                      </div>
                      <div className={s.field}>
                        <label className={s.fieldLabel} htmlFor="pt">
                          Заголовок
                        </label>
                        <input
                          id="pt"
                          className={s.fieldInput}
                          value={pTitle}
                          onChange={(e) => setPTitle(e.target.value)}
                          required
                          maxLength={500}
                        />
                      </div>
                      <div className={s.field}>
                        <label className={s.fieldLabel} htmlFor="pd">
                          Описание
                        </label>
                        <textarea
                          id="pd"
                          className={s.fieldTextarea}
                          value={pDesc}
                          onChange={(e) => setPDesc(e.target.value)}
                          maxLength={8000}
                        />
                      </div>
                      <div className={s.field}>
                        <label className={s.fieldLabel} htmlFor="ps">
                          Начало (если нужен слот в календаре)
                        </label>
                        <input
                          id="ps"
                          type="datetime-local"
                          className={s.fieldInput}
                          value={pStart}
                          onChange={(e) => setPStart(e.target.value)}
                        />
                      </div>
                      <div className={s.field}>
                        <label className={s.fieldLabel} htmlFor="pe">
                          Конец
                        </label>
                        <input
                          id="pe"
                          type="datetime-local"
                          className={s.fieldInput}
                          value={pEnd}
                          onChange={(e) => setPEnd(e.target.value)}
                        />
                      </div>
                      <p className={s.fieldHint}>Начало и конец указывайте вместе, иначе событие в календарь не попадёт.</p>
                      <button type="submit" className={a.addBtn} disabled={propBusy || !pTitle.trim()}>
                        {propBusy ? 'Отправка…' : 'Отправить'}
                      </button>
                    </form>
                  </div>
                )}
              </section>
            )}

            <section
              ref={proposalsSectionRef}
              id="space-proposals"
              className={`${a.card} ${a.cardWide}`}
              aria-labelledby="prop-list-h"
            >
              <div className={s.collapsibleHead}>
                <div className={s.collapsibleTitles}>
                  <div
                    className={a.cardHeader}
                    style={{ marginBottom: 0, paddingBottom: 0, borderBottom: 'none' }}
                  >
                    <h2 className={a.cardTitle} id="prop-list-h">
                      Предложения
                    </h2>
                  </div>
                  {proposals.length > 0 && !proposalsHistoryOpen ? (
                    <p className={s.collapsibleHint}>{proposalsCollapsedHint(proposals)}</p>
                  ) : null}
                </div>
                {proposals.length > 0 ? (
                  <div className={s.btnToolbar}>
                    <button
                      type="button"
                      className={s.btnSmall}
                      aria-expanded={proposalsHistoryOpen}
                      onClick={() => setProposalsHistoryOpen((o) => !o)}
                    >
                      {proposalsHistoryOpen ? 'Свернуть' : 'Развернуть'}
                    </button>
                  </div>
                ) : null}
              </div>
              {proposalsHistoryOpen ? (
                proposals.length === 0 ? (
                  <p className={a.muted}>Пока нет.</p>
                ) : (
                  <ul className={s.proposalList}>
                    {proposals.map((p) => (
                      <li key={p.id} className={s.proposalItem}>
                        <span className={statusPillClass(p.status)}>
                          {p.status === 'pending'
                            ? 'ожидание'
                            : p.status === 'accepted'
                              ? 'принято'
                              : p.status === 'declined'
                                ? 'отклонено'
                                : p.status}
                        </span>
                        <p className={s.proposalTitle}>{p.title}</p>
                        <p className={a.muted}>
                          {memberNameById.get(p.author_id) ?? `#${p.author_id}`}
                          {' → '}
                          {memberNameById.get(p.assignee_id) ?? `#${p.assignee_id}`}
                          {p.proposed_start && p.proposed_end
                            ? ` · ${new Date(p.proposed_start).toLocaleString('ru-RU', { dateStyle: 'short', timeStyle: 'short' })} — ${new Date(p.proposed_end).toLocaleString('ru-RU', { dateStyle: 'short', timeStyle: 'short' })}`
                            : ''}
                        </p>
                        {p.description ? <p className={s.proposalDesc}>{p.description}</p> : null}
                        {p.status === 'pending' && myId === p.assignee_id && (
                          <div className={s.respondRow}>
                            <input
                              type="text"
                              className={s.respondInput}
                              placeholder="Комментарий"
                              value={respondNote[p.id] ?? ''}
                              onChange={(e) => setRespondNote((m) => ({ ...m, [p.id]: e.target.value }))}
                            />
                            <button type="button" className={a.addBtn} onClick={() => onRespond(p.id, 'accepted')}>
                              Принять
                            </button>
                            <button type="button" className={s.btnGhost} onClick={() => onRespond(p.id, 'declined')}>
                              Отклонить
                            </button>
                          </div>
                        )}
                        {p.google_event_id ? (
                          <p className={a.muted}>Календарь: {p.google_event_id}</p>
                        ) : null}
                      </li>
                    ))}
                  </ul>
                )
              ) : null}
            </section>

            {isCreator ? (
              <section className={`${a.card} ${a.cardWide}`} aria-labelledby="del-h">
                <div className={a.cardHeader}>
                  <h2 className={a.cardTitle} id="del-h">
                    Опасная зона
                  </h2>
                </div>
                <p className={s.collapsibleHint}>
                  Удалить пространство может только создатель. У всех участников оно пропадёт из списка вместе с предложениями.
                </p>
                <div className={s.rowActions}>
                  <button type="button" className={s.btnDanger} disabled={deleteBusy} onClick={() => void onDeleteSpace()}>
                    {deleteBusy ? 'Удаляем…' : 'Удалить пространство'}
                  </button>
                </div>
              </section>
            ) : null}
          </>
        )}
      </div>
    </div>
  )
}
