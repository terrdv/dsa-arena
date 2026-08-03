package submission

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
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
	Input  string `json:"input"`
	Output string `json:"output"`
}

// CaseStatus is the outcome of a single test case.
type CaseStatus string

const (
	StatusPass    CaseStatus = "pass"
	StatusFail    CaseStatus = "fail"
	StatusError   CaseStatus = "error"
	StatusTimeout CaseStatus = "timeout"
)

// CaseResult is one test case's outcome, carrying enough detail for a client
// to show input/expected/actual side by side rather than just a pass count.
type CaseResult struct {
	Index    int        `json:"index"`
	Input    string     `json:"input"`
	Expected string     `json:"expected"`
	Actual   string     `json:"actual"`
	Passed   bool       `json:"passed"`
	Status   CaseStatus `json:"status"`
}

type Result struct {
	Cases []CaseResult
}

const (
	judgeImage = "python:3.12-slim"

	// perTestTimeout bounds a single test case's solve() call, enforced
	// inside the container via signal.alarm (see harnessTemplate) — a hang
	// on one test only fails that test instead of the whole run.
	perTestTimeout = 10 * time.Second
	// judgeStartupBudget covers container/interpreter startup and JSON
	// decoding, on top of the sum of every test's perTestTimeout, so the
	// outer docker-level timeout doesn't fire before the in-script alarms
	// get a chance to.
	judgeStartupBudget = 5 * time.Second

	judgeMemory = "256m"
	judgeCPUs   = "0.5"

	passMarker    = "__JUDGE_PASS__"
	failMarker    = "__JUDGE_FAIL__"
	errMarker     = "__JUDGE_ERROR__"
	timeoutMarker = "__JUDGE_TIMEOUT__"
)

var (
	pullMu   sync.Mutex
	pullDone bool
)

// ensureImage pulls judgeImage the first time it's needed so that a cold
// image cache doesn't eat into a test case's execution timeout: without
// this, the first `docker run` per process would spend its whole budget
// downloading layers instead of running code. A failed pull is not cached —
// only success is — so a transient failure (daemon not up yet, network
// blip) doesn't permanently break every submission for the rest of the
// process's life; the next submission just retries the pull.
func ensureImage(ctx context.Context) error {
	pullMu.Lock()
	defer pullMu.Unlock()

	if pullDone {
		return nil
	}

	pullCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	var errBuf bytes.Buffer
	cmd := exec.CommandContext(pullCtx, "docker", "pull", judgeImage)
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errBuf.String())
		if msg == "" {
			return err
		}
		return fmt.Errorf("%s: %s", err, msg)
	}

	pullDone = true
	return nil
}

// Judge runs every test case for sub in a single throwaway Docker container
// and reports which passed. Submissions must define a top-level `solve`
// function; each test case's input is exec'd as Python assignments (e.g.
// "n = 7, queries = [[0,5]]") and passed to solve as keyword arguments, and
// the result is compared against the eval'd output.
//
// Batching every test into one container (rather than one container per
// test) avoids paying container-startup cost N times and avoids re-running
// (and re-crashing on) the candidate's code from scratch per test — a
// syntax error, for instance, now surfaces once instead of once per test
// case. Per-test isolation is preserved inside the harness script itself:
// each test runs in its own try/except under a signal.alarm timeout, so one
// bad test can't take down the rest of the batch.
func Judge(ctx context.Context, sub Submission, tests []TestCase) (Result, error) {
	if sub.Language != "" && sub.Language != "python" {
		return Result{}, fmt.Errorf("submission: unsupported language %q", sub.Language)
	}

	if err := ensureImage(ctx); err != nil {
		return Result{}, fmt.Errorf("submission: pull judge image: %w", err)
	}

	if len(tests) == 0 {
		return Result{}, nil
	}

	stdout, crashDetail, err := runSubmission(ctx, sub.Code, tests)
	if err != nil {
		return Result{}, err
	}

	return parseJudgeOutput(stdout, crashDetail, tests), nil
}

// runSubmission spins up a single throwaway container running every test
// case for code, and tears it down. --rm removes the container on normal
// exit; on an overall timeout we explicitly force-remove it by name, since
// cancelling the local docker CLI process kills the client but not the
// container running on the daemon side.
//
// A nonzero exit from a well-formed run means the candidate's code itself
// blew up outside the per-test try/except (e.g. a syntax error, or an
// exception raised while defining top-level code) — that's reported back as
// crashDetail, not as an error, since it's the candidate's problem, not an
// infrastructure one. err is reserved for genuine infra failures (docker
// missing, container never started).
func runSubmission(ctx context.Context, code string, tests []TestCase) (stdout, crashDetail string, err error) {
	script, err := harnessScript(code, tests)
	if err != nil {
		return "", "", fmt.Errorf("submission: build harness: %w", err)
	}

	name := containerName()
	timeout := judgeStartupBudget + time.Duration(len(tests))*perTestTimeout

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "docker", "run",
		"--rm",
		"--name", name,
		"--network", "none",
		"--memory", judgeMemory,
		"--cpus", judgeCPUs,
		judgeImage,
		"python3", "-c", script,
	)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	runErr := cmd.Run()

	if runCtx.Err() == context.DeadlineExceeded {
		removeContainer(name)
		return outBuf.String(), "judge run timed out", nil
	}

	var exitErr *exec.ExitError
	if runErr != nil && !errors.As(runErr, &exitErr) {
		return "", "", fmt.Errorf("submission: run judge container: %w", runErr)
	}

	if runErr != nil {
		msg := strings.TrimSpace(errBuf.String())
		if msg == "" {
			msg = runErr.Error()
		}
		return outBuf.String(), msg, nil
	}

	return outBuf.String(), "", nil
}

