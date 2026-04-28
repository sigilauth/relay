package test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/sigilauth/relay/internal/handlers"
	"github.com/sigilauth/relay/internal/push"
	"github.com/sigilauth/relay/internal/ratelimit"
	"github.com/sigilauth/relay/internal/verify"
)

type mockStore struct {
	tokens   map[string]*push.PushToken
	failures map[string]int
}

func newMockStore() *mockStore {
	return &mockStore{
		tokens:   make(map[string]*push.PushToken),
		failures: make(map[string]int),
	}
}

func (m *mockStore) RegisterDevice(ctx context.Context, fingerprint, token, platform string) error {
	m.tokens[fingerprint] = &push.PushToken{
		Fingerprint: fingerprint,
		Token:       token,
		Platform:    platform,
	}
	m.failures[fingerprint] = 0
	return nil
}

func (m *mockStore) GetPushToken(ctx context.Context, fingerprint string) (*push.PushToken, error) {
	return m.tokens[fingerprint], nil
}

func (m *mockStore) EvictToken(ctx context.Context, fingerprint string) error {
	delete(m.tokens, fingerprint)
	delete(m.failures, fingerprint)
	return nil
}

func (m *mockStore) IncrementFailures(ctx context.Context, fingerprint string) error {
	m.failures[fingerprint]++
	return nil
}

func (m *mockStore) ResetFailures(ctx context.Context, fingerprint string) error {
	m.failures[fingerprint] = 0
	if pt := m.tokens[fingerprint]; pt != nil {
		now := time.Now().Unix()
		pt.LastDeliveredAt = &now
	}
	return nil
}

func (m *mockStore) GetStaleTokens(ctx context.Context, failureThreshold, staleDays int) ([]string, error) {
	var stale []string
	for fp, count := range m.failures {
		if count >= failureThreshold {
			stale = append(stale, fp)
		}
	}
	return stale, nil
}

type mockProvider struct {
	sent        []mockSentPush
	shouldError bool
	errorType   string
}

type mockSentPush struct {
	token   string
	payload []byte
}

func (m *mockProvider) Send(ctx context.Context, token string, payload []byte) error {
	m.sent = append(m.sent, mockSentPush{token: token, payload: payload})

	if m.shouldError {
		if m.errorType == "BadDeviceToken" {
			return &mockError{msg: "BadDeviceToken: device token invalid"}
		}
		return &mockError{msg: "network error"}
	}
	return nil
}

func (m *mockProvider) Platform() string {
	return "mock"
}

type mockError struct {
	msg string
}

func (e *mockError) Error() string {
	return e.msg
}

func setupTestServerWithVerifier() (*ecdsa.PrivateKey, *verify.ServerSignature) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}

	pubKeyBytes := elliptic.MarshalCompressed(elliptic.P256(), privateKey.PublicKey.X, privateKey.PublicKey.Y)
	serverPubKeyB64 := base64.StdEncoding.EncodeToString(pubKeyBytes)

	verifier, err := verify.NewServerSignature(serverPubKeyB64)
	if err != nil {
		panic(err)
	}

	return privateKey, verifier
}

func signPushRequest(privateKey *ecdsa.PrivateKey, serverID, fingerprint, timestamp string) string {
	payload := []byte(serverID + fingerprint + timestamp)
	hash := sha256.Sum256(payload)
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, hash[:])
	if err != nil {
		panic(err)
	}

	curveOrder := elliptic.P256().Params().N
	halfOrder := new(big.Int).Div(curveOrder, big.NewInt(2))
	if s.Cmp(halfOrder) > 0 {
		s = new(big.Int).Sub(curveOrder, s)
	}

	sigBytes := make([]byte, 64)
	r.FillBytes(sigBytes[:32])
	s.FillBytes(sigBytes[32:])
	return base64.StdEncoding.EncodeToString(sigBytes)
}

