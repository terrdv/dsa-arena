package matchmaking

import "sync"

type Room struct {
	serverIP string
	port     int
}

func NewRoom(serverIP string, port int) *Room {
	return &Room{serverIP: serverIP, port: port}
}

type RoomStack struct {
	mx     sync.Mutex
	rooms  []*Room
	signal chan struct{}
}

func NewRoomStack() *RoomStack {
	return &RoomStack{
		signal: make(chan struct{}, 1),
	}
}

func (s *RoomStack) Push(r *Room) {
	s.mx.Lock()
	s.rooms = append(s.rooms, r)
	s.mx.Unlock()

	select {
	case s.signal <- struct{}{}:
	default:
	}
}

func (s *RoomStack) Pop() (*Room, bool) {
	s.mx.Lock()
	defer s.mx.Unlock()

	n := len(s.rooms)
	if n == 0 {
		return nil, false
	}

	r := s.rooms[n-1]
	s.rooms = s.rooms[:n-1]
	return r, true
}

func (s *RoomStack) Signal() <-chan struct{} {
	return s.signal
}
