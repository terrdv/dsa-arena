import { useEffect, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import TopBar from '../components/TopBar'
import ProblemBody from '../components/ProblemBody'
import Editor from '../components/Editor'
import {
  ClockIcon,
  CodeIcon,
  ScrollIcon,
  SwordsIcon,
  AlertIcon,
  ZapIcon,
} from '../components/Icons'
import {
  fetchRoom,
  formatClock,
  loadDraft,
  loadHandle,
  roomSessionURL,
  saveDraft,
  RoomNotFoundError,
  type RoomState,
  type ClientSessionMsg,
  type ServerSessionMsg,
  type CaseResult,
} from '../lib/api'

type LoadState =
  | { status: 'loading' }
  | { status: 'not-found' }
  | { status: 'error' }
  | { status: 'ready'; room: RoomState }

interface JudgeResult {
  passed: number
  total: number
  cases: CaseResult[]
}

const STATUS_LABEL: Record<CaseResult['status'], string> = {
  pass: 'Passed',
  fail: 'Wrong answer',
  error: 'Error',
  timeout: 'Timed out',
}

interface OpponentResult {
  passed: number
  total: number
}

const LANGUAGE = 'python'

const DEFAULT_TEMPLATE = `def solve():
    # rename solve's parameters to match the problem's input names
    pass
`

// How long to sit on a keystroke before persisting the draft to localStorage.
const DRAFT_DEBOUNCE_MS = 500

function initials(name: string): string {
  return name.slice(0, 2) || '??'
}

export default function Room() {
  const { matchId } = useParams<{ matchId: string }>()
  const [state, setState] = useState<LoadState>({ status: 'loading' })
  const [elapsed, setElapsed] = useState(0)

  const [code, setCode] = useState('')
  const [judging, setJudging] = useState(false)
  const [myResult, setMyResult] = useState<JudgeResult | null>(null)
  const [oppResult, setOppResult] = useState<OpponentResult | null>(null)
  const [judgeError, setJudgeError] = useState('')
  const [connected, setConnected] = useState(false)
  const [connectionLost, setConnectionLost] = useState(false)

  const wsRef = useRef<WebSocket | null>(null)
  // Distinguishes closes we initiated (unmount) from the server dropping us.
  const closingRef = useRef(false)
  // Blocks the draft-save effect until the stored draft has been loaded, so
  // the initial empty editor doesn't clobber a real saved draft.
  const seededRef = useRef(false)

  const me = loadHandle()

  useEffect(() => {
    if (!matchId) return
    let cancelled = false
    fetchRoom(matchId)
      .then((room) => !cancelled && setState({ status: 'ready', room }))
      .catch((err) =>
        !cancelled &&
        setState({ status: err instanceof RoomNotFoundError ? 'not-found' : 'error' }),
      )
    return () => {
      cancelled = true
    }
  }, [matchId])

  useEffect(() => {
    if (state.status !== 'ready' || state.room.status !== 'active') return
    const startedAt = new Date(state.room.started_at).getTime()
    const tick = () => setElapsed((Date.now() - startedAt) / 1000)
    tick()
    const t = setInterval(tick, 1000)
    return () => clearInterval(t)
  }, [state])

  // Seed the editor from the locally saved draft once the room is known.
  useEffect(() => {
    if (state.status !== 'ready' || !matchId || seededRef.current) return
    const isPlayer = state.room.player1_id === me || state.room.player2_id === me
    if (!isPlayer) return
    seededRef.current = true
    setCode(loadDraft(matchId, me) ?? DEFAULT_TEMPLATE)
  }, [state, matchId, me])

  // Debounced draft persistence — drafts are localStorage-only, the server
  // never sees code until submit.
  useEffect(() => {
    if (!seededRef.current || !matchId) return
    const t = setTimeout(() => saveDraft(matchId, me, code), DRAFT_DEBOUNCE_MS)
    return () => clearTimeout(t)
  }, [code, matchId, me])

  // Room session socket: submit results in, opponent progress pings out.
  useEffect(() => {
    if (state.status !== 'ready' || !matchId) return
    const { room } = state
    const isPlayer = room.player1_id === me || room.player2_id === me
    if (!isPlayer || room.status !== 'active') return

    closingRef.current = false
    const ws = new WebSocket(roomSessionURL(matchId, me))
    wsRef.current = ws

    ws.onopen = () => {
      setConnected(true)
      setConnectionLost(false)
    }

    ws.onmessage = (ev) => {
      let msg: ServerSessionMsg
      try {
        msg = JSON.parse(ev.data)
      } catch {
        return
      }

      switch (msg.type) {
        case 'judging':
          setJudging(true)
          break
        case 'result':
          setJudging(false)
          setMyResult({ passed: msg.passed, total: msg.total, cases: msg.cases ?? [] })
          break
        case 'opponent_result':
          setOppResult({ passed: msg.passed, total: msg.total })
          break
        case 'error':
          setJudging(false)
          setJudgeError(msg.message)
          break
      }
    }

    const dropped = () => {
      if (closingRef.current) return
      closingRef.current = true
      setConnected(false)
      setConnectionLost(true)
      setJudging(false)
    }
    ws.onerror = dropped
    ws.onclose = dropped

    return () => {
      closingRef.current = true
      ws.close()
      wsRef.current = null
      setConnected(false)
    }
  }, [state, matchId, me])

  function submit() {
    const ws = wsRef.current
    if (!ws || ws.readyState !== WebSocket.OPEN || judging) return
    setMyResult(null)
    setJudgeError('')
    setJudging(true) // optimistic; the server acks with "judging" too
    const msg: ClientSessionMsg = { type: 'submit', code, language: LANGUAGE }
    ws.send(JSON.stringify(msg))
  }

  if (state.status === 'loading') {
    return (
      <div className="page">
        <TopBar />
        <div className="room-state" role="status">
          <div className="spinner" />
          <h2>Loading room…</h2>
        </div>
      </div>
    )
  }

  if (state.status === 'not-found' || state.status === 'error') {
    const notFound = state.status === 'not-found'
    return (
      <div className="page">
        <TopBar />
        <div className="room-state">
          <AlertIcon width={44} height={44} style={{ color: 'var(--warn)' }} />
          <h2>{notFound ? 'Room not found' : 'Something went wrong'}</h2>
          <p>
            {notFound
              ? 'This match may have expired — rooms only live for 2 hours.'
              : 'Could not load the room. Check that the server is running.'}
          </p>
          <Link to="/" className="btn btn-primary">
            Back to lobby
          </Link>
        </div>
      </div>
    )
  }

  const { room } = state
  // Orient the versus bar around the local player when we recognize them.
  const iAmPlayer2 = room.player2_id === me && room.player1_id !== me
  const left = iAmPlayer2 ? room.player2_id : room.player1_id
  const right = iAmPlayer2 ? room.player1_id : room.player2_id
  const knownPlayer = room.player1_id === me || room.player2_id === me

  const allPassed = myResult !== null && myResult.passed === myResult.total

  return (
    <div className="page">
      <TopBar
        right={
          <div className="room-meta">
            {room.status === 'active' ? (
              <span className="badge live">
                <span className="pulse-dot" />
                LIVE
                <span className="timer">{formatClock(elapsed)}</span>
              </span>
            ) : (
              <span className="badge finished">
                <ClockIcon />
                FINISHED
              </span>
            )}
            <span className="badge" title={room.match_id}>
              match&nbsp;{room.match_id.slice(0, 8)}
            </span>
          </div>
        }
      />

      <main className="room-main">
        <section className="versus" aria-label="Players">
          <div className={`fighter you${knownPlayer ? '' : ' opp'}`}>
            <span className="avatar" aria-hidden>
              {initials(left)}
            </span>
            <div>
              <p className="fighter-tag">{knownPlayer ? 'You' : 'Player 1'}</p>
              <p className="fighter-name">{left}</p>
              {knownPlayer && myResult && (
                <p className={`fighter-progress${allPassed ? ' ok' : ''}`}>
                  <ZapIcon width={12} height={12} />
                  {myResult.passed}/{myResult.total} passed
                </p>
              )}
            </div>
          </div>

          <div className="vs-badge">
            <SwordsIcon />
            <span className="vs-text">VS</span>
          </div>

          <div className="fighter opp right">
            <span className="avatar" aria-hidden>
              {initials(right)}
            </span>
            <div>
              <p className="fighter-tag">{knownPlayer ? 'Opponent' : 'Player 2'}</p>
              <p className="fighter-name">{right}</p>
              {knownPlayer && oppResult && (
                <p className="fighter-progress">
                  <ZapIcon width={12} height={12} />
                  {oppResult.passed}/{oppResult.total} passed
                </p>
              )}
            </div>
          </div>
        </section>

        <div className="room-grid">
          <section className="panel" aria-label="Problem">
            <div className="panel-head">
              <ScrollIcon />
              Problem&nbsp;·&nbsp;#{room.problem_id}
            </div>
            <div className="problem-body">
              <h2 className="problem-title">{room.problem_title}</h2>
              <ProblemBody description={room.problem_description} />
            </div>
          </section>

          <section className="panel" aria-label="Code editor">
            <div className="panel-head">
              <CodeIcon />
              Solution
              <span className="lang-chip">{LANGUAGE}</span>
              {knownPlayer && room.status === 'active' && (
                <span className={`conn-dot${connected ? ' on' : ''}`} title={connected ? 'Connected' : 'Not connected'} />
              )}
            </div>

            {knownPlayer ? (
              <div className="editor-wrap">
                {connectionLost && (
                  <p className="form-error editor-banner" role="alert">
                    <AlertIcon width={16} height={16} />
                    Connection lost — refresh the page to reconnect. Your code is
                    saved locally.
                  </p>
                )}

                <Editor
                  value={code}
                  language={LANGUAGE}
                  onChange={setCode}
                  readOnly={room.status !== 'active'}
                />

                <div className="editor-toolbar">
                  <div className="editor-status" aria-live="polite">
                    {judging && (
                      <span className="judging-label">
                        <span className="spinner spinner-sm" />
                        Judging…
                      </span>
                    )}
                    {!judging && myResult && (
                      <span className={`result-badge${allPassed ? ' ok' : ' bad'}`}>
                        {allPassed ? 'All tests passed' : `${myResult.passed}/${myResult.total} passed`}
                      </span>
                    )}
                    {!judging && judgeError && (
                      <span className="result-badge bad">{judgeError}</span>
                    )}
                  </div>
                  <button
                    type="button"
                    className="btn btn-primary"
                    onClick={submit}
                    disabled={!connected || judging || room.status !== 'active'}
                  >
                    <ZapIcon width={16} height={16} />
                    {judging ? 'Judging…' : 'Submit'}
                  </button>
                </div>

                {!judging && myResult !== null && myResult.cases.length > 0 && (
                  <div className="judge-cases">
                    <p className="section-label">Test cases</p>
                    <div className="case-table">
                      {myResult.cases.map((c) => (
                        <div className={`case-row${c.passed ? ' ok' : ' bad'}`} key={c.index}>
                          <div className="case-row-head">
                            <span className="case-name">Case {c.index + 1}</span>
                            <span className={`case-status${c.passed ? ' ok' : ' bad'}`}>
                              {STATUS_LABEL[c.status]}
                            </span>
                          </div>
                          <div className="case-row-body">
                            <div>
                              <span className="case-field-label">Input</span>
                              <code>{c.input}</code>
                            </div>
                            <div>
                              <span className="case-field-label">Expected</span>
                              <code>{c.expected}</code>
                            </div>
                            <div>
                              <span className="case-field-label">Actual</span>
                              <code>{c.actual}</code>
                            </div>
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            ) : (
              <div className="editor-placeholder">
                <CodeIcon />
                <strong>Spectating</strong>
                <p>
                  Only the two matched players can write and submit code in this
                  room.
                </p>
              </div>
            )}
          </section>
        </div>
      </main>
    </div>
  )
}
