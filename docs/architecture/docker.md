## 1. Docker for code isolation

Submitted code is arbitrary, untrusted Python (`solve()` provided by
whoever's playing) — it has to run somewhere that can't touch the host
filesystem, the network, the database, or the other player's submission.

`Judge` in
`server/internal/submission/submission.go` shells out to
`docker run --rm --network none --memory 256m --cpus 0.5 python:3.12-slim`
per submission — no network, capped memory, capped CPU, and the container is
gone as soon as the run ends (or is force-removed on timeout via
`removeContainer`).

## 2. Worker pool

Every submission does a synchronous `docker run` in its own
goroutine (`session.go`'s per-connection handler calls `submission.Judge`
inline) — there's no shared pool of pre-warmed workers or containers.
`server/internal/workers/workerpool.go` exists as a placeholder for that,
but isn't implemented.

