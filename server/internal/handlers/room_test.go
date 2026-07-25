package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/terrdv/dsa-arena/server/internal/matchmaking"
)

func newTestRoomStore(t *testing.T) *matchmaking.RoomStore {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })

	return matchmaking.NewRoomStore(client)
}

func TestGetRoomFound(t *testing.T) {
	rooms := newTestRoomStore(t)
	room := &matchmaking.Room{
		MatchID:      "match-1",
		Player1ID:    "alice",
		Player2ID:    "bob",
		ProblemTitle: "Two Sum",
		Status:       "active",
	}
	if err := rooms.Save(context.Background(), room); err != nil {
		t.Fatalf("save room: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/room/match-1", nil)
	req.SetPathValue("match_id", "match-1")
	rec := httptest.NewRecorder()

	GetRoom(rooms)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var got matchmaking.Room
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got.MatchID != "match-1" || got.Player1ID != "alice" || got.Player2ID != "bob" {
		t.Fatalf("unexpected room in response: %+v", got)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected content-type application/json, got %q", ct)
	}
}

func TestGetRoomNotFound(t *testing.T) {
	rooms := newTestRoomStore(t)

	req := httptest.NewRequest(http.MethodGet, "/room/missing", nil)
	req.SetPathValue("match_id", "missing")
	rec := httptest.NewRecorder()

	GetRoom(rooms)(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestGetRoomMissingMatchID(t *testing.T) {
	rooms := newTestRoomStore(t)

	req := httptest.NewRequest(http.MethodGet, "/room/", nil)
	req.SetPathValue("match_id", "")
	rec := httptest.NewRecorder()

	GetRoom(rooms)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
