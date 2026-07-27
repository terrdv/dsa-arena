package submission

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Submission is a player's code for a match's problem, ready to be judged.
type Submission struct {
	Player   string
	Match    string
	Code     string
	Language string
}

type TestCase struct {
	Input  string
	Output string
}

type Result struct {
	Passed []string
	Failed []string
	Err    error
}

const (
	judgeImage   = "python:3.12-slim"
	judgeTimeout = 10 * time.Second
	judgeMemory  = "256m"
	judgeCPUs    = "0.5"

	passMarker = "__JUDGE_PASS__"
	failMarker = "__JUDGE_FAIL__"
)

var (
	pullOnce sync.Once
	pullErr  error
)

// ensureImage pulls judgeImage the first time it's needed so that a cold
// image cache doesn't eat into a test case's execution timeout: without
// this, the first `docker run` per process would spend its whole budget
// downloading layers instead of running code.
func ensureImage(ctx context.Context) error {
	pullOnce.Do(func() {
		pullCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		pullErr = exec.CommandContext(pullCtx, "docker", "pull", judgeImage).Run()
	})
	return pullErr
}

// Judge runs each test case for sub in its own throwaway Docker container
// and reports which passed. Submissions must define a top-level `solve`
// function; each test case's input is exec'd as Python assignments (e.g.
// "n = 7, queries = [[0,5]]") and passed to solve as keyword arguments, and
// the result is compared against the eval'd output.
func Judge(ctx context.Context, sub Submission, tests []TestCase) (Result, error) {
	if sub.Language != "" && sub.Language != "python" {
		return Result{}, fmt.Errorf("submission: unsupported language %q", sub.Language)
	}

	if err := ensureImage(ctx); err != nil {
		return Result{}, fmt.Errorf("submission: pull judge image: %w", err)
	}

	var result Result
	for i, tc := range tests {
		label := fmt.Sprintf("case %d", i+1)
		if passed, detail := runTestCase(ctx, sub.Code, tc); passed {
			result.Passed = append(result.Passed, label)
		} else {
			result.Failed = append(result.Failed, fmt.Sprintf("%s: %s", label, detail))
		}
	}

	return result, nil
}

// runTestCase spins up a single throwaway container, runs the judge harness
// inside it, and tears it down. --rm removes the container on normal exit;
// on timeout we explicitly force-remove it by name, since cancelling the
// local docker CLI process kills the client but not the container running
// on the daemon side.
func runTestCase(ctx context.Context, code string, tc TestCase) (passed bool, detail string) {
	script := harnessScript(code, tc.Input, tc.Output)
	name := containerName()

	runCtx, cancel := context.WithTimeout(ctx, judgeTimeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "docker", "run",
		"--rm",
		"--name", name,
		"--network", "none",
		"--memory", judgeMemory,
		"--cpus", judgeCPUs,
		"-i",
		judgeImage,
		"python3", "-c", script,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	if runCtx.Err() == context.DeadlineExceeded {
		removeContainer(name)
		return false, "timed out"
	}

	out := stdout.String()
	switch {
	case strings.Contains(out, passMarker):
		return true, ""
	case strings.Contains(out, failMarker):
		got := strings.TrimSpace(strings.SplitN(out, failMarker, 2)[1])
		return false, fmt.Sprintf("got %s", got)
	default:
		msg := strings.TrimSpace(stderr.String())
		if msg == "" && runErr != nil {
			msg = runErr.Error()
		}
		if msg == "" {
			msg = "no output from judge harness"
		}
		return false, msg
	}
}

// removeContainer force-stops and removes a container by name. It's the
// cleanup fallback for the timeout path, covering containers that are
// still running or were never fully started.
func removeContainer(name string) {
	killCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = exec.CommandContext(killCtx, "docker", "rm", "-f", name).Run()
}

func containerName() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "judge-" + hex.EncodeToString(b)
}

// harnessScript builds a self-contained Python program that defines the
// candidate's solve function, feeds it one test case's input as keyword
// arguments, and prints a marker line reporting pass/fail. Code, input, and
// output are base64-encoded so they can be embedded as Python string
// literals without any quoting/escaping concerns.
//
// Test case input is stored as a bare kwargs-style string, e.g.
// "n = 7, queries = [[0,5],[1,6]]" — which is not valid Python on its own
// (top-level commas collide with chained-assignment syntax). Wrapping it in
// dict(...) reuses Python's own keyword-argument parser to turn it into a
// dict without hand-rolling a parser for nested lists/dicts in Go.
func harnessScript(code, input, output string) string {
	return fmt.Sprintf(harnessTemplate,
		base64.StdEncoding.EncodeToString([]byte(code)),
		base64.StdEncoding.EncodeToString([]byte(input)),
		base64.StdEncoding.EncodeToString([]byte(output)),
		passMarker,
		failMarker,
	)
}

const harnessTemplate = `import base64

exec(base64.b64decode("%s").decode())

_args = eval("dict(" + base64.b64decode("%s").decode() + ")")
_result = solve(**_args)
_expected = eval(base64.b64decode("%s").decode())

if _result == _expected:
    print("%s")
else:
    print("%s")
    print(repr(_result))
`
