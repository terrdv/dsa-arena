package matchmaking

import (
	"slices"
	"sync"
)

// MatchmakingPayload is what the server sends over a player's queue socket.
//   - {"type":"match_found","match_id":"..."}   — a match was proposed; respond with {"action":"accept"|"decline"}
//   - {"type":"match_ready","match_id":"..."}   — both players accepted; the room is live, safe to navigate to it
//   - {"type":"match_cancelled","reason":"..."} — the proposal fell through; a player who declines isn't sent
//     this (they already know), so reason is one of: "timeout" (you didn't respond in time),
//     "opponent_declined", "opponent_timeout", "opponent_disconnected", or "server_error"
type MatchmakingPayload struct {
	Type    string `json:"type"`
	MatchID string `json:"match_id,omitempty"`
	Reason  string `json:"reason,omitempty"`
}


type Queue struct {
	mx sync.Mutex
	players []*Player
	signal chan struct{}
}

func NewQueue() *Queue {
	return &Queue{
		signal: make(chan struct{}, 1),
	}
}

func (q * Queue) Push(p *Player) {
	q.mx.Lock()
	q.players = append(q.players, p)
	q.mx.Unlock()

	select {
	case q.signal <- struct{}{}:
	default:
	}
}

func (q * Queue) Pop() (*Player, bool) {
	q.mx.Lock()
	defer q.mx.Unlock()

	if len(q.players) == 0 {
		return nil, false
	}

	p := q.players[0]
    q.players = q.players[1:]
    return p, true
}

func (q *Queue) Signal() <-chan struct{} {
	return q.signal
}

func (q *Queue) Remove(playerID string) (*Player, bool) {
	q.mx.Lock()
	defer q.mx.Unlock()

	idx := slices.IndexFunc(q.players, func(p *Player) bool {
		return p.playerID == playerID
	})
	if idx == -1 {
		return nil, false
	}

	p := q.players[idx]
	q.players = slices.Delete(q.players, idx, idx+1)
	return p, true
}


