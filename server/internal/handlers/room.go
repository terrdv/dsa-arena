package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/terrdv/dsa-arena/server/internal/matchmaking"
)

// GetRoom returns the live room state for a match so the client can render
// the problem and players after being told its match_id over the queue socket.
func GetRoom(rooms *matchmaking.RoomStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		matchID := r.PathValue("match_id")
		if matchID == "" {
			http.Error(w, "match_id is required", http.StatusBadRequest)
			return
		}

		room, err := rooms.Get(r.Context(), matchID)
		if err != nil {
			log.Println("get room:", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if room == nil {
			http.Error(w, "room not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(room); err != nil {
			log.Println("encode room:", err)
		}
	}
}
