# dsa-arena

## Architecture

```
                          ┌─────────────────────────────┐
 client ── WebSocket ──►  │  server (Go)                │
 client ── WebSocket ──►  │   /queue  → in-memory queue │──► Postgres  (players, problems, match history)
                          │   matcher → pairs players   │──► Redis     (live room state, TTL 2h)
                          └─────────────────────────────┘
```


- **Matcher** (`internal/matchmaking/matcher.go`) — a goroutine that wakes when the queue signals, pops two players, creates a room, and notifies both players over their existing connections.
- **Room state in Redis** (`internal/matchmaking/room.go`) — the source of truth for a live match: `room:{match_id}` → JSON of player IDs, problem, status, start time, with a 2-hour TTL so abandoned matches clean themselves up.
- **Postgres** (`internal/database/`) — data: players, problems, finished match history.


## Running the server

Requires [Go] 1.22+, Postgres, and Redis.

```sh
cd server
cp .env.example .env   # fill in DATABASE_URL, REDIS_ADDR
set -a && source .env && set +a
go run ./cmd/server
```

The server listens on port `8080` by default

