// Package game implements the core game engine and combat logic
package game

import (
	"fmt"
	"math"
	"math/rand"
	"time"
)

// GameEngine handles all game logic and state management
type GameEngine struct {
	gameState  *GameState
	gameSpecs  *GameSpecs
	eventQueue []CombatAction
	gameTimer  *time.Timer
	isRunning  bool
	eventChan  chan CombatAction
}

// NewGameEngine creates a new game engine instance
func NewGameEngine(player1, player2 *Player, gameMode string, specs *GameSpecs) *GameEngine {
	// Initialize players with random troops and leveled stats
	initializePlayerForGame(player1, specs)
	initializePlayerForGame(player2, specs)

	player1.TroopsDeployedThisTurn = 0
	player2.TroopsDeployedThisTurn = 0

	gameState := &GameState{
		ID:          generateGameID(),
		GameMode:    gameMode,
		Status:      StatusWaiting,
		Player1:     *player1,
		Player2:     *player2,
		CurrentTurn: player1.ID,
		TimeLeft:    GameDurationSeconds,
		StartTime:   time.Now(),
		TowersKilled: struct {
			Player1 int `json:"player1"`
			Player2 int `json:"player2"`
		}{0, 0},
	}

	return &GameEngine{
		gameState:  gameState,
		gameSpecs:  specs,
		eventQueue: make([]CombatAction, 0),
		isRunning:  false,
		eventChan:  make(chan CombatAction, 100),
	}
}

// StartGame begins the game based on mode
func (ge *GameEngine) StartGame() error {
	ge.gameState.Status = StatusActive
	ge.isRunning = true

	if ge.gameState.GameMode == ModeEnhanced {
		return ge.startEnhancedMode()
	} else {
		return ge.startSimpleMode()
	}
}

// startSimpleMode initializes turn-based gameplay
func (ge *GameEngine) startSimpleMode() error {
	// Simple mode is turn-based, no timer needed
	ge.logEvent("GAME_START", ge.gameState.CurrentTurn, map[string]interface{}{
		"mode": "Simple TCR",
	})
	return nil
}

// startEnhancedMode initializes real-time gameplay with timer
func (ge *GameEngine) startEnhancedMode() error {
	// Start mana regeneration for both players
	go ge.manaRegeneration()

	// Start game timer (3 minutes)
	ge.gameTimer = time.NewTimer(time.Duration(GameDurationSeconds) * time.Second)
	go ge.gameTimeoutHandler()

	ge.logEvent("GAME_START", "", map[string]interface{}{
		"mode":       "Enhanced TCR",
		"duration":   GameDurationSeconds,
		"mana_regen": ManaRegenPerSecond,
	})

	return nil
}

