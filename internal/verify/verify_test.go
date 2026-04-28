package verify

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"math/big"
	"testing"
)

// TestNewServerSignature tests creation from hex or base64 public key
func TestNewServerSignature(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	compressedBytes := elliptic.MarshalCompressed(elliptic.P256(), privateKey.PublicKey.X, privateKey.PublicKey.Y)
	compressedHex := hex.EncodeToString(compressedBytes)
	compressedB64 := base64.StdEncoding.EncodeToString(compressedBytes)

	uncompressedBytes := elliptic.Marshal(elliptic.P256(), privateKey.PublicKey.X, privateKey.PublicKey.Y)
	uncompressedHex := hex.EncodeToString(uncompressedBytes)
	uncompressedB64 := base64.StdEncoding.EncodeToString(uncompressedBytes)

	tests := []struct {
		name      string
		publicKey string
		wantErr   bool
		wantLen   int
	}{
		{
			name:      "compressed hex (33 bytes)",
			publicKey: compressedHex,
			wantErr:   false,
			wantLen:   33,
		},
		{
			name:      "compressed base64 (33 bytes)",
			publicKey: compressedB64,
			wantErr:   false,
			wantLen:   33,
		},
		{
			name:      "uncompressed hex (65 bytes)",
			publicKey: uncompressedHex,
			wantErr:   false,
			wantLen:   65,
		},
		{
			name:      "uncompressed base64 (65 bytes)",
			publicKey: uncompressedB64,
			wantErr:   false,
			wantLen:   65,
		},
		{
			name:      "invalid hex",
			publicKey: "not-hex-zzz",
			wantErr:   true,
		},
		{
			name:      "invalid base64",
			publicKey: "not-base64!!!",
			wantErr:   true,
		},
		{
			name:      "wrong length hex",
			publicKey: hex.EncodeToString([]byte("short")),
			wantErr:   true,
		},
		{
			name:      "wrong length base64",
			publicKey: base64.StdEncoding.EncodeToString([]byte("short")),
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v, err := NewServerSignature(tc.publicKey)
			if (err != nil) != tc.wantErr {
				t.Errorf("NewServerSignature() error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr {
				if len(v.RawBytes) != tc.wantLen {
					t.Errorf("Expected RawBytes length %d, got %d", tc.wantLen, len(v.RawBytes))
				}
			}
		})
	}
}

// TestVerifySignature tests ECDSA signature verification
func TestVerifySignature(t *testing.T) {
	// Generate test keypair
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	// Compress public key
	pubKeyBytes := elliptic.MarshalCompressed(elliptic.P256(), privateKey.PublicKey.X, privateKey.PublicKey.Y)
	pubKeyB64 := base64.StdEncoding.EncodeToString(pubKeyBytes)

	payload := []byte("test payload")
	hash := sha256.Sum256(payload)

	// Sign payload
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, hash[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	// Normalize to low-S (BIP-62 compliance)
	curveOrder := elliptic.P256().Params().N
	halfOrder := new(big.Int).Div(curveOrder, big.NewInt(2))
	if s.Cmp(halfOrder) > 0 {
		s = new(big.Int).Sub(curveOrder, s)
	}

	// Encode signature as r || s
	sigBytes := make([]byte, 64)
	r.FillBytes(sigBytes[:32])
	s.FillBytes(sigBytes[32:])
	sigB64 := base64.StdEncoding.EncodeToString(sigBytes)

	// Create verifier
	verifier, err := NewServerSignature(pubKeyB64)
	if err != nil {
		t.Fatalf("NewServerSignature: %v", err)
	}

	tests := []struct {
		name      string
		payload   []byte
		signature string
		wantErr   bool
	}{
		{
			name:      "valid signature",
			payload:   payload,
			signature: sigB64,
			wantErr:   false,
		},
		{
			name:      "wrong payload",
			payload:   []byte("different payload"),
			signature: sigB64,
			wantErr:   true,
		},
		{
			name:      "invalid signature base64",
			payload:   payload,
			signature: "not-base64!!!",
			wantErr:   true,
		},
		{
			name:      "wrong signature length",
			payload:   payload,
			signature: base64.StdEncoding.EncodeToString([]byte("short")),
			wantErr:   true,
		},
		{
			name:      "random signature",
			payload:   payload,
			signature: base64.StdEncoding.EncodeToString(make([]byte, 64)),
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := verifier.Verify(tc.payload, tc.signature)
			if (err != nil) != tc.wantErr {
				t.Errorf("Verify() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// TestSignatureMalleability tests that high-S signatures are rejected per BIP-62
func TestSignatureMalleability(t *testing.T) {
	// Generate test keypair
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	// Compress public key
	pubKeyBytes := elliptic.MarshalCompressed(elliptic.P256(), privateKey.PublicKey.X, privateKey.PublicKey.Y)
	pubKeyB64 := base64.StdEncoding.EncodeToString(pubKeyBytes)

	payload := []byte("test malleability payload")
	hash := sha256.Sum256(payload)

	// Sign payload
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, hash[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	// P-256 curve order n
	curveOrder := elliptic.P256().Params().N
	halfOrder := new(big.Int).Div(curveOrder, big.NewInt(2))

	// Normalize s to low-S first (in case ecdsa.Sign already did this)
	if s.Cmp(halfOrder) > 0 {
		s = new(big.Int).Sub(curveOrder, s)
	}

	// Now create high-S signature by flipping s to n - s
	// This creates a malleable signature that's mathematically valid but violates BIP-62
	highS := new(big.Int).Sub(curveOrder, s)

	// Verify we have a high-S value (s > n/2)
	if highS.Cmp(halfOrder) <= 0 {
		t.Fatalf("Test setup failed: highS (%v) should be > halfOrder (%v)", highS, halfOrder)
	}

	// Encode high-S signature as r || highS
	highSigBytes := make([]byte, 64)
	r.FillBytes(highSigBytes[:32])
	highS.FillBytes(highSigBytes[32:])
	highSigB64 := base64.StdEncoding.EncodeToString(highSigBytes)

	// Create verifier
	verifier, err := NewServerSignature(pubKeyB64)
	if err != nil {
		t.Fatalf("NewServerSignature: %v", err)
	}

	// Test: high-S signature MUST be rejected
	err = verifier.Verify(payload, highSigB64)
	if err == nil {
		t.Fatal("Expected high-S signature to be rejected, but it was accepted (SIG-2026-001)")
	}

	// Error message should indicate high-S rejection
	errMsg := err.Error()
	if errMsg != "signature has high S value (malleability risk)" {
		t.Errorf("Expected 'signature has high S value (malleability risk)', got: %v", errMsg)
	}

	// Also verify that the low-S version (original s) would be accepted
	lowSigBytes := make([]byte, 64)
	r.FillBytes(lowSigBytes[:32])
	s.FillBytes(lowSigBytes[32:])
	lowSigB64 := base64.StdEncoding.EncodeToString(lowSigBytes)

	err = verifier.Verify(payload, lowSigB64)
	if err != nil {
		t.Fatalf("Low-S signature should be accepted, got error: %v", err)
	}
}
