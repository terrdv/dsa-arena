package matchmaking

import (
	"slices"
	"sync"
)

type MatchmakingPayload struct {
	MatchID         string `json:"match_id"`
	ServerIP        string `json:"server_ip"`
	Port            int    `json:"port"`
	ConnectionToken string `json:"connection_token"`
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