// SummonTroop handles troop summoning logic
func (ge *GameEngine) SummonTroop(playerID string, troopName TroopType) (*CombatAction, error) {
	player := ge.getPlayer(playerID)
	if player == nil {
		return nil, fmt.Errorf("player not found")
	}

	// Check if it's player's turn (Simple mode only)
	if ge.gameState.GameMode == ModeSimple && ge.gameState.CurrentTurn != playerID {
		return nil, fmt.Errorf("not your turn")
	}

	// Check deployment limit BEFORE any deployment
	if ge.gameState.GameMode == ModeSimple {
		if player.TroopsDeployedThisTurn >= 1 {
			return nil, fmt.Errorf("cannot deploy more than one troop per turn in simple mode")
		}
	}

	// Check if troop exists in player's troops
	var selectedTroop *Troop
	for i := range player.Troops {
		if player.Troops[i].Name == troopName {
			selectedTroop = &player.Troops[i]
			break
		}
	}

	if selectedTroop == nil {
		return nil, fmt.Errorf("troop not available")
	}

	// Check mana cost (Enhanced mode only)
	if ge.gameState.GameMode == ModeEnhanced {
		if player.Mana < selectedTroop.MANA {
			return nil, fmt.Errorf("insufficient mana: need %d, have %d", selectedTroop.MANA, player.Mana)
		}
		// Deduct mana only in Enhanced mode
		player.Mana -= selectedTroop.MANA
	}

	// ✅ INCREMENT COUNTER FOR ALL DEPLOYMENTS (including Queen)
	if ge.gameState.GameMode == ModeSimple {
		player.TroopsDeployedThisTurn++
	}

	// Handle special troops (Queen)
	if troopName == Queen {
		action, err := ge.handleQueenSummon(playerID)
		if err != nil {
			// ❗ Rollback counter if Queen summon fails
			if ge.gameState.GameMode == ModeSimple {
				player.TroopsDeployedThisTurn--
			}
			return nil, err
		}

		// Auto end turn after Queen summon in Simple mode
		if ge.gameState.GameMode == ModeSimple {
			go func() {
				time.Sleep(1 * time.Second)
				ge.autoEndTurn(playerID)
			}()
		}

		return action, nil
	}

	// Create summon event for normal troops
	action := CombatAction{
		Type:      ActionSummon,
		PlayerID:  playerID,
		TroopName: troopName,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"mana_left":                 player.Mana,
			"troops_deployed_this_turn": player.TroopsDeployedThisTurn,
		},
	}

	ge.logEvent("SUMMON", playerID, map[string]interface{}{
		"troop":                     troopName,
		"mana_left":                 player.Mana,
		"troops_deployed_this_turn": player.TroopsDeployedThisTurn,
	})

	// Update game state in memory
	ge.updatePlayerInState(player)

	if ge.gameState.GameMode == ModeEnhanced {
		go ge.autoAttackSequence(playerID, troopName)
	}

	return &action, nil
}

// autoAttackSequence handles the full combat sequence for Simple mode
func (ge *GameEngine) autoAttackSequence(playerID string, troopName TroopType) {
	// Wait a moment for deploy animation
	time.Sleep(500 * time.Millisecond)

	// 1. Troop attacks enemy tower
	attackAction := ge.executeAutoAttack(playerID, troopName)
	if attackAction != nil {
		ge.broadcastAction(*attackAction)

		// 2. Tower counter-attacks (1 second later)
		time.Sleep(1 * time.Second)
		counterAction := ge.executeCounterAttack(playerID, troopName)
		if counterAction != nil {
			ge.broadcastAction(*counterAction)
		}
	}

	// 3. Auto end turn (1 second after counter-attack)
	time.Sleep(1 * time.Second)
	ge.autoEndTurn(playerID)
}

