import { createPortal } from 'react-dom'
import {
  useCallback,
  useEffect,
  useId,
  useState,
  type DragEvent,
  type FormEvent,
} from 'react'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import {
  faBriefcase,
  faGraduationCap,
  faHeartPulse,
  faHouseChimney,
  faTag,
} from '@fortawesome/free-solid-svg-icons'
import {
  BACKLOG_DRAG_MIME,
  createBacklogIdea,
  deleteBacklogIdea,
  fetchBacklogIdeas,
  type BacklogIdea,
  type ScheduleBacklogOptions,
} from '../lib/backlogApi'
import { BACKLOG_DRAG_PAYLOAD_MIME, scheduleBacklogViaSmartTask } from '../lib/backlogSmartSchedule'
import { SmartTaskError } from '../lib/calendarApi'
import type { User } from '../auth/AuthContext'
import { formatQuotaResetsAtHint, parseSmartTasksToday } from '../lib/smartTaskQuota'
import bp from './BacklogIdeasPanel.module.css'

/** Без «зелёных» соседей: убраны 4 и 7 из выбора (остались прочие оттенки). */
const COLOR_IDS = ['11', '9', '6', '3', '5', '2', '1', '8', '10'] as const

const TAG_PRESETS = [
  { value: '', label: '—' },
  { value: 'Работа', label: 'Работа' },
  { value: 'Быт', label: 'Быт' },
  { value: 'Здоровье', label: 'Здоровье' },
  { value: 'Учёба', label: 'Учёба' },
  { value: '__other__', label: 'Своё…' },
] as const

type TagKind = 'none' | 'work' | 'home' | 'health' | 'study' | 'other'

function classifyTag(tag: string): TagKind {
  const x = tag.trim().toLowerCase()
  if (!x) return 'none'
  if (x === 'работа') return 'work'
  if (x === 'быт' || x === 'дом') return 'home'
  if (x === 'здоровье') return 'health'
  if (x === 'учёба' || x === 'учеба') return 'study'
  return 'other'
}

const TAG_ICON = {
  work: faBriefcase,
  home: faHouseChimney,
  health: faHeartPulse,
  study: faGraduationCap,
  other: faTag,
} as const

function TagChip({ tag }: { tag: string }) {
  const kind = classifyTag(tag)
  if (kind === 'none') return null
  const label = tag.trim()
  const icon = TAG_ICON[kind === 'other' ? 'other' : kind]
  const chipMod =
    kind === 'work'
      ? bp.tagChipWork
      : kind === 'home'
        ? bp.tagChipHome
        : kind === 'health'
          ? bp.tagChipHealth
          : kind === 'study'
            ? bp.tagChipStudy
            : bp.tagChipOther

  return (
    <span className={[bp.tagChip, chipMod].join(' ')} title={label} aria-label={`Тег: ${label}`}>
      <FontAwesomeIcon icon={icon} className={bp.tagFa} />
    </span>
  )
}

