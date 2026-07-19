package matchmaking

import (
	"github.com/gorilla/websocket"
)

type Player struct {
	playerID string
	conn     *websocket.Conn
}


func NewPlayer(playerID string, connection *websocket.Conn) *Player {
	return &Player{playerID: playerID, conn: connection}
}

func (p *Player) PlayerID() string {
	return p.playerID
}

func (p *Player) Conn() *websocket.Conn {
	return p.conn
}