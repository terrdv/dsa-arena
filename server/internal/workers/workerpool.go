package workers

import (
	"context"

	"github.com/terrdv/dsa-arena/server/internal/matchmaking"
	"github.com/terrdv/dsa-arena/server/internal/submission"
)

// CodeTask is one player's submission, ready to be judged. It carries its
// own delivery mechanism (hub/conn) so a worker never needs to report a
// result back to anything else — processTask handles the whole
// judge-then-deliver lifecycle itself.
type CodeTask struct {
	matchID  string
	playerID string
	sub      submission.Submission
	tests    []submission.TestCase
	hub      *matchmaking.Hub
	conn     *matchmaking.Conn
}

func NewCodeTask(matchID, playerID string, sub submission.Submission, tests []submission.TestCase, hub *matchmaking.Hub, conn *matchmaking.Conn) CodeTask {
	return CodeTask{
		matchID:  matchID,
		playerID: playerID,
		sub:      sub,
		tests:    tests,
		hub:      hub,
		conn:     conn,
	}
}

// resultMsg mirrors the wire shape handlers.RoomSession already sends over
// the room websocket, so a pool-judged result looks identical to the client
// as a synchronously-judged one.
type resultMsg struct {
	Type    string   `json:"type"`
	Passed  int      `json:"passed,omitempty"`
	Total   int      `json:"total,omitempty"`
	Failed  []string `json:"failed,omitempty"`
	Message string   `json:"message,omitempty"`
}

func (t *CodeTask) processTask(ctx context.Context) {
	result, err := submission.Judge(ctx, t.sub, t.tests)
	if err != nil {
		_ = t.conn.WriteJSON(resultMsg{Type: "error", Message: err.Error()})
		return
	}

	_ = t.conn.WriteJSON(resultMsg{
		Type:   "result",
		Passed: len(result.Passed),
		Total:  len(t.tests),
		Failed: result.Failed,
	})

	t.hub.BroadcastExcept(t.matchID, t.playerID, resultMsg{
		Type:   "opponent_result",
		Passed: len(result.Passed),
		Total:  len(t.tests),
	})
}

// WorkerPool runs CodeTasks across a fixed number of worker goroutines,
// bounding how many judge containers can run concurrently.
type WorkerPool struct {
	concurrency int
	tasksChan   chan CodeTask
}

func NewWorkerPool(concurrency int) *WorkerPool {
	return &WorkerPool{concurrency: concurrency}
}

// Start spins up the pool's worker goroutines. Call it once, before Submit.
func (wp *WorkerPool) Start() {
	wp.tasksChan = make(chan CodeTask, wp.concurrency)
	for i := 0; i < wp.concurrency; i++ {
		go wp.worker()
	}
}

// Submit queues task for a worker to pick up. It blocks once every worker is
// busy and the channel is full, applying backpressure to the caller rather
// than accepting unbounded work.
func (wp *WorkerPool) Submit(task CodeTask) {
	wp.tasksChan <- task
}

func (wp *WorkerPool) worker() {
	for task := range wp.tasksChan {
		task.processTask(context.Background())
	}
}
