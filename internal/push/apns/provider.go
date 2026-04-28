package apns

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"

	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/token"
)

// Provider implements push.PushProvider for APNs
type Provider struct {
	client *apns2.Client
}

// NewProvider creates an APNs provider from .p8 key file
func NewProvider(keyPath, keyID, teamID string, production bool) (*Provider, error) {
	// Read .p8 key file
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read key file: %w", err)
	}

	// Parse PEM
	block, _ := pem.Decode(keyBytes)
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block")
	}

	// Parse ECDSA private key
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	ecdsaKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("key is not ECDSA")
	}

	// Create token provider
	authKey := &token.Token{
		AuthKey: ecdsaKey,
		KeyID:   keyID,
		TeamID:  teamID,
	}

	// Create client
	client := apns2.NewTokenClient(authKey)
	if production {
		client = client.Production()
	} else {
		client = client.Development()
	}

	return &Provider{client: client}, nil
}

// Send delivers a push notification via APNs
func (p *Provider) Send(ctx context.Context, deviceToken string, payload []byte) error {
	notification := &apns2.Notification{
		DeviceToken: deviceToken,
		Payload:     payload,
	}

	res, err := p.client.PushWithContext(ctx, notification)
	if err != nil {
		return fmt.Errorf("apns push: %w", err)
	}

	// Check response
	if res.StatusCode != 200 {
		// Map APNs error reasons to our error types
		if res.Reason == apns2.ReasonBadDeviceToken ||
			res.Reason == apns2.ReasonUnregistered ||
			res.Reason == apns2.ReasonDeviceTokenNotForTopic {
			return fmt.Errorf("BadDeviceToken: %s", res.Reason)
		}
		return fmt.Errorf("apns error: %s (code %d)", res.Reason, res.StatusCode)
	}

	return nil
}

// Platform returns "apns"
func (p *Provider) Platform() string {
	return "apns"
}
