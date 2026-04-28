package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sigilauth/relay/internal/push"
	"github.com/sigilauth/relay/internal/store"
)

type MockStore struct {
	RegisterDeviceFunc func(ctx context.Context, fp, token, platform string) error
	GetPushTokenFunc   func(ctx context.Context, fp string) (*push.PushToken, error)
}

func (m *MockStore) RegisterDevice(ctx context.Context, fp, token, platform string) error {
	if m.RegisterDeviceFunc != nil {
		return m.RegisterDeviceFunc(ctx, fp, token, platform)
	}
	return nil
}

func (m *MockStore) GetPushToken(ctx context.Context, fp string) (*push.PushToken, error) {
	if m.GetPushTokenFunc != nil {
		return m.GetPushTokenFunc(ctx, fp)
	}
	return nil, nil
}

func (m *MockStore) EvictToken(ctx context.Context, fp string) error        { return nil }
func (m *MockStore) IncrementFailures(ctx context.Context, fp string) error { return nil }
func (m *MockStore) ResetFailures(ctx context.Context, fp string) error     { return nil }
func (m *MockStore) GetStaleTokens(ctx context.Context, threshold, days int) ([]string, error) {
	return nil, nil
}

func TestRegisterDevice_ValidFlow(t *testing.T) {
	challengeStore := store.NewChallengeStore()
	defer challengeStore.Close()

	mockStore := &MockStore{
		RegisterDeviceFunc: func(ctx context.Context, fp, token, platform string) error {
			return nil
		},
	}

	keyPair, err := generateTestKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate test keypair: %v", err)
	}

	challenge, err := challengeStore.CreateChallenge(context.Background())
	if err != nil {
		t.Fatalf("Failed to create challenge: %v", err)
	}

	pushToken := "apns-token-test-123"
	signature, err := keyPair.signRegistrationChallenge(challenge.Nonce, pushToken)
	if err != nil {
		t.Fatalf("Failed to sign challenge: %v", err)
	}

	requestBody := map[string]string{
		"device_public_key": keyPair.publicKeyB64,
		"push_token":        pushToken,
		"push_platform":     "apns",
		"challenge_id":      challenge.ID,
		"signature":         signature,
	}

	body, _ := json.Marshal(requestBody)
	req := httptest.NewRequest(http.MethodPost, "/devices/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler := NewRegisterHandler(mockStore, challengeStore)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	if resp["fingerprint"] == "" {
		t.Error("Expected fingerprint in response")
	}
	if resp["pictogram"] == nil {
		t.Error("Expected pictogram in response")
	}
	if resp["pictogram_speakable"] == "" {
		t.Error("Expected pictogram_speakable in response")
	}
	if resp["registered_at"] == "" {
		t.Error("Expected registered_at in response")
	}
}

func TestRegisterDevice_MissingChallenge(t *testing.T) {
	challengeStore := store.NewChallengeStore()
	defer challengeStore.Close()

	mockStore := &MockStore{}

	keyPair, _ := generateTestKeyPair()

	requestBody := map[string]string{
		"device_public_key": keyPair.publicKeyB64,
		"push_token":        "apns-token-test",
		"push_platform":     "apns",
		"challenge_id":      "",
		"signature":         "fake-signature",
	}

	body, _ := json.Marshal(requestBody)
	req := httptest.NewRequest(http.MethodPost, "/devices/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler := NewRegisterHandler(mockStore, challengeStore)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	errObj := resp["error"].(map[string]interface{})
	if errObj["code"] != "MISSING_CHALLENGE_ID" {
		t.Errorf("Expected MISSING_CHALLENGE_ID, got %v", errObj["code"])
	}
}

func TestRegisterDevice_InvalidSignature(t *testing.T) {
	challengeStore := store.NewChallengeStore()
	defer challengeStore.Close()

	mockStore := &MockStore{}

	keyPair, _ := generateTestKeyPair()
	challenge, _ := challengeStore.CreateChallenge(context.Background())

	fakeSignature := base64.StdEncoding.EncodeToString(make([]byte, 64))

	requestBody := map[string]string{
		"device_public_key": keyPair.publicKeyB64,
		"push_token":        "apns-token-test",
		"push_platform":     "apns",
		"challenge_id":      challenge.ID,
		"signature":         fakeSignature,
	}

	body, _ := json.Marshal(requestBody)
	req := httptest.NewRequest(http.MethodPost, "/devices/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler := NewRegisterHandler(mockStore, challengeStore)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	errObj := resp["error"].(map[string]interface{})
	if errObj["code"] != "INVALID_SIGNATURE" {
		t.Errorf("Expected INVALID_SIGNATURE, got %v", errObj["code"])
	}
}

func TestRegisterDevice_ExpiredChallenge(t *testing.T) {
	challengeStore := store.NewChallengeStore()
	defer challengeStore.Close()

	mockStore := &MockStore{}

	keyPair, _ := generateTestKeyPair()

	nonExistentChallengeID := "00000000-0000-0000-0000-000000000000"
	signature, _ := keyPair.signRegistrationChallenge([]byte("fake-nonce"), "apns-token")

	requestBody := map[string]string{
		"device_public_key": keyPair.publicKeyB64,
		"push_token":        "apns-token",
		"push_platform":     "apns",
		"challenge_id":      nonExistentChallengeID,
		"signature":         signature,
	}

	body, _ := json.Marshal(requestBody)
	req := httptest.NewRequest(http.MethodPost, "/devices/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler := NewRegisterHandler(mockStore, challengeStore)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	errObj := resp["error"].(map[string]interface{})
	if errObj["code"] != "INVALID_CHALLENGE" {
		t.Errorf("Expected INVALID_CHALLENGE, got %v", errObj["code"])
	}
}

func TestRegisterDevice_InvalidPlatform(t *testing.T) {
	challengeStore := store.NewChallengeStore()
	defer challengeStore.Close()

	mockStore := &MockStore{}

	keyPair, _ := generateTestKeyPair()
	challenge, _ := challengeStore.CreateChallenge(context.Background())
	signature, _ := keyPair.signRegistrationChallenge(challenge.Nonce, "token")

	requestBody := map[string]string{
		"device_public_key": keyPair.publicKeyB64,
		"push_token":        "token",
		"push_platform":     "invalid",
		"challenge_id":      challenge.ID,
		"signature":         signature,
	}

	body, _ := json.Marshal(requestBody)
	req := httptest.NewRequest(http.MethodPost, "/devices/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler := NewRegisterHandler(mockStore, challengeStore)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	errObj := resp["error"].(map[string]interface{})
	if errObj["code"] != "INVALID_PLATFORM" {
		t.Errorf("Expected INVALID_PLATFORM, got %v", errObj["code"])
	}
}

func TestFingerprintComputation(t *testing.T) {
	publicKeyBytes := []byte{0x02, 0x12, 0x64, 0x6c, 0x6d, 0x61, 0x6f, 0x70, 0x77, 0x78, 0x79, 0x7a, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x30, 0x61, 0x62, 0x63, 0x64, 0x65, 0x66, 0x67, 0x68, 0x69, 0x6a, 0x00}

	hash := sha256.Sum256(publicKeyBytes)
	expectedFingerprint := hex.EncodeToString(hash[:])

	if len(expectedFingerprint) != 64 {
		t.Errorf("Expected fingerprint length 64, got %d", len(expectedFingerprint))
	}

	_, err := hex.DecodeString(expectedFingerprint)
	if err != nil {
		t.Errorf("Fingerprint is not valid hex: %v", err)
	}
}
