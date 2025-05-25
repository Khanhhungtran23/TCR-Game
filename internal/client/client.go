// Package client handles the TCP client and game interaction
package client

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"

	"time"

	"tcr-game/internal/game"
	"tcr-game/internal/network"
	"tcr-game/pkg/logger"
)

// Client represents the game client
type Client struct {
	conn            net.Conn
	display         *Display
	input           *InputHandler
	player          *game.PlayerData
	gameState       *game.GameState
	myTroops        []game.Troop
	myTowers        []game.Tower
	isConnected     bool
	isInGame        bool
	waitingForMatch bool
	logger          *logger.Logger
	writer          *bufio.Writer
	reader          *bufio.Scanner
	serverAddr      string
	clientID        string
}

// NewClient creates a new client instance
func NewClient(serverAddr string) *Client {
	display := NewDisplay()
	return &Client{
		display:         display,
		input:           NewInputHandler(display),
		logger:          logger.Client,
		isConnected:     false,
		isInGame:        false,
		waitingForMatch: false,
		serverAddr:      serverAddr,
	}
}

// Start initializes and starts the client
func (c *Client) Start() error {
	c.display.PrintBanner()
	c.logger.Info("Client starting...")

	// Connect to server
	if err := c.connectToServer(); err != nil {
		c.display.PrintError(fmt.Sprintf("Failed to connect to server: %v", err))
		return err
	}

	// Start message handler
	go c.messageHandler()

	// Authentication flow
	if err := c.authenticate(); err != nil {
		c.display.PrintError(fmt.Sprintf("Authentication failed: %v", err))
		return err
	}

	// Main game loop
	return c.runMainLoop()
}

// connectToServer establishes TCP connection
func (c *Client) connectToServer() error {
	c.display.PrintInfo("Connecting to server...")

	conn, err := net.Dial("tcp", c.serverAddr)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.conn = conn
	c.writer = bufio.NewWriter(conn)
	c.reader = bufio.NewScanner(conn)
	c.isConnected = true

	c.display.PrintServerStatus("Connected to server")
	c.logger.Info("Connected to server at %s", c.serverAddr)
	return nil
}

// authenticate handles login/register flow
func (c *Client) authenticate() error {
	for {
		c.display.PrintSeparator()
		c.display.PrintInfo("🔐 AUTHENTICATION 🔐")
		c.display.PrintInfo("1. Login")
		c.display.PrintInfo("2. Register")
		c.display.PrintInfo("3. Quit")

		choice := c.input.GetMenuChoice(1, 3)

		switch choice {
		case 1:
			if err := c.handleLogin(); err != nil {
				c.display.PrintError(fmt.Sprintf("Login failed: %v", err))
				continue
			}
			return nil
		case 2:
			if err := c.handleRegister(); err != nil {
				c.display.PrintError(fmt.Sprintf("Registration failed: %v", err))
				continue
			}
			return nil
		case 3:
			return fmt.Errorf("user quit")
		}
	}
}

// handleLogin processes user login
func (c *Client) handleLogin() error {
	username := c.input.GetUsername()
	password := c.input.GetStringInput("Enter password: ", 4, 50)

	// Send login message
	msg := network.CreateAuthMessage(network.MsgLogin, username, password)
	if err := c.sendMessage(msg); err != nil {
		return err
	}

	// Wait for response (handled in messageHandler)
	return c.waitForAuth()
}

// handleRegister processes user registration
func (c *Client) handleRegister() error {
	username := c.input.GetUsername()
	password := c.input.GetStringInput("Enter password (min 4 chars): ", 4, 50)
	confirmPassword := c.input.GetStringInput("Confirm password: ", 4, 50)

	if password != confirmPassword {
		return fmt.Errorf("passwords do not match")
	}

	// Send register message
	msg := network.CreateAuthMessage(network.MsgRegister, username, password)
	if err := c.sendMessage(msg); err != nil {
		return err
	}

	return c.waitForAuth()
}

