// Package server implements the TCP server for Clash Royale TCR
package server

import (
	"bufio"
	// "encoding/json"
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

	// End turn using game engine
	if err := gameEngine.EndTurn(client.ID); err != nil {
		return s.sendError(client, "END_TURN_FAILED", err.Error())
	}

	// Get updated game state
	updatedGameState := gameEngine.GetGameState()

	// Broadcast turn change to both players
	response := network.NewMessage(network.MsgTurnChange, "", client.GameID)
	response.SetData("current_turn", updatedGameState.CurrentTurn)
	response.SetData("game_state", updatedGameState)

	s.logger.Info("Turn switched from %s to %s", client.Username, updatedGameState.CurrentTurn)

	return s.broadcastToGame(client.GameID, response)
}

// handleSurrender processes surrender actions
func (s *Server) handleSurrender(client *Client, msg *network.Message) error {
	gameEngine := s.getClientGame(client)
	if gameEngine == nil {
		return s.sendError(client, "NO_ACTIVE_GAME", "No active game found")
	}

	// Use GameEngine surrender method
	if err := gameEngine.Surrender(client.ID); err != nil {
		return s.sendError(client, "SURRENDER_FAILED", err.Error())
	}

	s.logger.Info("Player %s surrendered", client.Username)
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
	s.logger.Debug("Sending message to %s: %s", client.Username, msg.Type)
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
		// ✅ XỬ LÝ DISCONNECT TRONG GAME
		if client.GameID != "" {
			// Thông báo opponent win
			s.handlePlayerDisconnect(client.GameID, clientID)
		}

		client.IsActive = false
		delete(s.clients, clientID)
	}
}

