// Package server implements the TCP server for Clash Royale TCR
package server

import (
	"bufio"
	"fmt"
	"net"
	"sync"
	"time"

	"tcr-game/internal/game"
	"tcr-game/internal/network"
	"tcr-game/pkg/logger"
)

// Server represents the TCP server
type Server struct {
	address     string
	listener    net.Listener
	clients     map[string]*Client
	games       map[string]*game.GameEngine
	dataManager *game.DataManager
	matchmaking *MatchmakingQueue
	mu          sync.RWMutex
	isRunning   bool
	logger      *logger.Logger
}

// Client represents a connected client
type Client struct {
	ID       string
	Username string
	Conn     net.Conn
	Player   *game.PlayerData
	GameID   string
	IsActive bool
	LastPing time.Time
	Writer   *bufio.Writer
	mu       sync.Mutex
}

// MatchmakingQueue handles player matchmaking
type MatchmakingQueue struct {
	simpleQueue   []*Client
	enhancedQueue []*Client
	mu            sync.Mutex
}

// NewServer creates a new TCP server instance
func NewServer(address string, dataManager *game.DataManager) *Server {
	return &Server{
		address:     address,
		clients:     make(map[string]*Client),
		games:       make(map[string]*game.GameEngine),
		dataManager: dataManager,
		matchmaking: &MatchmakingQueue{
			simpleQueue:   make([]*Client, 0),
			enhancedQueue: make([]*Client, 0),
		},
		logger: logger.Server,
	}
}

// Start begins listening for client connections
func (s *Server) Start() error {
	var err error
	s.listener, err = net.Listen("tcp", s.address)
	if err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	s.isRunning = true
	s.logger.Info("Server started and listening on %s", s.address)

	// Start background services
	go s.matchmakingService()
	go s.cleanupService()

	// Accept client connections
	for s.isRunning {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.isRunning {
				s.logger.Error("Failed to accept connection: %v", err)
			}
			continue
		}

		go s.handleClient(conn)
	}

	return nil
}

// Stop shuts down the server
func (s *Server) Stop() error {
	s.isRunning = false

	if s.listener != nil {
		s.listener.Close()
	}

	// Close all client connections
	s.mu.Lock()
	for _, client := range s.clients {
		client.Conn.Close()
	}
	s.mu.Unlock()

	s.logger.Info("Server stopped")
	return nil
}

// handleClient manages individual client connections
func (s *Server) handleClient(conn net.Conn) {
	defer conn.Close()

	clientID := generateClientID()
	client := &Client{
		ID:       clientID,
		Conn:     conn,
		IsActive: true,
		LastPing: time.Now(),
		Writer:   bufio.NewWriter(conn),
	}

	s.logger.Info("New client connected: %s from %s", clientID, conn.RemoteAddr())

	// Add to clients map
	s.mu.Lock()
	s.clients[clientID] = client
	s.mu.Unlock()

	// Handle client messages
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		if !s.isRunning || !client.IsActive {
			break
		}

		data := scanner.Bytes()
		if err := s.processMessage(client, data); err != nil {
			s.logger.Error("Error processing message from %s: %v", clientID, err)
			s.sendError(client, "PROCESSING_ERROR", err.Error())
		}
	}

	// Cleanup client
	s.removeClient(clientID)
	s.logger.Info("Client disconnected: %s", clientID)
}

// processMessage handles incoming messages from clients
func (s *Server) processMessage(client *Client, data []byte) error {
	msg, err := network.FromJSON(data)
	if err != nil {
		return fmt.Errorf("failed to parse message: %w", err)
	}

	s.logger.Debug("Received message from %s: %s", client.ID, msg.Type)

	switch msg.Type {
	case network.MsgLogin:
		return s.handleLogin(client, msg)
	case network.MsgRegister:
		return s.handleRegister(client, msg)
	case network.MsgFindMatch:
		return s.handleFindMatch(client, msg)
	case network.MsgSummonTroop:
		return s.handleSummonTroop(client, msg)
	case network.MsgAttack:
		return s.handleAttack(client, msg)
	case network.MsgEndTurn:
		return s.handleEndTurn(client, msg)
	case network.MsgSurrender:
		return s.handleSurrender(client, msg)
	case network.MsgPing:
		return s.handlePing(client, msg)
	default:
		return fmt.Errorf("unknown message type: %s", msg.Type)
	}
}

