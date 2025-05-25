// Package client handles client-side display and user interface
package client

import (
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	"tcr-game/internal/game"
)

// Display handles all visual output for the client
type Display struct {
	// Color functions for different types of output
	serverColor  *color.Color
	connectColor *color.Color
	gameColor    *color.Color
	attackColor  *color.Color
	critColor    *color.Color
	healColor    *color.Color
	winColor     *color.Color
	loseColor    *color.Color
	warningColor *color.Color
	infoColor    *color.Color
	playerColor  *color.Color
	enemyColor   *color.Color
}

// NewDisplay creates a new display instance with configured colors
func NewDisplay() *Display {
	return &Display{
		serverColor:  color.New(color.FgCyan, color.Bold),
		connectColor: color.New(color.FgGreen, color.Bold),
		gameColor:    color.New(color.FgYellow, color.Bold),
		attackColor:  color.New(color.FgRed),
		critColor:    color.New(color.FgRed, color.Bold, color.BgYellow),
		healColor:    color.New(color.FgBlue),
		winColor:     color.New(color.FgGreen, color.Bold, color.BgBlack),
		loseColor:    color.New(color.FgRed, color.Bold, color.BgBlack),
		warningColor: color.New(color.FgYellow),
		infoColor:    color.New(color.FgWhite),
		playerColor:  color.New(color.FgCyan),
		enemyColor:   color.New(color.FgMagenta),
	}
}

// PrintBanner displays the game banner
func (d *Display) PrintBanner() {
	banner := `
╔═══════════════════════════════════════╗
║        CLASH ROYALE TCR CLIENT        ║
║              Text Combat              ║
╚═══════════════════════════════════════╝
`
	d.gameColor.Println(banner)
}

// PrintServerStatus displays server connection status
func (d *Display) PrintServerStatus(message string) {
	timestamp := time.Now().Format("15:04:05")
	d.serverColor.Printf("[%s] [SERVER] %s\n", timestamp, message)
}

// PrintConnection displays connection events
func (d *Display) PrintConnection(playerName, username string) {
	timestamp := time.Now().Format("15:04:05")
	d.connectColor.Printf("[%s] [CONNECTED] %s (username: %s)\n",
		timestamp, playerName, username)
}

// PrintMatchmaking displays matchmaking information
func (d *Display) PrintMatchmaking(player1, player2 string) {
	timestamp := time.Now().Format("15:04:05")
	d.gameColor.Printf("[%s] [MATCHMAKING] %s vs %s\n",
		timestamp, player1, player2)
}

// PrintGameMode displays the current game mode
func (d *Display) PrintGameMode(mode string) {
	timestamp := time.Now().Format("15:04:05")
	d.gameColor.Printf("[%s] [GAME MODE] %s\n", timestamp, mode)
}

// PrintGameStart displays game start countdown
func (d *Display) PrintGameStart(countdown int) {
	timestamp := time.Now().Format("15:04:05")
	d.gameColor.Printf("[%s] [GAME START] %d minutes countdown initiated.\n",
		timestamp, countdown)
}

// PrintCardPlayed displays when a troop is summoned
func (d *Display) PrintTroopSummoned(player string, troopName string, isPlayer bool) {
	timestamp := time.Now().Format("15:04:05")
	var colorFunc *color.Color
	if isPlayer {
		colorFunc = d.playerColor
	} else {
		colorFunc = d.enemyColor
	}

	colorFunc.Printf("[%s] [TURN LOG] %s summoned %s\n",
		timestamp, player, troopName)
}

// PrintAttack displays attack events
func (d *Display) PrintAttack(attacker, target string, damage int, isCrit bool) {
	timestamp := time.Now().Format("15:04:05")

	if isCrit {
		d.critColor.Printf("[%s] [CRIT!] %s landed a CRIT on %s - Damage: %d\n",
			timestamp, attacker, target, damage)
	} else {
		d.attackColor.Printf("[%s] [DMG LOG] %s attacked %s - Damage dealt: %d\n",
			timestamp, attacker, target, damage)
	}
}

// PrintHeal displays healing events
func (d *Display) PrintHeal(healer, target string, amount int) {
	timestamp := time.Now().Format("15:04:05")
	d.healColor.Printf("[%s] [HEAL LOG] %s healed %s for %d HP\n",
		timestamp, healer, target, amount)
}

