// Package game handles data persistence and JSON file operations
package game

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

// DataManager handles all data persistence operations
type DataManager struct {
	dataDir     string
	troopsFile  string
	towersFile  string
	playersFile string
	gameSpecs   *GameSpecs
	playerDB    *PlayerDatabase
}

// PlayerDatabase represents the player database structure
type PlayerDatabase struct {
	Players []PlayerData `json:"players"`
}

// NewDataManager creates a new data manager instance
func NewDataManager(dataDir string) *DataManager {
	return &DataManager{
		dataDir:     dataDir,
		troopsFile:  filepath.Join(dataDir, "troops.json"),
		towersFile:  filepath.Join(dataDir, "towers.json"),
		playersFile: filepath.Join(dataDir, "players.json"),
	}
}

// Initialize loads all game data from JSON files
func (dm *DataManager) Initialize() error {
	// Create data directory if it doesn't exist
	if err := os.MkdirAll(dm.dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Load game specifications
	if err := dm.loadGameSpecs(); err != nil {
		return fmt.Errorf("failed to load game specs: %w", err)
	}

	// Load player database
	if err := dm.loadPlayerDatabase(); err != nil {
		return fmt.Errorf("failed to load player database: %w", err)
	}

	return nil
}

// loadGameSpecs loads troop and tower specifications from JSON files
func (dm *DataManager) loadGameSpecs() error {
	// Load troops
	troopSpecs, err := dm.loadTroopSpecs()
	if err != nil {
		return err
	}

	// Load towers
	towerSpecs, err := dm.loadTowerSpecs()
	if err != nil {
		return err
	}

	dm.gameSpecs = &GameSpecs{
		TroopSpecs: troopSpecs,
		TowerSpecs: towerSpecs,
	}

	return nil
}

// loadTroopSpecs loads troop specifications from troops.json
func (dm *DataManager) loadTroopSpecs() (map[TroopType]TroopSpec, error) {
	data, err := ioutil.ReadFile(dm.troopsFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read troops file: %w", err)
	}

	var troopData struct {
		Troops map[TroopType]TroopSpec `json:"troops"`
	}

	if err := json.Unmarshal(data, &troopData); err != nil {
		return nil, fmt.Errorf("failed to parse troops JSON: %w", err)
	}

	return troopData.Troops, nil
}

// loadTowerSpecs loads tower specifications from towers.json
func (dm *DataManager) loadTowerSpecs() (map[TowerType]TowerSpec, error) {
	data, err := ioutil.ReadFile(dm.towersFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read towers file: %w", err)
	}

	var towerData struct {
		Towers map[TowerType]TowerSpec `json:"towers"`
	}

	if err := json.Unmarshal(data, &towerData); err != nil {
		return nil, fmt.Errorf("failed to parse towers JSON: %w", err)
	}

	return towerData.Towers, nil
}

// loadPlayerDatabase loads player data from players.json
func (dm *DataManager) loadPlayerDatabase() error {
	// Check if players file exists
	if _, err := os.Stat(dm.playersFile); os.IsNotExist(err) {
		// Create empty player database
		dm.playerDB = &PlayerDatabase{
			Players: make([]PlayerData, 0),
		}
		return dm.savePlayerDatabase()
	}

	data, err := ioutil.ReadFile(dm.playersFile)
	if err != nil {
		return fmt.Errorf("failed to read players file: %w", err)
	}

	dm.playerDB = &PlayerDatabase{}
	if err := json.Unmarshal(data, dm.playerDB); err != nil {
		return fmt.Errorf("failed to parse players JSON: %w", err)
	}

	return nil
}

// savePlayerDatabase saves player database to JSON file
func (dm *DataManager) savePlayerDatabase() error {
	data, err := json.MarshalIndent(dm.playerDB, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal player data: %w", err)
	}

	if err := ioutil.WriteFile(dm.playersFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write players file: %w", err)
	}

	return nil
}

// Authentication methods

// AuthenticatePlayer verifies user credentials
func (dm *DataManager) AuthenticatePlayer(username, password string) (*PlayerData, error) {
	for i := range dm.playerDB.Players {
		player := &dm.playerDB.Players[i]
		if player.Username == username {
			if player.Password == password {
				// Update last login
				player.LastLogin = time.Now()
				dm.savePlayerDatabase()
				return player, nil
			}
			return nil, fmt.Errorf("invalid password")
		}
	}
	return nil, fmt.Errorf("player not found")
}

// RegisterPlayer creates a new player account
func (dm *DataManager) RegisterPlayer(username, password string) (*PlayerData, error) {
	// Check if username already exists
	for _, player := range dm.playerDB.Players {
		if player.Username == username {
			return nil, fmt.Errorf("username already exists")
		}
	}

	// Create new player with default values
	newPlayer := PlayerData{
		Username: username,
		Password: password,
		Level:    1,
		EXP:      0,
		// Trophies:     0,
		TroopLevels: make(map[TroopType]int),
		TowerLevels: make(map[TowerType]int),
		GamesPlayed: 0,
		GamesWon:    0,
		LastLogin:   time.Now(),
	}

	// Initialize troop and tower levels to 1
	for troopType := range dm.gameSpecs.TroopSpecs {
		newPlayer.TroopLevels[troopType] = 1
	}
	for towerType := range dm.gameSpecs.TowerSpecs {
		newPlayer.TowerLevels[towerType] = 1
	}

	// Add to database
	dm.playerDB.Players = append(dm.playerDB.Players, newPlayer)

	if err := dm.savePlayerDatabase(); err != nil {
		return nil, fmt.Errorf("failed to save new player: %w", err)
	}

	return &newPlayer, nil
}

// UpdatePlayerData updates player statistics after a game
func (dm *DataManager) UpdatePlayerData(username string, expGained int, won bool, trophyChange int) error {
	for i := range dm.playerDB.Players {
		player := &dm.playerDB.Players[i]
		if player.Username == username {
			// Update statistics
			player.EXP += expGained
			// player.Trophies += trophyChange
			player.GamesPlayed++
			if won {
				player.GamesWon++
			}

			// Check for level up
			dm.checkLevelUp(player)

			return dm.savePlayerDatabase()
		}
	}
	return fmt.Errorf("player not found")
}

// checkLevelUp checks if player should level up and updates accordingly
func (dm *DataManager) checkLevelUp(player *PlayerData) {
	requiredEXP := dm.calculateRequiredEXP(player.Level)

	for player.EXP >= requiredEXP {
		player.Level++
		player.EXP -= requiredEXP
		requiredEXP = dm.calculateRequiredEXP(player.Level)

		// Level up all troops and towers
		for troopType := range player.TroopLevels {
			player.TroopLevels[troopType]++
		}
		for towerType := range player.TowerLevels {
			player.TowerLevels[towerType]++
		}
	}
}

// calculateRequiredEXP calculates EXP needed for next level
func (dm *DataManager) calculateRequiredEXP(level int) int {
	baseEXP := 100
	return int(float64(baseEXP) * (1.0 + float64(level-1)*EXPScalePerLevel))
}

// Game creation methods

// CreatePlayerForGame creates a Player instance for gameplay from PlayerData
func (dm *DataManager) CreatePlayerForGame(playerData *PlayerData, playerID string) *Player {
	player := &Player{
		ID:       playerID,
		Username: playerData.Username,
		Level:    playerData.Level,
		EXP:      playerData.EXP,
		// Trophies: playerData.Trophies,
		Mana:    StartingMana,
		MaxMana: MaxMana,
		Troops:  dm.generateRandomTroops(playerData),
		Towers:  dm.generateTowers(playerData),
	}

	return player
}

// generateRandomTroops generates 3 random troops for a player
func (dm *DataManager) generateRandomTroops(playerData *PlayerData) []Troop {
	troopTypes := make([]TroopType, 0, len(dm.gameSpecs.TroopSpecs))
	for troopType := range dm.gameSpecs.TroopSpecs {
		troopTypes = append(troopTypes, troopType)
	}

	// Shuffle and pick 3 random troops
	rand.Shuffle(len(troopTypes), func(i, j int) {
		troopTypes[i], troopTypes[j] = troopTypes[j], troopTypes[i]
	})

	troops := make([]Troop, TroopsPerPlayer)
	for i := 0; i < TroopsPerPlayer; i++ {
		troopType := troopTypes[i]
		baseSpec := dm.gameSpecs.TroopSpecs[troopType]
		playerLevel := playerData.TroopLevels[troopType]

		troops[i] = Troop{
			Name:    troopType,
			HP:      dm.scaleStatByLevel(baseSpec.HP, playerLevel),
			MaxHP:   dm.scaleStatByLevel(baseSpec.HP, playerLevel),
			ATK:     dm.scaleStatByLevel(baseSpec.ATK, playerLevel),
			DEF:     dm.scaleStatByLevel(baseSpec.DEF, playerLevel),
			CRIT:    baseSpec.CRIT,
			MANA:    baseSpec.MANA,
			EXP:     baseSpec.EXP,
			Special: baseSpec.Special,
			Level:   playerLevel,
		}
	}

	return troops
}

// generateTowers generates towers for a player
func (dm *DataManager) generateTowers(playerData *PlayerData) []Tower {
	towers := make([]Tower, TowersPerPlayer)

	// Create 1 King Tower + 2 Guard Towers
	kingSpec := dm.gameSpecs.TowerSpecs[KingTower]
	guardSpec1 := dm.gameSpecs.TowerSpecs[GuardTower1]
	guardSpec2 := dm.gameSpecs.TowerSpecs[GuardTower2]

	kingLevel := playerData.TowerLevels[KingTower]
	guardLevel1 := playerData.TowerLevels[GuardTower1]
	guardLevel2 := playerData.TowerLevels[GuardTower2]

	// King Tower
	towers[0] = Tower{
		Name:     KingTower,
		HP:       dm.scaleStatByLevel(kingSpec.HP, kingLevel),
		MaxHP:    dm.scaleStatByLevel(kingSpec.HP, kingLevel),
		ATK:      dm.scaleStatByLevel(kingSpec.ATK, kingLevel),
		DEF:      dm.scaleStatByLevel(kingSpec.DEF, kingLevel),
		CRIT:     kingSpec.CRIT,
		Level:    kingLevel,
		IsActive: true,
	}

	// Guard Tower 1
	towers[1] = Tower{
		Name:     GuardTower1,
		HP:       dm.scaleStatByLevel(guardSpec1.HP, guardLevel1),
		MaxHP:    dm.scaleStatByLevel(guardSpec1.HP, guardLevel1),
		ATK:      dm.scaleStatByLevel(guardSpec1.ATK, guardLevel1),
		DEF:      dm.scaleStatByLevel(guardSpec1.DEF, guardLevel1),
		CRIT:     guardSpec1.CRIT,
		Level:    guardLevel1,
		IsActive: true,
	}

	// Guard Tower 2
	towers[2] = Tower{
		Name:     GuardTower2,
		HP:       dm.scaleStatByLevel(guardSpec2.HP, guardLevel2),
		MaxHP:    dm.scaleStatByLevel(guardSpec2.HP, guardLevel2),
		ATK:      dm.scaleStatByLevel(guardSpec2.ATK, guardLevel2),
		DEF:      dm.scaleStatByLevel(guardSpec2.DEF, guardLevel2),
		CRIT:     guardSpec2.CRIT,
		Level:    guardLevel2,
		IsActive: true,
	}

	return towers
}

// scaleStatByLevel applies level scaling to stats (10% per level)
func (dm *DataManager) scaleStatByLevel(baseStat, level int) int {
	if baseStat == 0 { // Handle special troops like Queen
		return 0
	}
	scaleFactor := 1.0 + float64(level-1)*StatScalePerLevel
	return int(float64(baseStat) * scaleFactor)
}

// GetGameSpecs returns the loaded game specifications
func (dm *DataManager) GetGameSpecs() *GameSpecs {
	return dm.gameSpecs
}

// GetPlayerDatabase returns the player database (for server use)
func (dm *DataManager) GetPlayerDatabase() *PlayerDatabase {
	return dm.playerDB
}

// Helper function for game engine
func initializePlayerForGame(player *Player, specs *GameSpecs) {
	// This function is called by GameEngine to ensure player has proper stats
	// The stats are already calculated by DataManager, so this is just validation

	// Ensure mana is set correctly for game mode
	if player.Mana == 0 {
		player.Mana = StartingMana
	}
	if player.MaxMana == 0 {
		player.MaxMana = MaxMana
	}

	for i := range player.Troops {
		player.Troops[i].MaxHP = player.Troops[i].HP
	}
}
