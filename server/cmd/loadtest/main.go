// Command loadtest drives the real matchmaking -> room -> submit flow with
// many concurrent simulated players, to measure end-to-end submit latency
// (client send -> client receive) and how it behaves as concurrency exceeds
// the judge worker pool's size.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// judgeCode is submitted by every simulated player. It's deliberately
// trivial and always correct against loadTestProblemID's test cases, since
// this test measures latency/throughput, not judge correctness.
const judgeCode = "def solve(n):\n    return n + 1\n"

const loadTestProblemID = 999999001

func seedProblem(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	const testcases = `[{"input":"n = 1","output":"2"},{"input":"n = 41","output":"42"}]`
	_, err = db.ExecContext(ctx, `
		INSERT INTO problems (id, title, problem_description, testcases)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO NOTHING
	`, loadTestProblemID, "Load Test: Increment", "Return n + 1.", testcases)
	if err != nil {
		return fmt.Errorf("insert problem: %w", err)
	}
	return nil
}

// queueServerMsg / queueClientMsg mirror matchmaking.MatchmakingPayload and
// the {"action": "..."} shape handled by internal/handlers/matchmaking.go.
type queueServerMsg struct {
	Type    string `json:"type"`
	MatchID string `json:"match_id,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

type queueClientMsg struct {
	Action string `json:"action"`
}

// sessionClientMsg / sessionServerMsg mirror the shapes in
// internal/handlers/session.go and workers.resultMsg.
type sessionClientMsg struct {
	Type     string `json:"type"`
	Code     string `json:"code"`
	Language string `json:"language"`
}

type sessionServerMsg struct {
	Type    string   `json:"type"`
	Passed  int      `json:"passed,omitempty"`
	Total   int      `json:"total,omitempty"`
	Failed  []string `json:"failed,omitempty"`
	Message string   `json:"message,omitempty"`
}

// collector gathers latency samples from every player goroutine. A plain
// mutex-guarded slice, rather than a fixed-capacity channel, since in
// -duration mode nothing drains samples until every player finishes at the
// end of the run — a channel would need its full-run sample count guessed
// upfront or risk deadlocking on a full buffer.
type collector struct {
	mu  sync.Mutex
	lat []time.Duration
}

func (c *collector) add(d time.Duration) {
	c.mu.Lock()
	c.lat = append(c.lat, d)
	c.mu.Unlock()
}

// runPlayer drives one simulated player through the full flow: join the
// queue, accept the proposed match, connect to the room session, then submit
// judgeCode repeatedly, recording end-to-end (send -> result) latency for
// each.
//
// stagger delays the player's own start by a random amount in [0, stagger),
// so concurrent matches don't all begin in the same instant. Once judging,
// the player either submits a fixed `submits` times back-to-back (runFor
// == 0), or keeps submitting with a random think-time pause in
// [thinkMin, thinkMax) between each submission until runFor has elapsed
// (open-loop, sustained-load mode).
func runPlayer(wsBase, playerID string, submits int, runFor, stagger, thinkMin, thinkMax time.Duration, rec *collector, errs *int64) {
	if stagger > 0 {
		time.Sleep(time.Duration(rand.Int63n(int64(stagger))))
	}

	queueURL := wsBase + "/queue?player_id=" + url.QueryEscape(playerID)
	qc, _, err := websocket.DefaultDialer.Dial(queueURL, nil)
	if err != nil {
		log.Printf("%s: queue dial: %v", playerID, err)
		atomic.AddInt64(errs, 1)
		return
	}

	var matchID string
	for matchID == "" {
		var msg queueServerMsg
		if err := qc.ReadJSON(&msg); err != nil {
			log.Printf("%s: queue read: %v", playerID, err)
			atomic.AddInt64(errs, 1)
			qc.Close()
			return
		}

		switch msg.Type {
		case "match_found":
			if err := qc.WriteJSON(queueClientMsg{Action: "accept"}); err != nil {
				log.Printf("%s: accept write: %v", playerID, err)
				atomic.AddInt64(errs, 1)
				qc.Close()
				return
			}
		case "match_ready":
			matchID = msg.MatchID
		case "match_cancelled":
			log.Printf("%s: match cancelled: %s", playerID, msg.Reason)
			atomic.AddInt64(errs, 1)
			qc.Close()
			return
		}
	}
	qc.Close()

	sessionURL := wsBase + "/room/" + matchID + "/session?player_id=" + url.QueryEscape(playerID)
	sc, _, err := websocket.DefaultDialer.Dial(sessionURL, nil)
	if err != nil {
		log.Printf("%s: session dial: %v", playerID, err)
		atomic.AddInt64(errs, 1)
		return
	}
	defer sc.Close()

	deadline := time.Now().Add(runFor)

	for i := 0; ; i++ {
		if runFor > 0 {
			if time.Now().After(deadline) {
				return
			}
		} else if i >= submits {
			return
		}

		start := time.Now()
		if err := sc.WriteJSON(sessionClientMsg{Type: "submit", Code: judgeCode, Language: "python"}); err != nil {
			log.Printf("%s: submit write: %v", playerID, err)
			atomic.AddInt64(errs, 1)
			return
		}

	readLoop:
		for {
			var msg sessionServerMsg
			if err := sc.ReadJSON(&msg); err != nil {
				log.Printf("%s: session read: %v", playerID, err)
				atomic.AddInt64(errs, 1)
				return
			}

			switch msg.Type {
			case "result":
				rec.add(time.Since(start))
				break readLoop
			case "error":
				log.Printf("%s: judge error: %s", playerID, msg.Message)
				atomic.AddInt64(errs, 1)
				break readLoop
			// "judging" and "opponent_result" (the latter triggered by the
			// other player in this match, arriving at arbitrary times) are
			// not the response to our own submit — keep reading.
			default:
				continue
			}
		}

		if thinkMax > 0 {
			think := thinkMin
			if thinkMax > thinkMin {
				think += time.Duration(rand.Int63n(int64(thinkMax - thinkMin)))
			}
			time.Sleep(think)
		}
	}
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(p / 100 * float64(len(sorted)-1))
	return sorted[idx]
}

func main() {
	addr := flag.String("addr", "localhost:8080", "server host:port")
	matches := flag.Int("matches", 4, "number of concurrent simulated matches (2 players each)")
	submits := flag.Int("submits", 10, "submissions per player, sent back-to-back (ignored if -duration is set)")
	dsn := flag.String("dsn", os.Getenv("DATABASE_URL"), "Postgres DSN used to seed a test problem (defaults to $DATABASE_URL)")
	runFor := flag.Duration("duration", 0, "if set, run as sustained/open-loop load for this long instead of a fixed -submits count per player (e.g. 5m)")
	stagger := flag.Duration("stagger", 0, "max random delay before each simulated player starts, spreading match starts over this window instead of firing all at once (e.g. 30s)")
	thinkMin := flag.Duration("think-min", 0, "minimum random pause between a player's submissions")
	thinkMax := flag.Duration("think-max", 0, "maximum random pause between a player's submissions; 0 means back-to-back (closed-loop)")
	flag.Parse()

	if *dsn != "" {
		if err := seedProblem(context.Background(), *dsn); err != nil {
			log.Fatalf("seed problem: %v", err)
		}
	} else {
		log.Println("no -dsn/$DATABASE_URL set, skipping problem seed — make sure the problems table already has at least one row")
	}

	wsBase := "ws://" + *addr
	players := *matches * 2

	rec := &collector{}
	var errs int64
	var wg sync.WaitGroup

	if *runFor > 0 {
		log.Printf("starting: %d concurrent matches (%d players), sustained for %s, stagger up to %s, think-time [%s, %s)",
			*matches, players, *runFor, *stagger, *thinkMin, *thinkMax)
	} else {
		log.Printf("starting: %d concurrent matches (%d players), %d submits/player, stagger up to %s, think-time [%s, %s)",
			*matches, players, *submits, *stagger, *thinkMin, *thinkMax)
	}

	start := time.Now()
	for i := 0; i < players; i++ {
		wg.Add(1)
		playerID := fmt.Sprintf("loadtest-%d", i)
		go func() {
			defer wg.Done()
			runPlayer(wsBase, playerID, *submits, *runFor, *stagger, *thinkMin, *thinkMax, rec, &errs)
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	latencies := rec.lat
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	completed := len(latencies)

	fmt.Println()
	fmt.Println("=== Results ===")
	if *runFor > 0 {
		fmt.Printf("matches: %d  players: %d  sustained: %s\n", *matches, players, *runFor)
		fmt.Printf("completed: %d  errors: %d\n", completed, atomic.LoadInt64(&errs))
	} else {
		total := players * *submits
		fmt.Printf("matches: %d  players: %d  submits/player: %d\n", *matches, players, *submits)
		fmt.Printf("completed: %d/%d  errors: %d\n", completed, total, atomic.LoadInt64(&errs))
	}
	fmt.Printf("wall time: %s  throughput: %.2f submits/sec\n", elapsed, float64(completed)/elapsed.Seconds())
	if completed > 0 {
		fmt.Printf("latency  p50: %s  p95: %s  p99: %s  max: %s\n",
			percentile(latencies, 50), percentile(latencies, 95), percentile(latencies, 99), latencies[len(latencies)-1])
	}
}