// handleLogin processes login requests
func (s *Server) handleLogin(client *Client, msg *network.Message) error {
	authReq, ok := msg.Data["auth_request"].(map[string]interface{})
	if !ok {
		return s.sendError(client, "INVALID_REQUEST", "Invalid auth request format")
	}

	username, _ := authReq["username"].(string)
	password, _ := authReq["password"].(string)

	playerData, err := s.dataManager.AuthenticatePlayer(username, password)
	if err != nil {
		s.logger.Info("Login failed for %s: %v", username, err)
		return s.sendAuthResponse(client, false, "", err.Error(), nil)
	}

	client.Username = username
	client.Player = playerData

	s.logger.Info("Player %s logged in successfully", username)
	return s.sendAuthResponse(client, true, client.ID, "Login successful", playerData)
}

// handleRegister processes registration requests
func (s *Server) handleRegister(client *Client, msg *network.Message) error {
	authReq, ok := msg.Data["auth_request"].(map[string]interface{})
	if !ok {
		return s.sendError(client, "INVALID_REQUEST", "Invalid auth request format")
	}

	username, _ := authReq["username"].(string)
	password, _ := authReq["password"].(string)

	// Validate username and password
	if len(username) < 3 || len(username) > 20 {
		return s.sendAuthResponse(client, false, "", "Username must be 3-20 characters", nil)
	}
	if len(password) < 4 {
		return s.sendAuthResponse(client, false, "", "Password must be at least 4 characters", nil)
	}

	playerData, err := s.dataManager.RegisterPlayer(username, password)
	if err != nil {
		s.logger.Info("Registration failed for %s: %v", username, err)
		return s.sendAuthResponse(client, false, "", err.Error(), nil)
	}

	client.Username = username
	client.Player = playerData

	s.logger.Info("Player %s registered successfully", username)
	return s.sendAuthResponse(client, true, client.ID, "Registration successful", playerData)
}

// handleFindMatch processes matchmaking requests
func (s *Server) handleFindMatch(client *Client, msg *network.Message) error {
	if client.Player == nil {
		return s.sendError(client, "NOT_AUTHENTICATED", "Must login first")
	}

	matchReq, ok := msg.Data["match_request"].(map[string]interface{})
	if !ok {
		return s.sendError(client, "INVALID_REQUEST", "Invalid match request format")
	}

	gameMode, _ := matchReq["game_mode"].(string)
	if gameMode != game.ModeSimple && gameMode != game.ModeEnhanced {
		return s.sendError(client, "INVALID_GAME_MODE", "Game mode must be 'simple' or 'enhanced'")
	}

	// Add to matchmaking queue
	s.matchmaking.AddPlayer(client, gameMode)
	s.logger.Info("Player %s added to %s mode queue", client.Username, gameMode)

	// Send confirmation
	response := network.NewMessage(network.MsgFindMatch, client.ID, "")
	response.SetData("status", "searching")
	response.SetData("game_mode", gameMode)
	return s.sendMessage(client, response)
}

// handleSummonTroop processes troop summoning
func (s *Server) handleSummonTroop(client *Client, msg *network.Message) error {
	gameEngine := s.getClientGame(client)
	if gameEngine == nil {
		return s.sendError(client, "NO_ACTIVE_GAME", "No active game found")
	}

	summonReq, ok := msg.Data["summon_request"].(map[string]interface{})
	if !ok {
		return s.sendError(client, "INVALID_REQUEST", "Invalid summon request format")
	}

	troopName := game.TroopType(summonReq["troop_name"].(string))

	action, err := gameEngine.SummonTroop(client.ID, troopName)
	if err != nil {
		return s.sendError(client, "SUMMON_FAILED", err.Error())
	}

	// Broadcast event to both players
	return s.broadcastGameEvent(client.GameID, *action, *gameEngine.GetGameState())
}

