package game

import "time"

type TroopType string

// TroopType represents the type and name of troop
const (
	Pawn   TroopType = "Pawn"
	Bishop TroopType = "Bishop"
	Rook   TroopType = "Rook"
	Knight TroopType = "Knight"
	Prince TroopType = "Prince"
	Queen  TroopType = "Queen"
)

type TowerType string

// TowerType represents the type and name of tower
const (
	KingTower  TowerType = "King Tower"
	GuardTower TowerType = "Guard Tower"
)

type Troop struct {
	Name    TroopType `json:"name"`
	HP      int       `json:"hp"`
	ATK     int       `json:"atk"`
	DEF     int       `json:"def"`
	CRIT    float64   `json:"crit"` // Crit chance as percentage (E.g : 10% = 0.10)
	MANA    int       `json:"mana"`
	EXP     int       `json:"exp"`
	Special string    `json:"special,omitempty"`
	Level   int       `json:"level"`
}

type Tower struct {
	Name     TowerType `json:"name"`
	HP       int       `json:"hp"`
	MaxHP    int       `json:"max_hp"`
	ATK      int       `json:"atk"`
	DEF      int       `json:"def"`
	CRIT     float64   `json:"crit"` // Crit chance as percentage (E.g : 10% = 0.10)
	EXP      int       `json:"exp"`
	Level    int       `json:"level"`
	IsActive bool      `json:"is_active"` // For targeting rules
}

type Player struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Password string `json:"password,omitempty"` // Omit in network messages
	Level    int    `json:"level"`
	EXP      int    `json:"exp"`
	// Trophies int     `json:"trophies"`
	Mana                   int     `json:"mana"`
	MaxMana                int     `json:"max_mana"`
	Troops                 []Troop `json:"troops"` // 3 random troops for this match
	Towers                 []Tower `json:"towers"` // 3 towers: 1 King + 2 Guard
	TroopsDeployedThisTurn int     `json:"troops_deployed_this_turn"`
}

type GameState struct {
	ID           string    `json:"id"`
	GameMode     string    `json:"game_mode"` // "simple" or "enhanced"
	Status       string    `json:"status"`    // "waiting", "active", "finished"
	Player1      Player    `json:"player1"`
	Player2      Player    `json:"player2"`
	CurrentTurn  string    `json:"current_turn"` // Player ID (for Simple TCR)
	TimeLeft     int       `json:"time_left"`    // Seconds remaining (for Enhanced TCR)
	StartTime    time.Time `json:"start_time"`
	Winner       string    `json:"winner,omitempty"`
	TowersKilled struct {
		Player1 int `json:"player1"`
		Player2 int `json:"player2"`
	} `json:"towers_killed"`
}

type CombatAction struct {
	Type       string                 `json:"type"` // "attack", "summon", "heal"
	PlayerID   string                 `json:"player_id"`
	TroopName  TroopType              `json:"troop_name,omitempty"`
	TargetType string                 `json:"target_type"` // "tower"
	TargetName string                 `json:"target_name"`
	Damage     int                    `json:"damage,omitempty"`
	IsCrit     bool                   `json:"is_crit,omitempty"`
	HealAmount int                    `json:"heal_amount,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
	Data       map[string]interface{} `json:"data,omitempty"`
}

// GameSpecs holds all game specifications loaded from JSON
type GameSpecs struct {
	TroopSpecs map[TroopType]TroopSpec `json:"troops"`
	TowerSpecs map[TowerType]TowerSpec `json:"towers"`
}

// TroopSpec defines base specifications for each troop type
type TroopSpec struct {
	HP      int     `json:"hp"`
	ATK     int     `json:"atk"`
	DEF     int     `json:"def"`
	CRIT    float64 `json:"crit"`
	MANA    int     `json:"mana"`
	EXP     int     `json:"exp"`
	Special string  `json:"special,omitempty"`
}

// TowerSpec defines base specifications for each tower type
type TowerSpec struct {
	HP   int     `json:"hp"`
	ATK  int     `json:"atk"`
	DEF  int     `json:"def"`
	CRIT float64 `json:"crit"`
	EXP  int     `json:"exp"`
}

// PlayerData represents persistent player data
type PlayerData struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Level    int    `json:"level"`
	EXP      int    `json:"exp"`
	// Trophies    int               `json:"trophies"`
	TroopLevels map[TroopType]int `json:"troop_levels"`
	TowerLevels map[TowerType]int `json:"tower_levels"`
	GamesPlayed int               `json:"games_played"`
	GamesWon    int               `json:"games_won"`
	LastLogin   time.Time         `json:"last_login"`
}

// Game constants
const (
	// Enhanced TCR constants
	GameDurationSeconds = 180 // 3 minutes
	StartingMana        = 5
	MaxMana             = 10
	ManaRegenPerSecond  = 1

	// EXP rewards
	WinEXP  = 30
	DrawEXP = 10

	// Scaling factors
	StatScalePerLevel = 0.10 // 10% increase per level
	EXPScalePerLevel  = 0.10 // 10% increase in required EXP per level

	// Targeting rules for Simple TCR
	// Must destroy Guard Towers before attacking King Tower

	// Number of troops and towers
	TroopsPerPlayer = 3
	TowersPerPlayer = 3 // 1 King + 2 Guard
)

// ActionType constants
const (
	ActionSummon    = "summon"
	ActionAttack    = "attack"
	ActionHeal      = "heal"
	ActionEndTurn   = "end_turn"
	ActionSurrender = "surrender"
)

// GameStatus constants
const (
	StatusWaiting  = "waiting"
	StatusActive   = "active"
	StatusFinished = "finished"
)

// GameMode constants
const (
	ModeSimple   = "simple"
	ModeEnhanced = "enhanced"
)
