import { useCallback, useEffect, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'
import { APP_NAME, LOGO_URL } from '../lib/branding'
import s from './Landing.module.css'

const DECISION_CHOICES = [
  {
    key: 'A',
    title: 'Полистать Telegram «всего минут пять»',
    cost: '−42 минуты',
    kicker: 'Знакомый сценарий',
    result:
      '«Ок, понимаю. Но твой блок защиты диплома уже две недели не двигался, поставлю на завтра 09:00, без дискуссий. Просто чтобы ты не проснулся в панике.»',
  },
  {
    key: 'B',
    title: 'Открыть слайды и сделать один блок',
    cost: '+35 XP, +1 уровень',
    kicker: 'Тот, кого хвалят',
    result:
      '«Хороший ход. 25 минут, и всё. Потом честная пауза, не перерабатывай. Я дальше сам разложу остаток вечера.»',
  },
  {
    key: 'C',
    title: 'Сказать другу: «давай завтра» и лечь',
    cost: '+18 спокойствия',
    kicker: 'Тот, что бывает редко',
    result: '«Это не лень. Это режим. Завтрашний звонок я переставил на 19:30: у тебя будет окно. Спи.»',
  },
] as const

const HOW_STEPS = [
  {
    chip: '01 · знакомство',
    kicker: 'Знакомство',
    title: 'Две минуты о тебе',
    text: 'Цели, ритм сна, реальная загрузка и то, что бесит. Без анкеты на десять страниц: секретарю достаточно нескольких ответов, чтобы перестать предлагать тебе то, что ты не сделаешь.',
  },
  {
    chip: '02 · сборка',
    kicker: 'Сборка',
    title: 'День строится сам',
    text: 'Не ты складываешь календарь кубиками. Секретарь ставит окна, считает энергию и оставляет паузы там, где они нужны, а не там, где влезли.',
  },
  {
    chip: '03 · ведение',
    kicker: 'Ведение',
    title: 'Наставник не отпускает',
    text: 'Он помнит контекст, не давит моралью и доводит дело до «сделано», а не до «записано». Если ты слил блок, он не тычет, а перестраивает день.',
  },
  {
    chip: '04 · финиш',
    kicker: 'Финиш',
    title: 'Вечером: факт',
    text: 'Закрываешь день и видишь не «что осталось», а что реально сдвинулось. Это странно приятно: вместо вины, короткий итог.',
  },
] as const

const RPG_QUOTES = [
  '«Окей, следующий блок, 25 минут. Я не уйду, пока не закроешь.»',
  '«Хороший ход. Дальше, честная пауза, не перерабатывай.»',
  '«+35 опыта. Ты не на бумаге, ты в дне.»',
  '«Уровень. Завтра начнём чуть смелее.»',
] as const

const AUDIENCE = [
  {
    id: 'masha',
    photoSrc:
      'https://images.unsplash.com/photo-1573496359142-b8d87734a5a2?w=224&h=224&fit=crop&crop=faces&q=80',
    photoAlt: 'Портрет молодой женщины',
    name: 'Маша, 19',
    role: 'Студентка',
    pain: 'Три дедлайна и подработка, кажется, что не успеваю жить.',
    back: 'Утро: маршрут уже разложен, что сдать сейчас, где окно на работу и когда офлайн без чувства вины.',
  },
  {
    id: 'artem',
    photoSrc:
      'https://images.unsplash.com/photo-1560250097-0b93528c311a?w=224&h=224&fit=crop&crop=faces&q=80',
    photoAlt: 'Портрет молодого человека в деловом стиле',
    name: 'Артём, 27',
    role: 'Специалист',
    pain: 'Чужие созвоны съедают день, задачи копятся быстрее, чем ты их озвучиваешь.',
    back: 'Чужие просьбы становятся предложениями: принимаешь или отклоняешь. Твоё время больше не объявляется общим.',
  },
] as const

const CLOCK_TICK_ROTATIONS = Array.from({ length: 12 }, (_, i) => ({
  major: i % 3 === 0,
  deg: i * 30,
}))

function prefersReducedMotion(): boolean {
  return typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

export default function Landing() {
  const navigate = useNavigate()
  const { user, ready } = useAuth()
  const wrapRef = useRef<HTMLMainElement | null>(null)
  const howCardRefs = useRef<(HTMLElement | null)[]>([])
  const finalBlockRef = useRef<HTMLDivElement>(null)

  const [pickedDecision, setPickedDecision] = useState<number | null>(null)
  const [howActive, setHowActive] = useState(0)
  const [audienceFlipped, setAudienceFlipped] = useState<Record<string, boolean>>({})
  const [rpg, setRpg] = useState({ lvl: 4, xp: 0, max: 200 })
  const [rpgQuoteIdx, setRpgQuoteIdx] = useState(0)
  const [finalMinutes, setFinalMinutes] = useState(0)

  const goAuth = useCallback(() => {
    navigate('/login')
  }, [navigate])

  useEffect(() => {
    if (ready && user) {
      navigate('/app', { replace: true })
    }
  }, [navigate, user, ready])

  useEffect(() => {
    const root = wrapRef.current
    if (!root) return

    const revealAll = () => {
      root.querySelectorAll<HTMLElement>(`.${CSS.escape(s.reveal)}`).forEach((el) => {
        el.classList.add(s.revealed)
      })
    }

    if (prefersReducedMotion()) {
      revealAll()
      return
    }

    const io = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          if (!entry.isIntersecting) return
          entry.target.classList.add(s.revealed)
          io.unobserve(entry.target)
        })
      },
      { threshold: 0.08, rootMargin: '0px 0px -8% 0px' },
    )

    root.querySelectorAll<HTMLElement>(`.${CSS.escape(s.reveal)}`).forEach((el) => io.observe(el))
    return () => io.disconnect()
  }, [])

  useEffect(() => {
    if (prefersReducedMotion()) {
      setRpg((p: { lvl: number; xp: number; max: number }) => ({ ...p, xp: 142 }))
      return
    }
    const t = window.setTimeout(() => setRpg((p: { lvl: number; xp: number; max: number }) => ({ ...p, xp: 142 })), 700)
    return () => window.clearTimeout(t)
  }, [])

  useEffect(() => {
    const mq = window.matchMedia('(min-width: 880px)')
    const cards = howCardRefs.current.filter(Boolean) as HTMLElement[]

    const update = () => {
      if (!mq.matches) {
        setHowActive(0)
        return
      }
      const anchorY = window.innerHeight * 0.4
      let bestI = 0
      let bestD = Infinity
      cards.forEach((c, i) => {
        const r = c.getBoundingClientRect()
        const center = r.top + r.height / 2
        const d = Math.abs(center - anchorY)
        if (d < bestD) {
          bestD = d
          bestI = i
        }
      })
      setHowActive(bestI)
    }

    let raf = 0
    const onScroll = () => {
      if (raf) return
      raf = window.requestAnimationFrame(() => {
        raf = 0
        update()
      })
    }

    window.addEventListener('scroll', onScroll, { passive: true })
    window.addEventListener('resize', onScroll)
    mq.addEventListener('change', update)
    update()
    return () => {
      window.removeEventListener('scroll', onScroll)
      window.removeEventListener('resize', onScroll)
      mq.removeEventListener('change', update)
    }
  }, [])

  useEffect(() => {
    const el = finalBlockRef.current
    if (!el) return
    if (prefersReducedMotion()) {
      setFinalMinutes(24)
      return
    }
    let raf = 0
    const io = new IntersectionObserver(
      (entries) => {
        entries.forEach((e) => {
          if (!e.isIntersecting) return
          const start = performance.now()
          const duration = 1200
          const target = 24
          const tick = (now: number) => {
            const p = Math.min(1, (now - start) / duration)
            setFinalMinutes(Math.round(target * p))
            if (p < 1) raf = requestAnimationFrame(tick)
          }
          raf = requestAnimationFrame(tick)
          io.disconnect()
        })
      },
      { threshold: 0.35 },
    )
    io.observe(el)
    return () => {
      cancelAnimationFrame(raf)
      io.disconnect()
    }
  }, [])

  const onDecisionClick = (index: number) => {
    setPickedDecision((prev) => (prev === index ? null : index))
  }

  const onRpgFocus = () => {
    setRpg((p: { lvl: number; xp: number; max: number }) => {
      let nextXp = p.xp + 35
      let lvl = p.lvl
      let max = p.max
      while (nextXp >= max) {
        nextXp -= max
        lvl += 1
        max = Math.round(max * 1.18)
      }
      return { lvl, xp: nextXp, max }
    })
    setRpgQuoteIdx((i) => (i + 1) % RPG_QUOTES.length)
  }

  const toggleAudience = (id: string) => {
    setAudienceFlipped((prev) => ({ ...prev, [id]: !prev[id] }))
  }

  const stickyStep = HOW_STEPS[howActive] ?? HOW_STEPS[0]

  return (
    <div className={s.body}>
      <main ref={wrapRef} className={s.wrap}>
        <header className={[s.topbar, s.reveal].join(' ')}>
          <div className={s.logo}>
            <img src={LOGO_URL} alt="" className={s.logoImg} width={160} height={48} decoding="async" />
            <div className={s.logoText}>
              {APP_NAME}
              <small>pocketsecretary.ru</small>
            </div>
          </div>
          <div className={s.topbarRight}>
            <button type="button" className={[s.btnGhost, s.btnSmall].join(' ')} onClick={goAuth}>
              Кабинет
            </button>
          </div>
        </header>

        <section
          className={[s.hero, s.reveal].join(' ')}
          style={{ '--reveal-delay': '60ms' } as CSSProperties}
          aria-labelledby="hero-title"
        >
          <div>
            <span className={s.heroEyebrow}>Не список дел · Союзник в дне</span>
            <h1 className={s.h1} id="hero-title">
              Не догнать время.
              <br />
              Но день: <em>можно собрать</em>.
            </h1>
            <p className={s.heroLead}>
              «Карманный секретарь»: это не планировщик. Это спутник, который собирает твой день за тебя и не отпускает после «записал».
            </p>
            <p className={s.heroSub}>
              Ты приходишь, и вместо чистого экрана уже видишь, с чего начать. Чужие просьбы становятся предложениями, а не приказами. Вечер заканчивается фактом, а не виной.
            </p>
            <div className={s.heroCTA}>
              <button type="button" className={s.btn} onClick={goAuth}>
                Попробовать неделю бесплатно
              </button>
              <span className={s.heroMicro}>Без карты · без обязательств</span>
            </div>
          </div>

          <div className={s.clockWrap}>
            <div className={s.clock} aria-hidden>
              {CLOCK_TICK_ROTATIONS.map(({ major, deg }) => (
                <div
                  key={deg}
                  className={[s.clockTick, major ? s.clockTickMajor : ''].filter(Boolean).join(' ')}
                  style={{ transform: `translate(-50%, -100%) rotate(${deg}deg)` }}
                />
              ))}
              <div className={s.clockHandHour} />
              <div className={s.clockHandMin} />
              <div className={s.clockHandSec} />
              <div className={s.clockCap} />
            </div>
            <div className={s.clockStamp}>
              <small>сегодня</small>день в кармане
            </div>
            <div className={s.clockNote}>«Слайды к защите? Ставлю 45 минут в 15:00.»</div>
          </div>
        </section>

        <section className={s.reveal} aria-labelledby="dec-title">
          <div className={s.secHead}>
            <div className={s.secNum} aria-hidden>
              01
            </div>
            <div className={s.secMeta}>
              <span className={s.secKicker}>Один день, три развилки</span>
              <h2 className={s.secTitle} id="dec-title">
                Без секретаря ты <b>уже выбрал</b>, просто не заметил.
              </h2>
              <p className={s.secSub}>
                Это типичный вечер вторника, нажми на блоки с твоими решениями и посмотри, что тебе сказал бы секретарь на то, что ты выбрал.
              </p>
            </div>
          </div>

          <div
            className={[s.decisions, s.decisionsSpaced].join(' ')}
            role="radiogroup"
            aria-label="Развилка вечера"
          >
            {DECISION_CHOICES.map((c, i) => {
              const picked = pickedDecision === i
              const dim = pickedDecision !== null && pickedDecision !== i
              let cardClass = s.decisionCard
              if (picked) cardClass = s.decisionCardPicked
              else if (dim) cardClass = s.decisionCardDim

              return (
                <button
                  key={c.key}
                  type="button"
                  className={cardClass}
                  role="radio"
                  aria-checked={picked}
                  onClick={() => onDecisionClick(i)}
                >
                  <div className={s.dcBadge}>
                    <span>{c.kicker}</span>
                    <span className={s.dcKey}>{c.key}</span>
                  </div>
                  <h3 className={s.dcTitle}>{c.title}</h3>
                  <div className={s.dcResult}>{c.result}</div>
                  <div className={s.dcCost}>
                    <span>цена</span>
                    <span>{c.cost}</span>
                  </div>
                </button>
              )
            })}
          </div>

          <div className={s.decisionFooter}>
            <span>
              <b>Идея простая:</b> мы не запрещаем тебе листать ленту. Мы просто показываем, что ты уже выбрал, и спрашиваем, точно ли это сегодня было нужнее всего.
            </span>
          </div>
        </section>

        <section className={s.reveal} aria-labelledby="how-title">
          <div className={s.secHead}>
            <div className={s.secNum} aria-hidden>
              02
            </div>
            <div className={s.secMeta}>
              <span className={s.secKicker}>Как это работает</span>
              <h2 className={s.secTitle} id="how-title">
                Четыре кадра без слова «оптимизация».
              </h2>
            </div>
          </div>

          <div className={[s.howWrap, s.howSpaced].join(' ')}>
            <div className={s.howSticky}>
              <div className={s.howStickyInner}>
                <div className={s.howBigNum}>{String(howActive + 1).padStart(2, '0')}</div>
                <p className={s.howKicker}>
                  Шаг → <span>{stickyStep.kicker}</span>
                </p>
                <h3 className={s.howStickyTitle}>{stickyStep.title}</h3>
              </div>
            </div>
            <div className={s.howCards}>
              {HOW_STEPS.map((step, i) => (
                <article
                  key={step.chip}
                  ref={(el) => {
                    howCardRefs.current[i] = el
                  }}
                  className={howActive === i ? s.howCardActive : s.howCard}
                >
                  <span className={s.howChip}>{step.chip}</span>
                  <h3>{step.title}</h3>
                  <p>{step.text}</p>
                </article>
              ))}
            </div>
          </div>
        </section>

        <section className={s.reveal} aria-labelledby="rpg-title">
          <div className={s.secHead}>
            <div className={s.secNum} aria-hidden>
              03
            </div>
            <div className={s.secMeta}>
              <span className={s.secKicker}>День как партия, не таблица</span>
              <h2 className={s.secTitle} id="rpg-title">
                Первый планировщик, где задача, это <b>опыт</b>.
              </h2>
              <p className={s.secSub}>Закрой фокус-блок и увидишь, как растёт уровень дня.</p>
            </div>
          </div>

          <div className={[s.rpgMini, s.rpgSpaced].join(' ')}>
            <div className={s.rpgRow}>
              <div className={s.rpgRowTop}>
                <span>
                  УРОВЕНЬ <b>{rpg.lvl}</b>
                </span>
                <span>
                  {rpg.xp} / {rpg.max} XP
                </span>
              </div>
              <div className={s.rpgBar}>
                <div className={s.rpgBarFill} style={{ width: `${Math.min(100, (rpg.xp / rpg.max) * 100)}%` }} />
              </div>
            </div>
            <p className={s.rpgQuote}>{RPG_QUOTES[rpgQuoteIdx]}</p>
            <div className={s.rpgActions}>
              <button type="button" className={[s.btnGold, s.btnSmall].join(' ')} onClick={onRpgFocus}>
                Закрыть фокус-блок · +35 XP
              </button>
            </div>
          </div>
        </section>

        <section className={s.reveal} aria-labelledby="aud-title">
          <div className={s.secHead}>
            <div className={s.secNum} aria-hidden>
              04
            </div>
            <div className={s.secMeta}>
              <span className={s.secKicker}>Это для тебя, если</span>
              <h2 className={s.secTitle} id="aud-title">
                У тебя слишком много жизни и слишком мало <b>порядка</b>.
              </h2>
              <p className={s.secSub}>Переверни карточку и увидишь, как меняется день у такого же, как ты.</p>
            </div>
          </div>

          <div className={[s.audGrid, s.audSpaced].join(' ')}>
            {AUDIENCE.map((a) => (
              <div key={a.id} className={s.flipScene}>
                <button
                  type="button"
                  className={s.flipBtn}
                  aria-pressed={audienceFlipped[a.id]}
                  onClick={() => toggleAudience(a.id)}
                >
                  <div className={[s.flipInner, audienceFlipped[a.id] ? s.flipInnerFlipped : ''].filter(Boolean).join(' ')}>
                    <div className={s.flipFace}>
                      <img
                        className={s.flipAvatarImg}
                        src={a.photoSrc}
                        alt={a.photoAlt}
                        width={62}
                        height={62}
                        loading="lazy"
                        decoding="async"
                      />
                      <p className={s.flipName}>{a.name}</p>
                      <p className={s.flipRole}>{a.role}</p>
                      <p className={s.flipPain}>{a.pain}</p>
                      <span className={s.flipHint}>перевернуть</span>
                    </div>
                    <div className={s.flipBack}>
                      <p className={s.flipBackLabel}>как меняется день</p>
                      <p className={s.flipBackText}>{a.back}</p>
                      <span className={s.flipHint}>обратно</span>
                    </div>
                  </div>
                </button>
              </div>
            ))}
          </div>
        </section>

        <section
          ref={finalBlockRef}
          className={[s.finalCta, s.reveal].join(' ')}
          aria-labelledby="final-title"
        >
          <p className={s.finalEyebrow}>Без мотивационных плакатов</p>
          <h2 className={s.finalTitle} id="final-title">
            Попробуй <u>одну неделю</u>.
            <br />
            Не стало легче? Просто уйди.
          </h2>
          <div className={s.finalRow}>
            <button type="button" className={s.btn} onClick={goAuth}>
              Начать первую неделю
            </button>
          </div>
          <div className={s.finalSummary}>
            <span>
              Пока ты тут, в твоём кармане лежит{' '}
              <b>
                <span>{finalMinutes}</span> минут
              </b>
              . Это твой первый день вместе. Дальше больше.
            </span>
          </div>
        </section>

        <footer className={[s.footerRow, s.reveal].join(' ')}>
          <span>© {APP_NAME} · 2026</span>
          <span className={s.footerLinks}>
            <Link className={s.footerLink} to="/privacy">
              политика
            </Link>
            <span className={s.footerDot} aria-hidden>
              ·
            </span>
            <Link
              className={s.footerLink}
              to="/personal-data"
              title="Согласие на обработку персональных данных"
            >
              согласие
            </Link>
          </span>
        </footer>
      </main>
    </div>
  )
}
