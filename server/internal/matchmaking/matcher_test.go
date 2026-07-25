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

func TestRunMatcherMatchesTwoPlayers(t *testing.T) {
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

	q.Push(NewPlayer("alice", server1))
	q.Push(NewPlayer("bob", server2))

	client1.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, payload1, err := client1.ReadMessage()
	if err != nil {
		t.Fatalf("read from client1: %v", err)
	}

	client2.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, payload2, err := client2.ReadMessage()
	if err != nil {
		t.Fatalf("read from client2: %v", err)
	}

	var mp1, mp2 MatchmakingPayload
	if err := json.Unmarshal(payload1, &mp1); err != nil {
		t.Fatalf("unmarshal payload1: %v", err)
	}
	if err := json.Unmarshal(payload2, &mp2); err != nil {
		t.Fatalf("unmarshal payload2: %v", err)
	}

	if mp1.MatchID == "" {
		t.Fatalf("expected non-empty match id")
	}
	if mp1.MatchID != mp2.MatchID {
		t.Fatalf("expected both players to receive the same match id, got %q and %q", mp1.MatchID, mp2.MatchID)
	}

	room, err := rooms.Get(context.Background(), mp1.MatchID)
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
