package websocket

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"log/slog"
	"math/big"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sigilauth/relay/internal/push"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}

		allowedOrigins := []string{
			"https://app.sigilauth.com",
			"https://relay.sigilauth.com",
		}

		for _, allowed := range allowedOrigins {
			if origin == allowed {
				return true
			}
		}

		return false
	},
}

type Handler struct {
	manager *Manager
	store   interface {
		GetPushToken(ctx context.Context, fingerprint string) (*push.PushToken, error)
	}
}

func NewHandler(manager *Manager, store interface {
	GetPushToken(ctx context.Context, fingerprint string) (*push.PushToken, error)
}) *Handler {
	return &Handler{
		manager: manager,
		store:   store,
	}
}

// ServeHTTP handles the WebSocket upgrade and authentication
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("Failed to upgrade to WebSocket", "error", err)
		return
	}

	go h.handleConnection(conn)
}

func (h *Handler) handleConnection(conn *websocket.Conn) {
	defer conn.Close()

	challenge := make([]byte, 32)
	if _, err := rand.Read(challenge); err != nil {
		slog.Error("Failed to generate challenge", "error", err)
		return
	}

	authChallenge := map[string]string{
		"type":       "auth_challenge",
		"challenge":  base64.StdEncoding.EncodeToString(challenge),
		"expires_at": time.Now().Add(30 * time.Second).Format(time.RFC3339),
	}

	if err := conn.WriteJSON(authChallenge); err != nil {
		slog.Error("Failed to send auth challenge", "error", err)
		return
	}

	conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	var authResponse struct {
		Type            string `json:"type"`
		DevicePublicKey string `json:"device_public_key"`
		Signature       string `json:"signature"`
	}

	if err := conn.ReadJSON(&authResponse); err != nil {
		slog.Error("Failed to read auth response", "error", err)
		return
	}

	if authResponse.Type != "auth_response" {
		h.sendAuthFailure(conn, "invalid auth response type")
		return
	}

	pubKeyBytes, err := base64.StdEncoding.DecodeString(authResponse.DevicePublicKey)
	if err != nil {
		h.sendAuthFailure(conn, "invalid device public key encoding")
		return
	}

	x, y := elliptic.UnmarshalCompressed(elliptic.P256(), pubKeyBytes)
	if x == nil {
		h.sendAuthFailure(conn, "invalid device public key format")
		return
	}

	publicKey := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     x,
		Y:     y,
	}

	signatureBytes, err := base64.StdEncoding.DecodeString(authResponse.Signature)
	if err != nil || len(signatureBytes) != 64 {
		h.sendAuthFailure(conn, "invalid signature format")
		return
	}

	r := new(big.Int).SetBytes(signatureBytes[0:32])
	s := new(big.Int).SetBytes(signatureBytes[32:64])

	curveOrder := elliptic.P256().Params().N
	halfOrder := new(big.Int).Div(curveOrder, big.NewInt(2))

	if s.Cmp(halfOrder) > 0 {
		h.sendAuthFailure(conn, "signature has high S value (malleability risk)")
		return
	}

	domainTag := []byte("SIGIL-RELAY-WS-AUTH-V1\x00")
	message := append(domainTag, challenge...)
	hash := sha256.Sum256(message)

	if !ecdsa.Verify(publicKey, hash[:], r, s) {
		h.sendAuthFailure(conn, "invalid signature")
		return
	}

	fingerprintBytes := sha256.Sum256(pubKeyBytes)
	fingerprint := hex.EncodeToString(fingerprintBytes[:])

	ctx := context.WithoutCancel(context.Background())
	pushToken, err := h.store.GetPushToken(ctx, fingerprint)
	if err != nil {
		slog.Error("Failed to lookup fingerprint", "fingerprint_prefix", fingerprint[:16], "error", err)
		h.sendAuthFailure(conn, "registration check failed")
		return
	}
	if pushToken == nil {
		h.sendAuthFailure(conn, "fingerprint not registered")
		return
	}

	authSuccess := map[string]string{
		"type":        "auth_success",
		"fingerprint": fingerprint,
	}

	if err := conn.WriteJSON(authSuccess); err != nil {
		slog.Error("Failed to send auth success", "error", err)
		return
	}

	conn.SetReadDeadline(time.Time{})

	if err := h.manager.RegisterConnection(fingerprint, conn, *publicKey); err != nil {
		slog.Error("Failed to register connection", "error", err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go h.manager.StartHeartbeat(ctx, fingerprint)

	for {
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			slog.Info("Connection closed", "fingerprint_prefix", fingerprint[:16], "error", err)
			h.manager.Unregister(fingerprint)
			return
		}

		if msgType == websocket.PongMessage {
			slog.Debug("Received pong", "fingerprint_prefix", fingerprint[:16])
			continue
		}

		slog.Debug("Received message", "fingerprint_prefix", fingerprint[:16], "message", string(msg))
	}
}

func (h *Handler) sendAuthFailure(conn *websocket.Conn, reason string) {
	authFailure := map[string]string{
		"type":  "auth_failure",
		"error": reason,
	}
	conn.WriteJSON(authFailure)
	slog.Warn("Authentication failed", "reason", reason)
}
