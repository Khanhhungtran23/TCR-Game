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
func (ih *InputHandler) GetTroopChoice(troops []game.Troop, availableMana int) (int, error) {
	if len(troops) == 0 {
		return -1, fmt.Errorf("no troops available")
	}

	// Show available troops
	ih.display.PrintInfo("Available troops:")
	playableTroops := make([]int, 0)

	for i, troop := range troops {
		if troop.MANA <= availableMana {
			ih.display.PrintInfo(fmt.Sprintf("%d. %s (Cost: %d, HP: %d, ATK: %d) ✓",
				i+1, troop.Name, troop.MANA, troop.HP, troop.ATK))
			playableTroops = append(playableTroops, i)
		} else {
			ih.display.PrintWarning(fmt.Sprintf("%d. %s (Cost: %d, HP: %d, ATK: %d) ❌ Not enough mana",
				i+1, troop.Name, troop.MANA, troop.HP, troop.ATK))
		}
	}

	if len(playableTroops) == 0 {
		return -1, fmt.Errorf("no playable troops with current mana (%d)", availableMana)
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
			ih.display.PrintWarning("Not enough mana to play that troop. Choose another troop.")
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
func (ih *InputHandler) GetGameAction() string {
	validActions := []string{"play", "end", "surrender", "info"}

	for {
		ih.display.PrintInfo("Available actions:")
		ih.display.PrintInfo("1. 'play' - Play a card")
		ih.display.PrintInfo("2. 'end' - End turn")
		ih.display.PrintInfo("3. 'surrender' - Surrender the match")
		ih.display.PrintInfo("4. 'info' - Show game information")

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
			ih.display.PrintWarning("Invalid action. Please choose from: play, end, surrender, info")
			continue
		}

		return action
	}
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
