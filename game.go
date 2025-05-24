package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"
)

type GameAction struct {
	Type     string          `json:"type"`
	PlayerID string          `json:"player_id"`
	Data     json.RawMessage `json:"data"`
}

func (s *GameServer) CreateGameRoom(player1, player2 *Player, isEnhanced bool) *GameRoom {
	// Initialize random number generator
	rand.Seed(time.Now().UnixNano())

	room := &GameRoom{
		ID:        fmt.Sprintf("room_%d", time.Now().UnixNano()),
		Players:   [2]*Player{player1, player2},
		StartTime: time.Now(),
		IsEnhanced: isEnhanced,
	}

	// Initialize towers for both players
	for i := 0; i < 2; i++ {
		// King tower
		room.Towers[i][0] = &Tower{
			Type:  "King Tower",
			HP:    2000,
			ATK:   500,
			DEF:   300,
			CRIT:  0.10,
			EXP:   200,
			Level: 1,
		}
		// Guard towers
		room.Towers[i][1] = &Tower{
			Type:  "Guard Tower",
			HP:    1000,
			ATK:   300,
			DEF:   100,
			CRIT:  0.05,
			EXP:   100,
			Level: 1,
		}
		room.Towers[i][2] = &Tower{
			Type:  "Guard Tower",
			HP:    1000,
			ATK:   300,
			DEF:   100,
			CRIT:  0.05,
			EXP:   100,
			Level: 1,
		}
	}

	// Initialize random troops for each player
	troopTypes := []string{"pawn", "bishop", "rook", "knight", "prince", "queen"}
	for i := 0; i < 2; i++ {
		room.Troops[i] = make([]*Troop, 0, 3)
		for j := 0; j < 3; j++ {
			randomIndex := rand.Intn(len(troopTypes))
			troopType := troopTypes[randomIndex]
			var troop *Troop

			switch troopType {
			case "pawn":
				troop = &Troop{
					Name:    "Pawn",
					HP:      50,
					ATK:     150,
					DEF:     100,
					Mana:    3,
					EXP:     5,
					Level:   1,
					CRIT:    0.05,
				}
			case "bishop":
				troop = &Troop{
					Name:    "Bishop",
					HP:      100,
					ATK:     200,
					DEF:     150,
					Mana:    4,
					EXP:     10,
					Level:   1,
					CRIT:    0.10,
				}
			case "rook":
				troop = &Troop{
					Name:    "Rook",
					HP:      250,
					ATK:     200,
					DEF:     200,
					Mana:    5,
					EXP:     25,
					Level:   1,
					CRIT:    0.15,
				}
			case "knight":
				troop = &Troop{
					Name:    "Knight",
					HP:      200,
					ATK:     300,
					DEF:     150,
					Mana:    5,
					EXP:     25,
					Level:   1,
					CRIT:    0.20,
				}
			case "prince":
				troop = &Troop{
					Name:    "Prince",
					HP:      500,
					ATK:     400,
					DEF:     300,
					Mana:    6,
					EXP:     50,
					Level:   1,
					CRIT:    0.25,
				}
			case "queen":
				troop = &Troop{
					Name:    "Queen",
					HP:      300,
					ATK:     250,
					DEF:     200,
					Mana:    5,
					EXP:     30,
					Level:   1,
					CRIT:    0.30,
					Special: "Heals the friendly tower with lowest HP by 300",
				}
			}
			room.Troops[i] = append(room.Troops[i], troop)
		}
	}

	s.mu.Lock()
	s.gameRooms[room.ID] = room
	s.mu.Unlock()

	return room
}

func (r *GameRoom) HandleAction(action GameAction) error {
	playerIndex := -1
	for i, p := range r.Players {
		if p.Username == action.PlayerID {
			playerIndex = i
			break
		}
	}

	if playerIndex == -1 {
		return fmt.Errorf("player not found in room")
	}

	switch action.Type {
	case "deploy_troop":
		var troopType string
		if err := json.Unmarshal(action.Data, &troopType); err != nil {
			return err
		}
		return r.deployTroop(playerIndex, troopType)
	case "attack":
		var attackData struct {
			TroopIndex int `json:"troop_index"`
			TowerIndex int `json:"tower_index"`
		}
		if err := json.Unmarshal(action.Data, &attackData); err != nil {
			return err
		}
		return r.handleAttack(playerIndex, attackData.TroopIndex, attackData.TowerIndex)
	default:
		return fmt.Errorf("unknown action type: %s", action.Type)
	}
}

