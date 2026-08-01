package matchmaking

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

// testConnPair spins up a websocket echo-free server and returns the
// server-side connection (what RunMatcher writes to) and the client-side
// connection (what the test reads from), matching how handlers.JoinQueue
// wires up a real Player.
func testConnPair(t *testing.T) (server *websocket.Conn, client *websocket.Conn) {
	t.Helper()

	upgrader := websocket.Upgrader{}
	connCh := make(chan *websocket.Conn, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		connCh <- c
	}))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { clientConn.Close() })

	serverConn := <-connCh
	t.Cleanup(func() { serverConn.Close() })

	return serverConn, clientConn
}

// relayDecisions mimics handlers.listen: it reads accept/decline actions off
// a player's server-side conn and forwards them to Player.Deliver, which is
// how the real websocket handler wires client messages into the matcher's
// AwaitDecision channels.
func relayDecisions(player *Player) {
	for {
		_, payload, err := player.Conn().ReadMessage()
		if err != nil {
			player.Deliver("disconnect")
			return
		}
		var msg struct {
			Action string `json:"action"`
		}
		if err := json.Unmarshal(payload, &msg); err != nil {
			continue
		}
		if msg.Action == "accept" || msg.Action == "decline" {
			player.Deliver(msg.Action)
		}
	}
}

func readPayload(t *testing.T, conn *websocket.Conn) MatchmakingPayload {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	var mp MatchmakingPayload
	if err := json.Unmarshal(raw, &mp); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return mp
}

func TestRunMatcherMatchesTwoPlayersWhoBothAccept(t *testing.T) {
	server1, client1 := testConnPair(t)
	server2, client2 := testConnPair(t)

	q := NewQueue()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	rooms := NewRoomStore(rdb)

	sqldb, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { sqldb.Close() })

	rows := sqlmock.NewRows([]string{"id", "title", "problem_description", "testcases"}).
		AddRow(int64(7), "Two Sum", "find two numbers", []byte(`[{"input":"[2,7,11,15]","output":"[0,1]"}]`))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, title, problem_description, testcases")).
		WillReturnRows(rows)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go RunMatcher(ctx, q, rooms, sqldb)

	p1 := NewPlayer("alice", server1)
	p2 := NewPlayer("bob", server2)
	go relayDecisions(p1)
	go relayDecisions(p2)

	q.Push(p1)
	q.Push(p2)

	found1 := readPayload(t, client1)
	found2 := readPayload(t, client2)

	if found1.Type != "match_found" || found2.Type != "match_found" {
		t.Fatalf("expected match_found for both, got %q and %q", found1.Type, found2.Type)
	}
	if found1.MatchID == "" {
		t.Fatalf("expected non-empty match id")
	}
	if found1.MatchID != found2.MatchID {
		t.Fatalf("expected both players to receive the same match id, got %q and %q", found1.MatchID, found2.MatchID)
	}

	accept, _ := json.Marshal(map[string]string{"action": "accept"})
	if err := client1.WriteMessage(websocket.TextMessage, accept); err != nil {
		t.Fatalf("client1 accept: %v", err)
	}
	if err := client2.WriteMessage(websocket.TextMessage, accept); err != nil {
		t.Fatalf("client2 accept: %v", err)
	}

	ready1 := readPayload(t, client1)
	ready2 := readPayload(t, client2)

	if ready1.Type != "match_ready" || ready2.Type != "match_ready" {
		t.Fatalf("expected match_ready for both, got %q and %q", ready1.Type, ready2.Type)
	}
	if ready1.MatchID != found1.MatchID {
		t.Fatalf("expected match_ready to carry the same match id")
	}

	room, err := rooms.Get(context.Background(), found1.MatchID)
	if err != nil {
		t.Fatalf("get room: %v", err)
	}
	if room == nil {
		t.Fatalf("expected room to be saved")
	}
	if room.Player1ID != "alice" || room.Player2ID != "bob" {
		t.Fatalf("expected players alice/bob, got %s/%s", room.Player1ID, room.Player2ID)
	}
	if room.ProblemTitle != "Two Sum" {
		t.Fatalf("expected problem title 'Two Sum', got %q", room.ProblemTitle)
	}
	if room.Status != "active" {
		t.Fatalf("expected status 'active', got %q", room.Status)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock expectations: %v", err)
	}
}

