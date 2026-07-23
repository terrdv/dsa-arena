package matchmaking

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

func RunMatcher(ctx context.Context, q *Queue, rooms *RoomStore) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-q.Signal():
		}

		player1, ok := q.Pop()
		if !ok {
			continue
		}

		player2, ok := q.Pop()
		if !ok {
			q.Push(player1)
			continue
		}

		room := &Room{
			MatchID:   newID(),
			Player1ID: player1.PlayerID(),
			Player2ID: player2.PlayerID(),
			Status:    "active",
			StartedAt: time.Now(),
		}

		if err := rooms.Save(ctx, room); err != nil {
			log.Println("save room:", err)
			q.Push(player1)
			q.Push(player2)
			continue
		}

		mp := &MatchmakingPayload{
			MatchID: room.MatchID,
		}

		payload, err := json.Marshal(mp)
		if err != nil {
			log.Println("marshal matchmaking payload:", err)
			continue
		}

		if err := player1.Conn().WriteMessage(websocket.TextMessage, payload); err != nil {
			log.Println("write to player1:", err)
		}
		if err := player2.Conn().WriteMessage(websocket.TextMessage, payload); err != nil {
			log.Println("write to player2:", err)
		}
	}
}

func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