// executeAutoAttack makes troop attack the weakest available tower
func (ge *GameEngine) executeAutoAttack(playerID string, troopName TroopType) *CombatAction {
	player := ge.getPlayer(playerID)
	opponent := ge.getOpponent(playerID)

	if player == nil || opponent == nil {
		return nil
	}

	// Find attacker troop
	var attacker *Troop
	for i := range player.Troops {
		if player.Troops[i].Name == troopName {
			attacker = &player.Troops[i]
			break
		}
	}

	if attacker == nil {
		return nil
	}

	// Find target tower with UPDATED RULES
	var targetTowerIndex int = -1

	// Check if any Guard Tower is destroyed - if so, can attack King Tower
	guardTowersAlive := 0
	for i := range opponent.Towers {
		if opponent.Towers[i].Name == GuardTower && opponent.Towers[i].HP > 0 {
			guardTowersAlive++
		}
	}

	// If all Guard Towers destroyed, can attack King Tower
	if guardTowersAlive == 0 {
		for i := range opponent.Towers {
			if opponent.Towers[i].Name == KingTower && opponent.Towers[i].HP > 0 {
				targetTowerIndex = i
				break
			}
		}
	} else {
		// Otherwise, attack weakest Guard Tower
		for i := range opponent.Towers {
			if opponent.Towers[i].Name == GuardTower && opponent.Towers[i].HP > 0 {
				if targetTowerIndex == -1 || opponent.Towers[i].HP < opponent.Towers[targetTowerIndex].HP {
					targetTowerIndex = i
				}
			}
		}
	}

	if targetTowerIndex == -1 {
		return nil
	}

	// Get correct target tower reference
	var targetTower *Tower
	if opponent.ID == ge.gameState.Player1.ID {
		targetTower = &ge.gameState.Player1.Towers[targetTowerIndex]
	} else {
		targetTower = &ge.gameState.Player2.Towers[targetTowerIndex]
	}

	// ✅ CORRECT DAMAGE FORMULA: DMG = ATK_A - DEF_B (min 0)
	damage := attacker.ATK - targetTower.DEF
	if damage < 0 {
		damage = 0
	}

	// ✅ APPLY DAMAGE ONLY ONCE to correct tower
	oldHP := targetTower.HP
	targetTower.HP -= damage
	if targetTower.HP < 0 {
		targetTower.HP = 0
	}

	// ✅ CHECK AND LOG TOWER DESTRUCTION
	if targetTower.HP == 0 && oldHP > 0 {
		ge.logEvent("TOWER_DESTROYED", "", map[string]interface{}{
			"destroyer":    player.Username,
			"tower_name":   targetTower.Name,
			"tower_owner":  opponent.Username,
			"final_damage": damage,
		})

		// Broadcast tower destruction
		destroyAction := CombatAction{
			Type:       "TOWER_DESTROYED",
			PlayerID:   playerID,
			TroopName:  troopName,
			TargetType: "tower",
			TargetName: string(targetTower.Name),
			Damage:     damage,
			Timestamp:  time.Now(),
			Data: map[string]interface{}{
				"destroyer": player.Username,
				"owner":     opponent.Username,
			},
		}
		ge.broadcastAction(destroyAction)

		ge.handleTowerDestroyed(opponent, targetTower)

		// Check win conditions immediately
		if ge.checkWinConditions() {
			ge.endGame()
			winAction := CombatAction{
				Type:      "GAME_END",
				PlayerID:  ge.gameState.Winner,
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"winner": ge.gameState.Winner,
					"reason": "king_tower_destroyed",
				},
			}
			ge.broadcastAction(winAction)
		}
	}

	// Log attack details
	ge.logEvent("ATTACK", playerID, map[string]interface{}{
		"attacker":     troopName,
		"attacker_atk": attacker.ATK,
		"target":       targetTower.Name,
		"target_def":   targetTower.DEF,
		"damage":       damage,
		"target_hp":    targetTower.HP,
		"old_hp":       oldHP,
	})

	// Create action
	action := CombatAction{
		Type:       ActionAttack,
		PlayerID:   playerID,
		TroopName:  troopName,
		TargetType: "tower",
		TargetName: string(targetTower.Name),
		Damage:     damage,
		IsCrit:     false,
		Timestamp:  time.Now(),
		Data: map[string]interface{}{
			"target_hp": targetTower.HP,
			"old_hp":    oldHP,
		},
	}

	return &action
}

