package websocket

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_RegisterConnection(t *testing.T) {
	mgr := NewManager(30 * time.Second)

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	fingerprint := "test_fingerprint_abc123"
	conn := &mockWSConn{}

	err = mgr.RegisterConnection(fingerprint, conn, privKey.PublicKey)
	assert.NoError(t, err)
	assert.Equal(t, 1, mgr.ConnectionCount())
}

func TestManager_RegisterConnection_ReplacesExisting(t *testing.T) {
	mgr := NewManager(30 * time.Second)

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	fingerprint := "test_fingerprint_abc123"
	conn1 := &mockWSConn{closeCalled: make(chan bool, 1)}
	conn2 := &mockWSConn{}

	err = mgr.RegisterConnection(fingerprint, conn1, privKey.PublicKey)
	require.NoError(t, err)

	err = mgr.RegisterConnection(fingerprint, conn2, privKey.PublicKey)
	require.NoError(t, err)

	select {
	case <-conn1.closeCalled:
		// Good - old connection was closed
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected old connection to be closed")
	}

	assert.Equal(t, 1, mgr.ConnectionCount())
}

func TestManager_Send(t *testing.T) {
	mgr := NewManager(30 * time.Second)

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	fingerprint := "test_fingerprint_abc123"
	conn := &mockWSConn{writeChan: make(chan []byte, 1)}

	err = mgr.RegisterConnection(fingerprint, conn, privKey.PublicKey)
	require.NoError(t, err)

	payload := []byte(`{"type":"test","data":"hello"}`)
	err = mgr.Send(context.Background(), fingerprint, payload)
	assert.NoError(t, err)

	select {
	case received := <-conn.writeChan:
		assert.Equal(t, payload, received)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected message to be written to connection")
	}
}

func TestManager_Send_NotFound(t *testing.T) {
	mgr := NewManager(30 * time.Second)

	payload := []byte(`{"type":"test"}`)
	err := mgr.Send(context.Background(), "nonexistent_fingerprint", payload)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no active connection")
}

func TestManager_Unregister(t *testing.T) {
	mgr := NewManager(30 * time.Second)

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	fingerprint := "test_fingerprint_abc123"
	conn := &mockWSConn{}

	err = mgr.RegisterConnection(fingerprint, conn, privKey.PublicKey)
	require.NoError(t, err)
	assert.Equal(t, 1, mgr.ConnectionCount())

	mgr.Unregister(fingerprint)
	assert.Equal(t, 0, mgr.ConnectionCount())
}

func TestManager_Heartbeat(t *testing.T) {
	mgr := NewManager(50 * time.Millisecond)

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	fingerprint := "test_fingerprint_abc123"
	conn := &mockWSConn{writeChan: make(chan []byte, 10)}

	err = mgr.RegisterConnection(fingerprint, conn, privKey.PublicKey)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go mgr.StartHeartbeat(ctx, fingerprint)

	pingSeen := 0
	for {
		select {
		case msg := <-conn.writeChan:
			if websocket.PingMessage == 9 {
				pingSeen++
			}
			_ = msg
		case <-ctx.Done():
			assert.GreaterOrEqual(t, pingSeen, 2, "Expected at least 2 pings in 200ms with 50ms interval")
			return
		}
	}
}

type mockWSConn struct {
	writeChan   chan []byte
	closeCalled chan bool
}

func (m *mockWSConn) WriteMessage(messageType int, data []byte) error {
	if m.writeChan != nil {
		m.writeChan <- data
	}
	return nil
}

func (m *mockWSConn) WriteControl(messageType int, data []byte, deadline time.Time) error {
	if m.writeChan != nil {
		m.writeChan <- data
	}
	return nil
}

func (m *mockWSConn) Close() error {
	if m.closeCalled != nil {
		m.closeCalled <- true
	}
	return nil
}

func (m *mockWSConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockWSConn) SetWriteDeadline(t time.Time) error { return nil }
func (m *mockWSConn) ReadMessage() (int, []byte, error) {
	time.Sleep(100 * time.Millisecond)
	return websocket.PongMessage, nil, nil
}
