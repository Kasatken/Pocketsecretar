import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'

export type User = {
  id: number
  email: string
  display_name: string
  avatar_url?: string
  bio?: string
  personalization_note?: string
  chronotype?: string
  timezone?: string
  experience_points: number
  level: number
  plan: string
  created_at?: string
  updated_at?: string
  last_login_at?: string | null
  smart_tasks_today?: {
    used: number
    limit: number
    resets_at: string
  }
}

export type ProfilePatch = Partial<
  Pick<User, 'display_name' | 'bio' | 'personalization_note' | 'chronotype' | 'timezone'>
>

type AuthPayload = { access_token: string; user: User }

type AuthContextValue = {
  user: User | null
  accessToken: string | null
  ready: boolean
  login: (email: string, password: string) => Promise<void>
  register: (email: string, password: string, displayName: string) => Promise<void>
  logout: () => Promise<void>
  /** Returns new access token on success, false on failure. */
  refreshSession: () => Promise<string | false>
  updateProfile: (patch: ProfilePatch) => Promise<void>
  uploadAvatar: (file: File) => Promise<void>
  /** Активация Pro по промокоду (сервер: PRO_PROMO_CODE). */
  redeemPromo: (code: string) => Promise<{ alreadyPro: boolean }>
}

const AuthContext = createContext<AuthContextValue | undefined>(undefined)

const API = '/api/auth'

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [accessToken, setAccessToken] = useState<string | null>(null)
  const [ready, setReady] = useState(false)

  useEffect(() => {
    refreshSession().finally(() => setReady(true))
  }, [])

  async function authCall(path: string, body: Record<string, string>) {
    const response = await fetch(`${API}/${path}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify(body),
    })
    if (!response.ok) {
      const message = await response.text()
      throw new Error(message || 'auth request failed')
    }
    const payload = (await response.json()) as AuthPayload
    setUser(payload.user)
    setAccessToken(payload.access_token)
  }

  async function login(email: string, password: string) {
    await authCall('login', { email, password })
  }

  async function register(email: string, password: string, displayName: string) {
    await authCall('register', {
      email,
      password,
      display_name: displayName.trim(),
    })
  }

  async function refreshSession(): Promise<string | false> {
    const response = await fetch(`${API}/refresh`, {
      method: 'POST',
      credentials: 'include',
    })
    if (!response.ok) {
      setUser(null)
      setAccessToken(null)
      return false
    }
    const payload = (await response.json()) as AuthPayload
    setUser(payload.user)
    setAccessToken(payload.access_token)
    return payload.access_token
  }

  async function updateProfile(patch: ProfilePatch) {
    const token = accessToken
    if (!token) throw new Error('not logged in')
    const response = await fetch(`${API}/me`, {
      method: 'PATCH',
      headers: {
        Authorization: `Bearer ${token}`,
        'Content-Type': 'application/json',
      },
      credentials: 'include',
      body: JSON.stringify(patch),
    })
    if (!response.ok) {
      const message = await response.text()
      throw new Error(message || 'failed to update profile')
    }
    const data = (await response.json()) as { user: User }
    setUser(data.user)
  }

  async function uploadAvatar(file: File) {
    const token = accessToken
    if (!token) throw new Error('not logged in')
    const fd = new FormData()
    fd.append('avatar', file)
    const response = await fetch(`${API}/me/avatar`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}` },
      credentials: 'include',
      body: fd,
    })
    if (!response.ok) {
      const message = await response.text()
      throw new Error(message || 'failed to upload avatar')
    }
    const data = (await response.json()) as { user: User }
    setUser(data.user)
  }

  const redeemPromo = useCallback(async (code: string) => {
    const token = accessToken
    if (!token) throw new Error('Войдите в аккаунт')
    const response = await fetch(`${API}/me/promo`, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${token}`,
        'Content-Type': 'application/json',
      },
      credentials: 'include',
      body: JSON.stringify({ code: code.trim() }),
    })
    const raw = await response.text()
    let parsed: { detail?: string; user?: User; already_pro?: boolean } = {}
    try {
      parsed = JSON.parse(raw) as typeof parsed
    } catch {
      /* не JSON */
    }
    if (response.ok) {
      if (parsed.user) setUser(parsed.user)
      return { alreadyPro: Boolean(parsed.already_pro) }
    }
    if (response.status === 429) {
      throw new Error('Слишком много попыток. Подождите минуту.')
    }
    const detail = typeof parsed.detail === 'string' ? parsed.detail : ''
    throw new Error(detail || raw || 'Не удалось активировать промокод')
  }, [accessToken])

  async function logout() {
    await fetch(`${API}/logout`, { method: 'POST', credentials: 'include' })
    setUser(null)
    setAccessToken(null)
  }

  const value = useMemo(
    () => ({
      user,
      accessToken,
      ready,
      login,
      register,
      logout,
      refreshSession,
      updateProfile,
      uploadAvatar,
      redeemPromo,
    }),
    [user, accessToken, ready, redeemPromo],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuth must be used within AuthProvider')
  }
  return ctx
}
