package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/sigilauth/relay/internal/store"
	"github.com/sigilauth/relay/internal/types"
)

type RegisterChallengeHandler struct {
	challengeStore *store.ChallengeStore
}

func NewRegisterChallengeHandler(cs *store.ChallengeStore) *RegisterChallengeHandler {
	return &RegisterChallengeHandler{challengeStore: cs}
}

func (h *RegisterChallengeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	challenge, err := h.challengeStore.CreateChallenge(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CHALLENGE_GENERATION_FAILED", "Failed to generate registration challenge")
		return
	}

	resp := types.RegistrationChallengeResponse{
		ChallengeID: challenge.ID,
		Nonce:       challenge.NonceBase64(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}
