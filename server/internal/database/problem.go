package database

import (
	"context"
	"database/sql"
	"fmt"
)


type Problem struct {
	ID                  int64
	Title               string
	ProblemDescription  string
	Testcases           []byte
}

func GetProblem(ctx context.Context, db *sql.DB, id int64) (Problem, error) {
	var p Problem
	err := db.QueryRowContext(ctx, `
		SELECT id, title, problem_description, testcases
		FROM problems
		WHERE id = $1
	`, id).Scan(&p.ID, &p.Title, &p.ProblemDescription, &p.Testcases)
	if err != nil {
		return Problem{}, fmt.Errorf("query problem %d: %w", id, err)
	}

	return p, nil
}

// RandomProblem returns a uniformly random problem from the problems table.
func RandomProblem(ctx context.Context, db *sql.DB) (Problem, error) {
	var p Problem
	err := db.QueryRowContext(ctx, `
		SELECT id, title, problem_description, testcases
		FROM problems
		ORDER BY random()
		LIMIT 1
	`).Scan(&p.ID, &p.Title, &p.ProblemDescription, &p.Testcases)
	if err != nil {
		return Problem{}, fmt.Errorf("query random problem: %w", err)
	}

	return p, nil
}