// parseJudgeOutput turns the harness's stdout (one result line per test
// case it reached, in order) into a Result covering every test, each paired
// with its original input/expected output. Any test index the harness never
// reported — because the whole run crashed or timed out before getting to
// it — is filled in with crashDetail so every test still gets an entry.
func parseJudgeOutput(stdout, crashDetail string, tests []TestCase) Result {
	type parsedCase struct {
		kind   string
		detail string
	}
	n := len(tests)
	found := make(map[int]parsedCase, n)

	for line := range strings.SplitSeq(stdout, "\n") {
		fields := strings.SplitN(strings.TrimSpace(line), " ", 3)
		if len(fields) < 2 {
			continue
		}

		idx, err := strconv.Atoi(fields[1])
		if err != nil || idx < 0 || idx >= n {
			continue
		}

		detail := ""
		if len(fields) == 3 {
			if data, err := base64.StdEncoding.DecodeString(fields[2]); err == nil {
				detail = string(data)
			}
		}
		found[idx] = parsedCase{kind: fields[0], detail: detail}
	}

	var result Result
	for i, tc := range tests {
		cr := CaseResult{Index: i, Input: tc.Input, Expected: tc.Output}

		pc, ok := found[i]
		if !ok {
			cr.Status = StatusError
			cr.Actual = crashDetail
			if cr.Actual == "" {
				cr.Actual = "no output from judge harness"
			}
			result.Cases = append(result.Cases, cr)
			continue
		}

		switch pc.kind {
		case passMarker:
			cr.Status = StatusPass
			cr.Passed = true
			cr.Actual = pc.detail
		case failMarker:
			cr.Status = StatusFail
			cr.Actual = pc.detail
		case errMarker:
			cr.Status = StatusError
			cr.Actual = pc.detail
		case timeoutMarker:
			cr.Status = StatusTimeout
			cr.Actual = "timed out"
		default:
			cr.Status = StatusError
			cr.Actual = "unrecognized judge output"
		}
		result.Cases = append(result.Cases, cr)
	}

	return result
}

// removeContainer force-stops and removes a container by name. It's the
// cleanup fallback for the timeout path, covering containers that are still
// running or were never fully started.
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
// candidate's solve function once, then feeds it every test case's input in
// turn as keyword arguments, comparing each result against its eval'd
// output. Code and the test-case list are base64-encoded so they can be
// embedded as Python string literals without any quoting/escaping concerns;
// the test list itself is JSON so the harness can iterate it directly.
//
// Each test case's input is stored as a bare kwargs-style string, e.g.
// "n = 7, queries = [[0,5],[1,6]]" — which is not valid Python on its own
// (top-level commas collide with chained-assignment syntax). Wrapping it in
// dict(...) reuses Python's own keyword-argument parser to turn it into a
// dict without hand-rolling a parser for nested lists/dicts in Go.
func harnessScript(code string, tests []TestCase) (string, error) {
	pairs := make([][2]string, len(tests))
	for i, tc := range tests {
		pairs[i] = [2]string{tc.Input, tc.Output}
	}

	testsJSON, err := json.Marshal(pairs)
	if err != nil {
		return "", fmt.Errorf("marshal test cases: %w", err)
	}

	return fmt.Sprintf(harnessTemplate,
		base64.StdEncoding.EncodeToString([]byte(code)),
		base64.StdEncoding.EncodeToString(testsJSON),
		int(perTestTimeout/time.Second),
		passMarker,
		failMarker,
		timeoutMarker,
		errMarker,
	), nil
}

// Each result line is "<marker> <index>[ <base64 detail>]" — a single line
// per test case, detail base64-encoded, so stray print() output from the
// candidate's own code (or a repr() containing newlines) can never be
// mistaken for part of the structured result.
const harnessTemplate = `import base64
import json
import signal

class _JudgeTimeout(Exception):
    pass

def _judge_alarm(signum, frame):
    raise _JudgeTimeout()

signal.signal(signal.SIGALRM, _judge_alarm)

exec(base64.b64decode("%s").decode())

_tests = json.loads(base64.b64decode("%s").decode())

for _i, (_input, _output) in enumerate(_tests):
    signal.alarm(%d)
    try:
        _args = eval("dict(" + _input + ")")
        _result = solve(**_args)
        _expected = eval(_output)
        _detail = base64.b64encode(repr(_result).encode()).decode()
        if _result == _expected:
            print("%s " + str(_i) + " " + _detail)
        else:
            print("%s " + str(_i) + " " + _detail)
    except _JudgeTimeout:
        print("%s " + str(_i))
    except Exception as _e:
        print("%s " + str(_i) + " " + base64.b64encode(repr(_e).encode()).decode())
    finally:
        signal.alarm(0)
`
