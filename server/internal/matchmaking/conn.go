package matchmaking

import (
	"sync"

	"github.com/gorilla/websocket"
)


type Conn struct {
	ws *websocket.Conn
	wx sync.Mutex
}

func NewConn(ws *websocket.Conn) *Conn {
	return &Conn{ws: ws}
}

func (c *Conn) WriteJSON(v any) error {
	c.wx.Lock()
	defer c.wx.Unlock()
	return c.ws.WriteJSON(v)
}

func (c *Conn) ReadMessage() (messageType int, p []byte, err error) {
	return c.ws.ReadMessage()
}

func (c *Conn) Close() error {
	return c.ws.Close()
}
