import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  /** Нет статики в `public/` — всё через `src/` и сборку */
  publicDir: false,
  server: {
    proxy: {
      // smart-task: OpenRouter до ~90 с + создание событий в Google — без увеличения таймаута
      // прокси/браузер часто рвёт соединение → Safari «Сетевое соединение потеряно», Chrome «Load failed».
      '/api': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
        proxyTimeout: 300_000,
        timeout: 300_000,
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