func TestIntegration_RegisterThenPush(t *testing.T) {
	store := newMockStore()
	provider := &mockProvider{}
	limiter := ratelimit.New(10.0/60.0, 1)
	privateKey, verifier := setupTestServerWithVerifier()

	r := chi.NewRouter()
	r.Use(middleware.Timeout(10 * time.Second))
	r.Post("/devices/register", handlers.NewRegisterHandler(store).ServeHTTP)
	r.Post("/push", handlers.NewPushHandler(store, provider, limiter, verifier).ServeHTTP)

	server := httptest.NewServer(r)
	defer server.Close()

	devicePubKey := "AgECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8g"
	var fingerprint string

	t.Run("Register device", func(t *testing.T) {
		reqBody := map[string]string{
			"device_public_key": devicePubKey,
			"push_token":        "mock-token-123",
			"push_platform":     "apns",
		}
		body, _ := json.Marshal(reqBody)

		resp, err := http.Post(server.URL+"/devices/register", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Register request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 201 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected 201, got %d: %s", resp.StatusCode, string(bodyBytes))
		}

		var regResp struct {
			Fingerprint string   `json:"fingerprint"`
			Pictogram   []string `json:"pictogram"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&regResp); err != nil {
			t.Fatalf("Decode response failed: %v", err)
		}

		if regResp.Fingerprint == "" {
			t.Error("Expected fingerprint in response")
		}
		if len(regResp.Pictogram) != 5 {
			t.Errorf("Expected 5 pictogram emojis, got %d", len(regResp.Pictogram))
		}

		fingerprint = regResp.Fingerprint
		t.Logf("Fingerprint: %s", regResp.Fingerprint)
		t.Logf("Pictogram: %v", regResp.Pictogram)
	})

	t.Run("Push notification with valid signature", func(t *testing.T) {
		timestamp := time.Now().Format(time.RFC3339)
		signature := signPushRequest(privateKey, "test-server", fingerprint, timestamp)

		pushReq := map[string]interface{}{
			"server_id":         "test-server",
			"fingerprint":       fingerprint,
			"payload":           map[string]string{"action": "login", "device": "iPhone"},
			"timestamp":         timestamp,
			"request_signature": signature,
		}
		body, _ := json.Marshal(pushReq)

		resp, err := http.Post(server.URL+"/push", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Push request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Fatalf("Push should succeed, got status %d: %s", resp.StatusCode, string(bodyBytes))
		}

		if len(provider.sent) != 1 {
			t.Errorf("Expected 1 push sent, got %d", len(provider.sent))
		}
		if len(provider.sent) > 0 && provider.sent[0].token != "mock-token-123" {
			t.Errorf("Expected token mock-token-123, got %s", provider.sent[0].token)
		}
	})
}

func TestIntegration_RateLimit(t *testing.T) {
	store := newMockStore()
	provider := &mockProvider{}
	limiter := ratelimit.New(1.0, 1)
	privateKey, verifier := setupTestServerWithVerifier()

	r := chi.NewRouter()
	r.Use(middleware.Timeout(10 * time.Second))
	r.Post("/devices/register", handlers.NewRegisterHandler(store).ServeHTTP)
	r.Post("/push", handlers.NewPushHandler(store, provider, limiter, verifier).ServeHTTP)

	server := httptest.NewServer(r)
	defer server.Close()

	devicePubKey := "AgECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8g"

	regBody := map[string]string{
		"device_public_key": devicePubKey,
		"push_token":        "mock-token-rate",
		"push_platform":     "apns",
	}
	body, _ := json.Marshal(regBody)
	regResp, _ := http.Post(server.URL+"/devices/register", "application/json", bytes.NewReader(body))

	var regData struct {
		Fingerprint string `json:"fingerprint"`
	}
	json.NewDecoder(regResp.Body).Decode(&regData)
	regResp.Body.Close()
	fingerprint := regData.Fingerprint

	for i := 0; i < 3; i++ {
		timestamp := time.Now().Format(time.RFC3339)
		signature := signPushRequest(privateKey, "test-server", fingerprint, timestamp)

		pushReq := map[string]interface{}{
			"server_id":         "test-server",
			"fingerprint":       fingerprint,
			"payload":           map[string]string{"action": "test"},
			"timestamp":         timestamp,
			"request_signature": signature,
		}
		body, _ := json.Marshal(pushReq)
		resp, _ := http.Post(server.URL+"/push", "application/json", bytes.NewReader(body))
		defer resp.Body.Close()

		t.Logf("Request %d: status %d", i+1, resp.StatusCode)

		if i == 0 && resp.StatusCode != 200 {
			t.Errorf("First request should succeed, got %d", resp.StatusCode)
		}
		if i > 0 && resp.StatusCode != 429 {
			t.Errorf("Request %d should be rate limited (429), got %d", i+1, resp.StatusCode)
		}
	}
}

func TestIntegration_TokenEviction(t *testing.T) {
	store := newMockStore()
	provider := &mockProvider{
		shouldError: true,
		errorType:   "BadDeviceToken",
	}
	limiter := ratelimit.New(10.0/60.0, 1)
	privateKey, verifier := setupTestServerWithVerifier()

	r := chi.NewRouter()
	r.Use(middleware.Timeout(10 * time.Second))
	r.Post("/devices/register", handlers.NewRegisterHandler(store).ServeHTTP)
	r.Post("/push", handlers.NewPushHandler(store, provider, limiter, verifier).ServeHTTP)

	server := httptest.NewServer(r)
	defer server.Close()

	devicePubKey := "AgECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8g"

	regBody := map[string]string{
		"device_public_key": devicePubKey,
		"push_token":        "invalid-token",
		"push_platform":     "apns",
	}
	body, _ := json.Marshal(regBody)
	regResp, _ := http.Post(server.URL+"/devices/register", "application/json", bytes.NewReader(body))

	var regData struct {
		Fingerprint string `json:"fingerprint"`
	}
	json.NewDecoder(regResp.Body).Decode(&regData)
	regResp.Body.Close()
	fingerprint := regData.Fingerprint

	if store.tokens[fingerprint] == nil {
		t.Fatal("Device should be registered")
	}

	timestamp := time.Now().Format(time.RFC3339)
	signature := signPushRequest(privateKey, "test-server", fingerprint, timestamp)

	pushReq := map[string]interface{}{
		"server_id":         "test-server",
		"fingerprint":       fingerprint,
		"payload":           map[string]string{"action": "test"},
		"timestamp":         timestamp,
		"request_signature": signature,
	}
	body, _ = json.Marshal(pushReq)
	http.Post(server.URL+"/push", "application/json", bytes.NewReader(body))

	if store.tokens[fingerprint] != nil {
		t.Error("Token should have been evicted after BadDeviceToken error")
	}
}

func TestIntegration_FailureCounter(t *testing.T) {
	store := newMockStore()
	provider := &mockProvider{
		shouldError: true,
		errorType:   "network",
	}
	limiter := ratelimit.New(100.0, 10)
	privateKey, verifier := setupTestServerWithVerifier()

	r := chi.NewRouter()
	r.Use(middleware.Timeout(10 * time.Second))
	r.Post("/devices/register", handlers.NewRegisterHandler(store).ServeHTTP)
	r.Post("/push", handlers.NewPushHandler(store, provider, limiter, verifier).ServeHTTP)

	server := httptest.NewServer(r)
	defer server.Close()

	devicePubKey := "AgECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8g"

	regBody := map[string]string{
		"device_public_key": devicePubKey,
		"push_token":        "valid-token",
		"push_platform":     "apns",
	}
	body, _ := json.Marshal(regBody)
	regResp, _ := http.Post(server.URL+"/devices/register", "application/json", bytes.NewReader(body))

	var regData struct {
		Fingerprint string `json:"fingerprint"`
	}
	json.NewDecoder(regResp.Body).Decode(&regData)
	regResp.Body.Close()
	fingerprint := regData.Fingerprint

	for i := 0; i < 5; i++ {
		timestamp := time.Now().Format(time.RFC3339)
		signature := signPushRequest(privateKey, "test-server", fingerprint, timestamp)

		pushReq := map[string]interface{}{
			"server_id":         "test-server",
			"fingerprint":       fingerprint,
			"payload":           map[string]string{"action": "test"},
			"timestamp":         timestamp,
			"request_signature": signature,
		}
		body, _ := json.Marshal(pushReq)
		http.Post(server.URL+"/push", "application/json", bytes.NewReader(body))
	}

	if store.failures[fingerprint] != 5 {
		t.Errorf("Expected 5 failures, got %d", store.failures[fingerprint])
	}

	provider.shouldError = false
	timestamp := time.Now().Format(time.RFC3339)
	signature := signPushRequest(privateKey, "test-server", fingerprint, timestamp)

	pushReq := map[string]interface{}{
		"server_id":         "test-server",
		"fingerprint":       fingerprint,
		"payload":           map[string]string{"action": "test"},
		"timestamp":         timestamp,
		"request_signature": signature,
	}
	body, _ = json.Marshal(pushReq)
	http.Post(server.URL+"/push", "application/json", bytes.NewReader(body))

	if store.failures[fingerprint] != 0 {
		t.Errorf("Expected failures reset to 0, got %d", store.failures[fingerprint])
	}
}
