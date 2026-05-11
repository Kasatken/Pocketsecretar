/** Monday-based week (typical for RU locale). */
export function startOfWeekMonday(d: Date): Date {
  const x = new Date(d)
  const day = x.getDay()
  const diff = day === 0 ? -6 : 1 - day
  x.setDate(x.getDate() + diff)
  x.setHours(0, 0, 0, 0)
  return x
}

export function addDays(d: Date, n: number): Date {
  const x = new Date(d)
  x.setDate(x.getDate() + n)
  return x
}

export function addMonths(d: Date, n: number): Date {
  const x = new Date(d)
  x.setMonth(x.getMonth() + n)
  return x
}

export function addWeeks(d: Date, n: number): Date {
  return addDays(d, n * 7)
}

export function startOfMonth(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), 1, 0, 0, 0, 0)
}

export function endOfMonth(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth() + 1, 0, 23, 59, 59, 999)
}

/** First cell (Mon) and last cell (Sun) of the month grid. */
export function getMonthGridRange(year: number, monthIndex: number): { gridStart: Date; gridEnd: Date } {
  const first = new Date(year, monthIndex, 1)
  const last = new Date(year, monthIndex + 1, 0)
  const gridStart = startOfWeekMonday(first)
  const lastWeekStart = startOfWeekMonday(last)
  const gridEnd = new Date(lastWeekStart)
  gridEnd.setDate(gridEnd.getDate() + 6)
  gridEnd.setHours(23, 59, 59, 999)
  return { gridStart, gridEnd }
}

export function getWeekRange(weekStart: Date): { start: Date; end: Date } {
  const start = startOfWeekMonday(weekStart)
  const end = new Date(start)
  end.setDate(end.getDate() + 6)
  end.setHours(23, 59, 59, 999)
  return { start, end }
}

export function dayKey(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

export function parseDayKey(key: string): Date {
  const [y, m, d] = key.split('-').map(Number)
  return new Date(y, m - 1, d, 12, 0, 0, 0)
}

export function startOfLocalDay(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate(), 0, 0, 0, 0)
}

export function endOfLocalDay(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate(), 23, 59, 59, 999)
}

export function formatMonthTitle(d: Date): string {
  return d.toLocaleDateString('ru-RU', { month: 'long', year: 'numeric' })
}

/**
 * Крупный заголовок периода в шапке календаря: месяц просмотра + сегодняшняя дата,
 * напр. «МАЙ · 10 мая 2026 г.» (вместо двузначного года).
 */
export function formatCalendarHeroMonthYear(d: Date, today: Date = new Date()): string {
  const month = d.toLocaleDateString('ru-RU', { month: 'long' })
  const datePart = today.toLocaleDateString('ru-RU', {
    day: 'numeric',
    month: 'long',
    year: 'numeric',
  })
  return `${month.toUpperCase()} · ${datePart}`
}

export function formatWeekRange(start: Date, end: Date): string {
  const opts: Intl.DateTimeFormatOptions = { day: 'numeric', month: 'short' }
  const a = start.toLocaleDateString('ru-RU', opts)
  const b = end.toLocaleDateString('ru-RU', { ...opts, year: 'numeric' })
  return `${a} — ${b}`
}

/** Calendar days for month grid (Mon → Sun rows). */
export function buildMonthGridCells(year: number, monthIndex: number): Date[] {
  const { gridStart, gridEnd } = getMonthGridRange(year, monthIndex)
  const cells: Date[] = []
  const cur = new Date(gridStart)
  while (cur <= gridEnd) {
    cells.push(new Date(cur))
    cur.setDate(cur.getDate() + 1)
  }
  return cells
}

export type SpaceProposalOnEvent = {
  proposalId: number
  spaceId: number
  spaceName: string
  authorDisplayName: string
  authorAvatarUrl: string
  /** true, если уже есть событие в Google с тем же id. */
  calendarSynced: boolean
}

export type NormalizedEvent = {
  id: string
  title: string
  start: Date
  end: Date
  allDay: boolean
  colorId?: string
  completed?: boolean
  /** Событие создано умным планировщиком как перерыв (чай / отдых). */
  isSmartBreak?: boolean
  /** Id принятого предложения из Google extendedProperties (если есть). */
  spaceProposalId?: number
  /** Принятое предложение из пространства: полупрозрачный слой + автор. */
  spaceProposal?: SpaceProposalOnEvent
}

export function sortDayEvents(list: NormalizedEvent[]): NormalizedEvent[] {
  return [...list].sort((a, b) => {
    if (a.allDay !== b.allDay) return a.allDay ? -1 : 1
    return a.start.getTime() - b.start.getTime()
  })
}

const timeFmtRu: Intl.DateTimeFormatOptions = { hour: '2-digit', minute: '2-digit' }

export function formatEventTimeRange(ev: NormalizedEvent): string {
  if (ev.allDay) return 'Весь день'
  return `${ev.start.toLocaleTimeString('ru-RU', timeFmtRu)} — ${ev.end.toLocaleTimeString('ru-RU', timeFmtRu)}`
}

/** Assign event to each calendar day it spans (local dates). */
export function eventsByDayKey(events: NormalizedEvent[]): Map<string, NormalizedEvent[]> {
  const map = new Map<string, NormalizedEvent[]>()

  for (const ev of events) {
    const keys = expandEventToDayKeys(ev)
    for (const key of keys) {
      const arr = map.get(key)
      if (arr) arr.push(ev)
      else map.set(key, [ev])
    }
  }

  for (const [, arr] of map) {
    arr.sort((a, b) => a.start.getTime() - b.start.getTime())
  }

  return map
}

function expandEventToDayKeys(ev: NormalizedEvent): string[] {
  const keys: string[] = []
  const start = new Date(ev.start)
  const end = new Date(ev.end)

  if (ev.allDay) {
    let d = new Date(start.getFullYear(), start.getMonth(), start.getDate())
    const endTs = end.getTime()
    for (;;) {
      if (d.getTime() >= endTs) break
      keys.push(dayKey(d))
      d.setDate(d.getDate() + 1)
    }
    return keys.length ? keys : [dayKey(start)]
  }

  let d = new Date(start.getFullYear(), start.getMonth(), start.getDate())
  const last = new Date(end.getFullYear(), end.getMonth(), end.getDate())
  while (d <= last) {
    keys.push(dayKey(d))
    d.setDate(d.getDate() + 1)
  }
  return keys.length ? keys : [dayKey(start)]
}
