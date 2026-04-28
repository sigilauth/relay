package websocket

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WSConn abstracts websocket.Conn for testing
type WSConn interface {
	WriteMessage(messageType int, data []byte) error
	WriteControl(messageType int, data []byte, deadline time.Time) error
	ReadMessage() (messageType int, p []byte, err error)
	Close() error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
}

// Connection represents an active WebSocket connection
type Connection struct {
	Fingerprint string
	Conn        WSConn
	PublicKey   ecdsa.PublicKey
	ConnectedAt time.Time
	LastPingAt  time.Time
}

// Manager manages active WebSocket connections
type Manager struct {
	connections      map[string]*Connection
	mu               sync.RWMutex
	heartbeatInterval time.Duration
}

// NewManager creates a new WebSocket connection manager
func NewManager(heartbeatInterval time.Duration) *Manager {
	return &Manager{
		connections:      make(map[string]*Connection),
		heartbeatInterval: heartbeatInterval,
	}
}

// RegisterConnection registers a new WebSocket connection for a fingerprint
// If a connection already exists for this fingerprint, it closes the old one
// (max 1 connection per fingerprint)
func (m *Manager) RegisterConnection(fingerprint string, conn WSConn, publicKey ecdsa.PublicKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.connections[fingerprint]; ok {
		slog.Info("Replacing existing connection", "fingerprint_prefix", fingerprint[:16])
		existing.Conn.Close()
	}

	m.connections[fingerprint] = &Connection{
		Fingerprint: fingerprint,
		Conn:        conn,
		PublicKey:   publicKey,
		ConnectedAt: time.Now(),
		LastPingAt:  time.Now(),
	}

	slog.Info("WebSocket connection registered", "fingerprint_prefix", fingerprint[:16])
	return nil
}

// Send sends a message to the WebSocket connection for the given fingerprint
func (m *Manager) Send(ctx context.Context, fingerprint string, payload []byte) error {
	m.mu.RLock()
	conn, ok := m.connections[fingerprint]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no active connection for fingerprint: %s", fingerprint[:16])
	}

	if err := conn.Conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		slog.Error("Failed to write message to WebSocket", "fingerprint_prefix", fingerprint[:16], "error", err)
		m.Unregister(fingerprint)
		return fmt.Errorf("failed to write message: %w", err)
	}

	return nil
}

// Unregister removes a WebSocket connection
func (m *Manager) Unregister(fingerprint string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if conn, ok := m.connections[fingerprint]; ok {
		conn.Conn.Close()
		delete(m.connections, fingerprint)
		slog.Info("WebSocket connection unregistered", "fingerprint_prefix", fingerprint[:16])
	}
}

// ConnectionCount returns the number of active connections
func (m *Manager) ConnectionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.connections)
}

// StartHeartbeat sends periodic ping messages to keep the connection alive
// Runs until context is cancelled
func (m *Manager) StartHeartbeat(ctx context.Context, fingerprint string) {
	ticker := time.NewTicker(m.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.mu.RLock()
			conn, ok := m.connections[fingerprint]
			m.mu.RUnlock()

			if !ok {
				return
			}

			deadline := time.Now().Add(10 * time.Second)
			if err := conn.Conn.WriteControl(websocket.PingMessage, []byte{}, deadline); err != nil {
				slog.Error("Failed to send ping", "fingerprint_prefix", fingerprint[:16], "error", err)
				m.Unregister(fingerprint)
				return
			}

			m.mu.Lock()
			if conn, ok := m.connections[fingerprint]; ok {
				conn.LastPingAt = time.Now()
			}
			m.mu.Unlock()
		}
	}
}
