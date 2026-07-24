// Command seed loads the LeetCodeDataset JSONL files and inserts them into
// the postgres `problems` table (id, title, problem_description, testcases).
//
// Usage:
//
//	DATABASE_URL=postgres://... go run . [jsonl files...]
//
// If no files are given, it defaults to the two dataset files under
// ./leetcode-data.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type record struct {
	TaskID             string          `json:"task_id"`
	QuestionID         int64           `json:"question_id"`
	ProblemDescription string          `json:"problem_description"`
	InputOutput        json.RawMessage `json:"input_output"`
}

func main() {
	files := os.Args[1:]
	if len(files) == 0 {
		files = []string{
			"leetcode-data/LeetCodeDataset-train.jsonl",
			"leetcode-data/LeetCodeDataset-test.jsonl",
		}
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	var inserted int
	for _, path := range files {
		n, err := seedFile(ctx, pool, path)
		if err != nil {
			log.Fatalf("seed %s: %v", path, err)
		}
		inserted += n
		log.Printf("%s: inserted/updated %d problems", path, n)
	}

	log.Printf("done: %d problems total", inserted)
}

func seedFile(ctx context.Context, pool *pgxpool.Pool, path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	var count int
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var rec record
		if err := json.Unmarshal(line, &rec); err != nil {
			return count, err
		}

		_, err := tx.Exec(ctx, `
			INSERT INTO problems (id, title, problem_description, testcases)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (id) DO UPDATE SET
				title = EXCLUDED.title,
				problem_description = EXCLUDED.problem_description,
				testcases = EXCLUDED.testcases
		`, rec.QuestionID, titleFromTaskID(rec.TaskID), rec.ProblemDescription, rec.InputOutput)
		if err != nil {
			return count, err
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		return count, err
	}

	return count, tx.Commit(ctx)
}

// titleFromTaskID turns a slug like "shortest-distance-after-road-addition"
// into "Shortest Distance After Road Addition".
func titleFromTaskID(taskID string) string {
	words := strings.Split(taskID, "-")
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
