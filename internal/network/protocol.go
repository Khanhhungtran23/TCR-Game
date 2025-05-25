// Package network handles all network communication protocols
package network

import (
	"encoding/json"
	"time"

	"tcr-game/internal/game"
)

// MessageType represents different types of messages
type MessageType string

const (
	// Authentication messages
	MsgLogin    MessageType = "LOGIN"
	MsgRegister MessageType = "REGISTER"
	MsgAuthOK   MessageType = "AUTH_OK"
	MsgAuthFail MessageType = "AUTH_FAIL"

	// Matchmaking messages
	MsgFindMatch    MessageType = "FIND_MATCH"
	MsgMatchFound   MessageType = "MATCH_FOUND"
	MsgGameStart    MessageType = "GAME_START"
	MsgPlayerJoined MessageType = "PLAYER_JOINED"

	// Game action messages
	MsgSummonTroop MessageType = "SUMMON_TROOP"
	MsgAttack      MessageType = "ATTACK"
	MsgEndTurn     MessageType = "END_TURN"
	MsgSurrender   MessageType = "SURRENDER"

	// Game state messages
	MsgGameState  MessageType = "GAME_STATE"
	MsgGameEvent  MessageType = "GAME_EVENT"
	MsgGameEnd    MessageType = "GAME_END"
	MsgTurnChange MessageType = "TURN_CHANGE"

	// System messages
	MsgError      MessageType = "ERROR"
	MsgPing       MessageType = "PING"
	MsgPong       MessageType = "PONG"
	MsgDisconnect MessageType = "DISCONNECT"
)

// Message represents a network message between client and server
type Message struct {
	Type      MessageType            `json:"type"`
	PlayerID  string                 `json:"player_id,omitempty"`
	GameID    string                 `json:"game_id,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// AuthRequest represents login/register request
type AuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// AuthResponse represents authentication response
type AuthResponse struct {
	Success    bool             `json:"success"`
	PlayerID   string           `json:"player_id,omitempty"`
	Message    string           `json:"message,omitempty"`
	PlayerData *game.PlayerData `json:"player_data,omitempty"`
}

// MatchRequest represents a request to find a match
type MatchRequest struct {
	GameMode string `json:"game_mode"` // "simple" or "enhanced"
}

// MatchFoundResponse represents successful matchmaking
type MatchFoundResponse struct {
	GameID   string      `json:"game_id"`
	Opponent game.Player `json:"opponent"`
	GameMode string      `json:"game_mode"`
	YourTurn bool        `json:"your_turn,omitempty"` // For Simple TCR
}

// GameStartResponse represents game initialization
type GameStartResponse struct {
	GameState        game.GameState `json:"game_state"`
	YourTroops       []game.Troop   `json:"your_troops"`
	YourTowers       []game.Tower   `json:"your_towers"`
	CountdownSeconds int            `json:"countdown_seconds"`
}

// SummonTroopRequest represents summoning a troop
type SummonTroopRequest struct {
	TroopName game.TroopType `json:"troop_name"`
}

// AttackRequest represents an attack action
type AttackRequest struct {
	AttackerName game.TroopType `json:"attacker_name"`
	TargetType   string         `json:"target_type"` // "tower" or "troop"
	TargetName   string         `json:"target_name"`
}

// GameEventResponse represents a game event notification
type GameEventResponse struct {
	Event     game.CombatAction `json:"event"`
	GameState game.GameState    `json:"game_state"`
}

// GameEndResponse represents game conclusion
type GameEndResponse struct {
	Winner       string    `json:"winner"`
	Reason       string    `json:"reason"` // "king_tower_destroyed", "time_up", "surrender"
	EXPGained    int       `json:"exp_gained"`
	TrophyChange int       `json:"trophy_change"`
	Stats        GameStats `json:"stats"`
}

// GameStats represents end-game statistics
type GameStats struct {
	TowersDestroyed int `json:"towers_destroyed"`
	TroopsDeployed  int `json:"troops_deployed"`
	DamageDealt     int `json:"damage_dealt"`
	GameDuration    int `json:"game_duration"` // in seconds
}

// ErrorResponse represents an error message
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Helper functions for creating messages

// NewMessage creates a new message with timestamp
func NewMessage(msgType MessageType, playerID, gameID string) *Message {
	return &Message{
		Type:      msgType,
		PlayerID:  playerID,
		GameID:    gameID,
		Timestamp: time.Now(),
		Data:      make(map[string]interface{}),
	}
}

// SetData sets data payload for a message
func (m *Message) SetData(key string, value interface{}) {
	if m.Data == nil {
		m.Data = make(map[string]interface{})
	}
	m.Data[key] = value
}

// GetData retrieves data from message payload
func (m *Message) GetData(key string) (interface{}, bool) {
	if m.Data == nil {
		return nil, false
	}
	value, exists := m.Data[key]
	return value, exists
}

// ToJSON converts message to JSON bytes
func (m *Message) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// FromJSON creates message from JSON bytes
func FromJSON(data []byte) (*Message, error) {
	var msg Message
	err := json.Unmarshal(data, &msg)
	return &msg, err
}

// CreateAuthMessage creates authentication message
func CreateAuthMessage(msgType MessageType, username, password string) *Message {
	msg := NewMessage(msgType, "", "")
	msg.SetData("auth_request", AuthRequest{
		Username: username,
		Password: password,
	})
	return msg
}

// CreateMatchRequest creates match finding request
func CreateMatchRequest(playerID, gameMode string) *Message {
	msg := NewMessage(MsgFindMatch, playerID, "")
	msg.SetData("match_request", MatchRequest{
		GameMode: gameMode,
	})
	return msg
}

// CreateSummonMessage creates troop summoning message
func CreateSummonMessage(playerID, gameID string, troopName game.TroopType) *Message {
	msg := NewMessage(MsgSummonTroop, playerID, gameID)
	msg.SetData("summon_request", SummonTroopRequest{
		TroopName: troopName,
	})
	return msg
}

// CreateAttackMessage creates attack message
func CreateAttackMessage(playerID, gameID string, attacker game.TroopType, targetType, targetName string) *Message {
	msg := NewMessage(MsgAttack, playerID, gameID)
	msg.SetData("attack_request", AttackRequest{
		AttackerName: attacker,
		TargetType:   targetType,
		TargetName:   targetName,
	})
	return msg
}

// CreateGameEventMessage creates game event notification
func CreateGameEventMessage(gameID string, event game.CombatAction, gameState game.GameState) *Message {
	msg := NewMessage(MsgGameEvent, "", gameID)
	msg.SetData("game_event", GameEventResponse{
		Event:     event,
		GameState: gameState,
	})
	return msg
}

// CreateErrorMessage creates error message
func CreateErrorMessage(code, message string) *Message {
	msg := NewMessage(MsgError, "", "")
	msg.SetData("error", ErrorResponse{
		Code:    code,
		Message: message,
	})
	return msg
}
