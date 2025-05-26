// Package client handles user input validation and processing
package client

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"tcr-game/internal/game"
)

// InputHandler manages user input for the game
type InputHandler struct {
	scanner *bufio.Scanner
	display *Display
}

// NewInputHandler creates a new input handler
func NewInputHandler(display *Display) *InputHandler {
	return &InputHandler{
		scanner: bufio.NewScanner(os.Stdin),
		display: display,
	}
}

// GetMenuChoice gets and validates menu choices
func (ih *InputHandler) GetMenuChoice(min, max int) int {
	for {
		fmt.Printf("Enter your choice (%d-%d): ", min, max)

		if !ih.scanner.Scan() {
			ih.display.PrintError("Failed to read input")
			continue
		}

		input := strings.TrimSpace(ih.scanner.Text())
		choice, err := strconv.Atoi(input)

		if err != nil {
			ih.display.PrintWarning("Please enter a valid number")
			continue
		}

		if choice < min || choice > max {
			ih.display.PrintWarning(fmt.Sprintf("Please enter a number between %d and %d", min, max))
			continue
		}

		return choice
	}
}

// GetUsername prompts for and validates username input
func (ih *InputHandler) GetUsername() string {
	for {
		fmt.Print("Enter your username (3-20 characters): ")

		if !ih.scanner.Scan() {
			ih.display.PrintError("Failed to read input")
			continue
		}

		username := strings.TrimSpace(ih.scanner.Text())

		if len(username) < 3 {
			ih.display.PrintWarning("Username must be at least 3 characters long")
			continue
		}

		if len(username) > 20 {
			ih.display.PrintWarning("Username must be no more than 20 characters long")
			continue
		}

		// Check for valid characters (alphanumeric and underscore)
		if !isValidUsername(username) {
			ih.display.PrintWarning("Username can only contain letters, numbers, and underscores")
			continue
		}

		return username
	}
}

// GetTroopChoice gets and validates troop selection from available troops
func (ih *InputHandler) GetTroopChoice(troops []game.Troop, availableMana int, gameMode string) (int, error) {
	if len(troops) == 0 {
		return -1, fmt.Errorf("no troops available")
	}

	// Show available troops
	ih.display.PrintInfo("Available troops:")
	playableTroops := make([]int, 0)

	for i, troop := range troops {
		// In Simple mode, all troops are playable (no mana cost)
		// In Enhanced mode, check mana cost
		isPlayable := gameMode == "simple" || troop.MANA <= availableMana

		if isPlayable {
			if gameMode == "enhanced" {
				ih.display.PrintInfo(fmt.Sprintf("%d. %s (Cost: %d, HP: %d, ATK: %d) ✓",
					i+1, troop.Name, troop.MANA, troop.HP, troop.ATK))
			} else {
				ih.display.PrintInfo(fmt.Sprintf("%d. %s (HP: %d, ATK: %d) ✓",
					i+1, troop.Name, troop.HP, troop.ATK))
			}
			playableTroops = append(playableTroops, i)
		} else {
			ih.display.PrintWarning(fmt.Sprintf("%d. %s (Cost: %d, HP: %d, ATK: %d) ❌ Not enough mana",
				i+1, troop.Name, troop.MANA, troop.HP, troop.ATK))
		}
	}

	if len(playableTroops) == 0 {
		if gameMode == "enhanced" {
			return -1, fmt.Errorf("no playable troops with current mana (%d)", availableMana)
		} else {
			return -1, fmt.Errorf("no troops available")
		}
	}

	// Get choice
	for {
		choice := ih.GetMenuChoice(1, len(troops)) - 1 // Convert to 0-based index

		// Check if troop is playable
		isPlayable := false
		for _, playableIndex := range playableTroops {
			if choice == playableIndex {
				isPlayable = true
				break
			}
		}

		if !isPlayable {
			if gameMode == "enhanced" {
				ih.display.PrintWarning("Not enough mana to play that troop. Choose another troop.")
			} else {
				ih.display.PrintWarning("Invalid troop selection. Choose another troop.")
			}
			continue
		}

		return choice, nil
	}
}