// handleAttack processes attack actions
func (s *Server) handleAttack(client *Client, msg *network.Message) error {
	gameEngine := s.getClientGame(client)
	if gameEngine == nil {
		return s.sendError(client, "NO_ACTIVE_GAME", "No active game found")
	}

	attackReq, ok := msg.Data["attack_request"].(map[string]interface{})
	if !ok {
		return s.sendError(client, "INVALID_REQUEST", "Invalid attack request format")
	}

	attackerName := game.TroopType(attackReq["attacker_name"].(string))
	targetType, _ := attackReq["target_type"].(string)
	targetName, _ := attackReq["target_name"].(string)

	action, err := gameEngine.ExecuteAttack(client.ID, attackerName, targetType, targetName)
	if err != nil {
		return s.sendError(client, "ATTACK_FAILED", err.Error())
	}

	// Broadcast event to both players
	return s.broadcastGameEvent(client.GameID, *action, *gameEngine.GetGameState())
}

// handleEndTurn processes end turn actions (Simple mode)
func (s *Server) handleEndTurn(client *Client, msg *network.Message) error {
	gameEngine := s.getClientGame(client)
	if gameEngine == nil {
		return s.sendError(client, "NO_ACTIVE_GAME", "No active game found")
	}

	gameState := gameEngine.GetGameState()
	if gameState.GameMode != game.ModeSimple {
		return s.sendError(client, "INVALID_ACTION", "End turn only available in Simple mode")
	}

	if gameState.CurrentTurn != client.ID {
		return s.sendError(client, "NOT_YOUR_TURN", "It's not your turn")
	}

	// Switch turn logic is handled in game engine
	// For now, just broadcast turn change
	response := network.NewMessage(network.MsgTurnChange, "", client.GameID)
	response.SetData("current_turn", gameState.CurrentTurn)

	return s.broadcastToGame(client.GameID, response)
}

// handleSurrender processes surrender actions
func (s *Server) handleSurrender(client *Client, msg *network.Message) error {
	gameEngine := s.getClientGame(client)
	if gameEngine == nil {
		return s.sendError(client, "NO_ACTIVE_GAME", "No active game found")
	}

	// End game with opponent as winner
	gameState := gameEngine.GetGameState()
	if gameState.Player1.ID == client.ID {
		gameState.Winner = gameState.Player2.ID
	} else {
		gameState.Winner = gameState.Player1.ID
	}

	return s.endGame(client.GameID, "surrender")
}

// handlePing processes ping messages
func (s *Server) handlePing(client *Client, msg *network.Message) error {
	client.LastPing = time.Now()

	response := network.NewMessage(network.MsgPong, client.ID, "")
	return s.sendMessage(client, response)
}

// Matchmaking service runs in background
func (s *Server) matchmakingService() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for s.isRunning {
		<-ticker.C
		s.matchmaking.ProcessMatches(s)
	}
}

// Cleanup service for inactive connections
func (s *Server) cleanupService() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for s.isRunning {
		<-ticker.C
		s.cleanupInactiveClients()
	}
}

// Helper methods

func (s *Server) sendMessage(client *Client, msg *network.Message) error {
	client.mu.Lock()
	defer client.mu.Unlock()

	data, err := msg.ToJSON()
	if err != nil {
		return err
	}

	_, err = client.Writer.Write(append(data, '\n'))
	if err != nil {
		return err
	}

	return client.Writer.Flush()
}

func (s *Server) sendError(client *Client, code, message string) error {
	errorMsg := network.CreateErrorMessage(code, message)
	return s.sendMessage(client, errorMsg)
}

