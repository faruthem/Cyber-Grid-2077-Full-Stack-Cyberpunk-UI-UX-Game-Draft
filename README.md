# Cyber-Grid 2077

> A cyberpunk-themed full-stack web game built with Go and vanilla JavaScript.

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-ff00ff?style=for-the-badge)](LICENSE)
[![Tests](https://img.shields.io/badge/Tests-Passing-00ff00?style=for-the-badge)]()

![Cyberpunk Neon Grid](https://img.shields.io/badge/Theme-Cyberpunk-ff00ff?style=flat-square)
![Grid](https://img.shields.io/badge/Grid-12x12-00ffff?style=flat-square)
![Accessible](https://img.shields.io/badge/WCAG-2.2-AA-ffcc00?style=flat-square)

## Screenshot

![Game Preview](https://img.shields.io/badge/Preview-Coming_Soon-333?style=for-the-badge)

## Features

- **Dynamic 12x12 grid** with hackable nodes, firewalls, and roaming enemies
- **Real-time enemy AI** powered by goroutines — enemies chase the player across the grid
- **Hack mechanic** — breach adjacent nodes to earn credits and level up
- **Neon cyberpunk UI** with animated visual feedback (ripple effects, floating credits, scan lines)
- **First-run tutorial** with pause/resume support
- **WCAG 2.2 AA accessible** — keyboard navigation, screen reader support, reduced-motion mode
- **Graceful shutdown** with context-based goroutine cancellation

## Tech Stack

| Layer | Technology |
|---|---|
| Backend | Go (goroutines, context, net/http) |
| Frontend | Vanilla JS, CSS3 animations, HTML5 |
| Tests | Go standard library testing, race detector |
| Styling | CSS custom properties, grid/flexbox, `@keyframes` |

## Installation

### Prerequisites

| Requirement | Version |
|---|---|
| [Go](https://go.dev/dl/) | 1.22+ |
| [Git](https://git-scm.com/downloads) | 2.40+ |
| Any modern browser | Chrome, Firefox, Edge, Safari |

### Step 1 — Clone the repository

```bash
git clone https://github.com/faruthem/Cyber-Grid-2077-Full-Stack-Cyberpunk-UI-UX-Game-Draft.git
cd Cyber-Grid-2077-Full-Stack-Cyberpunk-UI-UX-Game-Draft
```

### Step 2 — Build the binary

```bash
go build -o cyber-grid-2077 .
```

### Step 3 — Run the server

```bash
./cyber-grid-2077
```

The server starts on **`http://localhost:8080`**. Open it in your browser to play.

### Step 4 — Run tests (optional)

```bash
go test -race -count=1 ./...
```

## How to Play

### Controls

| Key | Action |
|---|---|
| `W` / `↑` | Move up |
| `A` / `←` | Move left |
| `S` / `↓` | Move down |
| `D` / `→` | Move right |
| `H` / `Space` | Hack adjacent nodes |
| `R` | Restart after game over |
| `Escape` | Open/close tutorial |

### Objective

Navigate the grid and **hack yellow nodes** to earn credits. Avoid fuchsia **enemies** that chase you. If an enemy catches you, the game is over.

### Node Types

| Node | Color | Description |
|---|---|---|
| <span style="color:#00ffff">`◆`</span> | Cyan | Player cursor |
| <span style="color:#ff00ff">`◆`</span> | Fuchsia | Enemy — don't touch |
| <span style="color:#fff200">`●`</span> | Yellow | Hackable node — earn credits |
| <span style="color:#ff3333">`█`</span> | Red | Firewall — blocks movement |
| <span style="color:#00ff00">`✓`</span> | Green | Hacked node |

### Scoring

- Hack a node → earn **2x its data value** in credits
- Reach the score threshold → **level up**
- Game ends when an enemy reaches your position

## Project Structure

```
├── main.go          # Game server, grid logic, HTTP handlers, enemy AI
├── main_test.go     # Table-driven tests with race detection
├── index.html       # Complete frontend (HTML + CSS + JS)
├── go.mod           # Go module definition
└── .gitignore       # Git ignore rules
```

## API Endpoints

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/` | Serve the game UI |
| `GET` | `/status` | Get current game state (JSON) |
| `POST` | `/move` | Send a move command (JSON body) |
| `POST` | `/restart` | Reset the game |

### Move Payload

```json
{ "dir": "up" }
```

Valid directions: `up`, `down`, `left`, `right`, `hack`

### Status Response

```json
{
  "player": { "X": 6, "Y": 6 },
  "enemies": [
    { "X": 2, "Y": 8 },
    { "X": 10, "Y": 3 }
  ],
  "nodes": [...],
  "score": 150,
  "level": 1,
  "gameOver": false,
  "log": [...]
}
```

## Accessibility

This project follows **WCAG 2.2 AA** guidelines:

- Full keyboard navigation (no mouse required)
- `aria-live` regions for system log updates
- Focus-visible outlines on all interactive elements
- Skip-to-grid link for screen reader users
- `prefers-reduced-motion` media query for animation control
- Semantic HTML with ARIA roles (`grid`, `dialog`, `button`)

## License

MIT — see [LICENSE](LICENSE) for details.

## Author

**Farithem** — [GitHub](https://github.com/faruthem)

---

*"The grid is your playground. Hack everything. Trust nothing."*
