export type Space = {
  id: number
  name: string
  space_type: string
  policy: string
  created_by: number
  created_at: string
  updated_at: string
  my_role: string
  my_share: string
}

export type SpaceMember = {
  user_id: number
  display_name: string
  email: string
  role: string
  share_access: string
}

export type TaskProposal = {
  id: number
  space_id: number
  author_id: number
  assignee_id: number
  title: string
  description: string
  proposed_start?: string
  proposed_end?: string
  payload: Record<string, unknown>
  status: string
  assignee_note: string
  google_event_id: string
  created_at: string
  updated_at: string
}

/** Ответ GET /api/spaces/calendar-overlays — только для текущего пользователя как исполнителя. */
export type AcceptedProposalCalendarOverlay = {
  proposal_id: number
  space_id: number
  space_name: string
  author_id: number
  author_display_name: string
  author_avatar_url: string
  title: string
  proposed_start: string
  proposed_end: string
  google_event_id: string
}

/** Строка GET /api/spaces/pending-for-me — пространство с ожидающими для вас предложениями. */
export type PendingProposalSpaceRow = {
  space_id: number
  space_name: string
  pending_count: number
  latest_title: string
  latest_created: string
}

async function parseJson<T>(res: Response): Promise<T> {
  const data = (await res.json().catch(() => ({}))) as T & { error?: string }
  if (!res.ok) {
    const msg = (data as { error?: string }).error || res.statusText || 'Ошибка запроса'
    throw new Error(msg)
  }
  return data
}

export async function fetchSpaces(accessToken: string): Promise<Space[]> {
  const res = await fetch('/api/spaces', {
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  const data = await parseJson<{ spaces?: Space[] }>(res)
  return data.spaces ?? []
}

export async function createSpace(
  accessToken: string,
  body: { name: string; space_type?: string; policy?: string },
): Promise<Space> {
  const res = await fetch('/api/spaces', {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${accessToken}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(body),
  })
  const data = await parseJson<{ space?: Space }>(res)
  if (!data.space) throw new Error('Нет данных пространства')
  return data.space
}

export async function fetchSpace(accessToken: string, id: number): Promise<Space> {
  const res = await fetch(`/api/spaces/${encodeURIComponent(String(id))}`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  const data = await parseJson<{ space?: Space }>(res)
  if (!data.space) throw new Error('Нет данных пространства')
  return data.space
}

export async function deleteSpace(accessToken: string, id: number): Promise<void> {
  const res = await fetch(`/api/spaces/${encodeURIComponent(String(id))}`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  if (!res.ok) {
    const data = (await res.json().catch(() => ({}))) as { error?: string }
    const msg = data.error || res.statusText || 'Ошибка запроса'
    throw new Error(msg)
  }
}

export async function fetchSpaceMembers(accessToken: string, spaceId: number): Promise<SpaceMember[]> {
  const res = await fetch(`/api/spaces/${encodeURIComponent(String(spaceId))}/members`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  const data = await parseJson<{ members?: SpaceMember[] }>(res)
  return data.members ?? []
}

export async function createSpaceInvite(
  accessToken: string,
  spaceId: number,
  body: { invited_email?: string; role?: string; share_access?: string; expires_days?: number },
): Promise<{ token: string; expires_at: string }> {
  const res = await fetch(`/api/spaces/${encodeURIComponent(String(spaceId))}/invites`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${accessToken}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(body),
  })
  return parseJson(res)
}

export async function joinSpaceWithToken(accessToken: string, token: string): Promise<Space> {
  const res = await fetch('/api/spaces/join', {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${accessToken}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ token }),
  })
  const data = await parseJson<{ space?: Space }>(res)
  if (!data.space) throw new Error('Не удалось вступить в пространство')
  return data.space
}

export async function fetchAcceptedCalendarOverlays(
  accessToken: string,
  timeMin: Date,
  timeMax: Date,
): Promise<AcceptedProposalCalendarOverlay[]> {
  const params = new URLSearchParams({
    time_min: timeMin.toISOString(),
    time_max: timeMax.toISOString(),
  })
  const res = await fetch(`/api/spaces/calendar-overlays?${params}`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  const data = await parseJson<{ overlays?: AcceptedProposalCalendarOverlay[] }>(res)
  return data.overlays ?? []
}

export async function fetchPendingProposalsForMe(accessToken: string): Promise<{
  total: number
  spaces: PendingProposalSpaceRow[]
}> {
  const res = await fetch('/api/spaces/pending-for-me', {
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  const data = await parseJson<{ total?: number; spaces?: PendingProposalSpaceRow[] }>(res)
  return {
    total: typeof data.total === 'number' ? data.total : 0,
    spaces: data.spaces ?? [],
  }
}

export async function fetchProposals(accessToken: string, spaceId: number): Promise<TaskProposal[]> {
  const res = await fetch(`/api/spaces/${encodeURIComponent(String(spaceId))}/proposals`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  const data = await parseJson<{ proposals?: TaskProposal[] }>(res)
  return data.proposals ?? []
}

export async function createProposal(
  accessToken: string,
  spaceId: number,
  body: {
    assignee_id: number
    title: string
    description?: string
    proposed_start?: string
    proposed_end?: string
    payload?: Record<string, unknown>
  },
): Promise<TaskProposal> {
  const res = await fetch(`/api/spaces/${encodeURIComponent(String(spaceId))}/proposals`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${accessToken}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(body),
  })
  const data = await parseJson<{ proposal?: TaskProposal }>(res)
  if (!data.proposal) throw new Error('Нет данных предложения')
  return data.proposal
}

export async function respondToProposal(
  accessToken: string,
  spaceId: number,
  proposalId: number,
  body: { status: 'accepted' | 'declined'; assignee_note?: string },
): Promise<{ ok: boolean; status: string; proposal?: TaskProposal; google_event_id?: string; calendar_warning?: string }> {
  const res = await fetch(
    `/api/spaces/${encodeURIComponent(String(spaceId))}/proposals/${encodeURIComponent(String(proposalId))}/respond`,
    {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${accessToken}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(body),
    },
  )
  return parseJson(res)
}
