import { Link } from 'react-router-dom'
import { Heart } from 'lucide-react'
import s from './SiteFooter.module.css'

type Props = {
  /** Полная строка с доменом и подписью — как на лендинге */
  variant?: 'app' | 'marketing'
}

export default function SiteFooter({ variant = 'app' }: Props) {
  return (
    <footer className={[s.footer, variant === 'marketing' ? s.marketing : ''].join(' ')}>
      <nav className={s.legal} aria-label="Правовая информация">
        <Link className={s.link} to="/privacy">
          Политика конфиденциальности
        </Link>
        <span className={s.dot} aria-hidden>
          ·
        </span>
        <Link className={s.link} to="/personal-data">
          Согласие на обработку ПДн
        </Link>
      </nav>
      {variant === 'marketing' ? (
        <>
          <span className={s.dot} aria-hidden>
            ·
          </span>
          <a className={s.link} href="https://pocketsecretary.ru">
            pocketsecretary.ru
          </a>
          <span className={s.dot} aria-hidden>
            ·
          </span>
          <span className={s.tagline}>
            Разрабатывается с <Heart size={12} strokeWidth={2.5} className={s.heart} aria-hidden /> в Москве
          </span>
        </>
      ) : null}
    </footer>
  )
}
