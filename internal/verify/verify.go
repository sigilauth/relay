package verify

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
)

// ServerSignature verifies ECDSA P-256 signatures from Sigil server.
type ServerSignature struct {
	publicKey *ecdsa.PublicKey
	RawBytes  []byte
}

// NewServerSignature creates a verifier from hex or base64-encoded P-256 public key.
// Accepts 33-byte (compressed) or 65-byte (uncompressed) keys in either format.
func NewServerSignature(publicKeyStr string) (*ServerSignature, error) {
	pkBytes, err := parsePublicKey(publicKeyStr)
	if err != nil {
		return nil, err
	}

	var x, y *big.Int
	if len(pkBytes) == 33 {
		x, y = elliptic.UnmarshalCompressed(elliptic.P256(), pkBytes)
	} else if len(pkBytes) == 65 {
		x, y = elliptic.Unmarshal(elliptic.P256(), pkBytes)
	} else {
		return nil, fmt.Errorf("invalid public key length: got %d, want 33 (compressed) or 65 (uncompressed)", len(pkBytes))
	}

	if x == nil {
		return nil, fmt.Errorf("invalid P-256 public key")
	}

	publicKey := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     x,
		Y:     y,
	}

	return &ServerSignature{
		publicKey: publicKey,
		RawBytes:  pkBytes,
	}, nil
}

func parsePublicKey(s string) ([]byte, error) {
	if b, err := base64.StdEncoding.DecodeString(s); err == nil && (len(b) == 33 || len(b) == 65) {
		return b, nil
	}

	if b, err := hex.DecodeString(s); err == nil && (len(b) == 33 || len(b) == 65) {
		return b, nil
	}

	return nil, fmt.Errorf("SERVER_PUBLIC_KEY must be hex or base64 of a 33-byte (compressed) or 65-byte (uncompressed) P-256 pubkey")
}

// Verify checks an ECDSA signature against a payload hash.
// signatureB64 must be 64 bytes (r || s), base64-encoded.
func (sv *ServerSignature) Verify(payload []byte, signatureB64 string) error {
	sigBytes, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}

	if len(sigBytes) != 64 {
		return fmt.Errorf("invalid signature length: got %d, want 64", len(sigBytes))
	}

	r := new(big.Int).SetBytes(sigBytes[:32])
	s := new(big.Int).SetBytes(sigBytes[32:])

	// BIP-62 compliance: Reject high-S signatures to prevent malleability (SIG-2026-001)
	// S must be <= n/2 where n is the curve order
	curveOrder := elliptic.P256().Params().N
	halfOrder := new(big.Int).Div(curveOrder, big.NewInt(2))

	if s.Cmp(halfOrder) > 0 {
		return fmt.Errorf("signature has high S value (malleability risk)")
	}

	hash := sha256.Sum256(payload)

	if !ecdsa.Verify(sv.publicKey, hash[:], r, s) {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}
