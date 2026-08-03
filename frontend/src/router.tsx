import { Routes, Route, Navigate, Outlet, useLocation } from 'react-router-dom'
import { useAuth } from './auth/AuthContext'
import { safeInternalPath } from './lib/safePath'
import AppLayout from './layout/AppLayout'
import Landing from './pages/Landing'
import AppPage from './pages/AppPage'
import CalendarPage from './pages/CalendarPage'
import ProfilePage from './pages/ProfilePage'
import SpacesPage from './pages/SpacesPage'
import SpacePage from './pages/SpacePage'
import AppAuthPage from './pages/AppAuthPage'
import PrivacyPage from './pages/PrivacyPage'
import PersonalDataPage from './pages/PersonalDataPage'
import AppSessionLoading from './components/AppSessionLoading'

function RequireAuth() {
  const { user, ready } = useAuth()
  const location = useLocation()
  if (!ready) return <AppSessionLoading label="Загрузка сессии…" />
  if (!user) {
    // Сохраняем адрес с query (например /spaces?token=…), чтобы после входа
    // вернуть человека туда, куда он шёл — иначе ссылки-приглашения теряются.
    return <Navigate to="/login" replace state={{ from: location.pathname + location.search }} />
  }
  return <Outlet />
}

function LoginRoute() {
  const { user, ready } = useAuth()
  const location = useLocation()
  if (!ready) return <AppSessionLoading label="Загрузка…" />
  if (user) {
    const from = safeInternalPath((location.state as { from?: string } | null)?.from)
    return <Navigate to={from ?? '/app'} replace />
  }
  return <AppAuthPage />
}

export function Router() {
  return (
    <Routes>
      <Route path="/" element={<Landing />} />
      <Route path="/login" element={<LoginRoute />} />
      <Route path="/privacy" element={<PrivacyPage />} />
      <Route path="/personal-data" element={<PersonalDataPage />} />
      <Route path="/app/profile" element={<Navigate to="/profile" replace />} />
      <Route element={<RequireAuth />}>
        <Route path="/app/calendar" element={<Navigate to="/calendar" replace />} />
        <Route element={<AppLayout />}>
          <Route path="/app" element={<AppPage />} />
          <Route path="/calendar" element={<CalendarPage />} />
          <Route path="/spaces" element={<SpacesPage />} />
          <Route path="/spaces/:id" element={<SpacePage />} />
          <Route path="/profile" element={<ProfilePage />} />
        </Route>
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