// executeCounterAttack makes tower counter-attack the troop
func (ge *GameEngine) executeCounterAttack(playerID string, troopName TroopType) *CombatAction {
	player := ge.getPlayer(playerID)
	opponent := ge.getOpponent(playerID)

	if player == nil || opponent == nil {
		return nil
	}

	// Find the troop that just attacked
	var targetTroop *Troop
	for i := range player.Troops {
		if player.Troops[i].Name == troopName {
			targetTroop = &player.Troops[i]
			break
		}
	}

	if targetTroop == nil || targetTroop.HP <= 0 {
		return nil
	}

	// Find a tower that can counter-attack (alive towers)
	var attackingTower *Tower
	for i := range opponent.Towers {
		if opponent.Towers[i].HP > 0 {
			attackingTower = &opponent.Towers[i]
			break
		}
	}

	if attackingTower == nil {
		return nil
	}

	// ✅ CORRECT DAMAGE FORMULA: DMG = Tower_ATK - Troop_DEF (min 0)
	damage := attackingTower.ATK - targetTroop.DEF
	if damage < 0 {
		damage = 0
	}

	// Apply damage to troop
	oldHP := targetTroop.HP
	targetTroop.HP -= damage
	if targetTroop.HP < 0 {
		targetTroop.HP = 0
	}

	// ✅ CHECK AND LOG TROOP DESTRUCTION
	if targetTroop.HP == 0 && oldHP > 0 {
		ge.logEvent("TROOP_DESTROYED", "", map[string]interface{}{
			"destroyer":    opponent.Username,
			"troop_name":   targetTroop.Name,
			"troop_owner":  player.Username,
			"final_damage": damage,
		})

		// Broadcast troop destruction
		destroyAction := CombatAction{
			Type:       "TROOP_DESTROYED",
			PlayerID:   opponent.ID,
			TroopName:  TroopType(attackingTower.Name),
			TargetType: "troop",
			TargetName: string(troopName),
			Damage:     damage,
			Timestamp:  time.Now(),
			Data: map[string]interface{}{
				"destroyer": opponent.Username,
				"owner":     player.Username,
			},
		}
		ge.broadcastAction(destroyAction)
	}

	// Update game state
	ge.updatePlayerInState(player)

	// Log counter-attack details
	ge.logEvent("COUNTER_ATTACK", opponent.ID, map[string]interface{}{
		"attacker":     attackingTower.Name,
		"attacker_atk": attackingTower.ATK,
		"target":       troopName,
		"target_def":   targetTroop.DEF,
		"damage":       damage,
		"target_hp":    targetTroop.HP,
		"old_hp":       oldHP,
	})

	// Create counter-attack action
	action := CombatAction{
		Type:       ActionAttack,
		PlayerID:   opponent.ID,
		TroopName:  TroopType(attackingTower.Name),
		TargetType: "troop",
		TargetName: string(troopName),
		Damage:     damage,
		IsCrit:     false,
		Timestamp:  time.Now(),
		Data: map[string]interface{}{
			"target_hp":  targetTroop.HP,
			"old_hp":     oldHP,
			"is_counter": true,
		},
	}

	return &action
}

// autoEndTurn automatically ends turn and switches to opponent
func (ge *GameEngine) autoEndTurn(playerID string) {
	if ge.gameState.GameMode != ModeSimple {
		return
	}

	if ge.gameState.CurrentTurn != playerID {
		return
	}

	// Switch turn
	ge.switchTurn()

	// Create turn end action
	action := CombatAction{
		Type:      "TURN_END",
		PlayerID:  playerID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"next_turn": ge.gameState.CurrentTurn,
		},
	}

	ge.logEvent("TURN_END", playerID, map[string]interface{}{
		"next_turn": ge.gameState.CurrentTurn,
	})

	// Broadcast turn change
	ge.broadcastAction(action)
}

// broadcastAction sends action to event channel for server broadcasting
func (ge *GameEngine) broadcastAction(action CombatAction) {
	select {
	case ge.eventChan <- action:
		// Successfully sent
	default:
		// Channel full, add to queue
		ge.eventQueue = append(ge.eventQueue, action)
	}
}

// Check win conditions after each action
func (ge *GameEngine) checkAndHandleWinConditions() {
	if ge.checkWinConditions() {
		ge.endGame()
		// Broadcast game end
		action := CombatAction{
			Type:      "GAME_END",
			PlayerID:  ge.gameState.Winner,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"winner": ge.gameState.Winner,
				"reason": "king_tower_destroyed",
			},
		}
		ge.broadcastAction(action)
	}
}

