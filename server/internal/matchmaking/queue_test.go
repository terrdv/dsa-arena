package matchmaking

import (
	"testing"
	"time"
)

func TestQueuePushPop(t *testing.T) {
	q := NewQueue()

	p1 := NewPlayer("p1", nil)
	p2 := NewPlayer("p2", nil)

	q.Push(p1)
	q.Push(p2)

	got, ok := q.Pop()
	if !ok {
		t.Fatalf("expected a player, got none")
	}
	if got.PlayerID() != "p1" {
		t.Fatalf("expected p1 first (FIFO), got %s", got.PlayerID())
	}

	got, ok = q.Pop()
	if !ok {
		t.Fatalf("expected a player, got none")
	}
	if got.PlayerID() != "p2" {
		t.Fatalf("expected p2 second (FIFO), got %s", got.PlayerID())
	}
}

func TestQueuePopEmpty(t *testing.T) {
	q := NewQueue()

	_, ok := q.Pop()
	if ok {
		t.Fatalf("expected no player from empty queue")
	}
}

func TestQueueRemove(t *testing.T) {
	q := NewQueue()

	p1 := NewPlayer("p1", nil)
	p2 := NewPlayer("p2", nil)
	p3 := NewPlayer("p3", nil)

	q.Push(p1)
	q.Push(p2)
	q.Push(p3)

	removed, ok := q.Remove("p2")
	if !ok {
		t.Fatalf("expected to remove p2")
	}
	if removed.PlayerID() != "p2" {
		t.Fatalf("expected removed player to be p2, got %s", removed.PlayerID())
	}

	// remaining order should be p1, p3
	got, ok := q.Pop()
	if !ok || got.PlayerID() != "p1" {
		t.Fatalf("expected p1 next, got %v (ok=%v)", got, ok)
	}
	got, ok = q.Pop()
	if !ok || got.PlayerID() != "p3" {
		t.Fatalf("expected p3 next, got %v (ok=%v)", got, ok)
	}
}

func TestQueueRemoveMissing(t *testing.T) {
	q := NewQueue()
	q.Push(NewPlayer("p1", nil))

	_, ok := q.Remove("does-not-exist")
	if ok {
		t.Fatalf("expected Remove to fail for missing player")
	}
}

func TestQueueSignalOnPush(t *testing.T) {
	q := NewQueue()

	q.Push(NewPlayer("p1", nil))

	select {
	case <-q.Signal():
	case <-time.After(time.Second):
		t.Fatalf("expected signal after push")
	}
}

func TestQueueSignalCoalesces(t *testing.T) {
	q := NewQueue()

	// multiple pushes before the signal is drained should only buffer one signal
	q.Push(NewPlayer("p1", nil))
	q.Push(NewPlayer("p2", nil))
	q.Push(NewPlayer("p3", nil))

	select {
	case <-q.Signal():
	case <-time.After(time.Second):
		t.Fatalf("expected at least one signal")
	}

	select {
	case <-q.Signal():
		t.Fatalf("did not expect a second buffered signal")
	default:
	}
}
