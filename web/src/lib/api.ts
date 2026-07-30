export interface RoomState {
  match_id: string
  player1_id: string
  player2_id: string
  problem_id: string
  problem_title: string
  problem_description: string
  testcases: unknown
  status: 'active' | 'finished'
  started_at: string
}

export class RoomNotFoundError extends Error {
  constructor() {
    super('room not found')
  }
}

export async function fetchRoom(matchId: string): Promise<RoomState> {
  const res = await fetch(`/api/room/${encodeURIComponent(matchId)}`)
  if (res.status === 404) throw new RoomNotFoundError()
  if (!res.ok) throw new Error(`fetch room: ${res.status}`)
  return res.json()
}

export function queueSocketURL(playerId: string): string {
  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
  return `${proto}://${window.location.host}/queue?player_id=${encodeURIComponent(playerId)}`
}

// ---- room session (editor + judge) ---------------------------------------
//
// Mirrors sessionClientMsg / sessionServerMsg in server/internal/handlers/session.go.
// Keep these two files in sync by hand — there's no shared schema.

export type ClientSessionMsg = { type: 'submit'; code: string; language: string }

export type ServerSessionMsg =
  | { type: 'judging' }
  | { type: 'result'; passed: number; total: number; failed: string[] }
  | { type: 'opponent_result'; passed: number; total: number }
  | { type: 'error'; message: string }

// Goes through the /api proxy (see web/vite.config.ts — its entry needs
// `ws: true` for the upgrade to pass through), not straight to /room/...
// like the queue socket does.
export function roomSessionURL(matchId: string, playerId: string): string {
  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
  return `${proto}://${window.location.host}/api/room/${encodeURIComponent(matchId)}/session?player_id=${encodeURIComponent(playerId)}`
}

// ---- code drafts (localStorage) -------------------------------------------
//
// In-progress code lives only in this browser: nothing server-side needs to
// see a draft before submit, so there's no server round-trip — see the
// sessionClientMsg comment in server/internal/handlers/session.go.

function draftKey(matchId: string, playerId: string): string {
  return `dsa-arena:draft:${matchId}:${playerId}`
}

export function loadDraft(matchId: string, playerId: string): string | null {
  return localStorage.getItem(draftKey(matchId, playerId))
}

export function saveDraft(matchId: string, playerId: string, code: string) {
  localStorage.setItem(draftKey(matchId, playerId), code)
}

const HANDLE_KEY = 'dsa-arena:handle'

export function loadHandle(): string {
  return localStorage.getItem(HANDLE_KEY) ?? ''
}

export function saveHandle(handle: string) {
  localStorage.setItem(HANDLE_KEY, handle)
}

export function formatClock(totalSeconds: number): string {
  const s = Math.max(0, Math.floor(totalSeconds))
  const m = Math.floor(s / 60)
  return `${String(m).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`
}
