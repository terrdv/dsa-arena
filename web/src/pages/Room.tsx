import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import TopBar from '../components/TopBar'
import ProblemBody from '../components/ProblemBody'
import {
  ClockIcon,
  CodeIcon,
  ScrollIcon,
  SwordsIcon,
  AlertIcon,
} from '../components/Icons'
import {
  fetchRoom,
  formatClock,
  loadHandle,
  RoomNotFoundError,
  type RoomState,
} from '../lib/api'

type LoadState =
  | { status: 'loading' }
  | { status: 'not-found' }
  | { status: 'error' }
  | { status: 'ready'; room: RoomState }

function initials(name: string): string {
  return name.slice(0, 2) || '??'
}

export default function Room() {
  const { matchId } = useParams<{ matchId: string }>()
  const [state, setState] = useState<LoadState>({ status: 'loading' })
  const [elapsed, setElapsed] = useState(0)

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
  const me = loadHandle()
  // Orient the versus bar around the local player when we recognize them.
  const iAmPlayer2 = room.player2_id === me && room.player1_id !== me
  const left = iAmPlayer2 ? room.player2_id : room.player1_id
  const right = iAmPlayer2 ? room.player1_id : room.player2_id
  const knownPlayer = room.player1_id === me || room.player2_id === me

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
            </div>
            <div className="editor-placeholder">
              <CodeIcon />
              <strong>Editor coming soon</strong>
              <p>
                This is where you'll write and run your solution. For now, study
                the problem and plot your approach — brute force won't cut it.
              </p>
            </div>
          </section>
        </div>
      </main>
    </div>
  )
}
