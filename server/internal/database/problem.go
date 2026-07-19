package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)


type Problem struct {
	ID        int64
	
	CreatedAt time.Time
}


func GetProblem(ctx context.Context, db *sql.DB, playerID string) ([]Match, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, player1_id, player2_id, winner_id, status, created_at
		FROM matches
		WHERE player1_id = $1 OR player2_id = $1
		ORDER BY created_at DESC
	`, playerID)
	if err != nil {
		return nil, fmt.Errorf("query matches for player %s: %w", playerID, err)
	}
	defer rows.Close()

	var matches []Match
	for rows.Next() {
		var m Match
		if err := rows.Scan(&m.ID, &m.Player1ID, &m.Player2ID, &m.WinnerID, &m.Status, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan match: %w", err)
		}
		matches = append(matches, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate matches: %w", err)
	}

	return matches, nil
}