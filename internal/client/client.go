// Package client handles the TCP client and game interaction
package client

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"tcr-game/internal/game"
	"tcr-game/internal/network"
	"tcr-game/pkg/logger"
)

// Client represents the game client
type Client struct {
	conn               net.Conn
	display            *Display
	input              *InputHandler
	player             *game.PlayerData
	gameState          *game.GameState
	myTroops           []game.Troop
	myTowers           []game.Tower
	isConnected        bool
	isInGame           bool
	waitingForMatch    bool
	logger             *logger.Logger
	writer             *bufio.Writer
	reader             *bufio.Scanner
	serverAddr         string
	clientID           string
	deployedTroops     map[string]bool // Track which troops have been deployed
	troopAttackCount   map[string]int  // Track attacks per troop per turn
	deployedThisTurn   []string        // Only troops deployed THIS turn
	lastWaitingMessage string
	troopDestroyedTower map[string]bool // Track if a troop destroyed a tower in its last attack
	troopDestroyedKingTower map[string]bool // Track if a troop destroyed the King Tower in its last attack
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
		troopDestroyedTower: make(map[string]bool),
		troopDestroyedKingTower: make(map[string]bool),
	}
}

func (c *Client) handleGameEnd(msg *network.Message) error {
	c.logger.Debug("🎯 Received GAME_END message")

	c.isInGame = false
	c.waitingForMatch = false

	gameEndData, ok := msg.Data["game_end"].(map[string]interface{})
	if !ok {
		c.logger.Error("❌ Invalid game end format: %+v", msg.Data)
		return fmt.Errorf("invalid game end format")
	}

	c.logger.Debug("🎯 Game end data: %+v", gameEndData)

	winner, _ := gameEndData["winner"].(string)
	expGained, _ := gameEndData["exp_gained"].(string)
	opponentExpGained, _ := gameEndData["opponent_exp_gained"].(string)

	// Convert string to int for display
	playerExp := 0
	opponentExp := 0
	if exp, err := strconv.Atoi(expGained); err == nil {
		playerExp = exp
	}
	if exp, err := strconv.Atoi(opponentExpGained); err == nil {
		opponentExp = exp
	}

	isWinner := winner == c.clientID || winner == c.player.Username
	isDraw := winner == "draw"

	c.gameState = nil
	c.myTroops = nil
	c.myTowers = nil
	c.resetGameTracking()

	// Display results
	c.display.PrintSeparator()
	if isDraw {
		c.display.PrintInfo("🤝 GAME RESULT: DRAW!")
		c.display.PrintInfo("Both players fought equally!")
	} else if isWinner {
		c.display.PrintInfo("🎉 VICTORY! YOU WON! 🎉")
		c.display.PrintInfo("Congratulations on your triumph!")
	} else {
		c.display.PrintInfo("💀 DEFEAT! You lost this battle.")
		c.display.PrintInfo("Better luck next time!")
	}

	// Display EXP gains
	c.display.PrintExperience(playerExp, opponentExp)

	// Check for level up
	if c.player != nil {
		oldLevel := c.player.Level
		c.player.EXP += playerExp

		requiredEXP := 100 + (oldLevel-1)*15
		if c.player.EXP >= requiredEXP {
			c.player.Level++
			c.player.EXP -= requiredEXP
			c.display.PrintLevelUp(c.player.Level, true)
		}
	}

	c.display.PrintDataSaved()
	c.input.WaitForEnter("Press Enter to return to main menu...")

	c.logger.Debug("✅ Game end processed, returning to main menu")
	return nil
}

