package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sigilauth/relay/internal/push"
	"github.com/sigilauth/relay/internal/ratelimit"
	"github.com/sigilauth/relay/internal/replay"
	"github.com/sigilauth/relay/internal/store"
	"github.com/sigilauth/relay/internal/types"
	"github.com/sigilauth/relay/internal/verify"
)

type PushHandler struct {
	store      store.Store
	provider   push.PushProvider
	limiter    *ratelimit.Limiter
	verifier   *verify.ServerSignature
	nonceStore *replay.NonceStore
}

func NewPushHandler(s store.Store, p push.PushProvider, l *ratelimit.Limiter, v *verify.ServerSignature, ns *replay.NonceStore) *PushHandler {
	if v == nil {
		panic("relay: verifier must not be nil — signature verification is required for production security")
	}

	return &PushHandler{
		store:      s,
		provider:   p,
		limiter:    l,
		verifier:   v,
		nonceStore: ns,
	}
}

func (h *PushHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req types.PushRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Malformed JSON request")
		return
	}

	ctx := r.Context()

	// Rate limiting (10 req/min per fingerprint)
	if h.limiter != nil && !h.limiter.Allow(req.Fingerprint) {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Rate limit exceeded. Retry after 60 seconds.")
		return
	}

	ts, err := strconv.ParseInt(req.Timestamp, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_TIMESTAMP", "Timestamp must be Unix epoch in seconds")
		return
	}

	now := time.Now().Unix()
	diff := now - ts
	if diff < 0 {
		diff = -diff
	}
	if diff > 60 {
		writeError(w, http.StatusForbidden, "TIMESTAMP_EXPIRED", "Request timestamp outside 60s window")
		return
	}

	signaturePayload := fmt.Sprintf("SIGIL-RELAY-PUSH-V1\x00%s\x00%s\x00%s", req.ServerID, req.Fingerprint, req.Timestamp)
	if err := h.verifier.Verify([]byte(signaturePayload), req.RequestSignature); err != nil {
		writeError(w, http.StatusForbidden, "INVALID_SIGNATURE", "Signature verification failed")
		return
	}

	if h.nonceStore != nil {
		nonceKey := fmt.Sprintf("%s|%s|%s", req.ServerID, req.Fingerprint, req.Timestamp)
		if !h.nonceStore.CheckAndInsert(nonceKey) {
			writeError(w, http.StatusForbidden, "REPLAY_DETECTED", "Request has already been processed")
			return
		}
	}

	// Lookup push token
	pushToken, err := h.store.GetPushToken(ctx, req.Fingerprint)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "Failed to lookup push token")
		return
	}

	if pushToken == nil {
		writeError(w, http.StatusNotFound, "FINGERPRINT_NOT_FOUND", "Device not registered")
		return
	}

	// Marshal payload for push notification
	payloadBytes, err := json.Marshal(req.Payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PAYLOAD_ERROR", "Failed to marshal payload")
		return
	}

	// Deliver push notification
	if err := h.provider.Send(ctx, pushToken.Token, payloadBytes); err != nil {
		// Check if error indicates invalid token
		if isInvalidTokenError(err) {
			// Evict token and return success (per spec)
			h.store.EvictToken(ctx, req.Fingerprint)
			writeError(w, http.StatusOK, "TOKEN_EVICTED", "Invalid push token evicted")
			return
		}

		// Increment failure counter
		h.store.IncrementFailures(ctx, req.Fingerprint)

		// Return 502 Bad Gateway for APNs/FCM unreachable
		writeError(w, http.StatusBadGateway, "PUSH_FAILED", fmt.Sprintf("Push delivery failed: %v", err))
		return
	}

	// Success - reset failure counter
	h.store.ResetFailures(ctx, req.Fingerprint)

	// Return success response
	resp := types.PushResponse{
		Status:      "delivered",
		PushID:      uuid.New().String(),
		DeliveredAt: time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

func isInvalidTokenError(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "BadDeviceToken") ||
		strings.Contains(errStr, "NotRegistered") ||
		strings.Contains(errStr, "Unregistered")
}
