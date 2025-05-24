package main

import (
	"math/rand"
	"time"
)

type Player struct {
	Username string
	Password string
	EXP      int
	Level    int
	Mana     int
	MaxMana  int
	LastManaRegen time.Time
}

type Tower struct {
	Type     string
	HP       int
	ATK      int
	DEF      int
	CRIT     float64
	EXP      int
	Level    int
}

type Troop struct {
	Name     string
	HP       int
	ATK      int
	DEF      int
	Mana     int
	EXP      int
	Special  string
	Level    int
	CRIT     float64
}

type GameRoom struct {
	ID        string
	Players   [2]*Player
	Towers    [2][3]*Tower  // [player][tower]
	Troops    [2][]*Troop   // [player][troops]
	StartTime time.Time
	IsEnhanced bool
}

type GameState struct {
	RoomID    string      `json:"room_id"`
	Players   [2]*Player  `json:"players"`
	Towers    [2][3]*Tower `json:"towers"`
	Troops    [2][]*Troop  `json:"troops"`
	StartTime string      `json:"start_time"`
}

func NewTower(towerType string) *Tower {
	switch towerType {
	case "king":
		return &Tower{
			Type:  "King Tower",
			HP:    2000,
			ATK:   500,
			DEF:   300,
			CRIT:  0.10,
			EXP:   200,
			Level: 1,
		}
	case "guard":
		return &Tower{
			Type:  "Guard Tower",
			HP:    1000,
			ATK:   300,
			DEF:   100,
			CRIT:  0.05,
			EXP:   100,
			Level: 1,
		}
	default:
		return nil
	}
}

func NewTroop(troopType string) *Troop {
	switch troopType {
	case "pawn":
		return &Troop{
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
		return &Troop{
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
		return &Troop{
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
		return &Troop{
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
		return &Troop{
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
		return &Troop{
			Name:    "Queen",
			HP:      0,
			ATK:     0,
			DEF:     0,
			Mana:    5,
			EXP:     30,
			Special: "Heals the friendly tower with lowest HP by 300",
			Level:   1,
			CRIT:    0.30,
		}
	default:
		return nil
	}
}

func (t *Tower) CalculateDamage(attacker *Troop) int {
	baseDamage := attacker.ATK - t.DEF
	if baseDamage < 0 {
		baseDamage = 0
	}

	// Apply level scaling
	levelMultiplier := 1.0 + float64(attacker.Level-1)*0.1
	baseDamage = int(float64(baseDamage) * levelMultiplier)

	// Apply crit chance
	if rand.Float64() < attacker.CRIT {
		baseDamage = int(float64(baseDamage) * 1.2)
	}

	return baseDamage
}

func (t *Troop) CalculateDamage(defender *Tower) int {
	baseDamage := t.ATK - defender.DEF
	if baseDamage < 0 {
		baseDamage = 0
	}

	// Apply level scaling
	levelMultiplier := 1.0 + float64(t.Level-1)*0.1
	baseDamage = int(float64(baseDamage) * levelMultiplier)

	// Apply crit chance
	if rand.Float64() < t.CRIT {
		baseDamage = int(float64(baseDamage) * 1.2)
	}

	return baseDamage
} 