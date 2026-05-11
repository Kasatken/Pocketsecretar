export type BacklogIdea = {
  id: string
  body: string
  color_id: string
  tag: string
  created_at: string
}

/** MIME type for drag-and-drop of backlog sticker id (fallback: text/plain). */
export const BACKLOG_DRAG_MIME = 'application/x-pocketsecret-backlog-id'

export async function fetchBacklogIdeas(accessToken: string): Promise<BacklogIdea[]> {
  const res = await fetch('/api/backlog/ideas', {
    headers: { Authorization: `Bearer ${accessToken}` },
    credentials: 'include',
  })

  if (res.status === 401) {
    const err = new Error('401')
    err.name = 'Unauthorized'
    throw err
  }

  if (!res.ok) {
    const body = (await res.json().catch(() => ({}))) as { error?: string }
    throw new Error(typeof body.error === 'string' ? body.error : 'Не удалось загрузить идеи')
  }

  const data = (await res.json()) as { ideas?: BacklogIdea[] }
  return Array.isArray(data.ideas) ? data.ideas : []
}

export async function createBacklogIdea(
  accessToken: string,
  payload: { body: string; color_id: string; tag: string },
): Promise<BacklogIdea> {
  const res = await fetch('/api/backlog/ideas', {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${accessToken}`,
      'Content-Type': 'application/json',
    },
    credentials: 'include',
    body: JSON.stringify(payload),
  })

  if (res.status === 401) {
    const err = new Error('401')
    err.name = 'Unauthorized'
    throw err
  }

  const data = (await res.json().catch(() => ({}))) as { idea?: BacklogIdea; error?: string }

  if (!res.ok) {
    throw new Error(typeof data.error === 'string' ? data.error : 'Не удалось сохранить идею')
  }

  if (!data.idea) {
    throw new Error('Некорректный ответ сервера')
  }

  return data.idea
}

export async function deleteBacklogIdea(accessToken: string, id: string): Promise<void> {
  const res = await fetch(`/api/backlog/ideas/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${accessToken}` },
    credentials: 'include',
  })

  if (res.status === 401) {
    const err = new Error('401')
    err.name = 'Unauthorized'
    throw err
  }

  if (res.status === 404) {
    throw new Error('Стикер уже удалён')
  }

  if (!res.ok) {
    const body = (await res.json().catch(() => ({}))) as { error?: string }
    throw new Error(typeof body.error === 'string' ? body.error : 'Не удалось удалить')
  }
}

export type ScheduleBacklogResponse = {
  ok: boolean
  event_id?: string
  warning?: string
}

/** Если задано `start_time` (`HH:MM`), в календаре создаётся событие с временем (час по умолчанию). Иначе — на весь день. */
export type ScheduleBacklogOptions = {
  start_time?: string
  end_time?: string
}

export async function scheduleBacklogIdea(
  accessToken: string,
  id: string,
  date: string,
  opts?: ScheduleBacklogOptions,
): Promise<ScheduleBacklogResponse> {
  const body: Record<string, string> = { date }
  if (opts?.start_time) body.start_time = opts.start_time
  if (opts?.end_time) body.end_time = opts.end_time

  const res = await fetch(`/api/backlog/ideas/${encodeURIComponent(id)}/schedule`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${accessToken}`,
      'Content-Type': 'application/json',
    },
    credentials: 'include',
    body: JSON.stringify(body),
  })

  if (res.status === 401) {
    const err = new Error('401')
    err.name = 'Unauthorized'
    throw err
  }

  const data = (await res.json().catch(() => ({}))) as ScheduleBacklogResponse & { error?: string }

  if (!res.ok) {
    throw new Error(typeof data.error === 'string' ? data.error : 'Не удалось запланировать')
  }

  return data
}
