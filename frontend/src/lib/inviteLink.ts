/** Токен приглашения с бэкенда: 32 байта в hex = 64 символа. */
const TOKEN_HEX_64 = /^[0-9a-f]{64}$/i

const PARAM_KEYS = ['token', 'invite', 'join', 't', 'space_invite', 'spaceInvite']

function tryDecodeParam(value: string | null): string | null {
  if (!value) return null
  const v = value.trim()
  if (TOKEN_HEX_64.test(v)) return v.toLowerCase()
  return null
}

/**
 * Из поля ввода: голый токен, либо URL с параметром (?token= / ?invite= / #token= и т.д.),
 * либо строка, где где-то встречается 64 hex подряд.
 */
export function extractInviteToken(raw: string): string {
  const s = raw.trim()
  if (!s) return ''

  if (TOKEN_HEX_64.test(s)) return s.toLowerCase()

  let forUrl = s
  if (!/^https?:\/\//i.test(s)) {
    if (/^[a-z0-9.-]+\?/i.test(s) || /[./#]/.test(s) || /\//.test(s)) {
      forUrl = `https://${s}`
    }
  }

  try {
    const u = new URL(forUrl)
    for (const key of PARAM_KEYS) {
      const decoded = tryDecodeParam(u.searchParams.get(key))
      if (decoded) return decoded
    }
    const h = u.hash.replace(/^#/, '')
    if (h.includes('=')) {
      const hp = new URLSearchParams(h.startsWith('?') ? h : `?${h}`)
      for (const key of PARAM_KEYS) {
        const decoded = tryDecodeParam(hp.get(key))
        if (decoded) return decoded
      }
    }
    const parts = u.pathname.split('/').filter(Boolean)
    const last = parts[parts.length - 1]
    if (last && TOKEN_HEX_64.test(last)) return last.toLowerCase()
  } catch {
    /* не URL */
  }

  const embedded = s.match(/[0-9a-f]{64}/i)
  if (embedded) return embedded[0].toLowerCase()

  return s
}
