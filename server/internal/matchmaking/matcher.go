package matchmaking

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"

	"github.com/gorilla/websocket"
)

func RunMatcher(q *Queue, rooms *RoomStack) {
	for {
		select {
		case <-q.Signal():
		case <-rooms.Signal():
		}

		room, ok := rooms.Pop()
		if !ok {
			continue
		}

		player1, ok := q.Pop()
		if !ok {
			rooms.Push(room)
			continue
		}

		player2, ok := q.Pop()
		if !ok {
			q.Push(player1)
			rooms.Push(room)
			continue
		}

		mp := &MatchmakingPayload{
			MatchID:         newID(),
			ServerIP:        room.serverIP,
			Port:            room.port,
			ConnectionToken: newID(),
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
