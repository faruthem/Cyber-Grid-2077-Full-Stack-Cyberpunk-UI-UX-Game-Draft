package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const (
	gridSize   = 12
	numEnemies = 5
)

var (
	ErrInvalidDirection = errors.New("invalid direction")
	ErrGameOver         = errors.New("game over")
	ErrOutOfBounds      = errors.New("move out of bounds")
	ErrFirewallBlock    = errors.New("firewall blocked")
)

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

type NodeType int

const (
	NodeEmpty NodeType = iota
	NodePlayer
	NodeEnemy
	NodeHackable
	NodeFirewall
)

type Node struct {
	Type          NodeType `json:"type"`
	X             int      `json:"x"`
	Y             int      `json:"y"`
	Health        int      `json:"health"`
	Hacked        bool     `json:"hacked"`
	Data          int      `json:"data"`
	Vulnerable    bool     `json:"vulnerable"`
	VulnerableHits int    `json:"vulnerableHits"`
}

type Position struct {
	X, Y int
}

type LogEntry struct {
	Time    string `json:"time"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

type GameGrid struct {
	mu                  sync.RWMutex
	Nodes               [][]Node
	Player              Position
	Enemies             []Position
	Log                 []LogEntry
	Score               int
	Level               int
	GameOver            bool
	cancel              context.CancelFunc
	FrozenUntil         time.Time
	FreezeCooldownUntil time.Time
	ShieldUntil         time.Time
	FirstFirewallDestroyed bool
}

type StatusResponse struct {
	Nodes        [][]Node   `json:"nodes"`
	Player       Position   `json:"player"`
	Enemies      []Position `json:"enemies"`
	Log          []LogEntry `json:"log"`
	Score        int        `json:"score"`
	Level        int        `json:"level"`
	GameOver     bool       `json:"gameOver"`
	EnemiesFrozen bool      `json:"enemiesFrozen"`
	FreezeReady  bool       `json:"freezeReady"`
	HasShield    bool       `json:"hasShield"`
}

type Server struct {
	grid *GameGrid
	mu   sync.RWMutex
	rng  *rand.Rand
}

func NewServer() *Server {
	return &Server{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (s *Server) createGrid() *GameGrid {
	g := &GameGrid{
		Nodes:  make([][]Node, gridSize),
		Player: Position{gridSize / 2, gridSize / 2},
		Level:  1,
		Log:    make([]LogEntry, 0, 50),
	}

	for i := range g.Nodes {
		g.Nodes[i] = make([]Node, gridSize)
		for j := range g.Nodes[i] {
			g.Nodes[i][j] = Node{
				Type:   NodeEmpty,
				X:      i,
				Y:      j,
				Health: 100,
				Data:   s.rng.Intn(50) + 10,
			}
			r := s.rng.Float64()
			if r < 0.15 {
				g.Nodes[i][j].Type = NodeHackable
			} else if r < 0.20 {
				g.Nodes[i][j].Type = NodeFirewall
				g.Nodes[i][j].Health = 200
			}
		}
	}

	g.Nodes[g.Player.X][g.Player.Y].Type = NodePlayer

	g.Enemies = make([]Position, 0, numEnemies)
	for len(g.Enemies) < numEnemies {
		x := s.rng.Intn(gridSize)
		y := s.rng.Intn(gridSize)
		dist := abs(x-g.Player.X) + abs(y-g.Player.Y)
		if dist < 4 {
			continue
		}
		if g.Nodes[x][y].Type == NodeFirewall {
			continue
		}
		duplicate := false
		for _, e := range g.Enemies {
			if e.X == x && e.Y == y {
				duplicate = true
				break
			}
		}
		if !duplicate {
			g.Enemies = append(g.Enemies, Position{x, y})
			g.Nodes[x][y].Type = NodeEnemy
		}
	}

	g.addLog("SYSTEM INITIALIZED", "info")
	g.addLog("GRID ONLINE - ALL NODES ACTIVE", "info")
	g.addLog(fmt.Sprintf("WARNING: %d TRACKERS DETECTED", numEnemies), "warning")

	return g
}

func (g *GameGrid) addLog(msg, logType string) {
	entry := LogEntry{
		Time:    time.Now().Format("15:04:05"),
		Message: msg,
		Type:    logType,
	}
	g.Log = append(g.Log, entry)
	if len(g.Log) > 50 {
		g.Log = g.Log[len(g.Log)-50:]
	}
}

func (s *Server) moveEnemies(ctx context.Context) {
	ticker := time.NewTicker(1200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			g := s.grid
			if g == nil || g.GameOver {
				s.mu.Unlock()
				continue
			}

			if time.Now().Before(g.FrozenUntil) {
				s.mu.Unlock()
				continue
			}
			if !g.FrozenUntil.IsZero() {
				g.addLog("TRACKERS UNFROZEN - MOVEMENT RESUMED", "warning")
				g.FrozenUntil = time.Time{}
			}

			for i := range g.Enemies {
				oldPos := g.Enemies[i]
				if g.Nodes[oldPos.X][oldPos.Y].Type != NodePlayer {
					g.Nodes[oldPos.X][oldPos.Y].Type = NodeHackable
				}

				moves := []Position{
					{oldPos.X - 1, oldPos.Y},
					{oldPos.X + 1, oldPos.Y},
					{oldPos.X, oldPos.Y - 1},
					{oldPos.X, oldPos.Y + 1},
				}

				validMoves := make([]Position, 0, 4)
				for _, m := range moves {
					if m.X >= 0 && m.X < gridSize && m.Y >= 0 && m.Y < gridSize {
						if g.Nodes[m.X][m.Y].Type != NodeFirewall {
							isEnemy := false
							for _, e := range g.Enemies {
								if e.X == m.X && e.Y == m.Y {
									isEnemy = true
									break
								}
							}
							if !isEnemy {
								validMoves = append(validMoves, m)
							}
						}
					}
				}

				if len(validMoves) > 0 {
					newPos := validMoves[s.rng.Intn(len(validMoves))]

					dx := g.Player.X - oldPos.X
					dy := g.Player.Y - oldPos.Y

					if s.rng.Float64() < 0.4 {
						for _, m := range validMoves {
							ndX := g.Player.X - m.X
							ndY := g.Player.Y - m.Y
							if (ndX*ndX + ndY*ndY) < (dx*dx + dy*dy) {
								newPos = m
								break
							}
						}
					}

					g.Enemies[i] = newPos

					if newPos.X == g.Player.X && newPos.Y == g.Player.Y {
						if time.Now().Before(g.ShieldUntil) {
							g.addLog("DATA SHIELD ABSORBED TRACKER ATTACK", "success")
							g.ShieldUntil = time.Time{}
							g.Nodes[oldPos.X][oldPos.Y].Type = NodeEmpty
							g.Enemies[i] = oldPos
							g.Nodes[oldPos.X][oldPos.Y].Type = NodeEnemy
							continue
						}
						g.GameOver = true
						g.addLog("BREACH DETECTED - PLAYER COMPROMISED", "danger")
						g.addLog("CONNECTION TERMINATED", "danger")
						g.Nodes[newPos.X][newPos.Y].Type = NodeEnemy
						s.mu.Unlock()
						return
					}

					g.Nodes[newPos.X][newPos.Y].Type = NodeEnemy
				} else {
					g.Nodes[oldPos.X][oldPos.Y].Type = NodeEnemy
				}
			}

			if s.rng.Float64() < 0.15 {
				g.addLog("TRACKER MOVING...", "warning")
			}
			if s.rng.Float64() < 0.1 {
				g.addLog("SCANNING NETWORK SEGMENTS", "info")
			}

			s.mu.Unlock()
		}
	}
}

func (s *Server) spawnHackableNodes(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			g := s.grid
			if g == nil || g.GameOver {
				s.mu.Unlock()
				continue
			}

			for i := 0; i < 2; i++ {
				x := s.rng.Intn(gridSize)
				y := s.rng.Intn(gridSize)
				if g.Nodes[x][y].Type == NodeEmpty {
					g.Nodes[x][y].Type = NodeHackable
					g.Nodes[x][y].Data = s.rng.Intn(80) + 20
					g.Nodes[x][y].Hacked = false
				}
			}

			g.addLog("NEW DATA NODES DETECTED", "info")
			s.mu.Unlock()
		}
	}
}

var firewallDirs = []Position{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}

func (s *Server) checkFirewallSurrounded(g *GameGrid, fx, fy int, hackRequirement int) bool {
	hackedCount := 0
	for _, d := range firewallDirs {
		nx, ny := fx+d.X, fy+d.Y
		if nx >= 0 && nx < gridSize && ny >= 0 && ny < gridSize {
			node := &g.Nodes[nx][ny]
			if node.Hacked || node.Type == NodeEmpty {
				hackedCount++
			}
		}
	}
	return hackedCount >= hackRequirement
}

func (s *Server) markVulnerableFirewalls(g *GameGrid) {
	for i := 0; i < gridSize; i++ {
		for j := 0; j < gridSize; j++ {
			node := &g.Nodes[i][j]
			if node.Type == NodeFirewall && !node.Vulnerable {
				hackableAdj := 0
				hackedCount := 0
				for _, d := range firewallDirs {
					nx, ny := i+d.X, j+d.Y
					if nx >= 0 && nx < gridSize && ny >= 0 && ny < gridSize {
						adj := &g.Nodes[nx][ny]
						if adj.Type == NodeFirewall || adj.Type == NodeEnemy || adj.Type == NodePlayer {
							continue
						}
						if adj.Type == NodeHackable {
							hackableAdj++
							if adj.Hacked {
								hackedCount++
							}
						} else if adj.Hacked {
							hackableAdj++
							hackedCount++
						}
					}
				}
				if hackableAdj > 0 && hackedCount >= hackableAdj {
					node.Vulnerable = true
					g.addLog(fmt.Sprintf("FIREWALL %d,%d CRITICAL - BREACH IMMINENT", i, j), "warning")
				}
			}
		}
	}
}

func (s *Server) destroyFirewall(g *GameGrid, fx, fy int) {
	g.Nodes[fx][fy].Type = NodeEmpty
	g.Nodes[fx][fy].Hacked = true
	g.Nodes[fx][fy].Vulnerable = false
	g.Score += 50
	g.addLog(fmt.Sprintf("FIREWALL %d,%d BREACHED! +50 CREDITS", fx, fy), "success")

	g.FrozenUntil = time.Now().Add(5 * time.Second)
	g.addLog("EMP PULSE - ALL TRACKERS FROZEN 5s", "success")

	if !g.FirstFirewallDestroyed {
		g.FirstFirewallDestroyed = true
		g.ShieldUntil = time.Now().Add(3 * time.Second)
		g.addLog("DATA SHIELD ACTIVATED - 3s PROTECTION", "info")
	}

	s.applyFirewallChainReaction(g, fx, fy)
	s.markVulnerableFirewalls(g)
}

func (s *Server) applyFirewallChainReaction(g *GameGrid, fx, fy int) {
	for _, d := range firewallDirs {
		nx, ny := fx+d.X, fy+d.Y
		if nx >= 0 && nx < gridSize && ny >= 0 && ny < gridSize {
			node := &g.Nodes[nx][ny]
			if node.Type == NodeFirewall && !node.Vulnerable {
				node.VulnerableHits++
				if node.VulnerableHits >= 1 {
					node.Vulnerable = true
					g.addLog(fmt.Sprintf("FIREWALL %d,%d WEAKENED BY CHAIN REACTION", nx, ny), "warning")
				}
			}
		}
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	g := s.grid
	if g == nil {
		http.Error(w, "Game not initialized", http.StatusServiceUnavailable)
		return
	}

	resp := StatusResponse{
		Nodes:         g.Nodes,
		Player:        g.Player,
		Enemies:       g.Enemies,
		Log:           g.Log,
		Score:         g.Score,
		Level:         g.Level,
		GameOver:      g.GameOver,
		EnemiesFrozen: time.Now().Before(g.FrozenUntil),
		FreezeReady:   time.Now().After(g.FreezeCooldownUntil),
		HasShield:     time.Now().Before(g.ShieldUntil),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("error encoding status response: %v", err)
	}
}

func (s *Server) handleMove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var moveReq struct {
		Dir string `json:"dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&moveReq); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	g := s.grid
	if g == nil {
		http.Error(w, "Game not initialized", http.StatusServiceUnavailable)
		return
	}

	if g.GameOver {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":  false,
			"gameOver": true,
		})
		return
	}

	if moveReq.Dir == "hack" {
		s.hackAdjacent(g)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"score":   g.Score,
			"log":     g.Log[len(g.Log)-1],
		})
		return
	}

	if moveReq.Dir == "freeze" {
		cooldownExpired := time.Now().After(g.FreezeCooldownUntil)
		if !cooldownExpired {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "freeze on cooldown",
			})
			return
		}
		g.FrozenUntil = time.Now().Add(4 * time.Second)
		g.FreezeCooldownUntil = time.Now().Add(15 * time.Second)
		g.addLog("CRYO-PROTOCOL ACTIVATED - TRACKERS FROZEN", "success")
		g.addLog("FREEZE DURATION: 4s | COOLDOWN: 15s", "info")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"score":   g.Score,
			"log":     g.Log[len(g.Log)-1],
		})
		return
	}

	dx, dy := 0, 0
	switch moveReq.Dir {
	case "up":
		dx = -1
	case "down":
		dx = 1
	case "left":
		dy = -1
	case "right":
		dy = 1
	default:
		http.Error(w, ErrInvalidDirection.Error(), http.StatusBadRequest)
		return
	}

	newX := g.Player.X + dx
	newY := g.Player.Y + dy

	if newX < 0 || newX >= gridSize || newY < 0 || newY >= gridSize {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   ErrOutOfBounds.Error(),
		})
		return
	}

	target := &g.Nodes[newX][newY]
	if target.Type == NodeFirewall && !target.Vulnerable {
		g.addLog("FIREWALL BLOCKED - ACCESS DENIED", "danger")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   ErrFirewallBlock.Error(),
		})
		return
	}

	g.Nodes[g.Player.X][g.Player.Y].Type = NodeEmpty
	g.Player.X = newX
	g.Player.Y = newY

	if target.Type == NodeFirewall && target.Vulnerable {
		s.destroyFirewall(g, newX, newY)
	}

	for _, e := range g.Enemies {
		if e.X == g.Player.X && e.Y == g.Player.Y {
			if time.Now().Before(g.ShieldUntil) {
				g.addLog("DATA SHIELD ABSORBED TRACKER ATTACK", "success")
				g.ShieldUntil = time.Time{}
			} else {
				g.GameOver = true
				g.addLog("BREACH DETECTED - PLAYER COMPROMISED", "danger")
				g.addLog("CONNECTION TERMINATED", "danger")
			}
			g.Nodes[g.Player.X][g.Player.Y].Type = NodePlayer
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success":  true,
				"score":    g.Score,
				"level":    g.Level,
				"gameOver": g.GameOver,
			})
			return
		}
	}

	switch target.Type {
	case NodeHackable:
		g.addLog(fmt.Sprintf("UPLOADING VIRUS... +%d CREDITS", target.Data), "success")
		g.Score += target.Data
		target.Type = NodeEmpty
		target.Hacked = true
		s.markVulnerableFirewalls(g)
	case NodeEnemy:
		g.GameOver = true
		g.addLog("BREACH DETECTED - PLAYER COMPROMISED", "danger")
		g.addLog("CONNECTION TERMINATED", "danger")
	}

	g.Nodes[g.Player.X][g.Player.Y].Type = NodePlayer

	if g.Score >= g.Level*500 {
		g.Level++
		g.addLog(fmt.Sprintf("LEVEL UP! CLEARANCE %d GRANTED", g.Level), "success")
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"score":    g.Score,
		"level":    g.Level,
		"gameOver": g.GameOver,
	})
}