// handleQueenSummon handles Queen's special healing ability
func (ge *GameEngine) handleQueenSummon(playerID string) (*CombatAction, error) {
	player := ge.getPlayer(playerID)

	// Find tower with lowest HP
	var lowestTower *Tower
	lowestHP := math.MaxInt32

	for i := range player.Towers {
		if player.Towers[i].HP < lowestHP && player.Towers[i].HP > 0 {
			lowestHP = player.Towers[i].HP
			lowestTower = &player.Towers[i]
		}
	}

	if lowestTower == nil {
		return nil, fmt.Errorf("no towers to heal")
	}

	// Heal for 300 HP (capped at max HP)
	healAmount := 300
	if lowestTower.HP+healAmount > lowestTower.MaxHP {
		healAmount = lowestTower.MaxHP - lowestTower.HP
	}

	lowestTower.HP += healAmount

	action := CombatAction{
		Type:       ActionHeal,
		PlayerID:   playerID,
		TroopName:  Queen,
		TargetType: "tower",
		TargetName: string(lowestTower.Name),
		HealAmount: healAmount,
		Timestamp:  time.Now(),
		Data: map[string]interface{}{
			"tower_hp":  lowestTower.HP,
			"mana_left": player.Mana,
		},
	}

	ge.logEvent("HEAL", playerID, map[string]interface{}{
		"troop":       Queen,
		"target":      lowestTower.Name,
		"heal_amount": healAmount,
		"tower_hp":    lowestTower.HP,
	})

	ge.updatePlayerInState(player)
	return &action, nil
}

// ExecuteAttack handles combat between troops and towers
func (ge *GameEngine) ExecuteAttack(playerID string, attackerName TroopType, targetType, targetName string) (*CombatAction, error) {
	player := ge.getPlayer(playerID)
	opponent := ge.getOpponent(playerID)

	if player == nil || opponent == nil {
		return nil, fmt.Errorf("invalid players")
	}

	// Find attacker troop
	var attacker *Troop
	for i := range player.Troops {
		if player.Troops[i].Name == attackerName {
			attacker = &player.Troops[i]
			break
		}
	}

	if attacker == nil {
		return nil, fmt.Errorf("attacker troop not found")
	}

	if attacker.HP <= 0 {
		return nil, fmt.Errorf("troop is destroyed and cannot attack")
	}

	// ✅ UPDATED TARGETING RULES: Can attack King Tower if any Guard Tower is destroyed
	if ge.gameState.GameMode == ModeSimple {
		if err := ge.validateAttackTargetUpdated(opponent, targetType, targetName); err != nil {
			return nil, err
		}
	}

	// Find target
	var targetTower *Tower
	if targetType == "tower" {
		for i := range opponent.Towers {
			if string(opponent.Towers[i].Name) == targetName {
				if opponent.ID == ge.gameState.Player1.ID {
					targetTower = &ge.gameState.Player1.Towers[i]
				} else {
					targetTower = &ge.gameState.Player2.Towers[i]
				}
				break
			}
		}
		if targetTower == nil {
			return nil, fmt.Errorf("target tower not found")
		}
		if targetTower.HP <= 0 {
			return nil, fmt.Errorf("target tower is already destroyed")
		}
	}

	damage := attacker.ATK - targetTower.DEF
	if damage < 0 {
		damage = 0
	}
	isCrit := false

	// Apply damage
	oldHP := targetTower.HP
	targetTower.HP -= damage
	if targetTower.HP < 0 {
		targetTower.HP = 0
	}

	// Check if tower is destroyed
	if targetTower.HP == 0 && oldHP > 0 {
		ge.logEvent("TOWER_DESTROYED", "", map[string]interface{}{
			"destroyer":    player.Username,
			"tower_name":   targetTower.Name,
			"tower_owner":  opponent.Username,
			"final_damage": damage,
		})
		ge.handleTowerDestroyed(opponent, targetTower)
	}

	action := CombatAction{
		Type:       ActionAttack,
		PlayerID:   playerID,
		TroopName:  attackerName,
		TargetType: targetType,
		TargetName: targetName,
		Damage:     damage,
		IsCrit:     isCrit,
		Timestamp:  time.Now(),
		Data: map[string]interface{}{
			"target_hp": targetTower.HP,
			"old_hp":    oldHP,
		},
	}

	// Update game state
	ge.updatePlayerInState(opponent)

	if ge.gameState.GameMode == ModeSimple {
		go func() {
			time.Sleep(1 * time.Second)
			counterAction := ge.executeCounterAttack(playerID, attackerName)
			if counterAction != nil {
				ge.broadcastAction(*counterAction)
			}
		}()
	}

	// Check win conditions
	if ge.checkWinConditions() {
		ge.endGame()
	}

	return &action, nil
}

