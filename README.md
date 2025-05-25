# Clash Royale TCR (Text-Based Clash Royale)

A text-based implementation of Clash Royale built with Go, featuring TCP networking, real-time gameplay, and a comprehensive leveling system.

## 🎮 Game Features

### Game Modes
- **Simple TCR**: Turn-based strategy gameplay
- **Enhanced TCR**: Real-time battles with mana management

### Core Systems
- ⚔️ Combat system with damage formula: `DMG = ATK_A - DEF_B`
- 🏰 Tower defense mechanics (King Tower + Guard Towers)
- 👥 6 unique troop types with special abilities
- 📈 Leveling system with 10% stat scaling per level
- 💎 Mana management system (Enhanced mode)
- 🎯 Critical hit mechanics
- 🏆 EXP and trophy progression
- 💾 JSON-based data persistence

## 🏗️ Architecture

```
.
├── cmd/
│   ├── client/          # Client application entry point
│   └── server/          # Server application entry point
├── internal/
│   ├── client/          # Client-side logic
│   ├── server/          # Server-side logic
│   ├── game/            # Core game engine
│   └── network/         # Network protocol definitions
├── pkg/
│   └── logger/          # Logging system
├── data/                # JSON data files
│   ├── troops.json      # Troop specifications
│   ├── towers.json      # Tower specifications
│   └── players.json     # Player database
└── bin/                 # Compiled binaries
```

## 🚀 Quick Start

### Prerequisites
- Go 1.21 or higher
- Make (optional, for convenience)

### Installation

1. **Clone the repository**
   ```bash
   git clone <repository-url>
   cd clash-royale-tcr
   ```

2. **Setup development environment**
   ```bash
   make dev-setup
   ```

3. **Build the project**
   ```bash
   make build
   ```

### Running the Client (Client-Only Mode)

Since this implementation focuses on the client side, you can run the client in demo mode:

```bash
# Run client with default settings
make run-client

# Run with debug logging
make run-client-debug

# Run with custom server address
SERVER_ADDR="localhost:8080" make run-client-remote
```

### Manual Build and Run

```bash
# Install dependencies
go mod tidy
go get github.com/fatih/color@v1.16.0

# Build client
go build -o bin/tcr-client cmd/client/main.go

# Create data files
mkdir -p data
# Copy JSON files from artifacts above

# Run client
./bin/tcr-client
```

## 🎯 Game Mechanics

### Troops

| Name   | HP  | ATK | DEF | MANA | Special |
|--------|-----|-----|-----|------|---------|
| Pawn   | 50  | 150 | 100 | 3    | -       |
| Bishop | 100 | 200 | 150 | 4    | -       |
| Rook   | 250 | 200 | 200 | 5    | -       |
| Knight | 200 | 300 | 150 | 5    | -       |
| Prince | 500 | 400 | 300 | 6    | -       |
| Queen  | -   | -   | -   | 5    | Heals lowest HP tower by 300 |

### Towers

| Type        | HP   | ATK | DEF | CRIT |
|-------------|------|-----|-----|------|
| King Tower  | 2000 | 500 | 300 | 10%  |
| Guard Tower | 1000 | 300 | 100 | 5%   |

### Combat Rules

1. **Damage Formula**: `Damage = Attacker_ATK - Target_DEF` (minimum 0)
2. **Critical Hits**: Enhanced mode adds 20% damage on crits
3. **Targeting Rules** (Simple TCR): Must destroy Guard Towers before King Tower
4. **Win Conditions**:
   - Destroy opponent's King Tower
   - Destroy more towers when time expires (Enhanced mode)

### Mana System (Enhanced Mode)

- Starting mana: 5
- Maximum mana: 10
- Regeneration: 1 mana per second
- Used to summon troops

### Leveling System

- **Stat Scaling**: +10% per level for troops and towers
- **EXP Requirements**: Increase by 10% each level
- **EXP Rewards**: 30 for win, 10 for draw

## 🎮 Controls

### Main Menu
- `1-4`: Navigate menu options
- Enter numbers to select

### Gameplay
- `play`: Summon a troop
- `info`: Show detailed game information
- `end`: End turn (Simple mode only)
- `surrender`: Forfeit the match