func TestRunMatcherCancelsAndRequeuesOnDecline(t *testing.T) {
	server1, client1 := testConnPair(t)
	server2, client2 := testConnPair(t)

	q := NewQueue()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	rooms := NewRoomStore(rdb)

	sqldb, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { sqldb.Close() })

	rows := sqlmock.NewRows([]string{"id", "title", "problem_description", "testcases"}).
		AddRow(int64(7), "Two Sum", "find two numbers", []byte(`[{"input":"[2,7,11,15]","output":"[0,1]"}]`))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, title, problem_description, testcases")).
		WillReturnRows(rows)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go RunMatcher(ctx, q, rooms, sqldb)

	p1 := NewPlayer("alice", server1)
	p2 := NewPlayer("bob", server2)
	go relayDecisions(p1)
	go relayDecisions(p2)

	q.Push(p1)
	q.Push(p2)

	found1 := readPayload(t, client1)
	_ = readPayload(t, client2)

	accept, _ := json.Marshal(map[string]string{"action": "accept"})
	decline, _ := json.Marshal(map[string]string{"action": "decline"})
	if err := client1.WriteMessage(websocket.TextMessage, accept); err != nil {
		t.Fatalf("client1 accept: %v", err)
	}
	if err := client2.WriteMessage(websocket.TextMessage, decline); err != nil {
		t.Fatalf("client2 decline: %v", err)
	}

	cancelled1 := readPayload(t, client1)
	if cancelled1.Type != "match_cancelled" || cancelled1.Reason != "opponent_declined" {
		t.Fatalf("expected match_cancelled/opponent_declined for alice, got %+v", cancelled1)
	}

	// alice accepted, so she should be back in the queue automatically.
	deadline := time.Now().Add(2 * time.Second)
	requeued := false
	for time.Now().Before(deadline) {
		if _, ok := q.Remove("alice"); ok {
			requeued = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !requeued {
		t.Fatalf("expected alice to be requeued after bob declined")
	}

	room, err := rooms.Get(context.Background(), found1.MatchID)
	if err != nil {
		t.Fatalf("get room: %v", err)
	}
	if room != nil {
		t.Fatalf("expected no room to be created when one player declines")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock expectations: %v", err)
	}
}

func TestRunMatcherRequeuesSoloPlayer(t *testing.T) {
	server1, _ := testConnPair(t)

	q := NewQueue()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	rooms := NewRoomStore(rdb)

	sqldb, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { sqldb.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go RunMatcher(ctx, q, rooms, sqldb)

	q.Push(NewPlayer("solo", server1))

	// Give the matcher a moment to pop, fail to find a second player, and push back.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if p, ok := q.Remove("solo"); ok {
			return // found it requeued, test passes
		} else {
			_ = p
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected solo player to remain in queue after unmatched pop")
}

func TestRunMatcherRequeuesBothOnDBError(t *testing.T) {
	server1, _ := testConnPair(t)
	server2, _ := testConnPair(t)

	q := NewQueue()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	rooms := NewRoomStore(rdb)

	sqldb, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { sqldb.Close() })

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, title, problem_description, testcases")).
		WillReturnError(context.DeadlineExceeded)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go RunMatcher(ctx, q, rooms, sqldb)

	q.Push(NewPlayer("alice", server1))
	q.Push(NewPlayer("bob", server2))

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_, aliceOK := q.Remove("alice")
		_, bobOK := q.Remove("bob")
		if aliceOK && bobOK {
			return
		}
		if aliceOK {
			q.Push(NewPlayer("alice", server1))
		}
		if bobOK {
			q.Push(NewPlayer("bob", server2))
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected both players to be requeued after a db error")
}
