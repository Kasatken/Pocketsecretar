import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { AuthProvider } from './auth/AuthContext'
import { LOGO_URL } from './lib/branding'
import { Router } from './router'
import './index.css'

{
  const link = document.createElement('link')
  link.rel = 'icon'
  link.type = 'image/webp'
  link.href = LOGO_URL
  document.head.appendChild(link)
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <AuthProvider>
      <BrowserRouter>
        <Router />
      </BrowserRouter>
    </AuthProvider>
  </StrictMode>,
)
