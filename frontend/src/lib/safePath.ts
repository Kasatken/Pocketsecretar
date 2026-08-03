/** Внутренний путь для возврата после входа (только свои адреса, без внешних URL). */
export function safeInternalPath(p: unknown): string | null {
  if (typeof p !== 'string') return null
  if (!p.startsWith('/') || p.startsWith('//')) return null
  return p
}
