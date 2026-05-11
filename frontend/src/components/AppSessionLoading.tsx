import s from './AppSessionLoading.module.css'

export default function AppSessionLoading({
  label = 'Загрузка…',
}: {
  label?: string
}) {
  return (
    <div className={s.wrap} role="status" aria-live="polite">
      <div className={s.spinner} aria-hidden />
      <p className={s.label}>{label}</p>
    </div>
  )
}
