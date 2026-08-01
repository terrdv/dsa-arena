package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/terrdv/dsa-arena/server/internal/matchmaking"
)

func newQueueTestServer(t *testing.T, q *matchmaking.Queue) string {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /queue", JoinQueue(q))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/queue"
}

func TestJoinQueueRequiresPlayerID(t *testing.T) {
	q := matchmaking.NewQueue()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /queue", JoinQueue(q))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/queue")
	if err != nil {
		t.Fatalf("GET /queue: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 without player_id, got %d", resp.StatusCode)
	}
}

func TestJoinQueueAddsPlayer(t *testing.T) {
	q := matchmaking.NewQueue()
	wsURL := newQueueTestServer(t, q)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL+"?player_id=alice", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if p, ok := q.Remove("alice"); ok {
			if p.PlayerID() != "alice" {
				t.Fatalf("expected alice, got %s", p.PlayerID())
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected alice to be pushed onto the queue")
}

func TestJoinQueueLeaveMessageRemovesPlayer(t *testing.T) {
	q := matchmaking.NewQueue()
	wsURL := newQueueTestServer(t, q)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL+"?player_id=bob", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// wait until bob is actually in the queue first
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if p, ok := q.Remove("bob"); ok {
			q.Push(p)
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	leaveMsg, _ := json.Marshal(map[string]string{"action": "leave"})
	if err := conn.WriteMessage(websocket.TextMessage, leaveMsg); err != nil {
		t.Fatalf("write leave message: %v", err)
	}

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := q.Remove("bob"); !ok {
			return // successfully removed
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected bob to be removed from the queue after leave message")
}

func TestJoinQueueDisconnectRemovesPlayer(t *testing.T) {
	q := matchmaking.NewQueue()
	wsURL := newQueueTestServer(t, q)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL+"?player_id=carol", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	inQueue := false
	for time.Now().Before(deadline) {
		if p, ok := q.Remove("carol"); ok {
			q.Push(p)
			inQueue = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !inQueue {
		t.Fatalf("expected carol to be in the queue before disconnect")
	}

	conn.Close()

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := q.Remove("carol"); !ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected carol to be removed from the queue after disconnect")
}

func TestJoinQueueAcceptMessageIsDelivered(t *testing.T) {
	q := matchmaking.NewQueue()
	wsURL := newQueueTestServer(t, q)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL+"?player_id=dave", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	var player *matchmaking.Player
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if p, ok := q.Remove("dave"); ok {
			player = p
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if player == nil {
		t.Fatalf("expected dave to be queued")
	}

	decisions := player.AwaitDecision()

	acceptMsg, _ := json.Marshal(map[string]string{"action": "accept"})
	if err := conn.WriteMessage(websocket.TextMessage, acceptMsg); err != nil {
		t.Fatalf("write accept message: %v", err)
	}

	select {
	case d := <-decisions:
		if d != "accept" {
			t.Fatalf("expected accept, got %q", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("expected accept decision to be delivered")
	}
}