function localDateISO(d: Date): string {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

function toneClass(colorId: string): string {
  const n = String(colorId)
  if (n >= '1' && n <= '11' && bp[`tone${n}` as keyof typeof bp]) {
    return bp[`tone${n}` as keyof typeof bp] as string
  }
  return bp.tone4
}

function scheduleErrMessage(e: unknown): string {
  if (e instanceof SmartTaskError) {
    let m = e.message
    if (e.status === 403) {
      const st = parseSmartTasksToday(e.payload)
      const h = st?.resets_at ? formatQuotaResetsAtHint(st.resets_at) : ''
      if (h) m = `${m} ${h}`
    }
    return m
  }
  return e instanceof Error ? e.message : 'Не удалось запланировать'
}

export type BacklogIdeasPanelProps = {
  accessToken: string | null
  linked: boolean
  refreshKey: number
  onRefreshCalendar: () => void
  onModalLockChange?: (locked: boolean) => void
  refreshSession: () => Promise<boolean>
  navigateLogin: () => void
  /** Тариф Pro — без дневного лимита умных задач. */
  isPro?: boolean
  /** Квота из /api/auth/me; для Free показываем лимит и время сброса. */
  smartTasksToday?: User['smart_tasks_today'] | null
}

export function BacklogIdeasPanel({
  accessToken,
  linked,
  refreshKey,
  onRefreshCalendar,
  onModalLockChange,
  refreshSession,
  navigateLogin,
  isPro = false,
  smartTasksToday = null,
}: BacklogIdeasPanelProps) {
  const [ideas, setIdeas] = useState<BacklogIdea[]>([])
  const [loading, setLoading] = useState(false)
  const [listErr, setListErr] = useState<string | null>(null)

  const [createOpen, setCreateOpen] = useState(false)
  const [draftBody, setDraftBody] = useState('')
  const [draftColor, setDraftColor] = useState<string>('11')
  const [draftTagChoice, setDraftTagChoice] = useState<string>('')
  const [draftTagOther, setDraftTagOther] = useState('')
  const [createBusy, setCreateBusy] = useState(false)
  const [createErr, setCreateErr] = useState<string | null>(null)
  const [helpOpen, setHelpOpen] = useState(false)

  const modalTitleId = useId()
  const helpModalTitleId = useId()

  useEffect(() => {
    onModalLockChange?.(createOpen || helpOpen)
  }, [createOpen, helpOpen, onModalLockChange])

  const loadIdeas = useCallback(async () => {
    if (!accessToken) {
      setIdeas([])
      return
    }
    setLoading(true)
    setListErr(null)
    try {
      const rows = await fetchBacklogIdeas(accessToken)
      setIdeas(rows)
    } catch (e) {
      if (e instanceof Error && e.message === '401') {
        const ok = await refreshSession()
        if (!ok) {
          navigateLogin()
          return
        }
        try {
          const rows = await fetchBacklogIdeas(accessToken)
          setIdeas(rows)
          return
        } catch {
          setListErr('Не удалось загрузить идеи')
          setIdeas([])
          return
        }
      }
      setListErr(e instanceof Error ? e.message : 'Не удалось загрузить идеи')
      setIdeas([])
    } finally {
      setLoading(false)
    }
  }, [accessToken, refreshSession, navigateLogin])

  useEffect(() => {
    void loadIdeas()
  }, [loadIdeas, refreshKey])

  const openCreate = () => {
    setCreateErr(null)
    setDraftBody('')
    setDraftColor('11')
    setDraftTagChoice('')
    setDraftTagOther('')
    setCreateOpen(true)
  }

  const closeCreate = () => {
    setCreateOpen(false)
    setCreateErr(null)
  }

  const resolvedDraftTag = (): string => {
    if (draftTagChoice === '__other__') {
      return draftTagOther.trim()
    }
    return draftTagChoice.trim()
  }

  const submitCreate = async (e: FormEvent) => {
    e.preventDefault()
    if (!accessToken) return
    const body = draftBody.trim()
    if (!body) {
      setCreateErr('Введите текст идеи')
      return
    }
    const tag = resolvedDraftTag()
    setCreateBusy(true)
    setCreateErr(null)
    try {
      const row = await createBacklogIdea(accessToken, {
        body,
        color_id: draftColor,
        tag,
      })
      setIdeas((prev) => [row, ...prev])
      closeCreate()
    } catch (err) {
      if (err instanceof Error && err.message === '401') {
        const ok = await refreshSession()
        if (!ok) {
          navigateLogin()
          return
        }
        setCreateErr('Сессия обновлена — попробуйте ещё раз')
        return
      }
      setCreateErr(err instanceof Error ? err.message : 'Ошибка сохранения')
    } finally {
      setCreateBusy(false)
    }
  }

  const handleSchedule = async (idea: BacklogIdea, date: string, opts?: ScheduleBacklogOptions) => {
    if (!accessToken || !linked) return
    const snapshot = {
      id: idea.id,
      body: idea.body,
      color_id: idea.color_id,
      tag: idea.tag,
    }
    const run = async () => {
      await scheduleBacklogViaSmartTask(accessToken, snapshot, date, opts)
      setIdeas((prev) => prev.filter((x) => x.id !== idea.id))
      onRefreshCalendar()
      await refreshSession()
    }
    try {
      await run()
    } catch (err) {
      if (err instanceof Error && err.message === '401') {
        const ok = await refreshSession()
        if (!ok) {
          navigateLogin()
          return
        }
        await run()
        return
      }
      throw err
    }
  }

  const handleDelete = async (ideaId: string) => {
    if (!accessToken) return
    try {
      await deleteBacklogIdea(accessToken, ideaId)
      setIdeas((prev) => prev.filter((x) => x.id !== ideaId))
    } catch (err) {
      if (err instanceof Error && err.message === '401') {
        const ok = await refreshSession()
        if (!ok) {
          navigateLogin()
          return
        }
        await deleteBacklogIdea(accessToken, ideaId)
        setIdeas((prev) => prev.filter((x) => x.id !== ideaId))
        return
      }
      throw err instanceof Error ? err : new Error('Не удалось удалить')
    }
  }

  const modal =
    createOpen &&
    createPortal(
      <div className={bp.modalRoot}>
        <button type="button" className={bp.modalBackdrop} aria-label="Закрыть" onClick={closeCreate} />
        <div
          className={bp.modalPanel}
          role="dialog"
          aria-modal="true"
          aria-labelledby={modalTitleId}
          lang="ru"
          onClick={(ev) => ev.stopPropagation()}
        >
          <header className={bp.modalHeader}>
            <h2 id={modalTitleId} className={bp.modalTitle}>
              Новая идея
            </h2>
            <button type="button" className={bp.modalClose} onClick={closeCreate} aria-label="Закрыть">
              ×
            </button>
          </header>
          <form className={bp.modalBody} onSubmit={submitCreate}>
            <div>
              <label className={bp.label} htmlFor="backlog-body">
                Текст
              </label>
              <textarea
                id="backlog-body"
                className={bp.textarea}
                value={draftBody}
                onChange={(ev) => {
                  setDraftBody(ev.target.value)
                  setCreateErr(null)
                }}
                placeholder="Коротко, что нужно сделать"
                rows={3}
                disabled={createBusy}
              />
            </div>
            <div>
              <span className={bp.label}>Цвет</span>
              <div className={bp.colorRow}>
                {COLOR_IDS.map((id) => (
                  <button
                    key={id}
                    type="button"
                    className={[bp.colorSwatch, toneClass(id), draftColor === id ? bp.colorSwatchActive : '']
                      .filter(Boolean)
                      .join(' ')}
                    aria-label={`Цвет ${id}`}
                    aria-pressed={draftColor === id}
                    onClick={() => setDraftColor(id)}
                  />
                ))}
              </div>
            </div>
            <div>
              <label className={bp.label} htmlFor="backlog-tag">
                Тег
              </label>
              <select
                id="backlog-tag"
                className={bp.select}
                value={draftTagChoice}
                onChange={(ev) => setDraftTagChoice(ev.target.value)}
                disabled={createBusy}
              >
                {TAG_PRESETS.map((o) => (
                  <option key={o.value || 'empty'} value={o.value}>
                    {o.label}
                  </option>
                ))}
              </select>
              {draftTagChoice === '__other__' && (
                <input
                  className={bp.customTagInput}
                  style={{ marginTop: 8 }}
                  value={draftTagOther}
                  onChange={(ev) => setDraftTagOther(ev.target.value)}
                  placeholder="Свой тег"
                  disabled={createBusy}
                  aria-label="Свой тег"
                />
              )}
            </div>
            {createErr && (
              <p className={bp.modalErr} role="alert">
                {createErr}
              </p>
            )}
            <div className={bp.submitRow}>
              <button type="submit" className={bp.submitBtn} disabled={createBusy}>
                {createBusy ? 'Создаю…' : 'Создать'}
              </button>
            </div>
          </form>
        </div>
      </div>,
      document.body,
    )

  const helpModal =
    helpOpen &&
    createPortal(
      <div className={bp.modalRoot}>
        <button type="button" className={bp.modalBackdrop} aria-label="Закрыть" onClick={() => setHelpOpen(false)} />
        <div
          className={bp.modalPanel}
          role="dialog"
          aria-modal="true"
          aria-labelledby={helpModalTitleId}
          lang="ru"
          onClick={(ev) => ev.stopPropagation()}
        >
          <header className={bp.modalHeader}>
            <h2 id={helpModalTitleId} className={bp.modalTitle}>
              Как пользоваться бэклогом
            </h2>
            <button type="button" className={bp.modalClose} onClick={() => setHelpOpen(false)} aria-label="Закрыть">
              ×
            </button>
          </header>
          <div className={bp.helpBody}>
            <ul className={bp.helpSteps}>
              <li className={bp.helpStep}>
                <span className={bp.helpStepMark} aria-hidden>
                  <i className="fa-solid fa-lightbulb" />
                </span>
                <div>
                  <p className={bp.helpStepTitle}>Зафиксируйте мысль</p>
                  <p className={bp.helpStepText}>
                    Нажмите «Новая идея», кратко опишите задачу. Цвет и тег помогут быстро ориентироваться в списке.
                  </p>
                </div>
              </li>
              <li className={bp.helpStep}>
                <span className={bp.helpStepMark} aria-hidden>
                  <i className="fa-solid fa-wand-magic-sparkles" />
                </span>
                <div>
                  <p className={bp.helpStepTitle}>Отправьте в Google Календарь</p>
                  <p className={bp.helpStepText}>
                    Иконка с волшебной палочкой создаёт событие через умного помощника на выбранную дату. Пустое время
                    означает событие на весь день.
                  </p>
                </div>
              </li>
              <li className={bp.helpStep}>
                <span className={bp.helpStepMark} aria-hidden>
                  <i className="fa-solid fa-plus" />
                </span>
                <div>
                  <p className={bp.helpStepTitle}>Дата и время вручную</p>
                  <p className={bp.helpStepText}>
                    Кнопка «+» напротив бейджа «Когда» раскрывает поля даты и времени — удобно, если нужна точная
                    привязка.
                  </p>
                </div>
              </li>
              <li className={bp.helpStep}>
                <span className={bp.helpStepMark} aria-hidden>
                  <i className="fa-solid fa-hand-pointer" />
                </span>
                <div>
                  <p className={bp.helpStepTitle}>Перетащите на календарь</p>
                  <p className={bp.helpStepText}>
                    При подключённом календаре карточку можно перетащить на нужный день в сетке — так идея попадёт в
                    расписание без ручного ввода даты.
                  </p>
                </div>
              </li>
              <li className={bp.helpStep}>
                <span className={bp.helpStepMark} aria-hidden>
                  <i className="fa-solid fa-trash" />
                </span>
                <div>
                  <p className={bp.helpStepTitle}>Удалите лишнее</p>
                  <p className={bp.helpStepText}>Иконка корзины убирает стикер из бэклога без добавления в календарь.</p>
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
      {modal}
      {helpModal}
      <div className={bp.root}>
        <header className={bp.panelHeader}>
          <p className={bp.panelEyebrow}>Вне расписания</p>
          <div className={bp.panelTitleRow}>
            <h2 className={bp.panelTitle}>Бэклог идей</h2>
            <button
              type="button"
              className={bp.helpBtn}
              aria-label="Как пользоваться бэклогом"
              title="Подсказка"
              onClick={() => setHelpOpen(true)}
            >
              <i className="fa-regular fa-circle-question" aria-hidden />
            </button>
          </div>
        </header>
        <div className={bp.toolbar}>
          <button type="button" className={bp.addBtn} onClick={openCreate} disabled={!accessToken}>
            <i className="fa-solid fa-plus" aria-hidden />
            <span>Новая идея</span>
          </button>
        </div>
        {listErr && (
          <p className={bp.err} role="alert">
            {listErr}
          </p>
        )}
        {!linked && accessToken && (
          <p className={bp.unlinkNote}>
            <i className="fa-solid fa-link-slash" aria-hidden />
            <span>Подключите Google Календарь в профиле, чтобы переносить стикеры.</span>
          </p>
        )}
        {loading && ideas.length === 0 ? (
          <p className={bp.empty}>Загрузка…</p>
        ) : ideas.length === 0 ? (
          <p className={bp.empty}>Пока пусто.</p>
        ) : (
          <ul className={bp.list}>
            {ideas.map((idea) => (
              <StickerCard
                key={idea.id}
                idea={idea}
                draggable={linked}
                isPro={isPro}
                smartTasksToday={smartTasksToday}
                onDelete={() => handleDelete(idea.id)}
                onSchedule={(date, opts) => handleSchedule(idea, date, opts)}
              />
            ))}
          </ul>
        )}
      </div>
    </>
  )
}

function StickerCard({
  idea,
  draggable,
  isPro,
  smartTasksToday,
  onDelete,
  onSchedule,
}: {
  idea: BacklogIdea
  draggable: boolean
  isPro: boolean
  smartTasksToday: User['smart_tasks_today'] | null | undefined
  onDelete: () => Promise<void>
  onSchedule: (date: string, opts?: ScheduleBacklogOptions) => Promise<void>
}) {
  const [date, setDate] = useState(() => localDateISO(new Date()))
  const [timeVal, setTimeVal] = useState('')
  const [schedBusy, setSchedBusy] = useState(false)
  const [delBusy, setDelBusy] = useState(false)
  const [scheduleErr, setScheduleErr] = useState<string | null>(null)
  const [deleteErr, setDeleteErr] = useState<string | null>(null)
  const [manualScheduleOpen, setManualScheduleOpen] = useState(false)

  const quotaExhausted =
    !isPro && smartTasksToday != null && smartTasksToday.used >= smartTasksToday.limit

  useEffect(() => {
    setScheduleErr(null)
  }, [date, timeVal])

  const onDragStart = (e: DragEvent) => {
    if (!draggable) {
      e.preventDefault()
      return
    }
    const payload = JSON.stringify({
      id: idea.id,
      body: idea.body,
      color_id: idea.color_id,
      tag: idea.tag,
    })
    e.dataTransfer.setData(BACKLOG_DRAG_PAYLOAD_MIME, payload)
    e.dataTransfer.setData(BACKLOG_DRAG_MIME, idea.id)
    e.dataTransfer.setData('text/plain', idea.id)
    e.dataTransfer.effectAllowed = 'copy'
  }

  const runSchedule = async () => {
    setSchedBusy(true)
    setScheduleErr(null)
    try {
      const opts: ScheduleBacklogOptions | undefined =
        timeVal.trim() !== '' ? { start_time: timeVal.trim() } : undefined
      await onSchedule(date, opts)
    } catch (e) {
      setScheduleErr(scheduleErrMessage(e))
    } finally {
      setSchedBusy(false)
    }
  }

  const runDelete = async () => {
    setDelBusy(true)
    setDeleteErr(null)
    try {
      await onDelete()
    } catch (e) {
      setDeleteErr(e instanceof Error ? e.message : 'Не удалось удалить')
    } finally {
      setDelBusy(false)
    }
  }

  return (
    <li
      className={[bp.stickerRow, toneClass(idea.color_id), draggable ? '' : bp.stickerRowMuted].filter(Boolean).join(' ')}
      draggable={draggable}
      onDragStart={onDragStart}
    >
      <div className={bp.stickerTop}>
        <div className={bp.stickerTagWrap}>
          <TagChip tag={idea.tag} />
        </div>
        <p className={bp.stickerBody}>{idea.body}</p>
      </div>

      <div className={bp.stickerSchedule}>
        <div className={bp.scheduleBar} role="group" aria-label="Когда: дата и время">
          <span className={bp.schedulePill}>
            <i className="fa-regular fa-calendar" aria-hidden />
            Когда
          </span>
          <button
            type="button"
            className={bp.scheduleToggle}
            aria-expanded={manualScheduleOpen}
            aria-label={manualScheduleOpen ? 'Скрыть дату и время' : 'Показать дату и время'}
            title={manualScheduleOpen ? 'Скрыть дату и время' : 'Дата и время вручную'}
            disabled={schedBusy}
            onClick={() => setManualScheduleOpen((v) => !v)}
          >
            <i className={manualScheduleOpen ? 'fa-solid fa-minus' : 'fa-solid fa-plus'} aria-hidden />
          </button>
        </div>
        {manualScheduleOpen ? (
          <div className={bp.stickerInputs}>
            <input
              type="date"
              className={bp.dateInput}
              value={date}
              onChange={(e) => setDate(e.target.value)}
              disabled={schedBusy || !draggable}
              aria-label="Дата"
            />
            <input
              type="time"
              className={bp.timeInput}
              value={timeVal}
              onChange={(e) => setTimeVal(e.target.value)}
              disabled={schedBusy || !draggable}
              aria-label="Время (пусто — весь день)"
            />
          </div>
        ) : null}
        <div className={bp.stickerIconActions}>
          <button
            type="button"
            className={bp.iconActionPlan}
            disabled={!draggable || schedBusy || quotaExhausted}
            aria-label="Добавить в календарь с помощью ИИ"
            title="В календарь (ИИ)"
            onClick={() => void runSchedule()}
          >
            {schedBusy ? (
              <i className={`fa-solid fa-spinner ${bp.btnSpin}`} aria-hidden />
            ) : (
              <i className="fa-solid fa-wand-magic-sparkles" aria-hidden />
            )}
          </button>
          <button
            type="button"
            className={bp.iconActionDel}
            disabled={delBusy}
            aria-label="Удалить стикер"
            title="Удалить"
            onClick={() => void runDelete()}
          >
            {delBusy ? <i className={`fa-solid fa-spinner ${bp.btnSpin}`} aria-hidden /> : <i className="fa-solid fa-trash" aria-hidden />}
          </button>
        </div>
      </div>

      {quotaExhausted && smartTasksToday?.resets_at ? (
        <p className={bp.cardQuotaHint} role="status">
          {formatQuotaResetsAtHint(smartTasksToday.resets_at)}
        </p>
      ) : null}
      {scheduleErr ? (
        <p className={bp.cardErr} role="alert">
          {scheduleErr}
        </p>
      ) : null}
      {deleteErr ? (
        <p className={bp.cardErr} role="alert">
          {deleteErr}
        </p>
      ) : null}
    </li>
  )
}
