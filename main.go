package main

import (
	"fmt"
	"log"
	"net"
	"sync"
)

const (
	HOST = "localhost"
	PORT = "8080"
)

type GameServer struct {
	players    map[string]*Player
	gameRooms  map[string]*GameRoom
	connections map[string]net.Conn  // Map of username to connection
	mu         sync.RWMutex
}

func NewGameServer() *GameServer {
	return &GameServer{
		players:    make(map[string]*Player),
		gameRooms:  make(map[string]*GameRoom),
		connections: make(map[string]net.Conn),
	}
}

func (s *GameServer) Start() {
	listener, err := net.Listen("tcp", HOST+":"+PORT)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	defer listener.Close()

	fmt.Printf("TCR Server listening on %s:%s\n", HOST, PORT)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

func main() {
	server := NewGameServer()
	server.Start()
} 