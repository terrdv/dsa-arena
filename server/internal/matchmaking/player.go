package matchmaking

import (
	"sync"

	"github.com/gorilla/websocket"
)

type Player struct {
	playerID string
	conn     *websocket.Conn

	mu      sync.Mutex
	pending chan string
}

func NewPlayer(playerID string, connection *websocket.Conn) *Player {
	return &Player{playerID: playerID, conn: connection}
}

func (p *Player) PlayerID() string {
	return p.playerID
}

func (p *Player) Conn() *websocket.Conn {
	return p.conn
}

// AwaitDecision opens a one-shot channel that receives this player's next
// "accept"/"decline" (or "disconnect", if their connection drops first)
// once the matcher proposes a match. Call ClearPending once done waiting so
// a later, unrelated message isn't mistaken for a decision.
func (p *Player) AwaitDecision() <-chan string {
	ch := make(chan string, 1)
	p.mu.Lock()
	p.pending = ch
	p.mu.Unlock()
	return ch
}

// ClearPending detaches the pending decision channel so further messages
// stop being delivered to it.
func (p *Player) ClearPending() {
	p.mu.Lock()
	p.pending = nil
	p.mu.Unlock()
}

// Deliver forwards a decision to whoever is currently waiting via
// AwaitDecision. It's a no-op if nothing is waiting.
func (p *Player) Deliver(decision string) {
	p.mu.Lock()
	ch := p.pending
	p.mu.Unlock()

	if ch == nil {
		return
	}
	select {
	case ch <- decision:
	default:
	}
}
