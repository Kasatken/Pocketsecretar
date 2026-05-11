import { Routes, Route, Navigate, Outlet } from 'react-router-dom'
import { useAuth } from './auth/AuthContext'
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
  if (!ready) return <AppSessionLoading label="Загрузка сессии…" />
  if (!user) return <Navigate to="/login" replace />
  return <Outlet />
}

function LoginRoute() {
  const { user, ready } = useAuth()
  if (!ready) return <AppSessionLoading label="Загрузка…" />
  if (user) return <Navigate to="/app" replace />
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
