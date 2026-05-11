/** Значения `chronotype` в API и подписи для UI. */
export const CHRONOTYPE_OPTIONS = [
  { value: '', label: 'Не указывать' },
  { value: 'morning', label: 'Утро (до ~12)' },
  { value: 'midday', label: 'День (12–17)' },
  { value: 'evening', label: 'Вечер (17–22)' },
  { value: 'night', label: 'Ночь (после 22)' },
  { value: 'balanced', label: 'Равномерно по дню' },
] as const

export function chronotypeLabel(value: string | undefined | null): string {
  const v = (value ?? '').trim().toLowerCase()
  if (!v) return ''
  const row = CHRONOTYPE_OPTIONS.find((o) => o.value === v)
  return row?.label ?? ''
}
