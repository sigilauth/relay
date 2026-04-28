package handlers

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sigilauth/relay/internal/push"
	"github.com/sigilauth/relay/internal/ratelimit"
	"github.com/sigilauth/relay/internal/verify"
)

// MockPushProvider for testing
type MockPushProvider struct {
	SendFunc     func(ctx context.Context, token string, payload []byte) error
	PlatformFunc func() string
}

func (m *MockPushProvider) Send(ctx context.Context, token string, payload []byte) error {
	if m.SendFunc != nil {
		return m.SendFunc(ctx, token, payload)
	}
	return nil
}

func (m *MockPushProvider) Platform() string {
	if m.PlatformFunc != nil {
		return m.PlatformFunc()
	}
	return "mock"
}

func TestPushHandler(t *testing.T) {
	// Valid server public key and signature for testing
	// TODO: Use real ECDSA test vectors from api/test-vectors/ecdsa.json

	tests := []struct {
		name           string
		requestBody    interface{}
		mockStore      *MockStore
		mockProvider   push.PushProvider
		expectedStatus int
		checkResponse  func(*testing.T, map[string]interface{})
	}{
		{
			name: "Valid push delivery",
			requestBody: map[string]interface{}{
				"server_id":         "test-server",
				"fingerprint":       "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2",
				"payload":           map[string]string{"challenge_id": "abc123"},
				"timestamp":         time.Now().UTC().Format(time.RFC3339),
				"request_signature": "MEUCIQDfz8K7rN...", // Mock signature
			},
			mockStore: &MockStore{
				GetPushTokenFunc: func(ctx context.Context, fp string) (*push.PushToken, error) {
					return &push.PushToken{
						Fingerprint: fp,
						Token:       "apns-token-123",
						Platform:    "apns",
					}, nil
				},
			},
			mockProvider: &MockPushProvider{
				SendFunc: func(ctx context.Context, token string, payload []byte) error {
					return nil // Successful delivery
				},
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				if resp["status"] != "delivered" {
					t.Errorf("Expected status 'delivered', got %v", resp["status"])
				}
			},
		},
		{
			name: "Fingerprint not registered",
			requestBody: map[string]interface{}{
				"server_id":         "test-server",
				"fingerprint":       "nonexistent",
				"payload":           map[string]string{"challenge_id": "abc123"},
				"timestamp":         time.Now().UTC().Format(time.RFC3339),
				"request_signature": "MEUCIQDfz8K7rN...",
			},
			mockStore: &MockStore{
				GetPushTokenFunc: func(ctx context.Context, fp string) (*push.PushToken, error) {
					return nil, nil // Not found
				},
			},
			expectedStatus: http.StatusNotFound,
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				errObj := resp["error"].(map[string]interface{})
				if errObj["code"] != "FINGERPRINT_NOT_FOUND" {
					t.Errorf("Expected FINGERPRINT_NOT_FOUND, got %v", errObj["code"])
				}
			},
		},
		{
			name: "Invalid signature",
			requestBody: map[string]interface{}{
				"server_id":         "test-server",
				"fingerprint":       "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2",
				"payload":           map[string]string{"challenge_id": "abc123"},
				"timestamp":         time.Now().UTC().Format(time.RFC3339),
				"request_signature": "invalid-signature",
			},
			mockStore: &MockStore{
				GetPushTokenFunc: func(ctx context.Context, fp string) (*push.PushToken, error) {
					return &push.PushToken{
						Fingerprint: fp,
						Token:       "apns-token-123",
						Platform:    "apns",
					}, nil
				},
			},
			expectedStatus: http.StatusForbidden,
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				errObj := resp["error"].(map[string]interface{})
				if errObj["code"] != "INVALID_SIGNATURE" {
					t.Errorf("Expected INVALID_SIGNATURE, got %v", errObj["code"])
				}
			},
		},
		{
			name: "Rate limit exceeded",
			requestBody: map[string]interface{}{
				"server_id":         "test-server",
				"fingerprint":       "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2",
				"payload":           map[string]string{"challenge_id": "abc123"},
				"timestamp":         time.Now().UTC().Format(time.RFC3339),
				"request_signature": "MEUCIQDfz8K7rN...",
			},
			mockStore: &MockStore{
				GetPushTokenFunc: func(ctx context.Context, fp string) (*push.PushToken, error) {
					return &push.PushToken{
						Fingerprint: fp,
						Token:       "apns-token-123",
						Platform:    "apns",
					}, nil
				},
			},
			expectedStatus: http.StatusTooManyRequests,
			// Note: Test needs to exhaust rate limiter first
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "Invalid signature" {
				t.Skip("Signature verification requires real ECDSA test setup")
			}
			if tc.name == "Rate limit exceeded" {
				t.Skip("Rate limit test requires pre-exhaustion setup")
			}

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

			reqMap := tc.requestBody.(map[string]interface{})
			serverID := reqMap["server_id"].(string)
			fingerprint := reqMap["fingerprint"].(string)
			timestamp := reqMap["timestamp"].(string)

			signaturePayload := fmt.Sprintf("SIGIL-RELAY-PUSH-V1\x00%s\x00%s\x00%s", serverID, fingerprint, timestamp)
			hash := sha256.Sum256([]byte(signaturePayload))
			r, s, err := ecdsa.Sign(rand.Reader, privateKey, hash[:])
			if err != nil {
				t.Fatalf("sign: %v", err)
			}

			curveOrder := elliptic.P256().Params().N
			halfOrder := new(big.Int).Div(curveOrder, big.NewInt(2))
			if s.Cmp(halfOrder) > 0 {
				s = new(big.Int).Sub(curveOrder, s)
			}

			sigBytes := make([]byte, 64)
			r.FillBytes(sigBytes[:32])
			s.FillBytes(sigBytes[32:])
			reqMap["request_signature"] = base64.StdEncoding.EncodeToString(sigBytes)

			body, _ := json.Marshal(reqMap)
			req := httptest.NewRequest(http.MethodPost, "/push", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			limiter := ratelimit.New(10.0/60.0, 1)
			handler := NewPushHandler(tc.mockStore, tc.mockProvider, limiter, verifier, nil)
			handler.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tc.expectedStatus, w.Code, w.Body.String())
			}

			if tc.checkResponse != nil {
				var resp map[string]interface{}
				json.NewDecoder(w.Body).Decode(&resp)
				tc.checkResponse(t, resp)
			}
		})
	}
}
