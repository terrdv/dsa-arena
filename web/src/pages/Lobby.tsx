import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import TopBar from '../components/TopBar'
import { AlertIcon, CheckIcon, SwordsIcon, TargetIcon, XIcon, ZapIcon } from '../components/Icons'
import { formatClock, loadHandle, queueSocketURL, saveHandle } from '../lib/api'

type Phase = 'idle' | 'searching' | 'accepting' | 'found'

// Mirrors the server's acceptTimeout in server/internal/matchmaking/matcher.go —
// purely cosmetic here since the server enforces the real deadline.
const ACCEPT_SECONDS = 5

// Backend reasons: see MatchmakingPayload in server/internal/matchmaking/queue.go.
// A player who declines isn't sent a match_cancelled at all (they already
// know), so "declined" never shows up here.
function cancelReasonText(reason?: string): string {
  switch (reason) {
    case 'opponent_declined':
      return 'Opponent declined — searching again…'
    case 'opponent_timeout':
    case 'opponent_disconnected':
      return 'Opponent left — searching again…'
    case 'timeout':
      return "You didn't respond in time."
    default:
      return 'Match fell through — try again.'
  }
}

export default function Lobby() {
  const navigate = useNavigate()
  const [handle, setHandle] = useState(loadHandle)
  const [phase, setPhase] = useState<Phase>('idle')
  const [error, setError] = useState('')
  const [elapsed, setElapsed] = useState(0)
  const [acceptCountdown, setAcceptCountdown] = useState(ACCEPT_SECONDS)
  const [iAccepted, setIAccepted] = useState(false)
  const wsRef = useRef<WebSocket | null>(null)
  // Distinguishes closes we initiated (cancel / match found / unmount)
  // from the server dropping us mid-queue.
  const closingRef = useRef(false)
  // Mirrors iAccepted for the ws.onmessage closure, which otherwise only
  // sees the iAccepted value from when the socket was opened.
  const acceptedRef = useRef(false)

  useEffect(() => {
    if (phase !== 'searching') return
    setElapsed(0)
    const t = setInterval(() => setElapsed((s) => s + 1), 1000)
    return () => clearInterval(t)
  }, [phase])

  useEffect(() => {
    if (phase !== 'accepting') return
    setAcceptCountdown(ACCEPT_SECONDS)
    const t = setInterval(() => setAcceptCountdown((s) => Math.max(0, s - 1)), 1000)
    return () => clearInterval(t)
  }, [phase])

  useEffect(() => {
    return () => {
      closingRef.current = true
      wsRef.current?.close()
    }
  }, [])

  function findMatch() {
    const name = handle.trim()
    if (!name) {
      setError('Enter a handle to queue up.')
      return
    }
    setError('')
    saveHandle(name)
    closingRef.current = false

    const ws = new WebSocket(queueSocketURL(name)) // TCP HANDSHAKE
    wsRef.current = ws
    setPhase('searching')

    ws.onmessage = (ev) => {
      let msg: { type?: string; match_id?: string; reason?: string }
      try {
        msg = JSON.parse(ev.data)
      } catch {
        return
      }

      switch (msg.type) {
        case 'match_found':
          acceptedRef.current = false
          setIAccepted(false)
          setError('')
          setPhase('accepting')
          break

        case 'match_ready':
          closingRef.current = true
          ws.close()
          setPhase('found')
          // let the "match found" flash land before jumping into the room
          setTimeout(() => navigate(`/room/${msg.match_id}`), 900)
          break

        case 'match_cancelled':
          setError(cancelReasonText(msg.reason))
          if (acceptedRef.current) {
            // the server already put us back in the queue
            setPhase('searching')
          } else {
            closingRef.current = true
            ws.close()
            setPhase('idle')
          }
          break
      }
    }

    ws.onerror = () => {
      if (closingRef.current) return
      closingRef.current = true
      setPhase('idle')
      setError('Could not reach the arena. Is the server running?')
    }

    ws.onclose = () => {
      if (closingRef.current) return
      setPhase('idle')
      setError('Connection lost — try queueing again.')
    }
  }

  function cancel() {
    closingRef.current = true
    const ws = wsRef.current
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ action: 'leave' }))
    }
    ws?.close()
    setError('')
    setPhase('idle')
  }

  function acceptMatch() {
    acceptedRef.current = true
    setIAccepted(true)
    wsRef.current?.send(JSON.stringify({ action: 'accept' }))
  }

  function declineMatch() {
    const ws = wsRef.current
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ action: 'decline' }))
    }
    closingRef.current = true
    ws?.close()
    setPhase('idle')
  }

  return (
    <div className="page">
      <TopBar />
      <main className="lobby">
        <div>
          <p className="lobby-kicker">1v1 · real-time · data structures &amp; algorithms</p>
          <h1 className="lobby-title">
            ENTER THE <em>ARENA</em>
          </h1>
        </div>
        <p className="lobby-sub">
          Get matched against another player and race to crack the same data
          structures &amp; algorithms problem — arrays, graphs, DP and beyond.
          Fastest correct solution takes the round.
        </p>

        <section className="queue-card" aria-live="polite">
          {phase === 'idle' && (
            <>
              <div className="field">
                <label htmlFor="handle">Your handle</label>
                <input
                  id="handle"
                  value={handle}
                  maxLength={24}
                  placeholder="e.g. off_by_one"
                  autoComplete="username"
                  onChange={(e) => setHandle(e.target.value)}
                  onKeyDown={(e) => e.key === 'Enter' && findMatch()}
                />
              </div>
              {error && (
                <p className="form-error" role="alert">
                  <AlertIcon width={16} height={16} />
                  {error}
                </p>
              )}
              <button type="button" className="btn btn-primary" onClick={findMatch}>
                <SwordsIcon width={18} height={18} />
                Find match
              </button>
            </>
          )}

          {phase === 'searching' && (
            <>
              <div className="radar" role="status" aria-label="Searching for an opponent">
                <span className="radar-ring" />
                <span className="radar-ring" />
                <span className="radar-ring" />
                <span className="radar-core">
                  <TargetIcon />
                </span>
              </div>
              <p className="searching-label">
                Searching for opponent<span className="dots" />
              </p>
              <p className="queue-timer">{formatClock(elapsed)}</p>
              {error ? (
                <p className="form-error" role="alert">
                  <AlertIcon width={16} height={16} />
                  {error}
                </p>
              ) : (
                <p className="queue-hint">
                  Queued as <strong>{handle.trim()}</strong> — warm up those hash
                  maps.
                </p>
              )}
              <button type="button" className="btn btn-danger-ghost" onClick={cancel}>
                Leave queue
              </button>
            </>
          )}

          {phase === 'accepting' && (
            <div className="match-found" role="status">
              <SwordsIcon width={44} height={44} style={{ color: 'var(--accent)' }} />
              <p className="searching-label">Opponent found!</p>
              <p className="queue-hint">
                Accept within <strong>{acceptCountdown}s</strong> or the match falls
                through.
              </p>
              {iAccepted ? (
                <p className="queue-hint">Waiting on your opponent…</p>
              ) : (
                <div className="accept-actions">
                  <button type="button" className="btn btn-primary" onClick={acceptMatch}>
                    <CheckIcon width={18} height={18} />
                    Accept
                  </button>
                  <button type="button" className="btn btn-danger-ghost" onClick={declineMatch}>
                    <XIcon width={18} height={18} />
                    Decline
                  </button>
                </div>
              )}
            </div>
          )}

          {phase === 'found' && (
            <div className="match-found" role="status">
              <SwordsIcon width={44} height={44} style={{ color: 'var(--accent)' }} />
              <p className="searching-label">Match found!</p>
              <p className="queue-hint">Entering the room…</p>
            </div>
          )}
        </section>

        <div className="lobby-stats">
          <span>
            <ZapIcon /> instant matchmaking
          </span>
          <span>
            <TargetIcon /> random DSA problem, same for both
          </span>
        </div>
      </main>
    </div>
  )
}
