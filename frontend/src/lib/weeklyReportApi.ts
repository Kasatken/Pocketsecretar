export type DayCompletion = {
  weekday: string // "пн" | "вт" | ...
  count: number
}

export type UpcomingEvent = {
  id: string
  title: string
  start: string  // ISO 8601
  end: string    // ISO 8601
  all_day: boolean
}

export type WeeklyReport = {
  week_label: string
  completed_this_week: number
  completed_last_week: number
  delta_pct: number | null
  smart_tasks_this_week: number
  streak: number
  best_day: { weekday: string; count: number }
  completions_by_day: DayCompletion[]
  upcoming_events: UpcomingEvent[] | null
  ai_narrative: string
  google_linked: boolean
}

export async function fetchWeeklyReport(accessToken: string): Promise<WeeklyReport> {
  const res = await fetch('/api/reports/weekly', {
    headers: { Authorization: `Bearer ${accessToken}` },
    credentials: 'include',
  })
  if (!res.ok) {
    throw new Error(`${res.status}`)
  }
  return (await res.json()) as WeeklyReport
}

/** Форматирует ISO-дату события в короткий русский вид: "пн, 16 июн, 10:00" */
export function formatEventStart(iso: string): string {
  const d = new Date(iso)
  if (isNaN(d.getTime())) return iso
  return d.toLocaleString('ru-RU', {
    weekday: 'short',
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
    timeZone: 'Europe/Moscow',
  })
}
