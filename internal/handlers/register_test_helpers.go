package handlers

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/big"

	"github.com/codahale/rfc6979"
)

type testKeyPair struct {
	privateKey *ecdsa.PrivateKey
	publicKey  *ecdsa.PublicKey
	publicKeyCompressed []byte
	publicKeyB64 string
}

func generateTestKeyPair() (*testKeyPair, error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	compressedPubKey := elliptic.MarshalCompressed(elliptic.P256(), privKey.PublicKey.X, privKey.PublicKey.Y)

	return &testKeyPair{
		privateKey: privKey,
		publicKey: &privKey.PublicKey,
		publicKeyCompressed: compressedPubKey,
		publicKeyB64: base64.StdEncoding.EncodeToString(compressedPubKey),
	}, nil
}

func (kp *testKeyPair) signRegistrationChallenge(nonce []byte, pushToken string) (string, error) {
	var message bytes.Buffer
	message.WriteString(registrationDomainTag)
	message.Write(nonce)
	message.WriteString(pushToken)

	hash := sha256.Sum256(message.Bytes())

	r, s, err := rfc6979.SignECDSA(kp.privateKey, hash[:], sha256.New)
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}

	curveOrder := elliptic.P256().Params().N
	halfOrder := new(big.Int).Div(curveOrder, big.NewInt(2))
	if s.Cmp(halfOrder) > 0 {
		s = new(big.Int).Sub(curveOrder, s)
	}

	signature := make([]byte, 64)
	rBytes := r.Bytes()
	sBytes := s.Bytes()

	copy(signature[32-len(rBytes):32], rBytes)
	copy(signature[64-len(sBytes):64], sBytes)

	return base64.StdEncoding.EncodeToString(signature), nil
}
