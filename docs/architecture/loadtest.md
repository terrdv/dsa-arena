# Load testing the judge pipeline

Why a custom load-testing tool exists, what it measures, and how to read the
output — for `server/cmd/loadtest`.

## 1. What it measures

The number that matters is **end-to-end submit latency**: timestamped
client-side from sending `{"type":"submit",...}` to receiving
`{"type":"result",...}` — not a server-side approximation. That's what a
player actually experiences, including the WebSocket round-trip, not just
judge execution time.

Reported per run:

- **p50 / p95 / p99 / max latency** — median plus tail. p50 alone hides bad
  outlier experiences; p99 needs enough samples to mean anything (see
  `-submits`/`-duration` below).
- **Throughput** — completed submissions / wall-clock time.
- **Errors** — dial failures, disconnects, judge-reported `"error"`. A judge
  `"result"` (even a failing one) still counts as a completed sample, since
  latency is what's being measured, not correctness.


## 2. Method

**Open-loop / sustained** (`-duration`, `-stagger`, `-think-min`/
`-think-max`): matches start at random points spread across a window
instead of all at once, and each player pauses a random amount between
submissions instead of resubmitting immediately — closer to how
uncoordinated real players actually behave (read the result, edit code,
resubmit later). Run long enough and this doubles as a soak test, surfacing
slow degradation (leaked goroutines, container cleanup failures) that a
short burst wouldn't.

## 3. Running it

Prerequisites: Postgres, Redis, and the server all running
(`go run ./cmd/server`), with `DATABASE_URL`/`REDIS_ADDR` set in that
terminal. The load test needs its own `DATABASE_URL` (or `-dsn`) in whatever
terminal it runs from, to seed the test problem.

```sh
cd server

# each player pausing 2-10s between submissions, for 5 minutes
go run ./cmd/loadtest -matches 100 -duration 5m -stagger 30s -think-min 2s -think-max 10s
```

| Flag | Default | Meaning |
|---|---|---|
| `-addr` | `localhost:8080` | Server host:port |
| `-matches` | `4` | Concurrent simulated matches (2 players each) |
| `-submits` | `10` | Submissions per player, back-to-back (ignored if `-duration` set) |
| `-duration` | `0` | Run sustained for this long instead of a fixed `-submits` count |
| `-stagger` | `0` | Max random delay before a player starts, spread over this window |
| `-think-min`/`-think-max` | `0` | Random pause range between a player's submissions (`0` = back-to-back) |
| `-dsn` | `$DATABASE_URL` | Postgres DSN for seeding the test problem |

