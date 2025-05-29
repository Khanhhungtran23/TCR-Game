// Package game implements the core game engine and combat logic
package game

import (
	"fmt"
	"math"
	"math/rand"
	"tcr-game/pkg/logger"
	"time"
)

// GameEngine handles all game logic and state management
type GameEngine struct {
	gameState   *GameState
	gameSpecs   *GameSpecs
	eventQueue  []CombatAction
	gameTimer   *time.Timer
	isRunning   bool
	eventChan   chan CombatAction
	dataManager *DataManager
	logger      *logger.Logger
}

// NewGameEngine creates a new game engine instance
func NewGameEngine(player1, player2 *Player, gameMode string, specs *GameSpecs, dataManager *DataManager) *GameEngine {
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
		gameState:   gameState,
		gameSpecs:   specs,
		eventQueue:  make([]CombatAction, 0),
		isRunning:   false,
		eventChan:   make(chan CombatAction, 100),
		dataManager: dataManager,
		logger:      logger.Server,
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
	ge.logEvent("GAME_START", ge.gameState.CurrentTurn, map[string]interface{}{
		"mode": "Simple TCR",
	})
	return nil
}

func (ge *GameEngine) startEnhancedMode() error {
	// Start mana regeneration for both players
	go ge.manaRegeneration()

	ge.gameTimer = time.NewTimer(time.Duration(GameDurationSeconds) * time.Second)
	go ge.gameTimeoutHandler()

	ge.gameState.TimeLeft = GameDurationSeconds

	ge.logEvent("GAME_START", "", map[string]interface{}{
		"mode":       "Enhanced TCR",
		"duration":   GameDurationSeconds,
		"mana_regen": ManaRegenPerSecond,
		"start_time": time.Now(),
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

	if ge.gameState.GameMode == ModeEnhanced {
		baseSpec := ge.gameSpecs.TroopSpecs[troopName]
		playerLevel := selectedTroop.Level

		// Restore to full HP (scaled by level)
		fullHP := int(float64(baseSpec.HP) * (1.0 + float64(playerLevel-1)*StatScalePerLevel))
		oldHP := selectedTroop.HP
		selectedTroop.HP = fullHP
		selectedTroop.MaxHP = fullHP

		ge.logger.Debug("🔄 Revived %s: %d HP -> %d HP", troopName, oldHP, fullHP)

		if oldHP <= 0 {
			ge.logEvent("TROOP_REVIVED", playerID, map[string]interface{}{
				"troop":  troopName,
				"old_hp": oldHP,
				"new_hp": fullHP,
				"max_hp": fullHP,
			})
		}
	} else if ge.gameState.GameMode == ModeSimple {
		// Simple mode revive logic (existing)
		baseSpec := ge.gameSpecs.TroopSpecs[troopName]
		playerLevel := selectedTroop.Level

		fullHP := int(float64(baseSpec.HP) * (1.0 + float64(playerLevel-1)*StatScalePerLevel))
		selectedTroop.HP = fullHP
		selectedTroop.MaxHP = fullHP

		ge.logEvent("TROOP_REVIVED", playerID, map[string]interface{}{
			"troop":  troopName,
			"new_hp": fullHP,
			"max_hp": fullHP,
		})
	}

	// Check mana cost (Enhanced mode only)
	if ge.gameState.GameMode == ModeEnhanced {
		if player.Mana < selectedTroop.MANA {
			return nil, fmt.Errorf("insufficient mana: need %d, have %d", selectedTroop.MANA, player.Mana)
		}
		player.Mana -= selectedTroop.MANA
	}

	if ge.gameState.GameMode == ModeSimple {
		player.TroopsDeployedThisTurn++
	}

	// Handle special troops (Queen)
	if troopName == Queen {
		action, err := ge.handleQueenSummon(playerID)
		if err != nil {
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
			"troop_hp":                  selectedTroop.HP, // ✅ ADD: Current HP
		},
	}

	ge.logEvent("SUMMON", playerID, map[string]interface{}{
		"troop":                     troopName,
		"troop_hp":                  selectedTroop.HP,
		"mana_left":                 player.Mana,
		"troops_deployed_this_turn": player.TroopsDeployedThisTurn,
	})

	ge.updatePlayerInState(player)

	if ge.gameState.GameMode == ModeEnhanced {
		go ge.autoAttackSequence(playerID, troopName)
	}

	return &action, nil
}

// autoAttackSequence handles the full combat sequence for Enhanced mode
func (ge *GameEngine) autoAttackSequence(playerID string, troopName TroopType) {
	time.Sleep(500 * time.Millisecond)

	attackAction := ge.executeAutoAttack(playerID, troopName)
	if attackAction != nil {
		ge.broadcastAction(*attackAction)

		time.Sleep(2 * time.Second)
		counterAction := ge.executeCounterAttack(playerID, troopName)
		if counterAction != nil {
			ge.broadcastAction(*counterAction)
		}
	}

	time.Sleep(1 * time.Second)
	if !ge.checkWinConditions() {
		// Just continue, no turn ending
		return
	}
}

func (ge *GameEngine) executeAutoAttack(playerID string, troopName TroopType) *CombatAction {
	player := ge.getPlayer(playerID)
	opponent := ge.getOpponent(playerID)

	if player == nil || opponent == nil {
		return nil
	}

	var attacker *Troop
	for i := range player.Troops {
		if player.Troops[i].Name == troopName {
			attacker = &player.Troops[i]
			break
		}
	}

	if attacker == nil || attacker.HP <= 0 {
		return nil
	}

	// Find target tower with updated rules
	var targetTowerIndex int = -1

	guardTowersAlive := 0
	for i := range opponent.Towers {
		if (opponent.Towers[i].Name == GuardTower1 || opponent.Towers[i].Name == GuardTower2) && opponent.Towers[i].HP > 0 {
			guardTowersAlive++
		}
	}

	if guardTowersAlive == 0 {
		for i := range opponent.Towers {
			if opponent.Towers[i].Name == KingTower && opponent.Towers[i].HP > 0 {
				targetTowerIndex = i
				break
			}
		}
	} else {
		for i := range opponent.Towers {
			if (opponent.Towers[i].Name == GuardTower1 || opponent.Towers[i].Name == GuardTower2) && opponent.Towers[i].HP > 0 {
				if targetTowerIndex == -1 || opponent.Towers[i].HP < opponent.Towers[targetTowerIndex].HP {
					targetTowerIndex = i
				}
			}
		}
	}

	if targetTowerIndex == -1 {
		return nil
	}

	var targetTower *Tower
	if opponent.ID == ge.gameState.Player1.ID {
		targetTower = &ge.gameState.Player1.Towers[targetTowerIndex]
	} else {
		targetTower = &ge.gameState.Player2.Towers[targetTowerIndex]
	}

	isCrit := false
	attackDamage := attacker.ATK
	if ge.gameState.GameMode == ModeEnhanced {
		// Roll for crit chance
		if rand.Float64() < attacker.CRIT {
			isCrit = true
			attackDamage = int(float64(attacker.ATK) * 1.5) // 1.5x damage on crit
		}
	}

	damage := attackDamage - targetTower.DEF
	if damage < 0 {
		damage = 0
	}

	oldHP := targetTower.HP
	targetTower.HP -= damage
	if targetTower.HP < 0 {
		targetTower.HP = 0
	}

	if damage > 0 {
		ge.awardEXPForDamage(playerID, damage, "tower")
	}

	// Check tower destruction
	towerDestroyed := false
	if targetTower.HP == 0 && oldHP > 0 {
		towerDestroyed = true
		ge.awardEXPForDestruction(playerID, "tower", targetTower.Name)

		expGained := ge.dataManager.CalculateDestructionEXP("tower", targetTower.Name)
		expAction := CombatAction{
			Type:      "EXP_GAINED",
			PlayerID:  playerID,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"amount": expGained,
				"reason": fmt.Sprintf("destroying %s", targetTower.Name),
			},
		}
		ge.broadcastAction(expAction)

		ge.logEvent("TOWER_DESTROYED", "", map[string]interface{}{
			"destroyer":    player.Username,
			"tower_name":   targetTower.Name,
			"tower_owner":  opponent.Username,
			"final_damage": damage,
		})

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

		if ge.checkWinConditions() {
			ge.endGame()
		}
	}

	// Create action
	action := CombatAction{
		Type:       ActionAttack,
		PlayerID:   playerID,
		TroopName:  troopName,
		TargetType: "tower",
		TargetName: string(targetTower.Name),
		Damage:     damage,
		IsCrit:     isCrit, // ✅ NEW: Include crit info
		Timestamp:  time.Now(),
		Data: map[string]interface{}{
			"target_hp": targetTower.HP,
			"old_hp":    oldHP,
			"tower_destroyed": towerDestroyed,
		},
	}

	return &action
}

func (ge *GameEngine) executeCounterAttack(playerID string, troopName TroopType) *CombatAction {
	player := ge.getPlayer(playerID)
	opponent := ge.getOpponent(playerID)

	if player == nil || opponent == nil {
		return nil
	}

	var targetTroop *Troop
	for i := range player.Troops {
		if player.Troops[i].Name == troopName {
			targetTroop = &player.Troops[i]
			break
		}
	}

	if targetTroop == nil {
		return nil
	}

	// Special case for Queen - she can't be attacked
	if targetTroop.Name == Queen {
		return nil
	}

	// Check if troop is already destroyed
	if targetTroop.HP <= 0 {
		return nil
	}

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

	isCrit := false
	attackDamage := attackingTower.ATK
	if ge.gameState.GameMode == ModeEnhanced {
		// Roll for crit chance
		if rand.Float64() < attackingTower.CRIT {
			isCrit = true
			attackDamage = int(float64(attackingTower.ATK) * 1.5) // 1.5x damage on crit
		}
	}

	damage := attackDamage - targetTroop.DEF
	if damage < 0 {
		damage = 0
	}

	oldHP := targetTroop.HP
	targetTroop.HP -= damage
	if targetTroop.HP < 0 {
		targetTroop.HP = 0
	}

	if damage > 0 {
		ge.awardEXPForDamage(opponent.ID, damage, "troop")
	}

	// Check troop destruction
	if targetTroop.HP == 0 && oldHP > 0 {
		ge.awardEXPForDestruction(opponent.ID, "troop", TroopType(targetTroop.Name))

		ge.logEvent("TROOP_DESTROYED", "", map[string]interface{}{
			"destroyer":    opponent.Username,
			"troop_name":   targetTroop.Name,
			"troop_owner":  player.Username,
			"final_damage": damage,
		})

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

	ge.updatePlayerInState(player)

	ge.logEvent("COUNTER_ATTACK", opponent.ID, map[string]interface{}{
		"attacker":     attackingTower.Name,
		"attacker_atk": attackingTower.ATK,
		"target":       troopName,
		"target_def":   targetTroop.DEF,
		"damage":       damage,
		"target_hp":    targetTroop.HP,
		"old_hp":       oldHP,
		"is_crit":      isCrit,
		"message":      fmt.Sprintf("%s counter-attacks %s for %d damage!", attackingTower.Name, troopName, damage),
	})

	action := CombatAction{
		Type:       ActionAttack,
		PlayerID:   opponent.ID,
		TroopName:  TroopType(attackingTower.Name),
		TargetType: "troop",
		TargetName: string(troopName),
		Damage:     damage,
		IsCrit:     isCrit,
		Timestamp:  time.Now(),
		Data: map[string]interface{}{
			"target_hp":  targetTroop.HP,
			"old_hp":     oldHP,
			"is_counter": true,
		},
	}

	return &action
}

func (ge *GameEngine) awardEXPForDamage(playerID string, damage int, targetType string) {
	baseEXP := damage / 50
	if baseEXP < 1 {
		baseEXP = 1
	}

	player := ge.getPlayer(playerID)
	if player != nil {
		player.EXP += baseEXP
		ge.logEvent("EXP_GAINED", playerID, map[string]interface{}{
			"amount": baseEXP,
			"reason": fmt.Sprintf("dealing %d damage to %s", damage, targetType),
		})
	}
}

func (ge *GameEngine) awardEXPForDestruction(playerID string, targetType string, targetName interface{}) {
	var expAmount int

	if targetType == "tower" {
		switch targetName {
		case KingTower:
			expAmount = 200
		case GuardTower1, GuardTower2:
			expAmount = 100
		}
	} else if targetType == "troop" {
		if troopName, ok := targetName.(TroopType); ok {
			if spec, exists := ge.gameSpecs.TroopSpecs[troopName]; exists {
				expAmount = spec.EXP
			}
		}
	}

	if expAmount > 0 {
		player := ge.getPlayer(playerID)
		if player != nil {
			player.EXP += expAmount
			ge.logEvent("EXP_GAINED", playerID, map[string]interface{}{
				"amount": expAmount,
				"reason": fmt.Sprintf("destroying %s %s", targetType, targetName),
			})
		}
	}
}

// autoEndTurn automatically ends turn and switches to opponent
func (ge *GameEngine) autoEndTurn(playerID string) {
	if ge.gameState.GameMode != ModeSimple {
		return
	}

	if ge.gameState.CurrentTurn != playerID {
		return
	}

	ge.switchTurn()

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

	ge.broadcastAction(action)
}

// ExecuteAttack handles manual combat between troops and towers
func (ge *GameEngine) ExecuteAttack(playerID string, attackerName TroopType, targetType, targetName string) (*CombatAction, error) {
	player := ge.getPlayer(playerID)
	opponent := ge.getOpponent(playerID)

	if player == nil || opponent == nil {
		return nil, fmt.Errorf("invalid players")
	}

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

	if ge.gameState.GameMode == ModeSimple {
		if err := ge.validateAttackTargetUpdated(opponent, targetType, targetName); err != nil {
			return nil, err
		}
	}

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

	isCrit := false
	attackDamage := attacker.ATK
	if ge.gameState.GameMode == ModeEnhanced {
		// Roll for crit chance
		if rand.Float64() < attacker.CRIT {
			isCrit = true
			attackDamage = int(float64(attacker.ATK) * 1.5) // 1.5x damage on crit
		}
	}

	damage := attackDamage - targetTower.DEF
	if damage < 0 {
		damage = 0
	}

	oldHP := targetTower.HP
	targetTower.HP -= damage
	if targetTower.HP < 0 {
		targetTower.HP = 0
	}

	// Award EXP for damage
	if damage > 0 {
		ge.awardEXPForDamage(playerID, damage, "tower")
	}

	// Check if tower is destroyed
	towerDestroyed := false
	if targetTower.HP == 0 && oldHP > 0 {
		towerDestroyed = true
		ge.awardEXPForDestruction(playerID, "tower", targetTower.Name)
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
			"tower_destroyed": towerDestroyed,
		},
	}

	ge.updatePlayerInState(opponent)

	if ge.gameState.GameMode == ModeSimple {
		go func() {
			time.Sleep(2 * time.Second)
			counterAction := ge.executeCounterAttack(playerID, attackerName)
			if counterAction != nil {
				ge.broadcastAction(*counterAction)
			}
		}()
	}

	if ge.checkWinConditions() {
		ge.endGame()
	}

	return &action, nil
}

// validateAttackTargetUpdated with new targeting rules
func (ge *GameEngine) validateAttackTargetUpdated(opponent *Player, targetType, targetName string) error {
	if targetType != "tower" {
		return nil
	}

	if targetName == "King Tower" {
		guardTowersAlive := 0
		for _, tower := range opponent.Towers {
			if (tower.Name == GuardTower1 || tower.Name == GuardTower2) && tower.HP > 0 {
				guardTowersAlive++
			}
		}

		if guardTowersAlive == 2 {
			return fmt.Errorf("must destroy at least one Guard Tower before attacking King Tower")
		}
	}

	return nil
}

// handleQueenSummon handles Queen's special healing ability
func (ge *GameEngine) handleQueenSummon(playerID string) (*CombatAction, error) {
	player := ge.getPlayer(playerID)

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

// handleTowerDestroyed handles tower destruction logic
func (ge *GameEngine) handleTowerDestroyed(player *Player, tower *Tower) {
	tower.IsActive = false

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

// gameTimeoutHandler handles game timeout for Enhanced mode
func (ge *GameEngine) gameTimeoutHandler() {
	<-ge.gameTimer.C
	if ge.isRunning {
		ge.endGameByTimeout()
	}
}

// checkWinConditions checks if game should end
func (ge *GameEngine) checkWinConditions() bool {
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

	ge.switchTurn()

	ge.logEvent("TURN_END", playerID, map[string]interface{}{
		"next_turn": ge.gameState.CurrentTurn,
	})

	return nil
}

func (ge *GameEngine) Surrender(playerID string) error {
	// Determine winner (opponent of surrendering player)
	if playerID == ge.gameState.Player1.ID {
		ge.gameState.Winner = ge.gameState.Player2.ID
	} else {
		ge.gameState.Winner = ge.gameState.Player1.ID
	}

	// Award EXP for surrender
	ge.awardGameEndEXP()

	ge.endGame()

	ge.logEvent("SURRENDER", playerID, map[string]interface{}{
		"winner": ge.gameState.Winner,
		"reason": "opponent_surrendered",
	})

	return nil
}

func (ge *GameEngine) awardGameEndEXP() {
	var winnerEXP, loserEXP int

	if ge.gameState.Winner == "draw" {
		winnerEXP = DrawEXP
		loserEXP = DrawEXP
	} else {
		winnerEXP = WinEXP
		// loserEXP = LoseEXP
	}

	// Award EXP to both players
	ge.gameState.Player1.EXP += loserEXP
	ge.gameState.Player2.EXP += loserEXP

	if ge.gameState.Winner == ge.gameState.Player1.ID {
		ge.gameState.Player1.EXP += (winnerEXP - loserEXP) // Add difference for winner
	} else if ge.gameState.Winner == ge.gameState.Player2.ID {
		ge.gameState.Player2.EXP += (winnerEXP - loserEXP)
	}

	if ge.dataManager != nil {
		isWinner1 := ge.gameState.Winner == ge.gameState.Player1.ID
		isWinner2 := ge.gameState.Winner == ge.gameState.Player2.ID

		finalEXP1 := loserEXP
		finalEXP2 := loserEXP

		if isWinner1 {
			finalEXP1 = winnerEXP
		} else if isWinner2 {
			finalEXP2 = winnerEXP
		}

		ge.dataManager.UpdatePlayerData(ge.gameState.Player1.Username, finalEXP1, isWinner1, 0)
		ge.dataManager.UpdatePlayerData(ge.gameState.Player2.Username, finalEXP2, isWinner2, 0)
	}

	// Log EXP gains
	ge.logEvent("GAME_END_EXP", "", map[string]interface{}{
		"player1_exp": ge.gameState.Player1.EXP,
		"player2_exp": ge.gameState.Player2.EXP,
		"winner":      ge.gameState.Winner,
	})
}

// endGameByTimeout ends game when time runs out (Enhanced mode)
func (ge *GameEngine) endGameByTimeout() {
	if !ge.isRunning {
		return // Game already ended
	}

	ge.logger.Info("Game ending by timeout - determining winner...")

	player1TowersDestroyed := 0
	player2TowersDestroyed := 0

	// Count Player1's destroyed towers
	for _, tower := range ge.gameState.Player1.Towers {
		if tower.HP <= 0 {
			player1TowersDestroyed++
		}
	}

	// Count Player2's destroyed towers
	for _, tower := range ge.gameState.Player2.Towers {
		if tower.HP <= 0 {
			player2TowersDestroyed++
		}
	}

	// Check King Tower status
	player1KingAlive := false
	player2KingAlive := false

	for _, tower := range ge.gameState.Player1.Towers {
		if tower.Name == KingTower && tower.HP > 0 {
			player1KingAlive = true
			break
		}
	}

	for _, tower := range ge.gameState.Player2.Towers {
		if tower.Name == KingTower && tower.HP > 0 {
			player2KingAlive = true
			break
		}
	}

	if !player1KingAlive && player2KingAlive {
		ge.gameState.Winner = ge.gameState.Player2.ID
		ge.logger.Info("Player2 wins - Player1's King Tower destroyed")
	} else if !player2KingAlive && player1KingAlive {
		ge.gameState.Winner = ge.gameState.Player1.ID
		ge.logger.Info("Player1 wins - Player2's King Tower destroyed")
	} else if !player1KingAlive && !player2KingAlive {
		ge.gameState.Winner = "draw"
		ge.logger.Info("Draw - Both King Towers destroyed")
	} else {
		// Both King Towers alive - compare tower destruction count
		if player1TowersDestroyed < player2TowersDestroyed {
			ge.gameState.Winner = ge.gameState.Player1.ID
			ge.logger.Info("Player1 wins - lost fewer towers (%d vs %d)", player1TowersDestroyed, player2TowersDestroyed)
		} else if player2TowersDestroyed < player1TowersDestroyed {
			ge.gameState.Winner = ge.gameState.Player2.ID
			ge.logger.Info("Player2 wins - lost fewer towers (%d vs %d)", player2TowersDestroyed, player1TowersDestroyed)
		} else {
			ge.gameState.Winner = "draw"
			ge.logger.Info("Draw - same towers destroyed (%d vs %d)", player1TowersDestroyed, player2TowersDestroyed)
		}
	}

	ge.awardGameEndEXP()

	gameEndEvent := CombatAction{
		Type:      "GAME_END",
		PlayerID:  "",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"winner":         ge.gameState.Winner,
			"reason":         "timeout",
			"player1_towers": player1TowersDestroyed,
			"player2_towers": player2TowersDestroyed,
		},
	}
	ge.eventChan <- gameEndEvent

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
	if ge.gameState.GameMode == ModeSimple {
		ge.gameState.Player1.TroopsDeployedThisTurn = 0
		ge.gameState.Player2.TroopsDeployedThisTurn = 0
	}

	if ge.gameState.CurrentTurn == ge.gameState.Player1.ID {
		ge.gameState.CurrentTurn = ge.gameState.Player2.ID
	} else {
		ge.gameState.CurrentTurn = ge.gameState.Player1.ID
	}
}

// manaRegeneration handles mana regeneration for Enhanced mode
func (ge *GameEngine) manaRegeneration() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for ge.isRunning {
		select {
		case <-ticker.C:
			oldMana1 := ge.gameState.Player1.Mana
			oldMana2 := ge.gameState.Player2.Mana

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

			ge.gameState.TimeLeft--

			// Send mana update
			if oldMana1 != ge.gameState.Player1.Mana || oldMana2 != ge.gameState.Player2.Mana {
				manaUpdateEvent := CombatAction{
					Type:      "MANA_UPDATE",
					PlayerID:  "",
					Timestamp: time.Now(),
					Data: map[string]interface{}{
						"player1_mana": ge.gameState.Player1.Mana,
						"player2_mana": ge.gameState.Player2.Mana,
						"time_left":    ge.gameState.TimeLeft,
					},
				}
				ge.eventChan <- manaUpdateEvent
			}

			if ge.gameState.TimeLeft <= 0 {
				ge.logger.Info("Time's up! Ending game by timeout...")
				go ge.endGameByTimeout() // Use goroutine to prevent blocking
				return
			}
		}
	}
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

// StopGame stops the game and cleans up resources
func (ge *GameEngine) StopGame() {
	if !ge.isRunning {
		return // Game already stopped
	}

	ge.isRunning = false
	ge.gameState.Status = StatusFinished

	if ge.gameTimer != nil {
		ge.gameTimer.Stop()
	}

	// Send final game end event
	gameEndEvent := CombatAction{
		Type:      "GAME_END",
		PlayerID:  "",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"winner": ge.gameState.Winner,
			"reason": "game_stopped",
		},
	}
	ge.eventChan <- gameEndEvent

	ge.logEvent("GAME_STOPPED", "", map[string]interface{}{
		"towers_p1": ge.gameState.TowersKilled.Player1,
		"towers_p2": ge.gameState.TowersKilled.Player2,
	})
}
