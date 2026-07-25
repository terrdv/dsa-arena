package matchmaking

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestRoomStore(t *testing.T) (*RoomStore, *miniredis.Miniredis) {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })

	return NewRoomStore(client), mr
}

func TestRoomStoreSaveAndGet(t *testing.T) {
	rs, _ := newTestRoomStore(t)
	ctx := context.Background()

	room := &Room{
		MatchID:            "match-1",
		Player1ID:          "p1",
		Player2ID:          "p2",
		ProblemID:          "42",
		ProblemTitle:       "Two Sum",
		ProblemDescription: "desc",
		Testcases:          json.RawMessage(`[{"in":1,"out":2}]`),
		Status:             "active",
		StartedAt:          time.Now().UTC().Truncate(time.Second),
	}

	if err := rs.Save(ctx, room); err != nil {
		t.Fatalf("save room: %v", err)
	}

	got, err := rs.Get(ctx, "match-1")
	if err != nil {
		t.Fatalf("get room: %v", err)
	}
	if got == nil {
		t.Fatalf("expected room, got nil")
	}
	if got.MatchID != room.MatchID || got.Player1ID != room.Player1ID || got.Player2ID != room.Player2ID {
		t.Fatalf("got room %+v, want %+v", got, room)
	}
	if got.ProblemTitle != room.ProblemTitle || got.Status != room.Status {
		t.Fatalf("got room %+v, want %+v", got, room)
	}
}

func TestRoomStoreGetMissing(t *testing.T) {
	rs, _ := newTestRoomStore(t)

	got, err := rs.Get(context.Background(), "does-not-exist")
	if err != nil {
		t.Fatalf("expected no error for missing room, got %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil room, got %+v", got)
	}
}

func TestRoomStoreSaveOverwrites(t *testing.T) {
	rs, _ := newTestRoomStore(t)
	ctx := context.Background()

	room := &Room{MatchID: "match-1", Status: "active"}
	if err := rs.Save(ctx, room); err != nil {
		t.Fatalf("save room: %v", err)
	}

	room.Status = "finished"
	if err := rs.Save(ctx, room); err != nil {
		t.Fatalf("save room: %v", err)
	}

	got, err := rs.Get(ctx, "match-1")
	if err != nil {
		t.Fatalf("get room: %v", err)
	}
	if got.Status != "finished" {
		t.Fatalf("expected overwritten status 'finished', got %q", got.Status)
	}
}

func TestRoomStoreDelete(t *testing.T) {
	rs, _ := newTestRoomStore(t)
	ctx := context.Background()

	room := &Room{MatchID: "match-1", Status: "active"}
	if err := rs.Save(ctx, room); err != nil {
		t.Fatalf("save room: %v", err)
	}

	if err := rs.Delete(ctx, "match-1"); err != nil {
		t.Fatalf("delete room: %v", err)
	}

	got, err := rs.Get(ctx, "match-1")
	if err != nil {
		t.Fatalf("get room after delete: %v", err)
	}
	if got != nil {
		t.Fatalf("expected room to be gone after delete, got %+v", got)
	}
}

func TestRoomStoreDeleteMissingIsNotError(t *testing.T) {
	rs, _ := newTestRoomStore(t)

	if err := rs.Delete(context.Background(), "does-not-exist"); err != nil {
		t.Fatalf("expected deleting a missing room to be a no-op, got %v", err)
	}
}

func TestRoomStoreSetsTTL(t *testing.T) {
	rs, mr := newTestRoomStore(t)
	ctx := context.Background()

	room := &Room{MatchID: "match-1", Status: "active"}
	if err := rs.Save(ctx, room); err != nil {
		t.Fatalf("save room: %v", err)
	}

	ttl := mr.TTL(roomKey("match-1"))
	if ttl <= 0 {
		t.Fatalf("expected a positive TTL on the room key, got %v", ttl)
	}
	if ttl > roomTTL {
		t.Fatalf("expected TTL <= %v, got %v", roomTTL, ttl)
	}
}
