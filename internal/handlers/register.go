package handlers

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"github.com/sigilauth/relay/internal/pictogram"
	"github.com/sigilauth/relay/internal/store"
	"github.com/sigilauth/relay/internal/types"
)

const registrationDomainTag = "SIGIL-RELAY-REGISTER-V1\x00"

type RegisterHandler struct {
	store          store.Store
	challengeStore *store.ChallengeStore
}

func NewRegisterHandler(s store.Store, cs *store.ChallengeStore) *RegisterHandler {
	return &RegisterHandler{
		store:          s,
		challengeStore: cs,
	}
}

func (h *RegisterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req types.DeviceRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Malformed JSON request")
		return
	}

	if req.PushPlatform != "apns" && req.PushPlatform != "fcm" {
		writeError(w, http.StatusBadRequest, "INVALID_PLATFORM", "Platform must be 'apns' or 'fcm'")
		return
	}

	if req.ChallengeID == "" {
		writeError(w, http.StatusBadRequest, "MISSING_CHALLENGE_ID", "challenge_id is required")
		return
	}

	if req.Signature == "" {
		writeError(w, http.StatusBadRequest, "MISSING_SIGNATURE", "signature is required")
		return
	}

	publicKeyBytes, err := base64.StdEncoding.DecodeString(req.DevicePublicKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PUBLIC_KEY", "Public key is not valid base64")
		return
	}

	if len(publicKeyBytes) != 33 {
		writeError(w, http.StatusBadRequest, "INVALID_PUBLIC_KEY", fmt.Sprintf("Public key must be 33 bytes, got %d", len(publicKeyBytes)))
		return
	}

	ctx := r.Context()

	challenge, err := h.challengeStore.ConsumeChallenge(ctx, req.ChallengeID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "INVALID_CHALLENGE", err.Error())
		return
	}

	if err := verifyRegistrationSignature(publicKeyBytes, challenge.Nonce, req.PushToken, req.Signature); err != nil {
		writeError(w, http.StatusUnauthorized, "INVALID_SIGNATURE", err.Error())
		return
	}

	hash := sha256.Sum256(publicKeyBytes)
	fingerprint := hex.EncodeToString(hash[:])

	fingerprintBytes, _ := hex.DecodeString(fingerprint)
	pictogramArray, err := pictogram.Derive(fingerprintBytes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PICTOGRAM_ERROR", "Failed to derive pictogram")
		return
	}
	pictogramSpeakable := pictogram.Speakable(pictogramArray)

	if err := h.store.RegisterDevice(ctx, fingerprint, req.PushToken, req.PushPlatform); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "Failed to register device")
		return
	}

	resp := types.DeviceRegisterResponse{
		Fingerprint:        fingerprint,
		Pictogram:          pictogramArray,
		PictogramSpeakable: pictogramSpeakable,
		RegisteredAt:       time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func verifyRegistrationSignature(publicKeyBytes, nonce []byte, pushToken, signatureB64 string) error {
	x, y := elliptic.UnmarshalCompressed(elliptic.P256(), publicKeyBytes)
	if x == nil {
		return fmt.Errorf("invalid P-256 public key")
	}

	publicKey := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     x,
		Y:     y,
	}

	sigBytes, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}

	if len(sigBytes) != 64 {
		return fmt.Errorf("invalid signature length: got %d, want 64", len(sigBytes))
	}

	r := new(big.Int).SetBytes(sigBytes[:32])
	s := new(big.Int).SetBytes(sigBytes[32:])

	curveOrder := elliptic.P256().Params().N
	halfOrder := new(big.Int).Div(curveOrder, big.NewInt(2))

	if s.Cmp(halfOrder) > 0 {
		return fmt.Errorf("signature has high S value (malleability risk)")
	}

	var message bytes.Buffer
	message.WriteString(registrationDomainTag)
	message.Write(nonce)
	message.WriteString(pushToken)

	hash := sha256.Sum256(message.Bytes())

	if !ecdsa.Verify(publicKey, hash[:], r, s) {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(types.ErrorResponse{
		Error: types.ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}
