package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/terrdv/dsa-arena/server/internal/matchmaking"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  0,
	WriteBufferSize: 0,
	// CheckOrigin allows bypass for local development; adjust for production security
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, fmt.Errorf("upgrade connection: %w", err)
	}

	return conn, nil
}

func JoinQueue(q *matchmaking.Queue) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		playerID := r.URL.Query().Get("player_id")
		if playerID == "" {
			http.Error(w, "player_id is required", http.StatusBadRequest)
			return
		}

		// Upgrade the http connection to tcp
		conn, err := handleWebSocket(w, r)

		if err != nil {
			log.Println("TCP Handshake failed:", err)
			return
		}

		player := matchmaking.NewPlayer(playerID, conn)

		q.Push(player)
		go listen(q, player)
	}

}

// listen reads every message a player sends over their queue socket for the
// life of the connection: "leave" while queued, and "accept"/"decline" once
// the matcher has proposed a match and is waiting via Player.AwaitDecision.
// A read error or close is delivered as "disconnect" in case a decision is
// pending at the time.
func listen(q *matchmaking.Queue, player *matchmaking.Player) {
	for {
		_, payload, err := player.Conn().ReadMessage()
		if err != nil {
			// connection closed/errored — treat as an implicit leave
			q.Remove(player.PlayerID())
			player.Deliver("disconnect")
			return
		}

		var msg struct {
			Action string `json:"action"`
		}
		if err := json.Unmarshal(payload, &msg); err != nil {
			continue
		}

		switch msg.Action {
		case "leave":
			q.Remove(player.PlayerID())
			player.Conn().Close()
		case "accept", "decline":
			player.Deliver(msg.Action)
		}
	}
}
