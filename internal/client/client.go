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
	// ✅ UPDATED: Better troop deployment tracking
	deployedTroops     map[string]bool // Track which troops have been deployed
	troopAttackCount   map[string]int  // Track attacks per troop per turn
	deployedThisTurn   []string        // Only troops deployed THIS turn
	lastWaitingMessage string
}

// NewClient creates a new client instance
func NewClient(serverAddr string) *Client {
	display := NewDisplay()
	return &Client{
		display:          display,
		input:            NewInputHandler(display),
		logger:           logger.Client,
		isConnected:      false,
		isInGame:         false,
		waitingForMatch:  false,
		serverAddr:       serverAddr,
		deployedTroops:   make(map[string]bool),
		troopAttackCount: make(map[string]int),
		deployedThisTurn: []string{},
	}
}

// ✅ UPDATED: handleGameEnd with detailed EXP display
func (c *Client) handleGameEnd(msg *network.Message) error {
	c.isInGame = false

	gameEndData, ok := msg.Data["game_end"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid game end format")
	}

	winner, _ := gameEndData["winner"].(string)
	expGained, _ := gameEndData["exp_gained"].(float64)
	opponentExpGained, _ := gameEndData["opponent_exp_gained"].(float64)

	isWinner := winner == c.clientID
	isDraw := winner == "draw"

	towersDestroyed := map[string]int{
		"You":      c.gameState.TowersKilled.Player1,
		"Opponent": c.gameState.TowersKilled.Player2,
	}

	// ✅ IMPROVED: Better game end display
	if isDraw {
		c.display.PrintGameEnd("draw", false, towersDestroyed)
	} else {
		c.display.PrintGameEnd(winner, isWinner, towersDestroyed)
	}

	// ✅ NEW: Display detailed EXP breakdown
	c.display.PrintExperience(int(expGained), int(opponentExpGained))

	// ✅ NEW: Check for level up
	if c.player != nil {
		oldLevel := c.player.Level
		c.player.EXP += int(expGained)

		// Simple level up check (should match server logic)
		requiredEXP := 100 + (oldLevel-1)*15 // Matches server calculation
		if c.player.EXP >= requiredEXP {
			c.player.Level++
			c.player.EXP -= requiredEXP
			c.display.PrintLevelUp(c.player.Level, true)
		}
	}

	c.display.PrintDataSaved()
	c.input.WaitForEnter("Press Enter to return to menu...")
	return nil
}