// handleTurnChange processes turn changes
func (c *Client) handleTurnChange(msg *network.Message) error {
	currentTurn, _ := msg.Data["current_turn"].(string)

	c.logger.Debug("Received turn change: %s -> %s", c.gameState.CurrentTurn, currentTurn)

	// Update game state from server
	if gameStateData, exists := msg.Data["game_state"]; exists {
		gameStateJson, _ := json.Marshal(gameStateData)
		if err := json.Unmarshal(gameStateJson, &c.gameState); err != nil {
			c.logger.Error("Failed to parse updated game state: %v", err)
		}
	}

	// Update current turn
	c.gameState.CurrentTurn = currentTurn

	if c.gameState.GameMode == game.ModeSimple {
		// Clear any existing waiting messages
		c.lastWaitingMessage = ""

		if currentTurn == c.clientID {
			// Reset turn-specific state
			c.deployedThisTurn = []string{}
			c.troopAttackCount = make(map[string]int)
			// Initialize attack counters for deployed troops
			for troopName := range c.deployedTroops {
				c.troopAttackCount[troopName] = 0
			}
			
			// Display turn start message
			c.display.PrintSeparator()
			c.display.PrintInfo("🔥 It's YOUR TURN! 🔥")
			c.display.PrintInfo("Available actions: play, attack, info, debug, end, surrender")
			c.display.PrintInfo("💡 Remember: 1 troop deployment per turn, each deployed troop can attack once")
			c.display.PrintSeparator()
		} else {
			opponentName := c.getPlayerName(currentTurn)
			c.display.PrintSeparator()
			c.display.PrintInfo(fmt.Sprintf("⏳ Waiting for %s's turn...", opponentName))
			c.display.PrintInfo("You can use 'info' or 'debug' to check game status")
			c.display.PrintSeparator()
		}
	}

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

// sendMessage with better error handling
func (c *Client) sendMessage(msg *network.Message) error {
	if !c.isConnected {
		return fmt.Errorf("not connected to server")
	}

	data, err := msg.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize message: %w", err)
	}

	if c.writer == nil {
		return fmt.Errorf("connection lost")
	}

	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		_, err = c.writer.Write(append(data, '\n'))
		if err == nil {
			err = c.writer.Flush()
			if err == nil {
				return nil // Success
			}
		}

		c.logger.Debug("Write attempt %d failed: %v", i+1, err)
		if i < maxRetries-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	c.isConnected = false
	return fmt.Errorf("failed to send message after %d retries: %w", maxRetries, err)
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

		c.display.PrintInfo("Mana regenerates +1 every second")
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

		// Deployment status for Simple mode
		c.display.PrintInfo(fmt.Sprintf("Troops Deployed This Turn: %d/1", len(c.deployedThisTurn)))
		if len(c.deployedThisTurn) > 0 {
			c.display.PrintInfo(fmt.Sprintf("Deployed: %v", c.deployedThisTurn))
		}
	} else if c.gameState.GameMode == game.ModeEnhanced {
		var myMana int
		if c.gameState.Player1.ID == c.clientID {
			myMana = c.gameState.Player1.Mana
		} else {
			myMana = c.gameState.Player2.Mana
		}

		c.display.PrintInfo(fmt.Sprintf("⚡ Your Mana: %d/%d", myMana, game.MaxMana))
		c.display.PrintInfo(fmt.Sprintf("⏰ Time Left: %d seconds", c.gameState.TimeLeft))
		c.display.PrintInfo("🔄 Mana regenerates +1 every second")
		c.display.PrintInfo("🚀 Continuous combat - no turns!")
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

	// Show troops with deployment status
	c.display.PrintInfo("\n=== Your Troops ===")
	for i, troop := range c.myTroops {
		status := ""

		if c.gameState.GameMode == game.ModeEnhanced {
			if troop.HP > 0 || troop.Name == game.Queen {
				status = fmt.Sprintf(" [ALIVE - Cost: %d MANA]", troop.MANA)
			} else {
				status = " [DESTROYED]"
			}
		} else if c.gameState.GameMode == game.ModeSimple {
			troopName := string(troop.Name)
			if troop.HP <= 0 && troop.Name != game.Queen {
				status = " [DESTROYED]"
			} else if c.deployedTroops[troopName] {
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

		c.display.PrintInfo(fmt.Sprintf("%d. %s%s (HP: %d, ATK: %d, DEF: %d, CRIT: %.0f%%)",
			i+1, troop.Name, status, troop.HP, troop.ATK, troop.DEF, troop.CRIT*100))
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

	for {
		if err := c.authenticate(); err != nil {
			if err.Error() == "user quit" {
				return nil
			}
			c.display.PrintError(fmt.Sprintf("Authentication failed: %v", err))
			continue
		}
		break
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
		c.display.PrintInfo("")
		c.display.PrintInfo("💡 Tip: If you see 'account already logged in', try again in a few seconds")

		choice := c.input.GetMenuChoice(1, 3)

		var err error
		switch choice {
		case 1:
			err = c.handleLogin()
		case 2:
			err = c.handleRegister()
		case 3:
			return fmt.Errorf("user quit")
		}

		if err != nil {
			// Check for specific error types
			errMsg := err.Error()
			if strings.Contains(errMsg, "account is already logged in") {
				c.display.PrintError("⚠️ This account is currently in use. Please try again in a few seconds.")
				time.Sleep(2 * time.Second) // Give a small delay before retry
			} else if strings.Contains(errMsg, "invalid credentials") {
				c.display.PrintError("❌ Invalid username or password. Please try again.")
			} else if strings.Contains(errMsg, "username already exists") {
				c.display.PrintError("❌ This username is already taken. Please choose another one.")
			} else {
				c.display.PrintError(fmt.Sprintf("Authentication failed: %v", err))
			}
			continue
		}

		return nil
	}
}

// handleLogin processes user login
func (c *Client) handleLogin() error {
	c.display.PrintSeparator()
	c.display.PrintInfo("📝 LOGIN")
	
	username := c.input.GetUsername()
	password := c.input.GetStringInput("Enter password: ", 4, 50)

	msg := network.CreateAuthMessage(network.MsgLogin, username, password)
	if err := c.sendMessage(msg); err != nil {
		return fmt.Errorf("failed to send login request: %w", err)
	}

	return c.waitForAuth()
}

// handleRegister processes user registration
func (c *Client) handleRegister() error {
	c.display.PrintSeparator()
	c.display.PrintInfo("📝 REGISTRATION")
	
	username := c.input.GetUsername()
	password := c.input.GetStringInput("Enter password (min 4 chars): ", 4, 50)
	confirmPassword := c.input.GetStringInput("Confirm password: ", 4, 50)

	if password != confirmPassword {
		return fmt.Errorf("passwords do not match")
	}

	msg := network.CreateAuthMessage(network.MsgRegister, username, password)
	if err := c.sendMessage(msg); err != nil {
		return fmt.Errorf("failed to send registration request: %w", err)
	}

	return c.waitForAuth()
}

// waitForAuth waits for authentication response
func (c *Client) waitForAuth() error {
	timeout := time.NewTimer(30 * time.Second) // Reduced timeout to 30 seconds
	defer timeout.Stop()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// Show loading indicator
	loadingChars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	i := 0

	for {
		select {
		case <-timeout.C:
			return fmt.Errorf("authentication timeout - server not responding")
		case <-ticker.C:
			// Update loading indicator
			c.display.PrintInfo(fmt.Sprintf("\r%s Authenticating...", loadingChars[i]))
			i = (i + 1) % len(loadingChars)

			if c.player != nil {
				c.display.PrintInfo("\r✅ Authentication successful!")
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

func (c *Client) resetGameTracking() {
	c.deployedTroops = make(map[string]bool)
	c.troopAttackCount = make(map[string]int)
	c.deployedThisTurn = []string{}
	c.lastWaitingMessage = ""
	c.troopDestroyedTower = make(map[string]bool)
	c.troopDestroyedKingTower = make(map[string]bool)
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

func (c *Client) handleGameplay() error {
	if c.gameState == nil {
		return fmt.Errorf("no active game state")
	}

	c.showGameStatus()

	for c.isInGame {
		if c.gameState == nil {
			return nil
		}

		if c.gameState.GameMode == game.ModeEnhanced {
			return c.handleEnhancedGameplay()
		}

		// Simple mode handling (existing logic)
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

func (c *Client) handleEnhancedGameplay() error {
	c.display.PrintSeparator()
	c.display.PrintInfo("🚀 ENHANCED MODE - AUTO COMBAT!")
	c.display.PrintInfo("⚡ Focus on deploying troops strategically!")
	c.display.PrintSeparator()

	c.showEnhancedModeStatus()

	for c.isInGame && c.gameState != nil {
		if !c.isInGame || c.gameState == nil {
			c.display.PrintInfo("🎮 Game ended. Returning to main menu...")
			break
		}

		c.display.PrintInfo("\n--- What do you want to do? ---")
		c.display.PrintInfo("1. Deploy Troop")
		c.display.PrintInfo("2. View Detailed Info")
		c.display.PrintInfo("3. Surrender")
		c.display.PrintInfo("4. Wait 10 seconds")

		choice := c.input.GetMenuChoice(1, 4)

		if !c.isInGame || c.gameState == nil {
			c.display.PrintInfo("🎮 Game ended during input. Returning to main menu...")
			break
		}

		switch choice {
		case 1:
			if err := c.handlePlayCard(); err != nil {
				c.display.PrintError(fmt.Sprintf("Failed: %v", err))
			} else {
				c.display.PrintInfo("🚀 Troop deployed! Auto-attacking enemy towers...")
				c.showEnhancedModeStatus()
				time.Sleep(1 * time.Second)
				c.display.PrintInfo("📋 Returning to action menu...")
			}
		case 2:
			c.showDetailedGameInfo()
		case 3:
			if err := c.handleSurrender(); err == nil {
				return nil
			}
		case 4:
			c.display.PrintInfo("⏳ Observing combat for 10 seconds...")
			c.display.PrintInfo("💡 (Combat is already happening automatically)")
			c.display.PrintSeparator()

			for i := 10; i > 0; i-- {
				if !c.isInGame || c.gameState == nil {
					c.display.PrintInfo("🎮 Game ended during observation.")
					return nil
				}

				if i%2 == 0 || i <= 3 {
					c.showCombatDetails()
				}

				c.display.PrintInfo(fmt.Sprintf("⏰ %d seconds remaining...", i))
				time.Sleep(1 * time.Second)

				if i == 5 {
					c.showEnhancedModeStatus()
				}
			}

			c.display.PrintInfo("✅ Combat observation complete!")
			c.display.PrintSeparator()
		}
	}

	c.display.PrintInfo("🏠 Returning to main menu...")
	return nil
}

func (c *Client) startCombatForAllTroops() {
	if c.gameState == nil || c.gameState.GameMode != game.ModeEnhanced {
		return
	}

	c.display.PrintInfo("🚀 Initiating combat with all deployed troops...")

	c.syncLocalTroopsFromGameState()

	// Find all alive troops and start attacking
	aliveTroops := 0
	for _, troop := range c.myTroops {
		if troop.HP > 0 {
			aliveTroops++
			// Find target (prioritize Guard Towers)
			target := c.findBestTarget()
			if target != "" {
				c.display.PrintInfo(fmt.Sprintf("⚔️  %s attacking %s", troop.Name, target))

				go func(troopName game.TroopType, targetName string) {
					time.Sleep(500 * time.Millisecond) // Wait for server sync

					// Send attack message to server
					msg := network.CreateAttackMessage(c.clientID, c.gameState.ID, troopName, "tower", targetName)
					if err := c.sendMessage(msg); err != nil {
						c.display.PrintError(fmt.Sprintf("Attack failed for %s: %v", troopName, err))
					}
				}(troop.Name, target)
			}
		}
	}

	if aliveTroops == 0 {
		c.display.PrintWarning("⚠️  No troops available for combat!")
		return
	}

	c.display.PrintInfo(fmt.Sprintf("⚡ %d troops entering combat!", aliveTroops))
}

func (c *Client) findBestTarget() string {
	if c.gameState == nil {
		return ""
	}

	// Get opponent towers
	var opponentTowers []game.Tower
	if c.gameState.Player1.ID == c.clientID {
		opponentTowers = c.gameState.Player2.Towers
	} else {
		opponentTowers = c.gameState.Player1.Towers
	}

	// Priority 1: Attack Guard Towers first
	for _, tower := range opponentTowers {
		if (tower.Name == game.GuardTower1 || tower.Name == game.GuardTower2) && tower.HP > 0 {
			return string(tower.Name)
		}
	}

	// Priority 2: Attack King Tower if Guard Towers are destroyed
	for _, tower := range opponentTowers {
		if tower.Name == game.KingTower && tower.HP > 0 {
			return string(tower.Name)
		}
	}

	return "" // No targets available
}

func (c *Client) showCombatDetails() {
	if c.gameState == nil {
		return
	}

	c.display.PrintInfo("🔥 === CURRENT COMBAT STATUS ===")

	// Show my troops status
	c.display.PrintInfo("📋 Your Troops:")
	aliveTroops := 0
	for _, troop := range c.myTroops {
		if troop.HP > 0 {
			aliveTroops++
			c.display.PrintInfo(fmt.Sprintf("  ⚔️  %s: %d/%d HP (ATK: %d) - FIGHTING",
				troop.Name, troop.HP, troop.MaxHP, troop.ATK))
		} else {
			c.display.PrintInfo(fmt.Sprintf("  💀 %s: DESTROYED", troop.Name))
		}
	}

	if aliveTroops == 0 {
		c.display.PrintWarning("  ⚠️  No troops alive - consider deploying more!")
	}

	// Show enemy towers status
	c.display.PrintInfo("🏰 Enemy Towers:")
	var opponentTowers []game.Tower
	if c.gameState.Player1.ID == c.clientID {
		opponentTowers = c.gameState.Player2.Towers
	} else {
		opponentTowers = c.gameState.Player1.Towers
	}

	for _, tower := range opponentTowers {
		if tower.HP > 0 {
			healthPercent := float64(tower.HP) / float64(tower.MaxHP) * 100
			status := "🟢 HEALTHY"
			if healthPercent < 70 {
				status = "🟡 DAMAGED"
			}
			if healthPercent < 30 {
				status = "🔴 CRITICAL"
			}

			c.display.PrintInfo(fmt.Sprintf("  🏯 %s: %d/%d HP (%.0f%%) %s",
				tower.Name, tower.HP, tower.MaxHP, healthPercent, status))
		} else {
			c.display.PrintInfo(fmt.Sprintf("  💥 %s: DESTROYED", tower.Name))
		}
	}

	// Show targeting priority
	c.display.PrintInfo("🎯 Current Target Priority:")
	guardTowersAlive := 0
	for _, tower := range opponentTowers {
		if (tower.Name == game.GuardTower1 || tower.Name == game.GuardTower2) && tower.HP > 0 {
			guardTowersAlive++
		}
	}

	if guardTowersAlive > 0 {
		c.display.PrintInfo("  → Attacking Guard Towers first")
	} else {
		c.display.PrintInfo("  → Attacking King Tower (Guard Towers destroyed)")
	}

	c.display.PrintSeparator()
}

func (c *Client) showEnhancedModeStatus() {
	if c.gameState == nil {
		return
	}

	var myMana int
	if c.gameState.Player1.ID == c.clientID {
		myMana = c.gameState.Player1.Mana
	} else {
		myMana = c.gameState.Player2.Mana
	}

	minutes := c.gameState.TimeLeft / 60
	seconds := c.gameState.TimeLeft % 60

	targetInfo := c.getCurrentTargetInfo()

	c.display.PrintInfo(fmt.Sprintf("⚡ Mana: %d/%d | ⏰ Time: %d:%02d | 🎯 Target: %s",
		myMana, game.MaxMana, minutes, seconds, targetInfo))

	c.display.PrintInfo(fmt.Sprintf("🏰 Towers Destroyed: You: %d vs Opponent: %d",
		c.gameState.TowersKilled.Player2, c.gameState.TowersKilled.Player1))

	// Show alive troops count
	aliveTroops := 0
	for _, troop := range c.myTroops {
		if troop.HP > 0 {
			aliveTroops++
		}
	}
	c.display.PrintInfo(fmt.Sprintf("⚔️  Active Troops: %d/3", aliveTroops))
}

func (c *Client) getCurrentTargetInfo() string {
	if c.gameState == nil {
		return "Unknown"
	}

	// Get opponent towers
	var opponentTowers []game.Tower
	if c.gameState.Player1.ID == c.clientID {
		opponentTowers = c.gameState.Player2.Towers
	} else {
		opponentTowers = c.gameState.Player1.Towers
	}

	// Check targeting priority
	guardTowersAlive := 0
	var guardTowerNames []string

	for _, tower := range opponentTowers {
		if (tower.Name == game.GuardTower1 || tower.Name == game.GuardTower2) && tower.HP > 0 {
			guardTowersAlive++
			guardTowerNames = append(guardTowerNames, string(tower.Name))
		}
	}

	if guardTowersAlive > 0 {
		if guardTowersAlive == 2 {
			return "Guard Towers (both alive)"
		} else {
			return fmt.Sprintf("%s (last guard)", guardTowerNames[0])
		}
	} else {
		// Check if King Tower is alive
		for _, tower := range opponentTowers {
			if tower.Name == game.KingTower && tower.HP > 0 {
				return "King Tower (guards destroyed)"
			}
		}
		return "All towers destroyed"
	}
}

func (c *Client) handleAttack() error {
	c.syncLocalTroopsFromGameState()

	var availableTroops []game.Troop

	if c.gameState.GameMode == game.ModeSimple {
		for _, troop := range c.myTroops {
			troopName := string(troop.Name)
			// Check if troop is deployed and alive
			if c.deployedTroops[troopName] && troop.HP > 0 {
				// Allow attack if:
				// 1. Troop hasn't attacked this turn OR
				// 2. Troop destroyed a tower in its last attack AND it wasn't the King Tower
				if c.troopAttackCount[troopName] < 1 || (c.troopDestroyedTower[troopName] && !c.troopDestroyedKingTower[troopName]) {
					availableTroops = append(availableTroops, troop)
				}
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

	troopName := string(selectedTroop.Name)
	c.troopAttackCount[troopName]++
	
	// Reset the tower destruction flag before sending the attack
	// The server will set it back to true if the tower is destroyed
	c.troopDestroyedTower[troopName] = false

	msg := network.CreateAttackMessage(c.clientID, c.gameState.ID, selectedTroop.Name, targetType, string(targetTower.Name))
	return c.sendMessage(msg)
}

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

	// If troop is destroyed, respawn it with full HP
	if selectedTroop.HP <= 0 && selectedTroop.Name != game.Queen {
		// Calculate full HP (same formula as server)
		level := selectedTroop.Level
		if level == 0 {
			level = 1
		}

		// Get base HP for each troop type
		var baseHP int
		switch selectedTroop.Name {
		case game.Knight:
			baseHP = 350
		case game.Pawn:
			baseHP = 150
		case game.Bishop:
			baseHP = 250
		case game.Rook:
			baseHP = 300
		case game.Prince:
			baseHP = 500
		default:
			baseHP = 100 // Default fallback
		}

		// Apply level scaling: 10% increase per level
		fullHP := int(float64(baseHP) * (1.0 + float64(level-1)*0.10))

		// Update local troop HP
		c.myTroops[troopIndex].HP = fullHP
		c.myTroops[troopIndex].MaxHP = fullHP

		c.display.PrintInfo(fmt.Sprintf("🔄 %s has been respawned with %d HP!", selectedTroop.Name, fullHP))
	}

	if c.gameState.GameMode == game.ModeSimple {
		c.deployedThisTurn = append(c.deployedThisTurn, troopName)
		c.deployedTroops[troopName] = true

		if c.troopAttackCount == nil {
			c.troopAttackCount = make(map[string]int)
		}
		c.troopAttackCount[troopName] = 0
	} else if c.gameState.GameMode == game.ModeEnhanced {
		if c.gameState.Player1.ID == c.clientID {
			c.gameState.Player1.Mana -= selectedTroop.MANA
		} else {
			c.gameState.Player2.Mana -= selectedTroop.MANA
		}

		var remainingMana int
		if c.gameState.Player1.ID == c.clientID {
			remainingMana = c.gameState.Player1.Mana
		} else {
			remainingMana = c.gameState.Player2.Mana
		}

		c.display.PrintInfo(fmt.Sprintf("💰 Mana spent: %d (Remaining: %d)", selectedTroop.MANA, remainingMana))
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
		// Just return, don't show error
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

	// Do NOT update local state here. Wait for server's turn change message.
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

	c.logger.Debug("📨 Received message type: %s", msg.Type)
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
		c.logger.Debug("🎯 Processing GAME_END message")
		return c.handleGameEnd(msg)
	case network.MsgTurnChange:
		return c.handleTurnChange(msg)
	case network.MsgError:
		return c.handleError(msg)
	case "MANA_UPDATE":
		return c.handleManaUpdateMessage(msg)
	case "PLAYER_DISCONNECT":
		return c.handlePlayerDisconnectMessage(msg)
	default:
		c.logger.Debug("🤷 Unhandled message type: %s with data: %+v", msg.Type, msg.Data) // ✅ ADD: Show unhandled
	}

	return nil
}

func (c *Client) handleManaUpdateMessage(msg *network.Message) error {
	manaData, ok := msg.Data["mana_update"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid mana update format")
	}

	if timeLeft, ok := manaData["time_left"].(float64); ok {
		c.gameState.TimeLeft = int(timeLeft)
	}
	if player1Mana, ok := manaData["player1_mana"].(float64); ok {
		c.gameState.Player1.Mana = int(player1Mana)
	}
	if player2Mana, ok := manaData["player2_mana"].(float64); ok {
		c.gameState.Player2.Mana = int(player2Mana)
	}

	return nil
}

func (c *Client) handleManaUpdate(msg *network.Message) error {
	manaData, ok := msg.Data["mana_update"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid mana update format")
	}

	// Update mana values
	if player1Mana, ok := manaData["player1_mana"].(float64); ok {
		c.gameState.Player1.Mana = int(player1Mana)
	}
	if player2Mana, ok := manaData["player2_mana"].(float64); ok {
		c.gameState.Player2.Mana = int(player2Mana)
	}
	if timeLeft, ok := manaData["time_left"].(float64); ok {
		c.gameState.TimeLeft = int(timeLeft)
	}

	// Display mana update in Enhanced mode
	if c.gameState.GameMode == game.ModeEnhanced {
		var myMana int
		if c.gameState.Player1.ID == c.clientID {
			myMana = c.gameState.Player1.Mana
		} else {
			myMana = c.gameState.Player2.Mana
		}

		// Only show mana update every 10 seconds to avoid spam
		if c.gameState.TimeLeft%10 == 0 {
			c.display.PrintInfo(fmt.Sprintf("⚡ Mana: %d/%d | Time: %ds",
				myMana, game.MaxMana, c.gameState.TimeLeft))
		}
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
	return fmt.Errorf("authentication failed: %s", message)
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

	if c.gameState.GameMode == game.ModeEnhanced {
		c.startRealTimeTimer()
	}

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

	// Process the event
	c.displayGameEvent(event)

	// After displaying the event, check if a tower was destroyed
	if event.Type == game.ActionAttack {
		if targetHP, ok := event.Data["target_hp"].(float64); ok {
			if int(targetHP) <= 0 {
				// Tower was destroyed in this attack
				if event.PlayerID == c.clientID {
					troopName := string(event.TroopName)
					c.troopDestroyedTower[troopName] = true
					
					// Check if it was the King Tower
					if event.TargetName == string(game.KingTower) {
						c.troopDestroyedKingTower[troopName] = true
						c.display.PrintInfo(fmt.Sprintf("👑 %s destroyed the King Tower! This was the final blow!", troopName))
					} else {
						c.display.PrintInfo(fmt.Sprintf("🎯 %s destroyed a tower and can attack again!", troopName))
					}
				}
				// Always print a clear destruction message
				c.display.PrintTowerDestroyed(string(event.TroopName), event.TargetName, "opponent", event.PlayerID == c.clientID)
			}
		}
	}

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
				c.myTroops[i].MaxHP = serverTroops[j].MaxHP
				c.myTroops[i].ATK = serverTroops[j].ATK
				c.myTroops[i].DEF = serverTroops[j].DEF

				if oldHP != c.myTroops[i].HP {
					c.logger.Debug("🔄 Troop %s HP synced: %d -> %d",
						c.myTroops[i].Name, oldHP, c.myTroops[i].HP)
				}
				break
			}
		}
	}
}

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

		isCounter := false
		if data, ok := event.Data["is_counter"]; ok {
			isCounter, _ = data.(bool)
		}

		currentHP := 0
		if targetHP, ok := event.Data["target_hp"].(float64); ok {
			currentHP = int(targetHP)
		}

		if isCounter {
			c.display.PrintCounterAttack(attacker, target, event.Damage)
		} else {
			c.display.PrintAttack(attacker, target, event.Damage, event.IsCrit)
		}

		c.display.PrintInfo(fmt.Sprintf("   └─ %s now has %d HP remaining", target, currentHP))

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
		
		// If it was our attack that destroyed the tower, mark the troop as able to attack again
		if isMyDestruction {
			troopName := destroyer
			c.troopDestroyedTower[troopName] = true
			c.display.PrintInfo(fmt.Sprintf("🎯 %s destroyed a tower and can attack again!", troopName))
			
			expGained := 100
			if strings.Contains(towerName, "King") {
				expGained = 200
			}
			c.display.PrintEXPGain(expGained, fmt.Sprintf("destroying %s", towerName), true)
		}

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

func (c *Client) startRealTimeTimer() {
	if c.gameState.GameMode != game.ModeEnhanced {
		return
	}

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for c.isInGame && c.gameState != nil && c.gameState.GameMode == game.ModeEnhanced {
			select {
			case <-ticker.C:
				if c.gameState.TimeLeft > 0 {
					c.gameState.TimeLeft--

					// Update mana locally (will be synced by server)
					if c.gameState.Player1.ID == c.clientID {
						if c.gameState.Player1.Mana < game.MaxMana {
							c.gameState.Player1.Mana++
						}
					} else {
						if c.gameState.Player2.Mana < game.MaxMana {
							c.gameState.Player2.Mana++
						}
					}
				}

				if c.gameState.TimeLeft <= 0 {
					c.display.PrintInfo("⏰ TIME'S UP! Waiting for server to determine winner...")
					return
				}
			}
		}
	}()
}

// getOpponentID returns the ID of the opponent player
func (c *Client) getOpponentID() string {
	if c.gameState == nil {
		return ""
	}

	if c.gameState.Player1.ID == c.clientID {
		return c.gameState.Player2.ID
	}
	return c.gameState.Player1.ID
}
