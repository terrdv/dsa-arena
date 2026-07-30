package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/terrdv/dsa-arena/server/internal/matchmaking"
	"github.com/terrdv/dsa-arena/server/internal/submission"
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

// sessionServerMsg is what the server sends back. Not every field is used
// by every message type:
//   - {"type":"judging"}                                            — ack that a submission started running
//   - {"type":"result","passed":n,"total":n,"failed":["..."]}       — sent to the submitter
//   - {"type":"opponent_result","passed":n,"total":n}               — sent to the other player, no code/failure detail
//   - {"type":"error","message":"..."}                              — e.g. unsupported language
type sessionServerMsg struct {
	Type    string   `json:"type"`
	Passed  int      `json:"passed,omitempty"`
	Total   int      `json:"total,omitempty"`
	Failed  []string `json:"failed,omitempty"`
	Message string   `json:"message,omitempty"`
}

// RoomSession is the websocket endpoint a room's two players hold open for
// the life of the match: it runs submissions through the judge, reporting
// pass/fail counts back to the submitter and a lightweight progress ping to
// their opponent.
//
//  6. hub.Join(matchID, playerID, conn); defer hub.Leave(matchID, playerID, conn).
//  7. Loop on conn.ReadMessage(), json.Unmarshal into sessionClientMsg,
//     and dispatch on msg.Type:
//     - "submit": send "judging", call submission.Judge(ctx, sub, tests),
//       send "result" to the sender via conn.WriteJSON, then
//       hub.BroadcastExcept(matchID, playerID, opponentMsg) so the other
//       player sees a lightweight progress update.
//     Return from the loop (closing the connection) on any read error —
//     that's the disconnect path, same as JoinQueue's listenForLeave.
func RoomSession(rooms *matchmaking.RoomStore, hub *matchmaking.Hub) http.HandlerFunc {
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

				result, err := submission.Judge(r.Context(), sub, tests)
				if err != nil {
					if err := conn.WriteJSON(sessionServerMsg{Type: "error", Message: err.Error()}); err != nil {
						return
					}
					continue
				}

				if err := conn.WriteJSON(sessionServerMsg{
					Type:   "result",
					Passed: len(result.Passed),
					Total:  len(tests),
					Failed: result.Failed,
				}); err != nil {
					return
				}

				hub.BroadcastExcept(matchID, playerID, sessionServerMsg{
					Type:   "opponent_result",
					Passed: len(result.Passed),
					Total:  len(tests),
				})
			}
		}
	}
}
