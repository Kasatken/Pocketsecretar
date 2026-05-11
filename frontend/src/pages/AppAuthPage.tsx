import { FormEvent, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'
import SiteFooter from '../components/SiteFooter'
import { APP_NAME, LOGO_URL } from '../lib/branding'
import s from './AppAuth.module.css'

export default function AppAuthPage() {
  const navigate = useNavigate()
  const { login, register } = useAuth()
  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [displayName, setDisplayName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [agreePrivacy, setAgreePrivacy] = useState(false)
  const [agreePersonalData, setAgreePersonalData] = useState(false)

  const consentsOk = agreePrivacy && agreePersonalData
  const submitDisabled = loading || !consentsOk

  const handleAuth = async (e: FormEvent) => {
    e.preventDefault()
    if (!consentsOk) return
    setError('')
    setLoading(true)
    try {
      if (mode === 'login') {
        await login(email, password)
      } else {
        await register(email, password, displayName)
      }
      navigate('/app', { replace: true })
    } catch (authError) {
      const message = authError instanceof Error ? authError.message : 'Ошибка авторизации'
      setError(message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className={s.wrap}>
      <div className={s.centerCol}>
        <div className={s.card}>
          <div className={s.brandRow}>
            <img src={LOGO_URL} alt={APP_NAME} className={s.brandLogo} width={140} height={44} />
          </div>
          <h1 className={s.title}>{APP_NAME}</h1>
          <p className={s.sub}>Вход и регистрация на pocketsecretary.ru</p>

          <div className={s.sep} />

          <p className={s.subLabel}>{mode === 'login' ? 'Вход' : 'Регистрация'}</p>
          <div className={s.modeTabs}>
            <button
              type="button"
              className={[s.tab, mode === 'login' ? s.tabActive : ''].join(' ')}
              onClick={() => setMode('login')}
            >
              Вход
            </button>
            <button
              type="button"
              className={[s.tab, mode === 'register' ? s.tabActive : ''].join(' ')}
              onClick={() => setMode('register')}
            >
              Регистрация
            </button>
          </div>

          <form className={s.authForm} onSubmit={handleAuth}>
            {mode === 'register' ? (
              <input
                type="text"
                className={s.emailInput}
                placeholder="Как к вам обращаться"
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                required={mode === 'register'}
                minLength={2}
                maxLength={80}
                autoComplete="nickname"
              />
            ) : null}
            <input
              type="email"
              className={s.emailInput}
              placeholder="email@example.com"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
              autoComplete="email"
            />
            <input
              type="password"
              className={s.emailInput}
              placeholder="Пароль (минимум 8 символов)"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              minLength={8}
              required
              autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
            />
            <div className={s.consents}>
              <label className={s.consentLabel}>
                <input
                  type="checkbox"
                  className={s.consentCheckbox}
                  checked={agreePrivacy}
                  onChange={(e) => setAgreePrivacy(e.target.checked)}
                />
                <span className={s.consentText}>
                  Согласен с{' '}
                  <Link className={s.consentLink} to="/privacy" target="_blank" rel="noreferrer">
                    политикой конфиденциальности
                  </Link>
                </span>
              </label>
              <label className={s.consentLabel}>
                <input
                  type="checkbox"
                  className={s.consentCheckbox}
                  checked={agreePersonalData}
                  onChange={(e) => setAgreePersonalData(e.target.checked)}
                />
                <span className={s.consentText}>
                  Согласен на{' '}
                  <Link className={s.consentLink} to="/personal-data" target="_blank" rel="noreferrer">
                    обработку персональных данных
                  </Link>
                </span>
              </label>
            </div>
            <button className={s.btn} type="submit" disabled={submitDisabled}>
              {loading ? 'Подождите...' : mode === 'login' ? 'Войти' : 'Создать аккаунт'}
            </button>
          </form>
          {error ? <p className={s.error}>{error}</p> : null}

          <Link className={s.back} to="/">
            ← На главную
          </Link>
        </div>

        <SiteFooter />
      </div>
    </div>
  )
}
