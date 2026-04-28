package websocket

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvider_Send(t *testing.T) {
	mgr := NewManager(30 * time.Second)
	provider := NewProvider(mgr)

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	fingerprint := "test_fingerprint_abc123"
	conn := &mockWSConn{writeChan: make(chan []byte, 1)}

	err = mgr.RegisterConnection(fingerprint, conn, privKey.PublicKey)
	require.NoError(t, err)

	payload := []byte(`{"type":"test","message":"hello"}`)
	err = provider.Send(context.Background(), fingerprint, payload)
	assert.NoError(t, err)

	select {
	case received := <-conn.writeChan:
		assert.Equal(t, payload, received)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected message to be delivered")
	}
}

func TestProvider_Send_NoConnection(t *testing.T) {
	mgr := NewManager(30 * time.Second)
	provider := NewProvider(mgr)

	payload := []byte(`{"type":"test"}`)
	err := provider.Send(context.Background(), "nonexistent_fingerprint", payload)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no active connection")
}

func TestProvider_Platform(t *testing.T) {
	mgr := NewManager(30 * time.Second)
	provider := NewProvider(mgr)

	assert.Equal(t, "websocket", provider.Platform())
}