// waitForAuth waits for authentication response
func (c *Client) waitForAuth() error {
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout.C:
			return fmt.Errorf("authentication timeout")
		case <-ticker.C:
			if c.player != nil {
				return nil
			}
		}
	}
}

// runMainLoop handles the main game menu
func (c *Client) runMainLoop() error {
	for {
		// Check if we're in game first
		if c.isInGame {
			// Handle in-game actions
			if err := c.handleGameplay(); err != nil {
				c.display.PrintError(fmt.Sprintf("Gameplay error: %v", err))
				c.isInGame = false
				continue
			}
			continue
		}

		// Check if waiting for match
		if c.waitingForMatch {
			// Just wait and let message handler process responses
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Main menu
		c.display.PrintSeparator()
		c.display.PrintInfo("🎮 CLASH ROYALE TCR 🎮")
		c.display.PrintInfo(fmt.Sprintf("Welcome, %s!", c.player.Username))
		c.display.PrintInfo(fmt.Sprintf("Level: %d | EXP: %d",
			c.player.Level, c.player.EXP))
		c.display.PrintInfo("")
		c.display.PrintInfo("1. Find Match (Simple TCR)")
		c.display.PrintInfo("2. Find Match (Enhanced TCR)")
		c.display.PrintInfo("3. View Profile")
		c.display.PrintInfo("4. Quit")

		choice := c.input.GetMenuChoice(1, 4)

		switch choice {
		case 1:
			c.findMatch(game.ModeSimple)
		case 2:
			c.findMatch(game.ModeEnhanced)
		case 3:
			c.showProfile()
		case 4:
			c.display.PrintInfo("Thanks for playing!")
			return nil
		}
	}
}

// findMatch initiates matchmaking
func (c *Client) findMatch(gameMode string) {
	c.display.PrintInfo(fmt.Sprintf("Searching for %s mode match...", gameMode))

	msg := network.CreateMatchRequest(c.clientID, gameMode)
	if err := c.sendMessage(msg); err != nil {
		c.display.PrintError(fmt.Sprintf("Failed to request match: %v", err))
		return
	}

	c.display.PrintInfo("Waiting for opponent...")
	c.waitingForMatch = true
}

// handleGameplay manages in-game interactions
func (c *Client) handleGameplay() error {
	if c.gameState == nil {
		return fmt.Errorf("no active game state")
	}

	c.display.PrintSeparator()
	c.display.PrintInfo("🔥 BATTLE IN PROGRESS 🔥")

	// Show current game status
	c.showGameStatus()

	// Get player action (pass game mode)
	action := c.input.GetGameAction(c.gameState.GameMode)

	switch action {
	case "play":
		return c.handlePlayCard()
	case "attack":
		return c.handleAttack()
	case "info":
		c.showDetailedGameInfo()
		return nil
	case "end":
		return c.handleEndTurn()
	case "surrender":
		return c.handleSurrender()
	default:
		c.display.PrintWarning("Invalid action")
		return nil
	}
}

// handleAttack handles attacking phase
func (c *Client) handleAttack() error {
	if len(c.myTroops) == 0 {
		c.display.PrintWarning("No troops available")
		return nil
	}

	// Get enemy towers
	var enemyTowers []game.Tower
	if c.gameState.Player1.ID == c.clientID {
		enemyTowers = c.gameState.Player2.Towers
	} else {
		enemyTowers = c.gameState.Player1.Towers
	}

	// Let user choose attacker and target
	attackerIndex, targetType, targetIndex, err := c.input.GetAttackChoice(c.myTroops, enemyTowers, c.gameState.GameMode)
	if err != nil {
		c.display.PrintWarning(err.Error())
		return nil
	}

	selectedTroop := c.myTroops[attackerIndex]
	targetTower := enemyTowers[targetIndex]

	// Send attack message
	msg := network.CreateAttackMessage(c.clientID, c.gameState.ID, selectedTroop.Name, targetType, string(targetTower.Name))
	return c.sendMessage(msg)
}

// handlePlayCard handles troop summoning
func (c *Client) handlePlayCard() error {
	if len(c.myTroops) == 0 {
		c.display.PrintWarning("No troops available")
		return nil
	}

	// Get current mana (only for Enhanced mode)
	var currentMana int = 999 // Default: unlimited for Simple mode
	if c.gameState.GameMode == game.ModeEnhanced {
		if c.gameState.Player1.ID == c.clientID {
			currentMana = c.gameState.Player1.Mana
		} else {
			currentMana = c.gameState.Player2.Mana
		}
	}

	// Let user choose troop (pass game mode)
	troopIndex, err := c.input.GetTroopChoice(c.myTroops, currentMana, c.gameState.GameMode)
	if err != nil {
		c.display.PrintWarning(err.Error())
		return nil
	}

	selectedTroop := c.myTroops[troopIndex]

	// Send summon message
	msg := network.CreateSummonMessage(c.clientID, c.gameState.ID, selectedTroop.Name)
	return c.sendMessage(msg)
}

// handleEndTurn handles turn ending (Simple mode)
func (c *Client) handleEndTurn() error {
	if c.gameState.GameMode != game.ModeSimple {
		c.display.PrintWarning("End turn only available in Simple mode")
		return nil
	}

	msg := network.NewMessage(network.MsgEndTurn, c.clientID, c.gameState.ID)
	return c.sendMessage(msg)
}

// handleSurrender handles surrender
func (c *Client) handleSurrender() error {
	if !c.input.GetConfirmation("Are you sure you want to surrender?") {
		return nil
	}

	msg := network.NewMessage(network.MsgSurrender, c.clientID, c.gameState.ID)
	return c.sendMessage(msg)
}

// messageHandler processes incoming messages from server
func (c *Client) messageHandler() {
	for c.isConnected {
		if !c.reader.Scan() {
			if c.isConnected {
				c.logger.Error("Lost connection to server")
				c.display.PrintError("Lost connection to server")
			}
			break
		}

		data := c.reader.Bytes()
		c.logger.Debug("Received raw message: %s", string(data))

		if err := c.processServerMessage(data); err != nil {
			c.logger.Error("Error processing server message: %v", err)
		}
	}
}

// processServerMessage handles incoming server messages
func (c *Client) processServerMessage(data []byte) error {
	msg, err := network.FromJSON(data)
	if err != nil {
		return fmt.Errorf("failed to parse message: %w", err)
	}

	c.logger.Debug("Received message type: %s", msg.Type)

	switch msg.Type {
	case network.MsgAuthOK:
		return c.handleAuthSuccess(msg)
	case network.MsgAuthFail:
		return c.handleAuthFail(msg)
	case network.MsgMatchFound:
		return c.handleMatchFound(msg)
	case network.MsgGameStart:
		return c.handleGameStart(msg)
	case network.MsgGameEvent:
		return c.handleGameEvent(msg)
	case network.MsgGameEnd:
		return c.handleGameEnd(msg)
	case network.MsgTurnChange:
		return c.handleTurnChange(msg)
	case network.MsgError:
		return c.handleError(msg)
	default:
		c.logger.Debug("Unhandled message type: %s", msg.Type)
	}

	return nil
}

// handleAuthSuccess processes successful authentication
func (c *Client) handleAuthSuccess(msg *network.Message) error {
	authResp, ok := msg.Data["auth_response"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid auth response format")
	}

	c.clientID = msg.PlayerID

	// Extract player data
	playerDataJson, _ := json.Marshal(authResp["player_data"])
	if err := json.Unmarshal(playerDataJson, &c.player); err != nil {
		return fmt.Errorf("failed to parse player data: %w", err)
	}

	message, _ := authResp["message"].(string)
	c.display.PrintInfo(message)
	c.logger.Info("Authentication successful for %s", c.player.Username)
	return nil
}

// handleAuthFail processes failed authentication
func (c *Client) handleAuthFail(msg *network.Message) error {
	authResp, ok := msg.Data["auth_response"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid auth response format")
	}

	message, _ := authResp["message"].(string)
	c.display.PrintError(message)
	return nil
}

// handleMatchFound processes match found notification
func (c *Client) handleMatchFound(msg *network.Message) error {
	c.display.PrintInfo("Match found! Preparing for battle...")
	c.waitingForMatch = false
	return nil
}

// handleGameStart processes game start notification
func (c *Client) handleGameStart(msg *network.Message) error {
	c.logger.Debug("Processing game start message")

	gameStartData, ok := msg.Data["game_start"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid game start format")
	}

	// Parse game state
	gameStateJson, _ := json.Marshal(gameStartData["game_state"])
	if err := json.Unmarshal(gameStateJson, &c.gameState); err != nil {
		return fmt.Errorf("failed to parse game state: %w", err)
	}

	// Parse troops and towers
	troopsJson, _ := json.Marshal(gameStartData["your_troops"])
	if err := json.Unmarshal(troopsJson, &c.myTroops); err != nil {
		return fmt.Errorf("failed to parse troops: %w", err)
	}

	towersJson, _ := json.Marshal(gameStartData["your_towers"])
	if err := json.Unmarshal(towersJson, &c.myTowers); err != nil {
		return fmt.Errorf("failed to parse towers: %w", err)
	}

	// Important: Set game state flags
	c.isInGame = true
	c.waitingForMatch = false

	c.display.PrintGameStart(3)
	c.display.PrintGameMode(c.gameState.GameMode)
	c.display.PrintInfo("🔥 GAME STARTED! 🔥")

	c.logger.Info("Game started successfully")
	return nil
}

// handleGameEvent processes game events
func (c *Client) handleGameEvent(msg *network.Message) error {
	eventData, ok := msg.Data["game_event"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid game event format")
	}

	// Parse event
	eventJson, _ := json.Marshal(eventData["event"])
	var event game.CombatAction
	if err := json.Unmarshal(eventJson, &event); err != nil {
		return fmt.Errorf("failed to parse event: %w", err)
	}

	// Parse updated game state
	stateJson, _ := json.Marshal(eventData["game_state"])
	if err := json.Unmarshal(stateJson, &c.gameState); err != nil {
		return fmt.Errorf("failed to parse game state: %w", err)
	}

	// Display event based on type
	c.displayGameEvent(event)
	return nil
}

// displayGameEvent shows game events with appropriate colors
func (c *Client) displayGameEvent(event game.CombatAction) {
	isMyAction := event.PlayerID == c.clientID

	switch event.Type {
	case game.ActionSummon:
		playerName := c.getPlayerName(event.PlayerID)
		troopName := string(event.TroopName)
		c.display.PrintTroopSummoned(playerName, troopName, isMyAction)

	case game.ActionAttack:
		attacker := string(event.TroopName)
		target := event.TargetName
		c.display.PrintAttack(attacker, target, event.Damage, event.IsCrit)

	case game.ActionHeal:
		healer := string(event.TroopName)
		target := event.TargetName
		c.display.PrintHeal(healer, target, event.HealAmount)
	}
}

// handleGameEnd processes game conclusion
func (c *Client) handleGameEnd(msg *network.Message) error {
	c.isInGame = false

	gameEndData, ok := msg.Data["game_end"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid game end format")
	}

	winner, _ := gameEndData["winner"].(string)
	expGained, _ := gameEndData["exp_gained"].(float64)

	isWinner := winner == c.clientID || (winner == "draw")

	towersDestroyed := map[string]int{
		"You":      c.gameState.TowersKilled.Player1,
		"Opponent": c.gameState.TowersKilled.Player2,
	}

	c.display.PrintGameEnd(winner, isWinner, towersDestroyed)
	c.display.PrintExperience(int(expGained), 0)
	c.display.PrintDataSaved()

	c.input.WaitForEnter("Press Enter to return to menu...")
	return nil
}

// handleTurnChange processes turn changes
func (c *Client) handleTurnChange(msg *network.Message) error {
	currentTurn, _ := msg.Data["current_turn"].(string)

	// Update game state if provided
	if gameStateData, exists := msg.Data["game_state"]; exists {
		gameStateJson, _ := json.Marshal(gameStateData)
		if err := json.Unmarshal(gameStateJson, &c.gameState); err != nil {
			c.logger.Error("Failed to parse updated game state: %v", err)
		}
	}

	if currentTurn == c.clientID {
		c.display.PrintInfo("🔥 It's your turn! 🔥")
	} else {
		c.display.PrintInfo("⏳ Waiting for opponent's turn...")
	}

	return nil
}

// handleError processes error messages
func (c *Client) handleError(msg *network.Message) error {
	errorData, ok := msg.Data["error"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid error format")
	}

	message, _ := errorData["message"].(string)
	c.display.PrintError(message)
	return nil
}

// Helper methods

func (c *Client) sendMessage(msg *network.Message) error {
	data, err := msg.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize message: %w", err)
	}

	_, err = c.writer.Write(append(data, '\n'))
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return c.writer.Flush()
}

