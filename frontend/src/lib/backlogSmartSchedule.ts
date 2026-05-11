import { deleteBacklogIdea, type ScheduleBacklogOptions } from './backlogApi'
import { postSmartTask } from './calendarApi'

/** Полный снимок стикера для DnD и запроса к ИИ. */
export type BacklogStickerSnapshot = {
  id: string
  body: string
  color_id: string
  tag: string
}

export const BACKLOG_DRAG_PAYLOAD_MIME = 'application/x-pocketsecret-backlog'

function localTodayISO(): string {
  const d = new Date()
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

export function parseBacklogDragPayload(dt: DataTransfer): BacklogStickerSnapshot | null {
  const raw = dt.getData(BACKLOG_DRAG_PAYLOAD_MIME)
  if (!raw) return null
  try {
    const o = JSON.parse(raw) as Partial<BacklogStickerSnapshot>
    if (
      typeof o.id === 'string' &&
      typeof o.body === 'string' &&
      typeof o.color_id === 'string' &&
      typeof o.tag === 'string'
    ) {
      return { id: o.id, body: o.body, color_id: o.color_id, tag: o.tag }
    }
  } catch {
    /* ignore */
  }
  return null
}

/** Текст запроса для того же пайплайна, что и «Умный ввод». */
export function buildBacklogSmartPrompt(
  snapshot: BacklogStickerSnapshot,
  targetDate: string,
  opts?: ScheduleBacklogOptions,
): string {
  const lines = [
    'Создай ровно одно событие в Google Календаре пользователя.',
    `Дата события строго: ${targetDate}.`,
    `Текст события (summary): ${snapshot.body.trim()}.`,
    `Обязательно выставь у события colorId календаря Google: "${snapshot.color_id}" (допустимы только значения 1–11).`,
  ]
  if (snapshot.tag.trim()) {
    lines.push(`Тег для контекста (можно кратко отразить в description): ${snapshot.tag.trim()}.`)
  }
  if (opts?.start_time?.trim()) {
    lines.push(
      `Начало в ${opts.start_time.trim()}, длительность ровно 1 час, не all-day. Часовой пояс Europe/Moscow.`,
    )
  } else {
    lines.push('Событие на весь этот календарный день (all-day).')
  }
  lines.push(
    'Не добавляй других событий и подзадач — только это одно. Не переноси на другую дату.',
  )
  return lines.join('\n')
}

/** Создание дела через ИИ (как кнопка «Добавить задачу»), затем удаление стикера из бэклога. */
export async function scheduleBacklogViaSmartTask(
  accessToken: string,
  snapshot: BacklogStickerSnapshot,
  targetDate: string,
  opts?: ScheduleBacklogOptions,
): Promise<void> {
  const text = buildBacklogSmartPrompt(snapshot, targetDate, opts)
  await postSmartTask(accessToken, text, localTodayISO())
  try {
    await deleteBacklogIdea(accessToken, snapshot.id)
  } catch (e) {
    const msg = e instanceof Error ? e.message : ''
    if (msg.includes('Стикер уже удалён')) return
    throw e
  }
}