func (ge *GameEngine) validateAttackTargetUpdated(opponent *Player, targetType, targetName string) error {
	if targetType != "tower" {
		return nil // No restrictions on troop attacks
	}

	// ✅ UPDATED RULE: Can attack King Tower if ANY Guard Tower is destroyed
	if targetName == "King Tower" {
		guardTowersAlive := 0
		for _, tower := range opponent.Towers {
			if tower.Name == GuardTower && tower.HP > 0 {
				guardTowersAlive++
			}
		}

		// If all Guard Towers are destroyed, can attack King Tower
		if guardTowersAlive == 2 { // Both Guard Towers still alive
			return fmt.Errorf("must destroy at least one Guard Tower before attacking King Tower")
		}
	}

	return nil
}

// calculateDamage implements the damage formula: DMG = ATK_A - DEF_B
func (ge *GameEngine) calculateDamage(atk, def int, withCrit bool) int {
	baseDamage := atk - def
	if baseDamage < 0 {
		baseDamage = 0
	}

	if withCrit {
		// Enhanced mode: 20% damage increase for crits
		return int(float64(baseDamage) * 1.2)
	}

	return baseDamage
}

// validateAttackTarget enforces Simple TCR targeting rules
func (ge *GameEngine) validateAttackTarget(opponent *Player, targetType, targetName string) error {
	if targetType != "tower" {
		return nil // No restrictions on troop attacks
	}

	// Check if trying to attack King Tower while Guard Towers are alive
	if targetName == "King Tower" {
		for _, tower := range opponent.Towers {
			if tower.Name == GuardTower && tower.HP > 0 {
				return fmt.Errorf("must destroy Guard Towers before attacking King Tower")
			}
		}
	}

	return nil
}

// handleTowerDestroyed handles tower destruction logic
func (ge *GameEngine) handleTowerDestroyed(player *Player, tower *Tower) {
	tower.IsActive = false

	// Update towers killed count
	if player.ID == ge.gameState.Player1.ID {
		ge.gameState.TowersKilled.Player1++
	} else {
		ge.gameState.TowersKilled.Player2++
	}

	ge.logEvent("TOWER_DESTROYED", "", map[string]interface{}{
		"owner":      player.Username,
		"tower_name": tower.Name,
	})
}

// manaRegeneration handles mana regeneration for Enhanced mode
func (ge *GameEngine) manaRegeneration() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for ge.isRunning {
		select {
		case <-ticker.C:
			// Regenerate mana for both players
			if ge.gameState.Player1.Mana < MaxMana {
				ge.gameState.Player1.Mana += ManaRegenPerSecond
				if ge.gameState.Player1.Mana > MaxMana {
					ge.gameState.Player1.Mana = MaxMana
				}
			}

			if ge.gameState.Player2.Mana < MaxMana {
				ge.gameState.Player2.Mana += ManaRegenPerSecond
				if ge.gameState.Player2.Mana > MaxMana {
					ge.gameState.Player2.Mana = MaxMana
				}
			}

			// Decrease time left
			ge.gameState.TimeLeft--
		}
	}
}

// gameTimeoutHandler handles game timeout for Enhanced mode
func (ge *GameEngine) gameTimeoutHandler() {
	<-ge.gameTimer.C
	if ge.isRunning {
		ge.endGameByTimeout()
	}
}

// checkWinConditions checks if game should end
func (ge *GameEngine) checkWinConditions() bool {
	// Check if King Tower is destroyed
	for _, tower := range ge.gameState.Player1.Towers {
		if tower.Name == KingTower && tower.HP == 0 {
			ge.gameState.Winner = ge.gameState.Player2.ID
			return true
		}
	}

	for _, tower := range ge.gameState.Player2.Towers {
		if tower.Name == KingTower && tower.HP == 0 {
			ge.gameState.Winner = ge.gameState.Player1.ID
			return true
		}
	}

	return false
}

