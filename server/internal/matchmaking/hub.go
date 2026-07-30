package matchmaking

import (
	"sync"
)

// Hub tracks the live session connections for each match so one player's
// events (e.g. a submission result) can be pushed to their opponent.
//
// It's in-memory, matching this project's single-server MVP scope (the same
// tradeoff Queue already makes).

type match struct {
	conns map[string]*Conn // playerID -> conn
}

type Hub struct {
	mx      sync.Mutex
	matches map[string]*match
}

func NewHub() *Hub {
	return &Hub{
		matches: make(map[string]*match),
	}
}

// Join registers conn under (matchID, playerID). If a connection is already
// registered for that player in that match (e.g. a reconnect), it's replaced
// and the old connection is closed so it doesn't linger — the old
// connection's read loop will observe the close and run its own Leave, which
// is a no-op by the time it fires since the map already points elsewhere.
func (h *Hub) Join(matchID, playerID string, conn *Conn) {
	h.mx.Lock()
	m, ok := h.matches[matchID]
	if !ok {
		m = &match{conns: make(map[string]*Conn)}
		h.matches[matchID] = m
	}
	old := m.conns[playerID]
	m.conns[playerID] = conn
	h.mx.Unlock()

	if old != nil && old != conn {
		old.Close()
	}
}

// Leave removes (matchID, playerID), cleaning up the match entry if it was
// the last connection in the room.
//
// conn must be the same connection that was passed to Join. This guards
// against a reconnect race: if a player's old connection is still shutting
// down after a new one has already joined, the old connection's deferred
// Leave call must not delete the new, live entry — comparing conn identity
// before deleting ensures a stale Leave never clobbers a newer Join.
func (h *Hub) Leave(matchID, playerID string, conn *Conn) {
	h.mx.Lock()
	defer h.mx.Unlock()

	m, ok := h.matches[matchID]
	if !ok {
		return
	}

	if m.conns[playerID] == conn {
		delete(m.conns, playerID)
	}

	if len(m.conns) == 0 {
		delete(h.matches, matchID)
	}
}

// BroadcastExcept sends payload to every connection in matchID other than
// exceptPlayerID (the sender).
//
// A write error to a stale connection is ignored here: that connection's own
// read loop will notice the drop and call Leave on its own, so this doesn't
// need to do cleanup itself. The match's connections are copied out before
// writing so the Hub's lock — shared across every match on the server — isn't
// held during network I/O.
func (h *Hub) BroadcastExcept(matchID, exceptPlayerID string, payload any) {
	h.mx.Lock()
	m, ok := h.matches[matchID]
	if !ok {
		h.mx.Unlock()
		return
	}

	targets := make([]*Conn, 0, len(m.conns))
	for playerID, conn := range m.conns {
		if playerID == exceptPlayerID {
			continue
		}
		targets = append(targets, conn)
	}
	h.mx.Unlock()

	for _, conn := range targets {
		_ = conn.WriteJSON(payload)
	}
}