// handleTurnChange processes turn changes
func (c *Client) handleTurnChange(msg *network.Message) error {
	currentTurn, _ := msg.Data["current_turn"].(string)

	c.logger.Debug("Received turn change: %s -> %s", c.gameState.CurrentTurn, currentTurn)

	if gameStateData, exists := msg.Data["game_state"]; exists {
		gameStateJson, _ := json.Marshal(gameStateData)
		if err := json.Unmarshal(gameStateJson, &c.gameState); err != nil {
			c.logger.Error("Failed to parse updated game state: %v", err)
		}
	}

	oldTurn := c.gameState.CurrentTurn
	c.gameState.CurrentTurn = currentTurn

	// ✅ RESET: Only reset turn-specific counters when my turn starts
	if c.gameState.GameMode == game.ModeSimple && currentTurn == c.clientID {
		c.deployedThisTurn = []string{}
		c.troopAttackCount = make(map[string]int)
		// Initialize attack counters for deployed troops
		for troopName := range c.deployedTroops {
			c.troopAttackCount[troopName] = 0
		}
	}

	c.display.PrintSeparator()
	c.display.PrintInfo(fmt.Sprintf("🔄 TURN CHANGE: %s → %s",
		c.getPlayerName(oldTurn), c.getPlayerName(currentTurn)))

	if currentTurn == c.clientID {
		c.display.PrintInfo("🔥 It's YOUR TURN! 🔥")
		c.display.PrintInfo("Available actions: play, attack, info, debug, end, surrender")
		c.display.PrintInfo("💡 Remember: 1 troop deployment per turn, each deployed troop can attack once")
	} else {
		opponentName := c.getPlayerName(currentTurn)
		c.display.PrintInfo(fmt.Sprintf("⏳ Waiting for %s's turn...", opponentName))
		c.display.PrintInfo("You can use 'info' or 'debug' to check game status")
	}
	c.display.PrintSeparator()

	c.logger.Debug("Turn change processed: Current turn is now %s", c.gameState.CurrentTurn)
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

func (c *Client) showGameStatus() {
	if c.gameState == nil {
		return
	}

	c.display.PrintInfo(fmt.Sprintf("Game Mode: %s", c.gameState.GameMode))

	if c.gameState.GameMode == game.ModeEnhanced {
		c.display.PrintInfo(fmt.Sprintf("Time Left: %d seconds", c.gameState.TimeLeft))

		var myMana int
		if c.gameState.Player1.ID == c.clientID {
			myMana = c.gameState.Player1.Mana
		} else {
			myMana = c.gameState.Player2.Mana
		}
		c.display.PrintInfo(fmt.Sprintf("Your Mana: %d/%d", myMana, game.MaxMana))
	}

	if c.gameState.GameMode == game.ModeSimple {
		if c.gameState.CurrentTurn == c.clientID {
			c.display.PrintInfo("🔥 YOUR TURN 🔥")
		} else {
			c.display.PrintInfo("⏳ Opponent's Turn")
		}
	}

	c.display.PrintTroops(c.myTroops)
}

func (c *Client) showDetailedGameInfo() {
	c.display.PrintSeparator()
	c.display.PrintInfo("🎯 DETAILED GAME INFO 🎯")

	c.display.PrintInfo(fmt.Sprintf("Game Mode: %s", c.gameState.GameMode))
	if c.gameState.GameMode == game.ModeSimple {
		if c.gameState.CurrentTurn == c.clientID {
			c.display.PrintInfo("🔥 YOUR TURN 🔥")
		} else {
			c.display.PrintInfo("⏳ Opponent's Turn")
		}

		// ✅ UPDATED: Show deployment status
		c.display.PrintInfo(fmt.Sprintf("Troops Deployed This Turn: %d/1", len(c.deployedThisTurn)))
		if len(c.deployedThisTurn) > 0 {
			c.display.PrintInfo(fmt.Sprintf("Deployed: %v", c.deployedThisTurn))
		}
	}

	// Show my towers with detailed HP
	c.display.PrintInfo("\n=== Your Towers ===")
	var myTowers []game.Tower
	if c.gameState.Player1.ID == c.clientID {
		myTowers = c.gameState.Player1.Towers
	} else {
		myTowers = c.gameState.Player2.Towers
	}

	for i, tower := range myTowers {
		c.display.PrintInfo(fmt.Sprintf("%d. %s: %d/%d HP (%.1f%%)",
			i+1, tower.Name, tower.HP, tower.MaxHP,
			float64(tower.HP)/float64(tower.MaxHP)*100))
	}

	// Show opponent towers
	c.display.PrintInfo("\n=== Opponent Towers ===")
	var opponentTowers []game.Tower
	if c.gameState.Player1.ID == c.clientID {
		opponentTowers = c.gameState.Player2.Towers
	} else {
		opponentTowers = c.gameState.Player1.Towers
	}

	for i, tower := range opponentTowers {
		c.display.PrintInfo(fmt.Sprintf("%d. %s: %d/%d HP (%.1f%%)",
			i+1, tower.Name, tower.HP, tower.MaxHP,
			float64(tower.HP)/float64(tower.MaxHP)*100))
	}

	// ✅ UPDATED: Show troops with deployment and attack status
	c.display.PrintInfo("\n=== Your Troops ===")
	for i, troop := range c.myTroops {
		troopName := string(troop.Name)
		status := ""

		if c.gameState.GameMode == game.ModeSimple {
			if c.deployedTroops[troopName] {
				status = " [DEPLOYED"
				if c.troopAttackCount[troopName] >= 1 {
					status += " - ATTACKED]"
				} else {
					status += " - CAN ATTACK]"
				}
			} else {
				status = " [NOT DEPLOYED]"
			}
		}

		c.display.PrintInfo(fmt.Sprintf("%d. %s%s (HP: %d, ATK: %d, DEF: %d, MANA: %d)",
			i+1, troop.Name, status, troop.HP, troop.ATK, troop.DEF, troop.MANA))
	}

	c.display.PrintInfo(fmt.Sprintf("\nTowers Destroyed - You: %d | Opponent: %d",
		c.gameState.TowersKilled.Player2, c.gameState.TowersKilled.Player1))

	c.input.WaitForEnter("")
}

func (c *Client) getPlayerName(playerID string) string {
	if playerID == c.clientID {
		return c.player.Username
	}

	if c.gameState != nil {
		if c.gameState.Player1.ID == playerID {
			return c.gameState.Player1.Username
		} else if c.gameState.Player2.ID == playerID {
			return c.gameState.Player2.Username
		}
	}

	return "Unknown"
}

// ✅ UPDATED: Enhanced debug function with attack tracking
func (c *Client) debugGameState() {
	if c.gameState == nil {
		c.display.PrintError("No game state available")
		return
	}

	c.display.PrintInfo("=== DEBUG: GAME STATE ANALYSIS ===")

	c.display.PrintInfo("📋 TURN STATUS:")
	c.display.PrintInfo(fmt.Sprintf("  Game Mode: %s", c.gameState.GameMode))
	c.display.PrintInfo(fmt.Sprintf("  Current Turn (Server): %s", c.gameState.CurrentTurn))
	c.display.PrintInfo(fmt.Sprintf("  My Client ID: %s", c.clientID))
	c.display.PrintInfo(fmt.Sprintf("  Is My Turn: %t", c.gameState.CurrentTurn == c.clientID))

	// ✅ DEPLOYMENT AND ATTACK TRACKING
	c.display.PrintInfo("\n📦 DEPLOYMENT & ATTACK STATUS:")
	c.display.PrintInfo(fmt.Sprintf("  Deployed Troops: %v", c.deployedTroops))
	c.display.PrintInfo(fmt.Sprintf("  Deployed This Turn: %v", c.deployedThisTurn))
	c.display.PrintInfo(fmt.Sprintf("  Attack Counts: %v", c.troopAttackCount))

	availableAttackers := 0
	for troopName, isDeployed := range c.deployedTroops {
		if isDeployed && c.troopAttackCount[troopName] < 1 {
			// Check if troop is still alive
			for _, troop := range c.myTroops {
				if string(troop.Name) == troopName && troop.HP > 0 {
					availableAttackers++
					break
				}
			}
		}
	}
	c.display.PrintInfo(fmt.Sprintf("  Available Attackers: %d", availableAttackers))

	// ✅ ACTION RECOMMENDATIONS
	c.display.PrintInfo("\n💡 RECOMMENDATIONS:")
	if c.gameState.CurrentTurn != c.clientID {
		c.display.PrintInfo("  - Wait for your turn")
	} else {
		if len(c.deployedThisTurn) < 1 {
			c.display.PrintInfo("  - You can deploy 1 more troop this turn")
		}
		if availableAttackers > 0 {
			c.display.PrintInfo(fmt.Sprintf("  - You have %d troops that can attack", availableAttackers))
		}
		if len(c.deployedThisTurn) >= 1 && availableAttackers == 0 {
			c.display.PrintInfo("  - You can end your turn")
		}
	}

	c.input.WaitForEnter("Press Enter to continue...")
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

func (c *Client) handlePlayerDisconnect(opponentName string) {
	c.display.PrintSeparator()
	c.display.PrintInfo(fmt.Sprintf("🚪 %s has left the game!", opponentName))
	c.display.PrintInfo("🎉 You WIN by default! 🎉")
	c.display.PrintSeparator()

	c.isInGame = false
	c.waitingForMatch = false
	c.gameState = nil
	c.myTroops = nil
	c.myTowers = nil
	c.resetGameTracking()

	c.input.WaitForEnter("Press Enter to return to menu...")
}

func (c *Client) handlePlayerDisconnectMessage(msg *network.Message) error {
	_, ok := msg.Data["disconnect_info"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid disconnect info format")
	}

	opponentName := "Opponent"
	if c.gameState != nil {
		if c.gameState.Player1.ID != c.clientID {
			opponentName = c.gameState.Player1.Username
		} else {
			opponentName = c.gameState.Player2.Username
		}
	}

	go func() {
		c.handlePlayerDisconnect(opponentName)
	}()

	return nil
}

// Start initializes and starts the client
func (c *Client) Start() error {
	c.display.PrintBanner()
	c.logger.Info("Client starting...")

	if err := c.connectToServer(); err != nil {
		c.display.PrintError(fmt.Sprintf("Failed to connect to server: %v", err))
		return err
	}

	go c.messageHandler()

	if err := c.authenticate(); err != nil {
		c.display.PrintError(fmt.Sprintf("Authentication failed: %v", err))
		return err
	}

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

	msg := network.CreateAuthMessage(network.MsgLogin, username, password)
	if err := c.sendMessage(msg); err != nil {
		return err
	}

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

	msg := network.CreateAuthMessage(network.MsgRegister, username, password)
	if err := c.sendMessage(msg); err != nil {
		return err
	}

	return c.waitForAuth()
}

// waitForAuth waits for authentication response
func (c *Client) waitForAuth() error {
	timeout := time.NewTimer(30 * time.Second)
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
		if !c.isInGame && c.gameState != nil {
			c.gameState = nil
			c.myTroops = nil
			c.myTowers = nil
			c.resetGameTracking()
		}

		if c.isInGame {
			if err := c.handleGameplay(); err != nil {
				c.display.PrintError(fmt.Sprintf("Gameplay error: %v", err))
				c.isInGame = false
				continue
			}
			continue
		}

		if c.waitingForMatch {
			time.Sleep(100 * time.Millisecond)
			continue
		}

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

// ✅ NEW: Reset game tracking variables
func (c *Client) resetGameTracking() {
	c.deployedTroops = make(map[string]bool)
	c.troopAttackCount = make(map[string]int)
	c.deployedThisTurn = []string{}
	c.lastWaitingMessage = ""
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

	c.showGameStatus()

	for c.isInGame {
		if c.gameState == nil {
			return nil
		}

		if c.gameState.GameMode == game.ModeSimple {
			if c.gameState.CurrentTurn != c.clientID {
				opponentName := c.getPlayerName(c.gameState.CurrentTurn)
				waitingMsg := fmt.Sprintf("⏳ Waiting for %s's turn...", opponentName)

				if c.lastWaitingMessage != waitingMsg {
					c.display.PrintInfo(waitingMsg)
					c.lastWaitingMessage = waitingMsg
				}

				time.Sleep(1000 * time.Millisecond)
				continue
			} else {
				c.lastWaitingMessage = ""
			}
		}

		if !c.isInGame || c.gameState == nil {
			return nil
		}

		action := c.input.GetGameActionWithDebug(c.gameState.GameMode)

		var err error
		switch action {
		case "play":
			err = c.handlePlayCard()
		case "attack":
			err = c.handleAttack()
		case "info":
			c.showDetailedGameInfo()
			continue
		case "debug":
			c.debugGameState()
			continue
		case "end":
			err = c.handleEndTurn()
			if err == nil {
				c.display.PrintInfo("Turn ended. Waiting for server response...")
				time.Sleep(500 * time.Millisecond)
			}
		case "surrender":
			err = c.handleSurrender()
			if err == nil {
				return nil
			}
		default:
			c.display.PrintWarning("Invalid action")
			continue
		}

		if err != nil {
			c.display.PrintError(fmt.Sprintf("Action failed: %v", err))
		}

		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

// ✅ UPDATED: handleAttack with better troop tracking
func (c *Client) handleAttack() error {
	c.syncLocalTroopsFromGameState()

	// ✅ IMPROVED: Show available troops for attack with better filtering
	var availableTroops []game.Troop

	if c.gameState.GameMode == game.ModeSimple {
		for _, troop := range c.myTroops {
			troopName := string(troop.Name)
			// Check if troop is deployed and alive and hasn't attacked this turn
			if c.deployedTroops[troopName] && troop.HP > 0 && c.troopAttackCount[troopName] < 1 {
				availableTroops = append(availableTroops, troop)
			}
		}

		if len(availableTroops) == 0 {
			c.display.PrintWarning("No available troops for attack. Deploy troops first or all deployed troops have already attacked this turn.")
			return nil
		}
	} else {
		for _, troop := range c.myTroops {
			if troop.HP > 0 {
				availableTroops = append(availableTroops, troop)
			}
		}

		if len(availableTroops) == 0 {
			c.display.PrintWarning("No troops available for attack")
			return nil
		}
	}

	// Get enemy towers and filter alive ones
	var enemyTowers []game.Tower
	if c.gameState.Player1.ID == c.clientID {
		enemyTowers = c.gameState.Player2.Towers
	} else {
		enemyTowers = c.gameState.Player1.Towers
	}

	var aliveTowers []game.Tower
	for _, tower := range enemyTowers {
		if tower.HP > 0 {
			aliveTowers = append(aliveTowers, tower)
		}
	}

	if len(aliveTowers) == 0 {
		c.display.PrintWarning("No enemy towers left to attack")
		return nil
	}

	attackerIndex, targetType, targetIndex, err := c.input.GetAttackChoice(availableTroops, aliveTowers, c.gameState.GameMode)
	if err != nil {
		c.display.PrintWarning(err.Error())
		return nil
	}

	selectedTroop := availableTroops[attackerIndex]
	targetTower := aliveTowers[targetIndex]

	if selectedTroop.HP <= 0 {
		c.display.PrintError(fmt.Sprintf("%s is destroyed (HP: %d) and cannot attack", selectedTroop.Name, selectedTroop.HP))
		return nil
	}

	// ✅ UPDATE: Track attack count
	troopName := string(selectedTroop.Name)
	c.troopAttackCount[troopName]++

	msg := network.CreateAttackMessage(c.clientID, c.gameState.ID, selectedTroop.Name, targetType, string(targetTower.Name))
	return c.sendMessage(msg)
}

// ✅ UPDATED: handlePlayCard with better deployment tracking
func (c *Client) handlePlayCard() error {
	if len(c.myTroops) == 0 {
		c.display.PrintWarning("No troops available")
		return nil
	}

	if c.gameState.GameMode == game.ModeSimple {
		if len(c.deployedThisTurn) >= 1 {
			c.display.PrintError("Cannot deploy more than one troop per turn in simple mode")
			return nil
		}
	}

	var currentMana int = 999
	if c.gameState.GameMode == game.ModeEnhanced {
		if c.gameState.Player1.ID == c.clientID {
			currentMana = c.gameState.Player1.Mana
		} else {
			currentMana = c.gameState.Player2.Mana
		}
	}

	troopIndex, err := c.input.GetTroopChoice(c.myTroops, currentMana, c.gameState.GameMode)
	if err != nil {
		c.display.PrintWarning(err.Error())
		return nil
	}

	selectedTroop := c.myTroops[troopIndex]
	troopName := string(selectedTroop.Name)

	// ✅ TRACK: Mark troop as deployed
	if c.gameState.GameMode == game.ModeSimple {
		c.deployedThisTurn = append(c.deployedThisTurn, troopName)
		c.deployedTroops[troopName] = true

		// Initialize attack counter
		if c.troopAttackCount == nil {
			c.troopAttackCount = make(map[string]int)
		}
		c.troopAttackCount[troopName] = 0
	}
	msg := network.CreateSummonMessage(c.clientID, c.gameState.ID, selectedTroop.Name)
	return c.sendMessage(msg)
}

// handleEndTurn handles turn ending (Simple mode)
func (c *Client) handleEndTurn() error {
	if c.gameState.GameMode != game.ModeSimple {
		c.display.PrintWarning("End turn only available in Simple mode")
		return nil
	}

	c.display.PrintInfo("Checking turn status...")

	if c.gameState.CurrentTurn != c.clientID {
		opponentName := c.getPlayerName(c.gameState.CurrentTurn)
		c.display.PrintError(fmt.Sprintf("❌ Cannot end turn: It's %s's turn", opponentName))
		return nil
	}

	c.display.PrintInfo("✅ Confirmed: It's your turn. Ending turn...")
	c.logger.Debug("Sending end turn - Game ID: %s, Player ID: %s", c.gameState.ID, c.clientID)

	msg := network.NewMessage(network.MsgEndTurn, c.clientID, c.gameState.ID)

	err := c.sendMessage(msg)
	if err != nil {
		c.display.PrintError(fmt.Sprintf("Failed to send end turn message: %v", err))
		return err
	}

	c.display.PrintInfo("📤 End turn message sent. Waiting for server response...")
	c.logger.Debug("End turn message sent successfully")

	time.Sleep(1000 * time.Millisecond)
	return nil
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
	case "PLAYER_DISCONNECT":
		return c.handlePlayerDisconnectMessage(msg)
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

	gameStateJson, _ := json.Marshal(gameStartData["game_state"])
	if err := json.Unmarshal(gameStateJson, &c.gameState); err != nil {
		return fmt.Errorf("failed to parse game state: %w", err)
	}

	troopsJson, _ := json.Marshal(gameStartData["your_troops"])
	if err := json.Unmarshal(troopsJson, &c.myTroops); err != nil {
		return fmt.Errorf("failed to parse troops: %w", err)
	}

	towersJson, _ := json.Marshal(gameStartData["your_towers"])
	if err := json.Unmarshal(towersJson, &c.myTowers); err != nil {
		return fmt.Errorf("failed to parse towers: %w", err)
	}

	// ✅ RESET: Initialize tracking variables
	c.resetGameTracking()

	c.isInGame = true
	c.waitingForMatch = false

	c.display.PrintGameStart(3, c.gameState.GameMode)
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

	eventJson, _ := json.Marshal(eventData["event"])
	var event game.CombatAction
	if err := json.Unmarshal(eventJson, &event); err != nil {
		return fmt.Errorf("failed to parse event: %w", err)
	}

	stateJson, _ := json.Marshal(eventData["game_state"])
	if err := json.Unmarshal(stateJson, &c.gameState); err != nil {
		return fmt.Errorf("failed to parse game state: %w", err)
	}

	c.syncLocalTroopsFromGameState()
	c.displayGameEvent(event)
	return nil
}

func (c *Client) syncLocalTroopsFromGameState() {
	if c.gameState == nil {
		return
	}

	var serverTroops []game.Troop
	if c.gameState.Player1.ID == c.clientID {
		serverTroops = c.gameState.Player1.Troops
	} else {
		serverTroops = c.gameState.Player2.Troops
	}

	for i := range c.myTroops {
		for j := range serverTroops {
			if c.myTroops[i].Name == serverTroops[j].Name {
				oldHP := c.myTroops[i].HP
				c.myTroops[i].HP = serverTroops[j].HP
				c.myTroops[i].ATK = serverTroops[j].ATK
				c.myTroops[i].DEF = serverTroops[j].DEF

				if oldHP != c.myTroops[i].HP {
					c.logger.Debug("Troop %s HP changed: %d -> %d",
						c.myTroops[i].Name, oldHP, c.myTroops[i].HP)
				}
				break
			}
		}
	}
}

// ✅ UPDATED: displayGameEvent with counter-attack display and EXP notifications
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

		// ✅ CHECK: If this is a counter-attack
		isCounter := false
		if data, ok := event.Data["is_counter"]; ok {
			isCounter, _ = data.(bool)
		}

		if isCounter {
			c.display.PrintCounterAttack(attacker, target, event.Damage)
		} else {
			c.display.PrintAttack(attacker, target, event.Damage, event.IsCrit)
		}

	case game.ActionHeal:
		healer := string(event.TroopName)
		target := event.TargetName
		c.display.PrintHeal(healer, target, event.HealAmount)

	case "TOWER_DESTROYED":
		destroyer := event.Data["destroyer"].(string)
		owner := event.Data["owner"].(string)
		towerName := event.TargetName

		isMyDestruction := event.PlayerID == c.clientID
		c.display.PrintTowerDestroyed(destroyer, towerName, owner, isMyDestruction)

	case "TROOP_DESTROYED":
		destroyer := event.Data["destroyer"].(string)
		owner := event.Data["owner"].(string)
		troopName := event.TargetName

		isMyDestruction := event.PlayerID == c.clientID
		c.display.PrintTroopDestroyed(destroyer, troopName, owner, isMyDestruction)

	case "TROOP_REVIVED":
		troopName := string(event.TroopName)
		if event.PlayerID == c.clientID {
			c.display.PrintInfo(fmt.Sprintf("🔄 %s has been revived and is ready for battle!", troopName))
		}

	// ✅ NEW: Handle EXP events
	case "EXP_GAINED":
		if amount, ok := event.Data["amount"].(float64); ok {
			if reason, ok := event.Data["reason"].(string); ok {
				c.display.PrintEXPGain(int(amount), reason, event.PlayerID == c.clientID)
			}
		}

	case "LEVEL_UP":
		if level, ok := event.Data["new_level"].(float64); ok {
			c.display.PrintLevelUp(int(level), event.PlayerID == c.clientID)
		}
	}
}