// GetConfirmation gets yes/no confirmation from user
func (ih *InputHandler) GetConfirmation(prompt string) bool {
	for {
		fmt.Printf("%s (y/n): ", prompt)

		if !ih.scanner.Scan() {
			ih.display.PrintError("Failed to read input")
			continue
		}

		input := strings.ToLower(strings.TrimSpace(ih.scanner.Text()))

		switch input {
		case "y", "yes":
			return true
		case "n", "no":
			return false
		default:
			ih.display.PrintWarning("Please enter 'y' for yes or 'n' for no")
		}
	}
}

// WaitForEnter waits for user to press Enter
func (ih *InputHandler) WaitForEnter(message string) {
	if message == "" {
		message = "Press Enter to continue..."
	}

	fmt.Print(message)
	ih.scanner.Scan()
}

// GetStringInput gets general string input with validation
func (ih *InputHandler) GetStringInput(prompt string, minLength, maxLength int) string {
	for {
		fmt.Print(prompt)

		if !ih.scanner.Scan() {
			ih.display.PrintError("Failed to read input")
			continue
		}

		input := strings.TrimSpace(ih.scanner.Text())

		if len(input) < minLength {
			ih.display.PrintWarning(fmt.Sprintf("Input must be at least %d characters long", minLength))
			continue
		}

		if maxLength > 0 && len(input) > maxLength {
			ih.display.PrintWarning(fmt.Sprintf("Input must be no more than %d characters long", maxLength))
			continue
		}

		return input
	}
}

// GetIntegerInput gets and validates integer input within a range
func (ih *InputHandler) GetIntegerInput(prompt string, min, max int) int {
	for {
		fmt.Printf("%s (%d-%d): ", prompt, min, max)

		if !ih.scanner.Scan() {
			ih.display.PrintError("Failed to read input")
			continue
		}

		input := strings.TrimSpace(ih.scanner.Text())
		value, err := strconv.Atoi(input)

		if err != nil {
			ih.display.PrintWarning("Please enter a valid number")
			continue
		}

		if value < min || value > max {
			ih.display.PrintWarning(fmt.Sprintf("Please enter a number between %d and %d", min, max))
			continue
		}

		return value
	}
}

// GetGameAction gets and validates game actions during gameplay
func (ih *InputHandler) GetGameAction(gameMode string) string {
	validActions := []string{"play", "attack", "surrender", "info"}

	// Add "end" action only for Simple mode
	if gameMode == "simple" {
		validActions = append(validActions, "end")
	}

	for {
		ih.display.PrintInfo("Available actions:")
		ih.display.PrintInfo("1. 'play' - Deploy a troop")
		ih.display.PrintInfo("2. 'attack' - Attack with deployed troops")
		if gameMode == "simple" {
			ih.display.PrintInfo("3. 'end' - End turn")
		}
		ih.display.PrintInfo("4. 'surrender' - Surrender the match")
		ih.display.PrintInfo("5. 'info' - Show game information")

		fmt.Print("Enter action: ")

		if !ih.scanner.Scan() {
			ih.display.PrintError("Failed to read input")
			continue
		}

		action := strings.ToLower(strings.TrimSpace(ih.scanner.Text()))

		// Validate action
		isValid := false
		for _, validAction := range validActions {
			if action == validAction {
				isValid = true
				break
			}
		}

		if !isValid {
			if gameMode == "simple" {
				ih.display.PrintWarning("Invalid action. Please choose from: play, attack, end, surrender, info")
			} else {
				ih.display.PrintWarning("Invalid action. Please choose from: play, attack, surrender, info")
			}
			continue
		}

		return action
	}
}

