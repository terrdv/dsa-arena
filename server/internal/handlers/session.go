package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/terrdv/dsa-arena/server/internal/matchmaking"
	"github.com/terrdv/dsa-arena/server/internal/submission"
	"github.com/terrdv/dsa-arena/server/internal/workers"
)

const defaultLanguage = "python"

// sessionClientMsg is what the browser sends over the room session socket.
// There's only one message shape today:
//   - {"type":"submit","code":"...","language":"python"} — run the judge
//
// In-progress drafts are kept client-side (localStorage) and never sent to
// the server until submit — nothing server-side needs to see them before
// then, so there's no separate "code" draft message.
type sessionClientMsg struct {
	Type     string `json:"type"`
	Code     string `json:"code"`
	Language string `json:"language"`
}

// sessionServerMsg is what the server sends back over this handler
// directly. The judge's own results ({"type":"result"/"opponent_result"} to
// the submitter/opponent, {"type":"error"} on a judge failure) are sent by
// the worker pool once it processes the submission — see workers.CodeTask.
type sessionServerMsg struct {
	Type    string `json:"type"`
	Message string `json:"message,omitempty"`
}

// RoomSession is the websocket endpoint a room's two players hold open for
// the life of the match: it queues submissions onto the judge worker pool,
// which reports pass/fail counts back to the submitter and a lightweight
// progress ping to their opponent once judging finishes.
func RoomSession(rooms *matchmaking.RoomStore, hub *matchmaking.Hub, pool *workers.WorkerPool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		matchID := r.PathValue("match_id")
		if matchID == "" {
			http.Error(w, "match_id is required", http.StatusBadRequest)
			return
		}

		playerID := r.URL.Query().Get("player_id")
		if playerID == "" {
			http.Error(w, "player_id is required", http.StatusBadRequest)
			return
		}

		room, err := rooms.Get(r.Context(), matchID)

		if err != nil {
			http.Error(w, "failed to get room", http.StatusInternalServerError)
			return
		}
		if room == nil {
			http.Error(w, "room not found", http.StatusNotFound)
			return
		}

		if playerID != room.Player1ID && playerID != room.Player2ID {
			http.Error(w, "not permitted to join this match", http.StatusForbidden)
			return
		}

		var tests []submission.TestCase
		if err := json.Unmarshal(room.Testcases, &tests); err != nil {
			http.Error(w, "failed to parse test cases", http.StatusInternalServerError)
			return
		}

		ws, err := handleWebSocket(w, r)
		if err != nil {
			log.Println("room session handshake failed:", err)
			return
		}
		conn := matchmaking.NewConn(ws)

		hub.Join(matchID, playerID, conn)
		defer hub.Leave(matchID, playerID, conn)

		for {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}

			var msg sessionClientMsg
			if err := json.Unmarshal(payload, &msg); err != nil {
				continue
			}

			switch msg.Type {
			case "submit":
				if err := conn.WriteJSON(sessionServerMsg{Type: "judging"}); err != nil {
					return
				}

				sub := submission.Submission{
					Player:   playerID,
					Match:    matchID,
					Code:     msg.Code,
					Language: msg.Language,
				}

				pool.Submit(workers.NewCodeTask(matchID, playerID, sub, tests, hub, conn))
			}
		}
	}
}
