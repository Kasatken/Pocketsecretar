import c from './SpaceProposalCalendarBadge.module.css'

export function authorInitials(name: string): string {
  const n = name.trim()
  if (!n) return '?'
  const p = n.split(/\s+/).filter(Boolean)
  if (p.length >= 2) {
    const a = p[0][0] ?? ''
    const b = p[1][0] ?? ''
    return (a + b).toUpperCase().slice(0, 2)
  }
  return n.slice(0, 2).toUpperCase()
}

type Props = {
  displayName: string
  avatarUrl: string
  spaceName?: string
  size?: 'sm' | 'md'
}

export function SpaceProposalCalendarBadge({ displayName, avatarUrl, spaceName, size = 'sm' }: Props) {
  const url = (avatarUrl || '').trim()
  const title = spaceName ? `${displayName} · ${spaceName}` : displayName
  return (
    <span
      className={[c.wrap, size === 'md' ? c.md : c.sm].join(' ')}
      title={title}
      aria-label={`Предложено: ${title}`}
    >
      {url ? (
        <img src={url} alt="" className={c.img} />
      ) : (
        <span className={c.initials} aria-hidden>
          {authorInitials(displayName)}
        </span>
      )}
    </span>
  )
}