// GetAttackChoice lets player choose attacker and target
func (ih *InputHandler) GetAttackChoice(myTroops []game.Troop, enemyTowers []game.Tower, gameMode string) (attackerIndex int, targetType string, targetIndex int, err error) {
	// Show available attackers
	ih.display.PrintInfo("Choose your attacker:")
	availableAttackers := make([]int, 0)

	for i, troop := range myTroops {
		// Skip Queen as it can't attack
		if troop.Name != game.Queen {
			ih.display.PrintInfo(fmt.Sprintf("%d. %s (ATK: %d)", i+1, troop.Name, troop.ATK))
			availableAttackers = append(availableAttackers, i)
		}
	}

	if len(availableAttackers) == 0 {
		return -1, "", -1, fmt.Errorf("no troops available for attack")
	}

	// Get attacker choice
	for {
		attackerChoice := ih.GetMenuChoice(1, len(myTroops)) - 1

		// Check if valid attacker
		isValidAttacker := false
		for _, validIndex := range availableAttackers {
			if attackerChoice == validIndex {
				isValidAttacker = true
				break
			}
		}

		if !isValidAttacker {
			ih.display.PrintWarning("Invalid attacker. Choose a troop that can attack.")
			continue
		}

		attackerIndex = attackerChoice
		break
	}

	// Show available targets
	ih.display.PrintInfo("Choose your target:")
	availableTargets := make([]int, 0)

	for i, tower := range enemyTowers {
		if tower.HP > 0 {
			// Simple mode: enforce targeting rules
			if gameMode == "simple" && tower.Name == game.KingTower {
				// Check if Guard Towers are still alive
				guardTowersAlive := false
				for _, t := range enemyTowers {
					if t.Name == game.GuardTower && t.HP > 0 {
						guardTowersAlive = true
						break
					}
				}

				if guardTowersAlive {
					ih.display.PrintWarning(fmt.Sprintf("%d. %s (HP: %d/%d) ❌ Must destroy Guard Towers first",
						i+1, tower.Name, tower.HP, tower.MaxHP))
					continue
				}
			}

			ih.display.PrintInfo(fmt.Sprintf("%d. %s (HP: %d/%d)",
				i+1, tower.Name, tower.HP, tower.MaxHP))
			availableTargets = append(availableTargets, i)
		}
	}

	if len(availableTargets) == 0 {
		return -1, "", -1, fmt.Errorf("no valid targets available")
	}

	// Get target choice
	for {
		targetChoice := ih.GetMenuChoice(1, len(enemyTowers)) - 1

		// Check if valid target
		isValidTarget := false
		for _, validIndex := range availableTargets {
			if targetChoice == validIndex {
				isValidTarget = true
				break
			}
		}

		if !isValidTarget {
			ih.display.PrintWarning("Invalid target. Choose an available target.")
			continue
		}

		targetIndex = targetChoice
		targetType = "tower"
		break
	}

	return attackerIndex, targetType, targetIndex, nil
}

// ShowTypingEffect simulates typing effect for dramatic messages
func (ih *InputHandler) ShowTypingEffect(text string, delay time.Duration) {
	for _, char := range text {
		fmt.Print(string(char))
		time.Sleep(delay)
	}
	fmt.Println()
}

// Helper functions

// isValidUsername checks if username contains only valid characters
func isValidUsername(username string) bool {
	for _, char := range username {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '_') {
			return false
		}
	}
	return true
}

// ClearInputBuffer clears any remaining input in the buffer
func (ih *InputHandler) ClearInputBuffer() {
	// Create a new scanner to clear buffer
	ih.scanner = bufio.NewScanner(os.Stdin)
}

func (ih *InputHandler) GetGameActionWithDebug(gameMode string) string {
	for {
		ih.display.PrintInfo("\n=== GAME ACTIONS ===")
		ih.display.PrintInfo("play - Deploy a troop")
		ih.display.PrintInfo("attack - Attack with troop")
		ih.display.PrintInfo("info - Show detailed game info")
		ih.display.PrintInfo("debug - Show debug information")
		if gameMode == game.ModeSimple {
			ih.display.PrintInfo("end - End your turn")
		}
		ih.display.PrintInfo("surrender - Give up")

		action := ih.GetStringInput("Enter your command: ", 1, 20)
		action = strings.ToLower(strings.TrimSpace(action))

		validActions := []string{"play", "attack", "info", "debug", "surrender"}
		if gameMode == game.ModeSimple {
			validActions = append(validActions, "end")
		}

		for _, valid := range validActions {
			if action == valid {
				return action
			}
		}

		ih.display.PrintWarning("Invalid action. Please try again.")
	}
}
