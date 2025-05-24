# Text-Based Clash Royale (TCR) Server

A text-based implementation of Clash Royale game mechanics using Go.

## System Architecture

The TCR server is built using a TCP-based client-server architecture with the following components:

1. **Game Server**: Main server component that handles:
   - Player authentication
   - Game room management
   - Game state synchronization
   - Player connections

2. **Game Logic**: Core game mechanics including:
   - Tower and troop management
   - Combat system
   - Mana system
   - Experience and leveling system

3. **Network Layer**: TCP-based communication with:
   - JSON message format
   - Client-server protocol
   - State synchronization

## Application PDU Description

### Client Messages
```json
{
    "type": "string",
    "data": {
        // Message-specific data
    }
}
```

Message Types:
- `login`: Player authentication
- `create_room`: Create a new game room
- `join_room`: Join an existing game room
- `game_action`: Perform a game action
- `get_state`: Request current game state

### Server Messages
```json
{
    "type": "string",
    "success": boolean,
    "data": {
        // Response data
    },
    "error": "string"
}
```

Message Types:
- `login`: Authentication response
- `room_created`: Room creation confirmation
- `room_joined`: Room join confirmation
- `game_state`: Current game state
- `error`: Error message

## Sequence Diagram

```
Client A                    Server                    Client B
   |                          |                          |
   |-- login ---------------->|                          |
   |<-- login_success -------|                          |
   |                          |                          |
   |-- create_room ---------->|                          |
   |<-- room_created --------|                          |
   |                          |                          |
   |                          |<-- login ----------------|
   |                          |-- login_success -------->|
   |                          |                          |
   |                          |<-- join_room ------------|
   |                          |-- room_joined ---------->|
   |                          |                          |
   |<-- game_state ----------|                          |
   |                          |-- game_state ----------->|
   |                          |                          |
   |-- game_action ---------->|                          |
   |                          |-- game_state ----------->|
   |<-- game_state ----------|                          |
   |                          |                          |
```

## Deployment & Execution Instructions

1. **Prerequisites**
   - Go 1.21 or later
   - Git

2. **Installation**
   ```bash
   git clone https://github.com/yourusername/tcr-server.git
   cd tcr-server
   go mod download
   ```

3. **Running the Server**
   ```bash
   go run .
   ```

4. **Configuration**
   - Server host and port can be configured in `main.go`
   - Default: localhost:8080

5. **Testing**
   - Use a TCP client (e.g., netcat) to connect to the server
   - Send JSON messages following the protocol specification

## Game Rules

### Simple TCR Rules
- Two players connect using username and password
- Each player has 3 towers (1 King Tower, 2 Guard Towers)
- Players must destroy the 1st Guard Tower before attacking others
- Each player has 3 troops, randomly spawned
- Turn-based combat system

### Enhanced TCR Rules
- 3-minute time limit
- Continuous combat (no turns)
- Critical hit system
- Experience and leveling system
- Mana system (5-10 mana, regenerates 1/sec)

## Tower and Troop Specifications

### Towers
| Type        | HP   | ATK | DEF | CRIT | EXP |
|-------------|------|-----|-----|------|-----|
| King Tower  | 2000 | 500 | 300 | 10%  | 200 |
| Guard Tower | 1000 | 300 | 100 | 5%   | 100 |

### Troops
| Name   | HP  | ATK | DEF | Mana | EXP | Special                    |
|--------|-----|-----|-----|------|-----|----------------------------|
| Pawn   | 50  | 150 | 100 | 3    | 5   | -                          |
| Bishop | 100 | 200 | 150 | 4    | 10  | -                          |
| Rook   | 250 | 200 | 200 | 5    | 25  | -                          |
| Knight | 200 | 300 | 150 | 5    | 25  | -                          |
| Prince | 500 | 400 | 300 | 6    | 50  | -                          |
| Queen  | N/A | N/A | N/A | 5    | 30  | Heals lowest HP tower by 300 | 