func (s *Server) hackAdjacent(g *GameGrid) {
	adjacent := []Position{
		{g.Player.X - 1, g.Player.Y},
		{g.Player.X + 1, g.Player.Y},
		{g.Player.X, g.Player.Y - 1},
		{g.Player.X, g.Player.Y + 1},
	}

	hacked := false
	for _, p := range adjacent {
		if p.X >= 0 && p.X < gridSize && p.Y >= 0 && p.Y < gridSize {
			node := &g.Nodes[p.X][p.Y]
			if node.Type == NodeHackable && !node.Hacked {
				node.Hacked = true
				g.Score += node.Data * 2
				node.Type = NodeEmpty
				g.addLog(fmt.Sprintf("BREACH DETECTED - NODE %d,%d HACKED! +%d CREDITS", p.X, p.Y, node.Data*2), "success")
				hacked = true
			}
		}
	}

	if !hacked {
		g.addLog("NO HACKABLE NODES IN RANGE", "warning")
	}

	s.markVulnerableFirewalls(g)
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.grid != nil && s.grid.cancel != nil {
		s.grid.cancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	g := s.createGrid()
	g.cancel = cancel
	s.grid = g

	go s.moveEnemies(ctx)
	go s.spawnHackableNodes(ctx)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}

func main() {
	srv := NewServer()

	ctx, cancel := context.WithCancel(context.Background())
	g := srv.createGrid()
	g.cancel = cancel
	srv.grid = g

	go srv.moveEnemies(ctx)
	go srv.spawnHackableNodes(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/status", srv.handleStatus)
	mux.HandleFunc("/move", srv.handleMove)
	mux.HandleFunc("/restart", srv.handleRestart)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		fmt.Println("╔════════════════════════════════════════╗")
		fmt.Println("║       CYBER-GRID 2077 SERVER          ║")
		fmt.Printf("║       Listening on :%-15s ║\n", port)
		fmt.Println("╚════════════════════════════════════════╝")

		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-quit
	log.Println("Shutting down server...")

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}
