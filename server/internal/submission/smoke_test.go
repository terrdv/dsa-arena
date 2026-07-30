package submission

import (
	"context"
	"testing"
	"time"
)

// Smoke test for the batched-container rewrite — hits real Docker. Not meant
// to stay in the suite long-term; just here to confirm the harness script
// actually runs correctly end to end.
func TestJudgeSmoke(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	sub := Submission{
		Player:   "alice",
		Match:    "smoke",
		Language: "python",
		Code: `def solve(n):
    return n + 1
`,
	}

	tests := []TestCase{
		{Input: "n = 2", Output: "3"},  // pass
		{Input: "n = 2", Output: "99"}, // wrong answer
	}

	result, err := Judge(ctx, sub, tests)
	if err != nil {
		t.Fatalf("Judge returned error: %v", err)
	}
	t.Logf("Passed: %v", result.Passed)
	t.Logf("Failed: %v", result.Failed)

	if len(result.Passed) != 1 {
		t.Errorf("expected 1 pass, got %d", len(result.Passed))
	}
	if len(result.Failed) != 1 {
		t.Errorf("expected 1 failure, got %d", len(result.Failed))
	}
}

func TestJudgeSmokeCrash(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	sub := Submission{
		Player:   "bob",
		Match:    "smoke",
		Language: "python",
		Code:     `this is not valid python`,
	}

	tests := []TestCase{
		{Input: "n = 2", Output: "3"},
		{Input: "n = 3", Output: "4"},
	}

	result, err := Judge(ctx, sub, tests)
	if err != nil {
		t.Fatalf("Judge returned error: %v", err)
	}
	t.Logf("Passed: %v", result.Passed)
	t.Logf("Failed: %v", result.Failed)

	if len(result.Passed) != 0 {
		t.Errorf("expected 0 passes for a crashing submission, got %d", len(result.Passed))
	}
	if len(result.Failed) != 2 {
		t.Errorf("expected both cases marked failed, got %d", len(result.Failed))
	}
}

func TestJudgeSmokeTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	sub := Submission{
		Player:   "carol",
		Match:    "smoke",
		Language: "python",
		Code: `def solve(n):
    while True:
        pass
`,
	}

	// Use a short per-test window by only having one test — still bounded by
	// the real perTestTimeout constant, so this test just confirms the
	// signal.alarm path fires and reports a timeout instead of hanging
	// forever or crashing the whole run.
	tests := []TestCase{
		{Input: "n = 2", Output: "3"},
	}

	result, err := Judge(ctx, sub, tests)
	if err != nil {
		t.Fatalf("Judge returned error: %v", err)
	}
	t.Logf("Failed: %v", result.Failed)

	if len(result.Failed) != 1 {
		t.Fatalf("expected 1 failed (timeout) case, got %d", len(result.Failed))
	}
}