### Authentication
- Register new account or login
- Username: 3-20 characters (alphanumeric + underscore)
- Password: Minimum 4 characters

## 🎨 Visual Features

The client features colored terminal output:
- 🔵 **Server messages**: Cyan
- 🟢 **Connection events**: Green
- 🟡 **Game events**: Yellow
- 🔴 **Attacks**: Red
- ⚡ **Critical hits**: Red with yellow background
- 💙 **Healing**: Blue
- 🏆 **Victory**: Green background
- 💀 **Defeat**: Red background

## 🔧 Command Line Options

```bash
./tcr-client [OPTIONS]

Options:
  -server string      Server address (default "localhost:8080")
  -log-level string   Log level: DEBUG, INFO, WARN, ERROR (default "INFO")
  -log-file string    Custom log file path
  -help              Show help information
  -version           Show version information
```

### Examples

```bash
# Connect to specific server
./tcr-client -server "192.168.1.100:8080"

# Debug mode
./tcr-client -log-level DEBUG

# Custom log file
./tcr-client -log-file ./my-game.log

# Remote server with debug
./tcr-client -server "game.example.com:8080" -log-level DEBUG
```

## 📁 Data Persistence

Player data is stored in JSON format:

- **`data/players.json`**: Player accounts, levels, stats
- **`data/troops.json`**: Troop specifications and balance
- **`data/towers.json`**: Tower specifications

All data is automatically created on first run.

## 🛠️ Development

### Available Make Commands

```bash
make help           # Show all available commands
make dev            # Full development setup
make build          # Build client and server
make client         # Build client only
make test           # Run tests
make clean          # Clean build artifacts
make fmt            # Format code
make release        # Create release package
```

### Code Structure

- **Clean Architecture**: Separation of concerns with internal packages
- **Dependency Injection**: Configurable components
- **Error Handling**: Comprehensive error handling throughout
- **Logging**: Structured logging with levels and file output
- **Networking**: TCP-based client-server communication
- **Concurrency**: Safe concurrent operations with mutexes

## 🎓 Educational Value

This project demonstrates:

- **Network Programming**: TCP client-server architecture
- **Protocol Design**: Custom message protocol with JSON
- **Game Engine**: Turn-based and real-time game mechanics
- **Data Persistence**: JSON-based storage and retrieval
- **User Interface**: Terminal-based UI with colors
- **Error Handling**: Robust error handling patterns
- **Concurrency**: Safe concurrent programming
- **Testing**: Unit testing and integration testing patterns

## 📝 Assignment Requirements

This implementation fulfills all project requirements:

- ✅ **Simple TCR**: Turn-based gameplay with targeting rules
- ✅ **Enhanced TCR**: Real-time gameplay with mana system
- ✅ **Critical Damage**: 20% damage bonus implementation
- ✅ **Mana System**: Starting at 5, max 10, 1/sec regeneration
- ✅ **Leveling System**: 10% stat scaling, EXP progression
- ✅ **JSON Storage**: Player data and specifications
- ✅ **TCP Networking**: Client-server communication
- ✅ **Authentication**: Username/password system

## 🐛 Troubleshooting

### Common Issues

1. **Connection Failed**
   ```bash
   # Check if server is running and address is correct
   ./tcr-client -server "localhost:8080"
   ```

2. **Permission Denied (Logs)**
   ```bash
   # Create logs directory manually
   mkdir -p logs
   chmod 755 logs
   ```

3. **JSON Parse Error**
   ```bash
   # Reset data files
   make setup-data
   ```

4. **Build Errors**
   ```bash
   # Clean and rebuild
   make clean
   make deps
   make build
   ```

## 📈 Future Enhancements

- WebSocket support for lower latency
- Replay system for match analysis
- Spectator mode
- Tournament brackets
- AI opponents for single-player
- Mobile client support
- Web-based admin interface

## 📄 License

This project is created for educational purposes as part of the Network Programming course (IT096IU).

## 👥 Contributing

This is an academic project. For improvements or bug fixes, please follow standard Go conventions and include tests for new features.

---

**Built with ❤️ in Go for IT096IU Network Programming Course**