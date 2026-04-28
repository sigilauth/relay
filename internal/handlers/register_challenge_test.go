package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sigilauth/relay/internal/store"
	"github.com/sigilauth/relay/internal/types"
)

func TestRegisterChallenge_Success(t *testing.T) {
	challengeStore := store.NewChallengeStore()
	defer challengeStore.Close()

	req := httptest.NewRequest(http.MethodGet, "/devices/register/challenge", nil)
	w := httptest.NewRecorder()

	handler := NewRegisterChallengeHandler(challengeStore)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp types.RegistrationChallengeResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.ChallengeID == "" {
		t.Error("Expected non-empty challenge_id")
	}

	if resp.Nonce == "" {
		t.Error("Expected non-empty nonce")
	}
}

func TestRegisterChallenge_MethodNotAllowed(t *testing.T) {
	challengeStore := store.NewChallengeStore()
	defer challengeStore.Close()

	req := httptest.NewRequest(http.MethodPost, "/devices/register/challenge", nil)
	w := httptest.NewRecorder()

	handler := NewRegisterChallengeHandler(challengeStore)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}
