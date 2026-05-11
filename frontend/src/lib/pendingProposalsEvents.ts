/** Событие для обновления бейджа / полоски ожидающих предложений (например после ответа на предложение). */
export const PENDING_PROPOSALS_CHANGED = 'pocketsecret:pending-proposals-changed'

export function notifyPendingProposalsChanged(): void {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent(PENDING_PROPOSALS_CHANGED))
}
