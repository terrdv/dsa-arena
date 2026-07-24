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
