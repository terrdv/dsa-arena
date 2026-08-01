package matchmaking

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log"
	"strconv"
	"time"

	"github.com/gorilla/websocket"

	"github.com/terrdv/dsa-arena/server/internal/database"
)

// acceptTimeout is how long a proposed match waits for both players to
// accept before treating a silent player as having declined.
const acceptTimeout = 5 * time.Second

func RunMatcher(ctx context.Context, q *Queue, rooms *RoomStore, db *sql.DB) {
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

		go proposeMatch(ctx, q, rooms, db, player1, player2)
	}
}

// proposeMatch picks a problem for player1/player2 and asks both to accept
// over their queue sockets, creating the room only once both have. It runs
// in its own goroutine per pairing so a slow responder doesn't stall
// matching for everyone else still in the queue.
func proposeMatch(ctx context.Context, q *Queue, rooms *RoomStore, db *sql.DB, player1, player2 *Player) {
	problem, err := database.RandomProblem(ctx, db)
	if err != nil {
		log.Println("pick random problem:", err)
		q.Push(player1)
		q.Push(player2)
		return
	}

	matchID := newID()

	decisions1 := player1.AwaitDecision()
	decisions2 := player2.AwaitDecision()
	defer player1.ClearPending()
	defer player2.ClearPending()

	found := &MatchmakingPayload{Type: "match_found", MatchID: matchID}
	if err := send(player1, found); err != nil {
		q.Push(player2)
		return
	}
	if err := send(player2, found); err != nil {
		q.Push(player1)
		return
	}

	decision1, decision2, ok := awaitBothDecisions(ctx, decisions1, decisions2)
	if !ok {
		return // shutting down
	}

	if decision1 == "accept" && decision2 == "accept" {
		room := &Room{
			MatchID:            matchID,
			Player1ID:          player1.PlayerID(),
			Player2ID:          player2.PlayerID(),
			ProblemID:          strconv.FormatInt(problem.ID, 10),
			ProblemTitle:       problem.Title,
			ProblemDescription: problem.ProblemDescription,
			Testcases:          json.RawMessage(problem.Testcases),
			Status:             "active",
			StartedAt:          time.Now(),
		}

		if err := rooms.Save(ctx, room); err != nil {
			log.Println("save room:", err)
			_ = send(player1, &MatchmakingPayload{Type: "match_cancelled", Reason: "server_error"})
			_ = send(player2, &MatchmakingPayload{Type: "match_cancelled", Reason: "server_error"})
			return
		}

		ready := &MatchmakingPayload{Type: "match_ready", MatchID: matchID}
		_ = send(player1, ready)
		_ = send(player2, ready)
		return
	}

	resolveDeclinedMatch(q, player1, decision1, decision2)
	resolveDeclinedMatch(q, player2, decision2, decision1)
}

// awaitBothDecisions waits for both players' accept/decline, the accept
// timeout, or ctx cancellation. A player who never responds is recorded as
// "timeout". ok is false only when ctx is cancelled mid-wait.
func awaitBothDecisions(ctx context.Context, ch1, ch2 <-chan string) (decision1, decision2 string, ok bool) {
	timer := time.NewTimer(acceptTimeout)
	defer timer.Stop()

	for decision1 == "" || decision2 == "" {
		select {
		case d := <-ch1:
			decision1 = d
		case d := <-ch2:
			decision2 = d
		case <-timer.C:
			if decision1 == "" {
				decision1 = "timeout"
			}
			if decision2 == "" {
				decision2 = "timeout"
			}
		case <-ctx.Done():
			return "", "", false
		}
	}
	return decision1, decision2, true
}

// resolveDeclinedMatch tells player what became of a match that didn't get
// both accepts. A player who accepted but whose opponent didn't goes back
// into the queue automatically; a player who declined or timed out isn't
// requeued — they'll have to search again if they want another match.
func resolveDeclinedMatch(q *Queue, player *Player, myDecision, theirDecision string) {
	switch myDecision {
	case "accept":
		_ = send(player, &MatchmakingPayload{Type: "match_cancelled", Reason: cancelReason(theirDecision)})
		q.Push(player)
	case "timeout":
		_ = send(player, &MatchmakingPayload{Type: "match_cancelled", Reason: "timeout"})
	case "decline", "disconnect":
		// player already knows they declined, or their connection is dead —
		// nothing to notify either way.
	}
}

func cancelReason(theirDecision string) string {
	switch theirDecision {
	case "decline":
		return "opponent_declined"
	case "timeout":
		return "opponent_timeout"
	default:
		return "opponent_disconnected"
	}
}

func send(player *Player, mp *MatchmakingPayload) error {
	payload, err := json.Marshal(mp)
	if err != nil {
		log.Println("marshal matchmaking payload:", err)
		return err
	}
	if err := player.Conn().WriteMessage(websocket.TextMessage, payload); err != nil {
		log.Println("write matchmaking payload:", err)
		return err
	}
	return nil
}

func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