// PrintGameEnd displays game end results
func (d *Display) PrintGameEnd(winner string, isPlayerWinner bool, towersDestroyed map[string]int) {
	d.infoColor.Println("\n[TIME'S UP]")

	// Display towers destroyed
	var parts []string
	for player, count := range towersDestroyed {
		parts = append(parts, fmt.Sprintf("%s destroyed %d tower(s)", player, count))
	}
	d.infoColor.Printf("[WINNER] %s\n", strings.Join(parts, " | "))

	// Display winner with appropriate color
	if isPlayerWinner {
		d.winColor.Printf("\n🎉 VICTORY! You defeated your opponent! 🎉\n")
	} else {
		d.loseColor.Printf("\n💀 DEFEAT! Better luck next time! 💀\n")
	}
}

// PrintExperience displays experience gained
func (d *Display) PrintExperience(player1Exp, player2Exp int) {
	d.infoColor.Printf("[EXP] Player1 +%d EXP | Player2 +%d EXP\n",
		player1Exp, player2Exp)
}

// PrintDataSaved displays data persistence confirmation
func (d *Display) PrintDataSaved() {
	d.infoColor.Println("[DATA SAVED] JSON updated for both players")
}

// PrintPlayerStatus displays current player status
func (d *Display) PrintPlayerStatus(player game.Player, isCurrentPlayer bool) {
	var colorFunc *color.Color
	if isCurrentPlayer {
		colorFunc = d.playerColor
	} else {
		colorFunc = d.enemyColor
	}

	// colorFunc.Printf("Player: %s | Level: %d | Trophies: %d | Mana: %d/%d\n",
	// 	player.Username, player.Level, player.Trophies, player.Mana, player.MaxMana)
	colorFunc.Printf("Player: %s | Level: %d | Mana: %d/%d\n",
		player.Username, player.Level, player.Mana, player.MaxMana)
}

// PrintTowerStatus displays tower health
func (d *Display) PrintTowerStatus(towers []game.Tower, playerName string) {
	d.infoColor.Printf("\n=== %s's Towers ===\n", playerName)
	for _, tower := range towers {
		healthPercent := float64(tower.HP) / float64(tower.MaxHP) * 100
		var healthColor *color.Color

		switch {
		case healthPercent > 70:
			healthColor = d.healColor // Blue for healthy
		case healthPercent > 30:
			healthColor = d.warningColor // Yellow for damaged
		default:
			healthColor = d.attackColor // Red for critical
		}

		healthColor.Printf("%s: %d/%d HP (%.1f%%)\n",
			tower.Name, tower.HP, tower.MaxHP, healthPercent)
	}
}

// PrintTroops displays player's current troops
func (d *Display) PrintTroops(troops []game.Troop) {
	d.infoColor.Println("\n=== Your Troops ===")
	for i, troop := range troops {
		d.playerColor.Printf("%d. %s (Cost: %d, HP: %d, ATK: %d, DEF: %d) - %s\n",
			i+1, troop.Name, troop.MANA, troop.HP, troop.ATK, troop.DEF, troop.Special)
	}
}

// PrintAttackOptions displays attack interface
func (d *Display) PrintAttackOptions(troops []game.Troop, towers []game.Tower) {
	d.infoColor.Println("\n=== ATTACK PHASE ===")

	d.infoColor.Println("Your Troops:")
	for i, troop := range troops {
		if troop.Name != game.Queen {
			d.playerColor.Printf("%d. %s (ATK: %d)\n", i+1, troop.Name, troop.ATK)
		}
	}

	d.infoColor.Println("\nEnemy Towers:")
	for i, tower := range towers {
		if tower.HP > 0 {
			d.enemyColor.Printf("%d. %s (HP: %d/%d, DEF: %d)\n",
				i+1, tower.Name, tower.HP, tower.MaxHP, tower.DEF)
		}
	}
}

// PrintError displays error messages
func (d *Display) PrintError(message string) {
	d.loseColor.Printf("[ERROR] %s\n", message)
}

// PrintWarning displays warning messages
func (d *Display) PrintWarning(message string) {
	d.warningColor.Printf("[WARNING] %s\n", message)
}

// PrintInfo displays informational messages
func (d *Display) PrintInfo(message string) {
	d.infoColor.Printf("[INFO] %s\n", message)
}

// Clear clears the screen (basic implementation)
func (d *Display) Clear() {
	fmt.Print("\033[2J\033[H")
}

// PrintSeparator prints a visual separator
func (d *Display) PrintSeparator() {
	d.infoColor.Println("═══════════════════════════════════════════════════════════════")
}
