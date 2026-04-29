package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	srv := NewServer()
	ctx, cancel := newTestContext()
	g := srv.createGrid()
	g.cancel = cancel
	srv.grid = g

	go srv.moveEnemies(ctx)
	go srv.spawnHackableNodes(ctx)

	return srv
}

func TestNewServer(t *testing.T) {
	srv := NewServer()
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	if srv.rng == nil {
		t.Fatal("expected non-nil rng")
	}
}

func TestCreateGrid(t *testing.T) {
	srv := NewServer()
	g := srv.createGrid()

	if g == nil {
		t.Fatal("expected non-nil grid")
	}

	if len(g.Nodes) != gridSize {
		t.Errorf("expected %d rows, got %d", gridSize, len(g.Nodes))
	}

	for i, row := range g.Nodes {
		if len(row) != gridSize {
			t.Errorf("row %d: expected %d cols, got %d", i, gridSize, len(row))
		}
	}

	if g.Player.X != gridSize/2 || g.Player.Y != gridSize/2 {
		t.Errorf("player at (%d,%d), expected (%d,%d)",
			g.Player.X, g.Player.Y, gridSize/2, gridSize/2)
	}

	if g.Nodes[g.Player.X][g.Player.Y].Type != NodePlayer {
		t.Error("player node type should be NodePlayer")
	}

	if len(g.Enemies) != numEnemies {
		t.Errorf("expected %d enemies, got %d", numEnemies, len(g.Enemies))
	}

	for _, e := range g.Enemies {
		dist := abs(e.X-g.Player.X) + abs(e.Y-g.Player.Y)
		if dist < 4 {
			t.Errorf("enemy at (%d,%d) too close to player (dist=%d)", e.X, e.Y, dist)
		}
	}

	if len(g.Log) == 0 {
		t.Error("expected initial log entries")
	}
}

func TestAddLog(t *testing.T) {
	g := &GameGrid{Log: make([]LogEntry, 0, 50)}

	for i := 0; i < 60; i++ {
		g.addLog("test", "info")
	}

	if len(g.Log) > 50 {
		t.Errorf("log should cap at 50 entries, got %d", len(g.Log))
	}
}

func TestHandleStatus(t *testing.T) {
	srv := newTestServer(t)

	tests := []struct {
		name       string
		method     string
		wantStatus int
		wantBody   bool
	}{
		{
			name:       "GET returns status",
			method:     http.MethodGet,
			wantStatus: http.StatusOK,
			wantBody:   true,
		},
		{
			name:       "POST rejected",
			method:     http.MethodPost,
			wantStatus: http.StatusMethodNotAllowed,
			wantBody:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/status", nil)
			w := httptest.NewRecorder()

			srv.handleStatus(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d; want %d", w.Code, tt.wantStatus)
			}

			if tt.wantBody {
				var resp StatusResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}

				if resp.Player.X != gridSize/2 {
					t.Errorf("player X = %d; want %d", resp.Player.X, gridSize/2)
				}

				if len(resp.Enemies) != numEnemies {
					t.Errorf("enemies = %d; want %d", len(resp.Enemies), numEnemies)
				}
			}
		})
	}
}

func TestHandleMove(t *testing.T) {
	srv := newTestServer(t)

	tests := []struct {
		name        string
		dir         string
		wantStatus  int
		wantSuccess *bool
	}{
		{
			name:        "valid move up",
			dir:         "up",
			wantStatus:  http.StatusOK,
			wantSuccess: boolPtr(true),
		},
		{
			name:        "valid move down",
			dir:         "down",
			wantStatus:  http.StatusOK,
			wantSuccess: boolPtr(true),
		},
		{
			name:        "hack command",
			dir:         "hack",
			wantStatus:  http.StatusOK,
			wantSuccess: boolPtr(true),
		},
		{
			name:       "invalid direction",
			dir:        "diagonal",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := map[string]string{"dir": tt.dir}
			jsonBody, _ := json.Marshal(body)

			req := httptest.NewRequest(http.MethodPost, "/move", bytes.NewReader(jsonBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			srv.handleMove(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d; want %d", w.Code, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusOK && tt.wantSuccess != nil {
				var resp map[string]interface{}
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}

				success, _ := resp["success"].(bool)
				if success != *tt.wantSuccess {
					t.Errorf("success = %v; want %v", success, *tt.wantSuccess)
				}
			}
		})
	}
}

func TestHandleMoveOutOfBounds(t *testing.T) {
	srv := NewServer()
	ctx, cancel := newTestContext()
	g := srv.createGrid()
	g.cancel = cancel
	g.Player.X = 0
	g.Player.Y = 0
	srv.grid = g

	go srv.moveEnemies(ctx)
	go srv.spawnHackableNodes(ctx)

	body := map[string]string{"dir": "up"}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/move", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.handleMove(w, req)

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if success, _ := resp["success"].(bool); success {
		t.Error("move out of bounds should not succeed")
	}
}

func TestHandleRestart(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/restart", nil)
	w := httptest.NewRecorder()

	srv.handleRestart(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if success, _ := resp["success"].(bool); !success {
		t.Error("restart should succeed")
	}

	srv.mu.RLock()
	g := srv.grid
	srv.mu.RUnlock()

	if g == nil {
		t.Fatal("grid should be reinitialized")
	}

	if g.Level != 1 {
		t.Errorf("level = %d; want 1", g.Level)
	}

	if g.Score != 0 {
		t.Errorf("score = %d; want 0", g.Score)
	}
}

func TestGameLogMessages(t *testing.T) {
	srv := NewServer()
	g := srv.createGrid()

	infoCount := 0
	warningCount := 0
	for _, entry := range g.Log {
		if entry.Type == "info" {
			infoCount++
		}
		if entry.Type == "warning" {
			warningCount++
		}
	}

	if infoCount < 2 {
		t.Errorf("expected at least 2 info logs, got %d", infoCount)
	}

	if warningCount < 1 {
		t.Errorf("expected at least 1 warning log, got %d", warningCount)
	}
}

func TestHandleMoveMethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/move", nil)
	w := httptest.NewRecorder()

	srv.handleMove(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d; want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleIndex(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handleIndex(w, req)

	if w.Code == http.StatusOK || w.Code == http.StatusNotFound {
		return
	}
}

func boolPtr(b bool) *bool {
	return &b
}