func (c *Client) showProfile() {
	c.display.PrintSeparator()
	c.display.PrintInfo("📊 PLAYER PROFILE 📊")
	c.display.PrintInfo(fmt.Sprintf("Username: %s", c.player.Username))
	c.display.PrintInfo(fmt.Sprintf("Level: %d", c.player.Level))
	c.display.PrintInfo(fmt.Sprintf("EXP: %d", c.player.EXP))
	c.display.PrintInfo(fmt.Sprintf("Games Played: %d", c.player.GamesPlayed))
	c.display.PrintInfo(fmt.Sprintf("Games Won: %d", c.player.GamesWon))

	if c.player.GamesPlayed > 0 {
		winRate := float64(c.player.GamesWon) / float64(c.player.GamesPlayed) * 100
		c.display.PrintInfo(fmt.Sprintf("Win Rate: %.1f%%", winRate))
	}

	c.input.WaitForEnter("")
}

// showGameStatus displays current game status
func (c *Client) showGameStatus() {
	if c.gameState == nil {
		return
	}

	// Show basic game info
	c.display.PrintInfo(fmt.Sprintf("Game Mode: %s", c.gameState.GameMode))

	// Only show mana in Enhanced mode
	if c.gameState.GameMode == game.ModeEnhanced {
		var myMana int
		if c.gameState.Player1.ID == c.clientID {
			myMana = c.gameState.Player1.Mana
		} else {
			myMana = c.gameState.Player2.Mana
		}
		c.display.PrintInfo(fmt.Sprintf("Your Mana: %d/%d", myMana, game.MaxMana))
	}

	// Show turn info for Simple mode
	if c.gameState.GameMode == game.ModeSimple {
		if c.gameState.CurrentTurn == c.clientID {
			c.display.PrintInfo("🔥 YOUR TURN 🔥")
		} else {
			c.display.PrintInfo("⏳ Opponent's Turn")
		}
	}

	// Show available troops
	c.display.PrintTroops(c.myTroops)
}

func (c *Client) showDetailedGameInfo() {
	c.display.PrintSeparator()
	c.display.PrintInfo("🎯 DETAILED GAME INFO 🎯")

	// Show my towers
	c.display.PrintTowerStatus(c.myTowers, "Your")

	// Show my troops
	c.display.PrintInfo("\n=== Your Troops ===")
	for i, troop := range c.myTroops {
		c.display.PrintInfo(fmt.Sprintf("%d. %s (HP: %d, ATK: %d, DEF: %d, MANA: %d)",
			i+1, troop.Name, troop.HP, troop.ATK, troop.DEF, troop.MANA))
	}

	c.input.WaitForEnter("")
}

func (c *Client) getPlayerName(playerID string) string {
	if playerID == c.clientID {
		return c.player.Username
	}

	// Return opponent name
	if c.gameState != nil {
		if c.gameState.Player1.ID == playerID {
			return c.gameState.Player1.Username
		} else if c.gameState.Player2.ID == playerID {
			return c.gameState.Player2.Username
		}
	}

	return "Unknown"
}

// Close closes the client connection
func (c *Client) Close() error {
	c.isConnected = false
	c.isInGame = false

	if c.conn != nil {
		return c.conn.Close()
	}

	return nil
}
