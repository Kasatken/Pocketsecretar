export type TimezoneOption = { value: string; label: string }

/** Частые пояса аудитории; любой другой IANA-идентификатор тоже валиден — селект добавит его отдельным пунктом. */
export const TIMEZONE_OPTIONS: TimezoneOption[] = [
  { value: 'Europe/Kaliningrad', label: 'Калининград (UTC+2)' },
  { value: 'Europe/Moscow', label: 'Москва, Санкт-Петербург (UTC+3)' },
  { value: 'Europe/Samara', label: 'Самара (UTC+4)' },
  { value: 'Asia/Yekaterinburg', label: 'Екатеринбург (UTC+5)' },
  { value: 'Asia/Omsk', label: 'Омск (UTC+6)' },
  { value: 'Asia/Krasnoyarsk', label: 'Красноярск, Новосибирск (UTC+7)' },
  { value: 'Asia/Irkutsk', label: 'Иркутск (UTC+8)' },
  { value: 'Asia/Yakutsk', label: 'Якутск (UTC+9)' },
  { value: 'Asia/Vladivostok', label: 'Владивосток (UTC+10)' },
  { value: 'Asia/Magadan', label: 'Магадан (UTC+11)' },
  { value: 'Asia/Kamchatka', label: 'Петропавловск-Камчатский (UTC+12)' },
  { value: 'Europe/Minsk', label: 'Минск (UTC+3)' },
  { value: 'Asia/Almaty', label: 'Алматы (UTC+5)' },
  { value: 'Asia/Tashkent', label: 'Ташкент (UTC+5)' },
  { value: 'Asia/Bishkek', label: 'Бишкек (UTC+6)' },
  { value: 'Asia/Yerevan', label: 'Ереван (UTC+4)' },
  { value: 'Asia/Tbilisi', label: 'Тбилиси (UTC+4)' },
  { value: 'Asia/Baku', label: 'Баку (UTC+4)' },
  { value: 'Asia/Dubai', label: 'Дубай (UTC+4)' },
  { value: 'Europe/Istanbul', label: 'Стамбул (UTC+3)' },
  { value: 'Europe/Belgrade', label: 'Белград (UTC+1/+2)' },
  { value: 'Europe/Berlin', label: 'Берлин (UTC+1/+2)' },
  { value: 'Europe/London', label: 'Лондон (UTC+0/+1)' },
  { value: 'America/New_York', label: 'Нью-Йорк (UTC−5/−4)' },
  { value: 'America/Los_Angeles', label: 'Лос-Анджелес (UTC−8/−7)' },
]

export const DEFAULT_TIMEZONE = 'Europe/Moscow'

/** IANA-таймзона устройства (пустая строка, если браузер её не отдал). */
export function detectBrowserTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone ?? ''
  } catch {
    return ''
  }
}

export function timezoneLabel(tz: string | undefined | null): string {
  const v = (tz ?? '').trim()
  if (!v) return ''
  return TIMEZONE_OPTIONS.find((o) => o.value === v)?.label ?? v
}
