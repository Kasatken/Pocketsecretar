import { Component, type ErrorInfo, type ReactNode } from 'react'
import s from './ErrorBoundary.module.css'

type Props = { children: ReactNode }
type State = { hasError: boolean }

export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false }

  static getDerivedStateFromError(): State {
    return { hasError: true }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('Необработанная ошибка рендера:', error, info.componentStack)
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className={s.wrap} role="alert">
          <p className={s.title}>Что-то пошло не так</p>
          <p className={s.text}>
            Попробуйте обновить страницу. Если не поможет — очистите кеш браузера.
          </p>
          <button type="button" className={s.button} onClick={() => window.location.reload()}>
            Обновить страницу
          </button>
        </div>
      )
    }
    return this.props.children
  }
}
