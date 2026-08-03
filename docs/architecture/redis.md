# Redis: live room state


## 1. The one job: match room state

Redis holds the live state of an active match, via
`RoomStore` (`server/internal/matchmaking/room.go`).

- Every match gets a key `room:{matchID}` → a JSON blob of both player IDs,
  the assigned problem (title, description, test cases), status, and start
  time (`Room` struct, `room.go:12-22`).
- **1-hour TTL** (`roomTTL`, `room.go:25`) on every key — if a match gets
  abandoned (tab closed, connection dropped, nobody finishes), Redis expires
  the key on its own. No explicit cleanup job needed.
- Written once by the matcher when both players accept
  (`matcher.go`'s `proposeMatch`, via `rooms.Save`), then read by
  `handlers.RoomSession` (`session.go:59`) when each player's room-session
  websocket connects, to validate they belong to the match and load the
  problem/test cases.

## 2. Why Redis

- **TTL cleanup for free.** Redis's `SET ... EX` gives auto-expiration
  natively. An in-memory map would need its own sweeper goroutine to get the
  same "abandoned matches clean themselves up" behavior.
- **Survives a server restart.** If the process
  redeploys or crashes mid-match, an in-memory room map is wiped instantly;
  a player hitting refresh (`GET /room/{match_id}`) would hit a dead end.
  Redis-backed room state survives.
- **Multi-instance readiness** — not relevant yet on a single server, but
  if more than one server process ever ran, an in-memory map is invisible
  across processes; only an external shared store lets two instances see
  the same room.

## 3. What actually happens on a server crash/restart

Assuming Redis itself stays up and only the Go process restarts:

**Survives:**
- Room data (players, problem, test cases, `started_at`) — still in Redis,
  so `GET /room/{match_id}` still succeeds.
- The match clock — `Room.tsx`'s elapsed-time display is computed from
  `room.started_at`, so it shows real elapsed time on reconnect, not 0.
- Rejoining `/room/{match_id}/session` — `RoomSession` re-reads the room
  from Redis and `Hub.Join` just creates a fresh entry in the (now-empty)
  in-memory `Hub`; nothing about rejoining depends on old Hub state.