// EndTurn handles ending a player's turn (Simple mode only)
func (ge *GameEngine) EndTurn(playerID string) error {
	if ge.gameState.GameMode != ModeSimple {
		return fmt.Errorf("end turn only available in Simple mode")
	}

	if ge.gameState.CurrentTurn != playerID {
		return fmt.Errorf("not your turn")
	}

	// Switch to next player
	ge.switchTurn()

	ge.logEvent("TURN_END", playerID, map[string]interface{}{
		"next_turn": ge.gameState.CurrentTurn,
	})

	return nil
}

// endGameByTimeout ends game when time runs out (Enhanced mode)
func (ge *GameEngine) endGameByTimeout() {
	// Determine winner by towers destroyed
	if ge.gameState.TowersKilled.Player1 > ge.gameState.TowersKilled.Player2 {
		ge.gameState.Winner = ge.gameState.Player2.ID // Player 2 destroyed more of Player 1's towers
	} else if ge.gameState.TowersKilled.Player2 > ge.gameState.TowersKilled.Player1 {
		ge.gameState.Winner = ge.gameState.Player1.ID
	} else {
		ge.gameState.Winner = "draw"
	}

	ge.endGame()
}

// endGame handles game conclusion
func (ge *GameEngine) endGame() {
	ge.isRunning = false
	ge.gameState.Status = StatusFinished

	if ge.gameTimer != nil {
		ge.gameTimer.Stop()
	}

	ge.logEvent("GAME_END", ge.gameState.Winner, map[string]interface{}{
		"towers_p1": ge.gameState.TowersKilled.Player1,
		"towers_p2": ge.gameState.TowersKilled.Player2,
	})
}

// switchTurn changes current turn (Simple mode)
func (ge *GameEngine) switchTurn() {
	// Reset deployment counters for both players
	if ge.gameState.GameMode == ModeSimple {
		ge.gameState.Player1.TroopsDeployedThisTurn = 0
		ge.gameState.Player2.TroopsDeployedThisTurn = 0
	}

	// Switch turn
	if ge.gameState.CurrentTurn == ge.gameState.Player1.ID {
		ge.gameState.CurrentTurn = ge.gameState.Player2.ID
	} else {
		ge.gameState.CurrentTurn = ge.gameState.Player1.ID
	}
}

// Helper functions

func (ge *GameEngine) getPlayer(playerID string) *Player {
	if ge.gameState.Player1.ID == playerID {
		return &ge.gameState.Player1
	} else if ge.gameState.Player2.ID == playerID {
		return &ge.gameState.Player2
	}
	return nil
}

func (ge *GameEngine) getOpponent(playerID string) *Player {
	if ge.gameState.Player1.ID == playerID {
		return &ge.gameState.Player2
	} else if ge.gameState.Player2.ID == playerID {
		return &ge.gameState.Player1
	}
	return nil
}

func (ge *GameEngine) updatePlayerInState(player *Player) {
	if player.ID == ge.gameState.Player1.ID {
		ge.gameState.Player1 = *player
	} else if player.ID == ge.gameState.Player2.ID {
		ge.gameState.Player2 = *player
	}
}

func (ge *GameEngine) logEvent(eventType, playerID string, data map[string]interface{}) {
	// This would normally send to event channel for broadcasting
	// For now, we'll store in the event queue
	action := CombatAction{
		Type:      eventType,
		PlayerID:  playerID,
		Timestamp: time.Now(),
		Data:      data,
	}
	ge.eventQueue = append(ge.eventQueue, action)
}

// GetGameState returns current game state
func (ge *GameEngine) GetGameState() *GameState {
	return ge.gameState
}

// GetEventChannel returns the event channel for broadcasting
func (ge *GameEngine) GetEventChannel() <-chan CombatAction {
	return ge.eventChan
}

// IsRunning returns if game is currently active
func (ge *GameEngine) IsRunning() bool {
	return ge.isRunning
}

// generateGameID creates a unique game ID
func generateGameID() string {
	return fmt.Sprintf("game_%d_%d", time.Now().Unix(), rand.Intn(1000))
}
