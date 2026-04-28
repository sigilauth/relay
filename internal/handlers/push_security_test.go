package handlers

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sigilauth/relay/internal/push"
	"github.com/sigilauth/relay/internal/ratelimit"
	"github.com/sigilauth/relay/internal/verify"
)

func TestPushHandler_NilVerifierRejection(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when constructing PushHandler with nil verifier, but no panic occurred")
		}
	}()

	mockStore := &MockStore{}
	mockProvider := &MockPushProvider{}
	limiter := ratelimit.New(10.0/60.0, 1)

	NewPushHandler(mockStore, mockProvider, limiter, nil)
}

func TestPushHandler_UnsignedRequestRejection(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	pubKeyBytes := elliptic.MarshalCompressed(elliptic.P256(), privateKey.PublicKey.X, privateKey.PublicKey.Y)
	serverPubKeyB64 := base64.StdEncoding.EncodeToString(pubKeyBytes)

	verifier, err := verify.NewServerSignature(serverPubKeyB64)
	if err != nil {
		t.Fatalf("Failed to create verifier: %v", err)
	}

	mockStore := &MockStore{
		GetPushTokenFunc: func(ctx context.Context, fp string) (*push.PushToken, error) {
			return &push.PushToken{
				Fingerprint: fp,
				Token:       "apns-token-123",
				Platform:    "apns",
			}, nil
		},
	}
	mockProvider := &MockPushProvider{}
	limiter := ratelimit.New(10.0/60.0, 1)

	handler := NewPushHandler(mockStore, mockProvider, limiter, verifier)

	requestBody := map[string]interface{}{
		"server_id":   "test-server",
		"fingerprint": "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2",
		"payload":     map[string]string{"challenge_id": "abc123"},
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
	}

	body, _ := json.Marshal(requestBody)
	req := httptest.NewRequest(http.MethodPost, "/push", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status %d for missing signature, got %d. Body: %s",
			http.StatusForbidden, w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	if resp["error"] == nil {
		t.Fatal("Expected error object in response")
	}

	errObj := resp["error"].(map[string]interface{})
	if errObj["code"] != "INVALID_SIGNATURE" {
		t.Errorf("Expected error code INVALID_SIGNATURE, got %v", errObj["code"])
	}
}

func TestPushHandler_InvalidSignatureRejection(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	pubKeyBytes := elliptic.MarshalCompressed(elliptic.P256(), privateKey.PublicKey.X, privateKey.PublicKey.Y)
	serverPubKeyB64 := base64.StdEncoding.EncodeToString(pubKeyBytes)

	verifier, err := verify.NewServerSignature(serverPubKeyB64)
	if err != nil {
		t.Fatalf("Failed to create verifier: %v", err)
	}

	mockStore := &MockStore{
		GetPushTokenFunc: func(ctx context.Context, fp string) (*push.PushToken, error) {
			return &push.PushToken{
				Fingerprint: fp,
				Token:       "apns-token-123",
				Platform:    "apns",
			}, nil
		},
	}
	mockProvider := &MockPushProvider{}
	limiter := ratelimit.New(10.0/60.0, 1)

	handler := NewPushHandler(mockStore, mockProvider, limiter, verifier)

	requestBody := map[string]interface{}{
		"server_id":         "test-server",
		"fingerprint":       "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2",
		"payload":           map[string]string{"challenge_id": "abc123"},
		"timestamp":         time.Now().UTC().Format(time.RFC3339),
		"request_signature": "aW52YWxpZC1zaWduYXR1cmUtZGF0YQ==",
	}

	body, _ := json.Marshal(requestBody)
	req := httptest.NewRequest(http.MethodPost, "/push", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status %d for invalid signature, got %d. Body: %s",
			http.StatusForbidden, w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	errObj := resp["error"].(map[string]interface{})
	if errObj["code"] != "INVALID_SIGNATURE" {
		t.Errorf("Expected error code INVALID_SIGNATURE, got %v", errObj["code"])
	}
}