func (r *GameRoom) deployTroop(playerIndex int, troopType string) error {
	player := r.Players[playerIndex]
	troop := NewTroop(troopType)

	if troop == nil {
		return fmt.Errorf("invalid troop type: %s", troopType)
	}

	if player.Mana < troop.Mana {
		return fmt.Errorf("insufficient mana")
	}

	player.Mana -= troop.Mana
	r.Troops[playerIndex] = append(r.Troops[playerIndex], troop)
	return nil
}

func (r *GameRoom) handleAttack(playerIndex int, troopIndex, towerIndex int) error {
	if troopIndex >= len(r.Troops[playerIndex]) {
		return fmt.Errorf("invalid troop index")
	}

	if towerIndex < 0 || towerIndex > 2 {
		return fmt.Errorf("invalid tower index")
	}

	opponentIndex := (playerIndex + 1) % 2
	troop := r.Troops[playerIndex][troopIndex]
	tower := r.Towers[opponentIndex][towerIndex]

	// Check if first guard tower is still alive before attacking second guard tower or king tower
	if towerIndex > 0 && r.Towers[opponentIndex][0].HP > 0 {
		return fmt.Errorf("must destroy first guard tower before attacking other towers")
	}

	damage := troop.CalculateDamage(tower)
	tower.HP -= damage

	// Check if tower is destroyed
	if tower.HP <= 0 {
		tower.HP = 0
		// Award EXP to the player
		r.Players[playerIndex].EXP += tower.EXP
	}

	// Check if troop is destroyed by tower counter-attack
	towerDamage := tower.CalculateDamage(troop)
	troop.HP -= towerDamage

	if troop.HP <= 0 {
		// Remove destroyed troop
		r.Troops[playerIndex] = append(r.Troops[playerIndex][:troopIndex], r.Troops[playerIndex][troopIndex+1:]...)
	}

	return nil
}

func (r *GameRoom) UpdateMana() {
	for i := range r.Players {
		player := r.Players[i]
		if time.Since(player.LastManaRegen) >= time.Second {
			if player.Mana < player.MaxMana {
				player.Mana++
			}
			player.LastManaRegen = time.Now()
		}
	}
}

func (r *GameRoom) CheckGameEnd() (bool, string) {
	// Check if king tower is destroyed
	for i, player := range r.Players {
		if r.Towers[i][0].HP <= 0 {
			return true, player.Username
		}
	}

	// Check if time is up (for enhanced mode)
	if r.IsEnhanced && time.Since(r.StartTime) >= 3*time.Minute {
		towers1 := 0
		towers2 := 0
		for i := 0; i < 3; i++ {
			if r.Towers[0][i].HP > 0 {
				towers1++
			}
			if r.Towers[1][i].HP > 0 {
				towers2++
			}
		}
		if towers1 > towers2 {
			return true, r.Players[0].Username
		} else if towers2 > towers1 {
			return true, r.Players[1].Username
		} else {
			return true, "draw"
		}
	}

	return false, ""
}

func (r *GameRoom) getGameState() map[string]interface{} {
	// Create player data
	players := make([]map[string]interface{}, 2)
	for i, player := range r.Players {
		if player != nil {
			players[i] = map[string]interface{}{
				"username": player.Username,
				"level":    player.Level,
				"exp":      player.EXP,
				"mana":     player.Mana,
			}
		}
	}

	// Create tower data
	towers := make([][]map[string]interface{}, 2)
	for i := 0; i < 2; i++ {
		towers[i] = make([]map[string]interface{}, 3)
		for j := 0; j < 3; j++ {
			if r.Towers[i][j] != nil {
				towers[i][j] = map[string]interface{}{
					"type":  r.Towers[i][j].Type,
					"hp":    r.Towers[i][j].HP,
					"atk":   r.Towers[i][j].ATK,
					"def":   r.Towers[i][j].DEF,
					"level": r.Towers[i][j].Level,
				}
			}
		}
	}

	// Create troop data
	troops := make([][]map[string]interface{}, 2)
	for i := 0; i < 2; i++ {
		troops[i] = make([]map[string]interface{}, len(r.Troops[i]))
		for j, troop := range r.Troops[i] {
			if troop != nil {
				troops[i][j] = map[string]interface{}{
					"name":    troop.Name,
					"hp":      troop.HP,
					"atk":     troop.ATK,
					"def":     troop.DEF,
					"mana":    troop.Mana,
					"level":   troop.Level,
					"special": troop.Special,
				}
			}
		}
	}

	return map[string]interface{}{
		"room_id":    r.ID,
		"players":    players,
		"towers":     towers,
		"troops":     troops,
		"start_time": r.StartTime.Format(time.RFC3339),
	}
} 