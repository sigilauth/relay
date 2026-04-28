package websocket

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_Upgrade(t *testing.T) {
	mgr := NewManager(30 * time.Second)
	handler := NewHandler(mgr)

	server := httptest.NewServer(http.HandlerFunc(handler.ServeHTTP))
	defer server.Close()

	wsURL := "ws" + server.URL[len("http"):]
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	// Should receive auth challenge immediately
	msgType, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, websocket.TextMessage, msgType)

	var challenge AuthChallenge
	err = json.Unmarshal(msg, &challenge)
	require.NoError(t, err)
	assert.Equal(t, "auth_challenge", challenge.Type)
	assert.NotEmpty(t, challenge.Challenge)
}

func TestHandler_AuthSuccess(t *testing.T) {
	mgr := NewManager(30 * time.Second)
	handler := NewHandler(mgr)

	server := httptest.NewServer(http.HandlerFunc(handler.ServeHTTP))
	defer server.Close()

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	wsURL := "ws" + server.URL[len("http"):]
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	msgType, msg, err := conn.ReadMessage()
	require.NoError(t, err)

	var challenge AuthChallenge
	err = json.Unmarshal(msg, &challenge)
	require.NoError(t, err)

	challengeBytes, err := base64.StdEncoding.DecodeString(challenge.Challenge)
	require.NoError(t, err)

	hash := sha256.Sum256(challengeBytes)
	r, s, err := ecdsa.Sign(rand.Reader, privKey, hash[:])
	require.NoError(t, err)

	signature := make([]byte, 64)
	r.FillBytes(signature[0:32])
	s.FillBytes(signature[32:64])

	pubKeyBytes := elliptic.MarshalCompressed(privKey.Curve, privKey.X, privKey.Y)

	authResponse := AuthResponse{
		Type:            "auth_response",
		DevicePublicKey: base64.StdEncoding.EncodeToString(pubKeyBytes),
		Signature:       base64.StdEncoding.EncodeToString(signature),
		Timestamp:       time.Now().Format(time.RFC3339),
	}

	err = conn.WriteJSON(authResponse)
	require.NoError(t, err)

	msgType, msg, err = conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, websocket.TextMessage, msgType)

	var authResult map[string]interface{}
	err = json.Unmarshal(msg, &authResult)
	require.NoError(t, err)
	assert.Equal(t, "auth_success", authResult["type"])
	assert.NotEmpty(t, authResult["fingerprint"])

	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 1, mgr.ConnectionCount())
}

func TestHandler_AuthFailure_InvalidSignature(t *testing.T) {
	mgr := NewManager(30 * time.Second)
	handler := NewHandler(mgr)

	server := httptest.NewServer(http.HandlerFunc(handler.ServeHTTP))
	defer server.Close()

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	wsURL := "ws" + server.URL[len("http"):]
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	msgType, msg, err := conn.ReadMessage()
	require.NoError(t, err)

	var challenge AuthChallenge
	err = json.Unmarshal(msg, &challenge)
	require.NoError(t, err)

	pubKeyBytes := elliptic.MarshalCompressed(privKey.Curve, privKey.X, privKey.Y)
	invalidSignature := make([]byte, 64)

	authResponse := AuthResponse{
		Type:            "auth_response",
		DevicePublicKey: base64.StdEncoding.EncodeToString(pubKeyBytes),
		Signature:       base64.StdEncoding.EncodeToString(invalidSignature),
		Timestamp:       time.Now().Format(time.RFC3339),
	}

	err = conn.WriteJSON(authResponse)
	require.NoError(t, err)

	msgType, msg, err = conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, websocket.TextMessage, msgType)

	var authResult map[string]interface{}
	err = json.Unmarshal(msg, &authResult)
	require.NoError(t, err)
	assert.Equal(t, "auth_failure", authResult["type"])
	assert.Contains(t, authResult["error"], "invalid signature")

	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 0, mgr.ConnectionCount())
}

func TestHandler_AuthFailure_FingerprintMismatch(t *testing.T) {
	mgr := NewManager(30 * time.Second)
	handler := NewHandler(mgr)

	privKey1, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	privKey2, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(handler.ServeHTTP))
	defer server.Close()

	wsURL := "ws" + server.URL[len("http"):]
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	msgType, msg, err := conn.ReadMessage()
	require.NoError(t, err)

	var challenge AuthChallenge
	err = json.Unmarshal(msg, &challenge)
	require.NoError(t, err)

	challengeBytes, err := base64.StdEncoding.DecodeString(challenge.Challenge)
	require.NoError(t, err)

	hash := sha256.Sum256(challengeBytes)
	r, s, err := ecdsa.Sign(rand.Reader, privKey1, hash[:])
	require.NoError(t, err)

	signature := make([]byte, 64)
	r.FillBytes(signature[0:32])
	s.FillBytes(signature[32:64])

	pubKeyBytes2 := elliptic.MarshalCompressed(privKey2.Curve, privKey2.X, privKey2.Y)

	authResponse := AuthResponse{
		Type:            "auth_response",
		DevicePublicKey: base64.StdEncoding.EncodeToString(pubKeyBytes2),
		Signature:       base64.StdEncoding.EncodeToString(signature),
		Timestamp:       time.Now().Format(time.RFC3339),
	}

	err = conn.WriteJSON(authResponse)
	require.NoError(t, err)

	msgType, msg, err = conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, websocket.TextMessage, msgType)

	var authResult map[string]interface{}
	err = json.Unmarshal(msg, &authResult)
	require.NoError(t, err)
	assert.Equal(t, "auth_failure", authResult["type"])

	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 0, mgr.ConnectionCount())
}

type AuthChallenge struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
	ExpiresAt string `json:"expires_at"`
}

type AuthResponse struct {
	Type            string `json:"type"`
	DevicePublicKey string `json:"device_public_key"`
	Signature       string `json:"signature"`
	Timestamp       string `json:"timestamp"`
}
