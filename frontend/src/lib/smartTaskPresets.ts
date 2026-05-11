export type SmartTaskPreset = {
  id: string
  label: string
  text: string
}

const STORAGE_KEY = 'pocketsecret-smart-task-presets-v1'

function isPreset(x: unknown): x is SmartTaskPreset {
  if (x == null || typeof x !== 'object') return false
  const o = x as Record<string, unknown>
  return (
    typeof o.id === 'string' &&
    typeof o.label === 'string' &&
    typeof o.text === 'string'
  )
}

export function loadSmartTaskPresets(): SmartTaskPreset[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return []
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.filter(isPreset)
  } catch {
    return []
  }
}

export function saveSmartTaskPresets(presets: SmartTaskPreset[]): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(presets))
  } catch {
    /* quota / private mode */
  }
}

export function newSmartPresetId(): string {
  return `p${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}`
}