func (s *Server) sendAuthResponse(client *Client, success bool, playerID, message string, playerData *game.PlayerData) error {
	response := network.NewMessage(network.MsgAuthOK, playerID, "")
	if !success {
		response.Type = network.MsgAuthFail
	}

	response.SetData("auth_response", network.AuthResponse{
		Success:    success,
		PlayerID:   playerID,
		Message:    message,
		PlayerData: playerData,
	})

	return s.sendMessage(client, response)
}

func (s *Server) broadcastGameEvent(gameID string, event game.CombatAction, gameState game.GameState) error {
	msg := network.CreateGameEventMessage(gameID, event, gameState)
	return s.broadcastToGame(gameID, msg)
}

func (s *Server) broadcastToGame(gameID string, msg *network.Message) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, client := range s.clients {
		if client.GameID == gameID && client.IsActive {
			s.sendMessage(client, msg)
		}
	}
	return nil
}

func (s *Server) getClientGame(client *Client) *game.GameEngine {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.games[client.GameID]
}

func (s *Server) removeClient(clientID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if client, exists := s.clients[clientID]; exists {
		client.IsActive = false
		delete(s.clients, clientID)
	}
}

func (s *Server) cleanupInactiveClients() {
	s.mu.Lock()
	defer s.mu.Unlock()

	timeout := 5 * time.Minute
	now := time.Now()

	for clientID, client := range s.clients {
		if now.Sub(client.LastPing) > timeout {
			client.IsActive = false
			client.Conn.Close()
			delete(s.clients, clientID)
			s.logger.Info("Removed inactive client: %s", clientID)
		}
	}
}

func (s *Server) endGame(gameID string, reason string) error {
	// Implementation for ending games and calculating rewards
	// This would handle EXP/trophy updates and cleanup
	s.logger.Info("Game %s ended: %s", gameID, reason)
	return nil
}

// Helper function to generate client IDs
func generateClientID() string {
	return fmt.Sprintf("client_%d", time.Now().UnixNano())
}

// AddPlayer adds a player to matchmaking queue
func (mq *MatchmakingQueue) AddPlayer(client *Client, gameMode string) {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	if gameMode == game.ModeSimple {
		mq.simpleQueue = append(mq.simpleQueue, client)
	} else {
		mq.enhancedQueue = append(mq.enhancedQueue, client)
	}
}

// ProcessMatches tries to create matches from queued players
func (mq *MatchmakingQueue) ProcessMatches(server *Server) {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	// Process simple mode queue
	if len(mq.simpleQueue) >= 2 {
		player1 := mq.simpleQueue[0]
		player2 := mq.simpleQueue[1]
		mq.simpleQueue = mq.simpleQueue[2:]

		go server.createMatch(player1, player2, game.ModeSimple)
	}

	// Process enhanced mode queue
	if len(mq.enhancedQueue) >= 2 {
		player1 := mq.enhancedQueue[0]
		player2 := mq.enhancedQueue[1]
		mq.enhancedQueue = mq.enhancedQueue[2:]

		go server.createMatch(player1, player2, game.ModeEnhanced)
	}
}

// createMatch creates a new game between two players
func (s *Server) createMatch(client1, client2 *Client, gameMode string) {
	gameID := fmt.Sprintf("game_%d", time.Now().Unix())

	// Create players for game
	gamePlayer1 := s.dataManager.CreatePlayerForGame(client1.Player, client1.ID)
	gamePlayer2 := s.dataManager.CreatePlayerForGame(client2.Player, client2.ID)

	// Create game engine
	gameEngine := game.NewGameEngine(gamePlayer1, gamePlayer2, gameMode, s.dataManager.GetGameSpecs())

	// Store game
	s.mu.Lock()
	s.games[gameID] = gameEngine
	client1.GameID = gameID
	client2.GameID = gameID
	s.mu.Unlock()

	// Start game
	gameEngine.StartGame()

	s.logger.Info("Match created: %s vs %s in %s mode", client1.Username, client2.Username, gameMode)
}
