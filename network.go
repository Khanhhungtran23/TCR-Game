package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

type ClientMessage struct {
	Type    string          `json:"type"`
	Data    json.RawMessage `json:"data"`
}

type ServerMessage struct {
	Type    string      `json:"type"`
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func (s *GameServer) handleConnection(conn net.Conn) {
	defer func() {
		conn.Close()
		// Remove connection from the map when client disconnects
		s.mu.Lock()
		for username, c := range s.connections {
			if c == conn {
				delete(s.connections, username)
				break
			}
		}
		s.mu.Unlock()
	}()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	// Handle authentication
	var player *Player
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		var msg ClientMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			sendError(writer, "invalid message format")
			continue
		}

		if msg.Type == "login" {
			var loginData struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			if err := json.Unmarshal(msg.Data, &loginData); err != nil {
				sendError(writer, "invalid login data")
				continue
			}

			player = s.authenticatePlayer(loginData.Username, loginData.Password)
			if player == nil {
				sendError(writer, "invalid credentials")
				continue
			}

			// Store the connection
			s.mu.Lock()
			s.connections[player.Username] = conn
			s.mu.Unlock()

			sendSuccess(writer, "login", map[string]string{
				"username": player.Username,
			})
			break
		}
	}

	// Main game loop
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		var msg ClientMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			sendError(writer, "invalid message format")
			continue
		}

		switch msg.Type {
		case "create_room":
			var roomData struct {
				IsEnhanced bool `json:"is_enhanced"`
			}
			if err := json.Unmarshal(msg.Data, &roomData); err != nil {
				sendError(writer, "invalid room data")
				continue
			}

			room := s.CreateGameRoom(player, nil, roomData.IsEnhanced)
			sendSuccess(writer, "room_created", map[string]string{
				"room_id": room.ID,
			})

		case "join_room":
			var roomData struct {
				RoomID string `json:"room_id"`
			}
			if err := json.Unmarshal(msg.Data, &roomData); err != nil {
				sendError(writer, "invalid room data")
				continue
			}

			room, err := s.joinRoom(roomData.RoomID, player)
			if err != nil {
				sendError(writer, err.Error())
				continue
			}

			sendSuccess(writer, "room_joined", map[string]string{
				"room_id": room.ID,
			})

		case "game_action":
			var action GameAction
			if err := json.Unmarshal(msg.Data, &action); err != nil {
				sendError(writer, "invalid action data")
				continue
			}

			room, err := s.getPlayerRoom(player.Username)
			if err != nil {
				sendError(writer, err.Error())
				continue
			}

			if err := room.HandleAction(action); err != nil {
				sendError(writer, err.Error())
				continue
			}

			// Broadcast game state to all players in the room
			s.broadcastGameState(room)

		case "get_state":
			room, err := s.getPlayerRoom(player.Username)
			if err != nil {
				sendError(writer, err.Error())
				continue
			}

			state := room.getGameState()
			sendSuccess(writer, "game_state", state)

		default:
			sendError(writer, "unknown message type")
		}
	}
}

func (s *GameServer) authenticatePlayer(username, password string) *Player {
	s.mu.RLock()
	player, exists := s.players[username]
	s.mu.RUnlock()

	if !exists {
		// Create new player
		player = &Player{
			Username: username,
			Password: password,
			EXP:      0,
			Level:    1,
			Mana:     5,
			MaxMana:  10,
			LastManaRegen: time.Now(),
		}
		s.mu.Lock()
		s.players[username] = player
		s.mu.Unlock()
		return player
	}

	if player.Password != password {
		return nil
	}

	return player
}

func (s *GameServer) joinRoom(roomID string, player *Player) (*GameRoom, error) {
	s.mu.RLock()
	room, exists := s.gameRooms[roomID]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("room not found")
	}

	if room.Players[1] != nil {
		return nil, fmt.Errorf("room is full")
	}

	room.Players[1] = player
	return room, nil
}

func (s *GameServer) getPlayerRoom(username string) (*GameRoom, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, room := range s.gameRooms {
		for _, player := range room.Players {
			if player != nil && player.Username == username {
				return room, nil
			}
		}
	}

	return nil, fmt.Errorf("player not in any room")
}

func (s *GameServer) broadcastGameState(room *GameRoom) {
	state := room.getGameState()
	msg := ServerMessage{
		Type:    "game_state",
		Success: true,
		Data:    state,
	}

	data, _ := json.Marshal(msg)
	data = append(data, '\n')

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, player := range room.Players {
		if player != nil {
			if conn, exists := s.connections[player.Username]; exists {
				writer := bufio.NewWriter(conn)
				writer.Write(data)
				writer.Flush()
			}
		}
	}

	// Check for game end
	if isEnd, winner := room.CheckGameEnd(); isEnd {
		endMsg := ServerMessage{
			Type:    "game_end",
			Success: true,
			Data: map[string]string{
				"winner": winner,
			},
		}
		endData, _ := json.Marshal(endMsg)
		endData = append(endData, '\n')

		for _, player := range room.Players {
			if player != nil {
				if conn, exists := s.connections[player.Username]; exists {
					writer := bufio.NewWriter(conn)
					writer.Write(endData)
					writer.Flush()
				}
			}
		}

		// Clean up the room
		s.mu.Lock()
		delete(s.gameRooms, room.ID)
		s.mu.Unlock()
	}
}

func sendError(writer *bufio.Writer, errMsg string) {
	msg := ServerMessage{
		Type:    "error",
		Success: false,
		Error:   errMsg,
	}
	data, _ := json.Marshal(msg)
	data = append(data, '\n')
	writer.Write(data)
	writer.Flush()
}

func sendSuccess(writer *bufio.Writer, msgType string, data interface{}) {
	msg := ServerMessage{
		Type:    msgType,
		Success: true,
		Data:    data,
	}
	jsonData, _ := json.Marshal(msg)
	jsonData = append(jsonData, '\n')
	writer.Write(jsonData)
	writer.Flush()
} 