func (s *Server) cleanupInactiveClients() {
	s.mu.Lock()
	defer s.mu.Unlock()

	timeout := 100 * time.Minute
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

// endGame handles game conclusion properly
func (s *Server) endGame(gameID string, reason string) error {
	s.mu.Lock()
	gameEngine, exists := s.games[gameID]
	if !exists {
		s.mu.Unlock()
		return fmt.Errorf("game not found")
	}

	// Remove game from active games
	delete(s.games, gameID)
	s.mu.Unlock()

	gameState := gameEngine.GetGameState()

	// Find clients in this game
	var client1, client2 *Client
	for _, client := range s.clients {
		if client.GameID == gameID {
			if client.ID == gameState.Player1.ID {
				client1 = client
			} else if client.ID == gameState.Player2.ID {
				client2 = client
			}
		}
	}

	// Determine winner/loser based on reason
	var winner, loser *Client
	if reason == "surrender" {
		// The current player (who called surrender) loses
		// Find who surrendered by checking current context
		// For now, assume Player1 surrendered if we can't determine
		loser = client1
		winner = client2
	} else if gameState.Winner == gameState.Player1.ID {
		winner = client1
		loser = client2
	} else if gameState.Winner == gameState.Player2.ID {
		winner = client2
		loser = client1
	}

	// Calculate rewards
	var winnerExp, loserExp int
	if reason == "draw" || gameState.Winner == "draw" {
		winnerExp = s.dataManager.CalculateGameEndEXP(false, true) // draw
		loserExp = winnerExp
	} else {
		winnerExp = s.dataManager.CalculateGameEndEXP(true, false) // win
		loserExp = s.dataManager.CalculateGameEndEXP(false, false) // lose
	}

	// Send game end notifications
	if winner != nil && loser != nil {
		s.sendGameEndNotification(winner, true, fmt.Sprintf("%d", winnerExp), reason)
		s.sendGameEndNotification(loser, false, fmt.Sprintf("%d", loserExp), reason)
		winner.GameID = ""
		loser.GameID = ""
	} else if gameState.Winner == "draw" {
		// Handle draw case
		if client1 != nil {
			s.sendGameEndNotification(client1, false, fmt.Sprintf("%d", loserExp), reason)
			client1.GameID = ""
		}
		if client2 != nil {
			s.sendGameEndNotification(client2, false, fmt.Sprintf("%d", loserExp), reason)
			client2.GameID = ""
		}
	}

	s.logger.Info("Game %s ended: %s", gameID, reason)
	return nil
}

// sendGameEndNotification sends game end notification to a player
func (s *Server) sendGameEndNotification(client *Client, won bool, expGained, reason string) error {
	if client == nil {
		return fmt.Errorf("client is nil")
	}

	msg := network.NewMessage(network.MsgGameEnd, client.ID, "")

	var winnerName string
	if won {
		winnerName = client.Username
	} else {
		if reason == "draw" {
			winnerName = "draw"
		} else {
			winnerName = "opponent"
		}
	}

	msg.SetData("game_end", map[string]interface{}{
		"winner":     winnerName,
		"reason":     reason,
		"exp_gained": expGained,
		"stats": map[string]interface{}{
			"towers_destroyed": 0,   // Would be calculated from game state
			"troops_deployed":  0,   // Would be tracked during game
			"damage_dealt":     0,   // Would be tracked during game
			"game_duration":    180, // Default for now
		},
	})

	return s.sendMessage(client, msg)
}

// sendGameEndToClients sends game end notification to all players in game
func (s *Server) sendGameEndToClients(gameID, reason string, winnerExp, loserExp, winnerTrophy, loserTrophy int) {
	for _, client := range s.clients {
		var expGained int
		var isWinner bool
		if client.GameID == gameID && client.IsActive {

			if reason == "surrender" {
				// Logic to determine winner/loser
				// For now, assume all clients are winners for demonstration
				isWinner = true
			}
			_ = isWinner // Suppress unused variable warning
			expGained = winnerExp
		}

		msg := network.NewMessage(network.MsgGameEnd, client.ID, "")
		msg.SetData("game_end", map[string]interface{}{
			"winner":     client.ID, // Simplified
			"reason":     reason,
			"exp_gained": expGained,
			// "trophy_change": trophyChange,
		})

		s.sendMessage(client, msg)
		client.GameID = "" // Clear game ID
	}
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
	gameEngine := game.NewGameEngine(gamePlayer1, gamePlayer2, gameMode, s.dataManager.GetGameSpecs(), s.dataManager)

	// Store game
	s.mu.Lock()
	s.games[gameID] = gameEngine
	client1.GameID = gameID
	client2.GameID = gameID
	s.mu.Unlock()

	// 🔥 ADD THIS: Notify players of match found
	s.notifyMatchFound(client1, client2, gameID, gameMode)

	// Start game
	gameEngine.StartGame()
	go s.handleGameEvents(gameEngine)

	// 🔥 ADD THIS: Send game start data
	s.sendGameStart(client1, client2, gameEngine)

	s.logger.Info("Match created: %s vs %s in %s mode", client1.Username, client2.Username, gameMode)
}

// handleGameEvents listens to game engine events and broadcasts them
func (s *Server) handleGameEvents(gameEngine *game.GameEngine) {
	eventChan := gameEngine.GetEventChannel()

	for s.isRunning && gameEngine.IsRunning() {
		select {
		case event := <-eventChan:
			// Broadcast event to all players in this game
			gameState := gameEngine.GetGameState()
			s.broadcastGameEvent(gameState.ID, event, *gameState)

			// Handle special events
			if event.Type == "TURN_END" {
				// Send turn change message
				response := network.NewMessage(network.MsgTurnChange, "", gameState.ID)
				response.SetData("current_turn", gameState.CurrentTurn)
				response.SetData("game_state", gameState)
				s.broadcastToGame(gameState.ID, response)
			} else if event.Type == "GAME_END" {
				// Handle game end
				s.endGame(gameState.ID, "king_tower_destroyed")
			}
			if event.Type == "EXP_GAINED" {
				// Broadcast EXP gain to client
				s.broadcastGameEvent(gameState.ID, event, *gameState)
			}
		case <-time.After(100 * time.Millisecond):
			continue
		}
	}
}

// notifyMatchFound sends match found notification to both players
func (s *Server) notifyMatchFound(client1, client2 *Client, gameID, gameMode string) {
	// Notify client1
	msg1 := network.NewMessage(network.MsgMatchFound, client1.ID, gameID)
	msg1.SetData("match_found", map[string]interface{}{
		"game_id":   gameID,
		"opponent":  map[string]interface{}{"username": client2.Username, "level": client2.Player.Level},
		"game_mode": gameMode,
		"your_turn": gameMode == game.ModeSimple,
	})
	s.sendMessage(client1, msg1)

	// Notify client2
	msg2 := network.NewMessage(network.MsgMatchFound, client2.ID, gameID)
	msg2.SetData("match_found", map[string]interface{}{
		"game_id":   gameID,
		"opponent":  map[string]interface{}{"username": client1.Username, "level": client1.Player.Level},
		"game_mode": gameMode,
		"your_turn": false,
	})
	s.sendMessage(client2, msg2)
}

// sendGameStart sends game initialization data to both players
func (s *Server) sendGameStart(client1, client2 *Client, gameEngine *game.GameEngine) {
	gameState := gameEngine.GetGameState()

	// Send to player 1
	msg1 := network.NewMessage(network.MsgGameStart, client1.ID, gameState.ID)
	msg1.SetData("game_start", map[string]interface{}{
		"game_state":        gameState,
		"your_troops":       gameState.Player1.Troops,
		"your_towers":       gameState.Player1.Towers,
		"countdown_seconds": 3,
	})
	s.sendMessage(client1, msg1)

	// Send to player 2
	msg2 := network.NewMessage(network.MsgGameStart, client2.ID, gameState.ID)
	msg2.SetData("game_start", map[string]interface{}{
		"game_state":        gameState,
		"your_troops":       gameState.Player2.Troops,
		"your_towers":       gameState.Player2.Towers,
		"countdown_seconds": 3,
	})
	s.sendMessage(client2, msg2)
}

func (s *Server) handlePlayerDisconnect(gameID, disconnectedClientID string) {
	// Tìm opponent
	for _, client := range s.clients {
		if client.GameID == gameID && client.ID != disconnectedClientID && client.IsActive {
			// Gửi thông báo disconnect
			msg := network.NewMessage("PLAYER_DISCONNECT", client.ID, gameID)
			msg.SetData("disconnect_info", map[string]interface{}{
				"disconnected_player": disconnectedClientID,
				"winner":              client.ID,
				"reason":              "opponent_disconnect",
			})
			s.sendMessage(client, msg)

			// Clear game ID
			client.GameID = ""
			break
		}
	}

	// Remove game
	delete(s.games, gameID)
	s.logger.Info("Game %s ended due to player disconnect", gameID)
}
