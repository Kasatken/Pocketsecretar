import { useEffect, useId, useState } from 'react'
import { createPortal } from 'react-dom'
import { POMODORO_PRESETS, sanitizeSettings, type PomodoroSettings } from '../hooks/usePomodoro'
import s from './PomodoroSettingsModal.module.css'

type Props = {
  settings: PomodoroSettings
  onSave: (settings: PomodoroSettings) => void
  onClose: () => void
}

type DraftMinutes = {
  workMin: number
  shortBreakMin: number
  longBreakMin: number
  longBreakInterval: number
}

function toDraft(settings: PomodoroSettings): DraftMinutes {
  return {
    workMin: Math.round(settings.workSec / 60),
    shortBreakMin: Math.round(settings.shortBreakSec / 60),
    longBreakMin: Math.round(settings.longBreakSec / 60),
    longBreakInterval: settings.longBreakInterval,
  }
}

function toSettings(draft: DraftMinutes): PomodoroSettings {
  return sanitizeSettings({
    workSec: draft.workMin * 60,
    shortBreakSec: draft.shortBreakMin * 60,
    longBreakSec: draft.longBreakMin * 60,
    longBreakInterval: draft.longBreakInterval,
  })
}

function sameSettings(a: PomodoroSettings, b: PomodoroSettings): boolean {
  return (
    a.workSec === b.workSec &&
    a.shortBreakSec === b.shortBreakSec &&
    a.longBreakSec === b.longBreakSec &&
    a.longBreakInterval === b.longBreakInterval
  )
}

export function PomodoroSettingsModal({ settings, onSave, onClose }: Props) {
  const [draft, setDraft] = useState<DraftMinutes>(() => toDraft(settings))
  const titleId = useId()

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])

  const activePresetId = POMODORO_PRESETS.find((p) => sameSettings(p.settings, toSettings(draft)))?.id ?? null

  const handleSave = () => {
    onSave(toSettings(draft))
    onClose()
  }

  return createPortal(
    <div className={s.modalRoot}>
      <button type="button" className={s.modalBackdrop} aria-label="Закрыть" onClick={onClose} />
      <div
        className={s.modalPanel}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        lang="ru"
        onClick={(e) => e.stopPropagation()}
      >
        <header className={s.modalHeader}>
          <h2 id={titleId} className={s.modalTitle}>
            Настройки таймера
          </h2>
          <button type="button" className={s.modalClose} onClick={onClose} aria-label="Закрыть">
            ×
          </button>
        </header>

        <div className={s.body}>
          <div className={s.presetRow}>
            {POMODORO_PRESETS.map((preset) => (
              <button
                key={preset.id}
                type="button"
                className={`${s.presetBtn} ${activePresetId === preset.id ? s.presetBtnActive : ''}`}
                onClick={() => setDraft(toDraft(preset.settings))}
              >
                {preset.label}
              </button>
            ))}
          </div>

          <div className={s.fieldGrid}>
            <label className={s.field}>
              <span className={s.fieldLabel}>Работа, мин</span>
              <input
                type="number"
                min={1}
                max={180}
                className={s.fieldInput}
                value={draft.workMin}
                onChange={(e) => setDraft((d) => ({ ...d, workMin: Number(e.target.value) }))}
              />
            </label>
            <label className={s.field}>
              <span className={s.fieldLabel}>Короткий перерыв, мин</span>
              <input
                type="number"
                min={1}
                max={60}
                className={s.fieldInput}
                value={draft.shortBreakMin}
                onChange={(e) => setDraft((d) => ({ ...d, shortBreakMin: Number(e.target.value) }))}
              />
            </label>
            <label className={s.field}>
              <span className={s.fieldLabel}>Длинный перерыв, мин</span>
              <input
                type="number"
                min={1}
                max={120}
                className={s.fieldInput}
                value={draft.longBreakMin}
                onChange={(e) => setDraft((d) => ({ ...d, longBreakMin: Number(e.target.value) }))}
              />
            </label>
            <label className={s.field}>
              <span className={s.fieldLabel}>Длинный перерыв через, блоков</span>
              <input
                type="number"
                min={1}
                max={12}
                className={s.fieldInput}
                value={draft.longBreakInterval}
                onChange={(e) => setDraft((d) => ({ ...d, longBreakInterval: Number(e.target.value) }))}
              />
            </label>
          </div>

          <p className={s.hint}>
            Если сейчас идёт отсчёт рабочей фазы, новые длительности применятся начиная со следующей фазы.
          </p>
        </div>

        <div className={s.actions}>
          <button type="button" className={s.cancelBtn} onClick={onClose}>
            Отмена
          </button>
          <button type="button" className={s.saveBtn} onClick={handleSave}>
            Сохранить
          </button>
        </div>
      </div>
    </div>,
    document.body,
  )
}
