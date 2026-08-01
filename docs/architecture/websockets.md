# Real-time transport & code execution

Why dsa-arena uses WebSockets end-to-end, and why submissions run in
throwaway Docker containers rather than in-process — with a note on the
worker-pool idea that's planned but not built yet.

## 1. WebSockets, not polling/SSE

Every stateful interaction in dsa-arena is bidirectional and latency
sensitive: the server needs to push to the client (a match was found, an
opponent submitted, judging finished) and the client needs to push to the
server (leave the queue, accept/decline a match, submit code) — often within
the same short-lived exchange. REST polling (adds latency), not event driven
TCP connection with the server able to push when an event occurs.

There are two sockets, one per phase of a match's lifecycle:

- **Queue socket** (`GET /queue`, `server/internal/handlers/matchmaking.go`) —
  held open from "find match" through matchmaking and match-accept. One
  connection carries the player through (queued →
  proposed → accepted/declined) rather than reconnecting between them: the
  matcher just keeps writing to the same conn, and the player's read loop
  (`listen` in `matchmaking.go`) reinterprets incoming `{"action": "..."}`
  messages differently depending on what the matcher is currently waiting
  for (`"leave"` while queued, `"accept"`/`"decline"` once a match is
  proposed) via `Player.AwaitDecision`/`Deliver`

- **Room session socket** (`GET /api/room/:match_id/session`,
  `server/internal/handlers/session.go`) — held open for the life of the
  match once both players are in the room. 

