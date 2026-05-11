import type { CSSProperties } from 'react'

/**
 * Градиент активных сегментов «Ход дня» — по локальному времени (рассвет / день / закат / ночь).
 *
 * Проверка цветов:
 * — В URL: `?skyHour=6` (рассвет), `12` (день), `19` (закат); дробный час: `?skyHour=19.5`
 * — В консоли: `sessionStorage.setItem('skyHour','7'); location.reload()` → сброс: `removeItem('skyHour')`
 */

type SkyStop = { t: number; start: string; end: string }

/** Ключевые моменты суток (час от полуночи, можно дробный). */
const SKY_STOPS: SkyStop[] = [
  { t: 0, start: '#5c6478', end: '#3d4458' },
  { t: 4.5, start: '#4a5168', end: '#323852' },
  { t: 5.5, start: '#ffc9b8', end: '#ff8b5c' },
  { t: 7, start: '#ffe8c8', end: '#fd9a76' },
  { t: 9, start: '#c5e8ff', end: '#5eb3f5' },
  { t: 13, start: '#9ee5ff', end: '#3d9adf' },
  /** До 17:00 — дневной лазурь; раньше переход к «закату» начинался уже в 16:00 и к 17:00 давал бежевую «грязь». */
  { t: 16, start: '#b8dcfa', end: '#6bb5ff' },
  { t: 17, start: '#9ed4fa', end: '#3d94ea' },
  /** ~17:00–17:45: остаёмся в «чистом» лазури; прямой lerp к персику давал серо-бежевую грязь к 17:30. */
  { t: 17.45, start: '#8ec8f6', end: '#3d94ea' },
  { t: 17.65, start: '#c8e6ff', end: '#7ab8f5' },
  { t: 17.88, start: '#ffd8a8', end: '#ff9a6c' },
  { t: 19, start: '#ffb199', end: '#ff5e62' },
  { t: 20.5, start: '#c9b6ff', end: '#7c6bcf' },
  { t: 22.5, start: '#5c6b9a', end: '#3a4260' },
  { t: 24, start: '#5c6478', end: '#3d4458' },
]

function hexToRgb(hex: string): { r: number; g: number; b: number } {
  const n = hex.replace('#', '').slice(0, 6)
  const v = Number.parseInt(n, 16)
  return { r: (v >> 16) & 255, g: (v >> 8) & 255, b: v & 255 }
}

function lerpChannel(a: number, b: number, t: number): number {
  return Math.round(a + (b - a) * t)
}

function lerpHex(a: string, b: string, t: number): string {
  const A = hexToRgb(a)
  const B = hexToRgb(b)
  const r = lerpChannel(A.r, B.r, t)
  const g = lerpChannel(A.g, B.g, t)
  const bl = lerpChannel(A.b, B.b, t)
  return `#${[r, g, bl].map((x) => x.toString(16).padStart(2, '0')).join('')}`
}

function decimalHourLocal(d: Date): number {
  return d.getHours() + d.getMinutes() / 60 + d.getSeconds() / 3600
}

function normalizeHour(n: number): number {
  let h = n % 24
  if (h < 0) h += 24
  return h
}

/** Для отладки: `?skyHour=…` или sessionStorage `skyHour`, иначе реальное локальное время. */
export function getEffectiveSkyHour(now: Date): number {
  if (typeof window === 'undefined') return decimalHourLocal(now)

  try {
    const params = new URLSearchParams(window.location.search)
    const q = params.get('skyHour')
    if (q != null && q !== '') {
      const n = Number(q.replace(',', '.'))
      if (!Number.isNaN(n)) return normalizeHour(n)
    }
  } catch {
    /* ignore */
  }

  try {
    const s = sessionStorage.getItem('skyHour')
    if (s != null && s !== '') {
      const n = Number(s.replace(',', '.'))
      if (!Number.isNaN(n)) return normalizeHour(n)
    }
  } catch {
    /* private mode */
  }

  return decimalHourLocal(now)
}

function skyColorsAtHour(hour: number): { start: string; end: string } {
  let h = hour % 24
  if (h < 0) h += 24

  for (let i = 0; i < SKY_STOPS.length - 1; i++) {
    const a = SKY_STOPS[i]
    const b = SKY_STOPS[i + 1]
    const lastSegment = i === SKY_STOPS.length - 2
    const inRange = lastSegment ? h >= a.t && h <= b.t : h >= a.t && h < b.t
    if (inRange) {
      const span = b.t - a.t
      const t = span <= 0 ? 0 : (h - a.t) / span
      return {
        start: lerpHex(a.start, b.start, t),
        end: lerpHex(a.end, b.end, t),
      }
    }
  }

  const last = SKY_STOPS[SKY_STOPS.length - 1]
  return { start: last.start, end: last.end }
}

/** Inline-стили с CSS-переменными для родителя шкалы (например `section.dayGauge`). */
export function dayGaugeSkyCssVars(d: Date): CSSProperties {
  const { start, end } = skyColorsAtHour(getEffectiveSkyHour(d))
  return {
    '--gauge-on-start': start,
    '--gauge-on-end': end,
  } as CSSProperties
}

/** Утилита для консоли / тестов: какие цвета при данном часе (0–24). */
export function previewSkyGradientAtHour(hour: number): { start: string; end: string } {
  return skyColorsAtHour(normalizeHour(hour))
}
