package matchmaking

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Room struct {
	MatchID   string    `json:"match_id"`
	Player1ID string    `json:"player1_id"`
	Player2ID string    `json:"player2_id"`
	ProblemID string    `json:"problem_id"`
	Status    string    `json:"status"` // "active", "finished"
	StartedAt time.Time `json:"started_at"`
}

// roomTTL caps how long a room lives in Redis so abandoned matches clean themselves up.
const roomTTL = 2 * time.Hour

type RoomStore struct {
	rdb *redis.Client
}

func NewRoomStore(rdb *redis.Client) *RoomStore {
	return &RoomStore{rdb: rdb}
}

func roomKey(matchID string) string {
	return "room:" + matchID
}

// Save writes the room to Redis, overwriting any existing state for the match.
func (rs *RoomStore) Save(ctx context.Context, r *Room) error {
	data, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("marshal room: %w", err)
	}

	if err := rs.rdb.Set(ctx, roomKey(r.MatchID), data, roomTTL).Err(); err != nil {
		return fmt.Errorf("set room: %w", err)
	}

	return nil
}

// Get returns the room for matchID, or (nil, nil) if it doesn't exist.
func (rs *RoomStore) Get(ctx context.Context, matchID string) (*Room, error) {
	data, err := rs.rdb.Get(ctx, roomKey(matchID)).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get room: %w", err)
	}

	var r Room
	if err := json.Unmarshal([]byte(data), &r); err != nil {
		return nil, fmt.Errorf("unmarshal room: %w", err)
	}

	return &r, nil
}

// Delete removes the room for matchID. Deleting a missing room is not an error.
func (rs *RoomStore) Delete(ctx context.Context, matchID string) error {
	if err := rs.rdb.Del(ctx, roomKey(matchID)).Err(); err != nil {
		return fmt.Errorf("delete room: %w", err)
	}
	return nil
}
