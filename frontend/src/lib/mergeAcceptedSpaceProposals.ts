import type { NormalizedEvent, SpaceProposalOnEvent } from './calendarUtils'
import type { AcceptedProposalCalendarOverlay } from './spacesApi'

/** Варианты id одного и того же события Google для сопоставления с `google_event_id` в БД. */
function googleEventIdLookupKeys(raw: string): string[] {
  const s = raw.trim()
  if (!s) return []
  const out: string[] = [s]
  const base = s.replace(/@google\.com$/i, '')
  if (base && base !== s) out.push(base)
  if (base && !s.toLowerCase().endsWith('@google.com')) out.push(`${base}@google.com`)
  return [...new Set(out)]
}

function normTitle(t: string): string {
  return t.trim().toLowerCase().replace(/\s+/g, ' ')
}

function timesCloseMs(a: Date, b: Date, tolMs: number): boolean {
  return Math.abs(a.getTime() - b.getTime()) <= tolMs
}

function overlayToMeta(o: AcceptedProposalCalendarOverlay, calendarSynced: boolean): SpaceProposalOnEvent {
  return {
    proposalId: o.proposal_id,
    spaceId: o.space_id,
    spaceName: o.space_name,
    authorDisplayName: o.author_display_name,
    authorAvatarUrl: o.author_avatar_url,
    calendarSynced,
  }
}

/**
 * Подмешивает принятые предложения из пространств в список событий календаря:
 * 1) `space_proposal_id` из Google extendedProperties (новые события после принятия);
 * 2) совпадение `google_event_id` с id события;
 * 3) эвристика: то же название и близкое начало/конец (старые события без метки);
 * 4) отдельные слоты, если в календаре нет пары.
 */
export function mergeAcceptedSpaceProposalsIntoEvents(
  events: NormalizedEvent[],
  overlays: AcceptedProposalCalendarOverlay[],
): NormalizedEvent[] {
  const next = events.map((e) => ({ ...e }))
  const used = new Set<number>()

  const byProposalId = new Map<number, AcceptedProposalCalendarOverlay>()
  const byGid = new Map<string, AcceptedProposalCalendarOverlay>()
  for (const o of overlays) {
    byProposalId.set(o.proposal_id, o)
    const gid = (o.google_event_id || '').trim()
    if (gid) {
      for (const k of googleEventIdLookupKeys(gid)) {
        byGid.set(k, o)
      }
    }
  }

  for (const ev of next) {
    if (ev.spaceProposal) continue
    const sid = ev.spaceProposalId
    if (sid != null && sid > 0) {
      const o = byProposalId.get(sid)
      if (o && !used.has(o.proposal_id)) {
        ev.spaceProposal = overlayToMeta(o, Boolean((o.google_event_id || '').trim()))
        used.add(o.proposal_id)
      }
    }
  }

  for (const ev of next) {
    if (ev.spaceProposal) continue
    let o: AcceptedProposalCalendarOverlay | undefined
    for (const k of googleEventIdLookupKeys(ev.id)) {
      o = byGid.get(k)
      if (o) break
    }
    if (!o || used.has(o.proposal_id)) continue
    ev.spaceProposal = overlayToMeta(o, true)
    used.add(o.proposal_id)
  }

  const tol = 240_000
  for (const ev of next) {
    if (ev.spaceProposal) continue
    for (const o of overlays) {
      if (used.has(o.proposal_id)) continue
      if (!o.proposed_start || !o.proposed_end) continue
      const ps = new Date(o.proposed_start)
      const pe = new Date(o.proposed_end)
      if (normTitle(ev.title) !== normTitle(o.title)) continue
      if (!timesCloseMs(ev.start, ps, tol) || !timesCloseMs(ev.end, pe, tol)) continue
      ev.spaceProposal = overlayToMeta(o, Boolean((o.google_event_id || '').trim()))
      used.add(o.proposal_id)
      break
    }
  }

  const extra: NormalizedEvent[] = []
  for (const o of overlays) {
    if (used.has(o.proposal_id)) continue
    extra.push({
      id: `space-proposal-${o.proposal_id}`,
      title: o.title,
      start: new Date(o.proposed_start),
      end: new Date(o.proposed_end),
      allDay: false,
      spaceProposalId: o.proposal_id,
      spaceProposal: overlayToMeta(o, false),
    })
  }
  return [...next, ...extra]
}

export function isSpaceProposalGhostEvent(ev: NormalizedEvent): boolean {
  return Boolean(ev.spaceProposal && ev.id.startsWith('space-proposal-'))
}
