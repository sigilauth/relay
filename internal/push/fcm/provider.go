package fcm

import (
	"context"
	"fmt"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

// Provider implements push.PushProvider for FCM
type Provider struct {
	client *messaging.Client
}

// NewProvider creates an FCM provider from service account JSON file
func NewProvider(ctx context.Context, serviceAccountPath string) (*Provider, error) {
	opt := option.WithCredentialsFile(serviceAccountPath)
	app, err := firebase.NewApp(ctx, nil, opt)
	if err != nil {
		return nil, fmt.Errorf("firebase app: %w", err)
	}

	client, err := app.Messaging(ctx)
	if err != nil {
		return nil, fmt.Errorf("messaging client: %w", err)
	}

	return &Provider{client: client}, nil
}

// Send delivers a push notification via FCM
func (p *Provider) Send(ctx context.Context, deviceToken string, payload []byte) error {
	message := &messaging.Message{
		Token: deviceToken,
		Data: map[string]string{
			"payload": string(payload),
		},
	}

	response, err := p.client.Send(ctx, message)
	if err != nil {
		// Check for invalid token errors
		errMsg := err.Error()
		if contains(errMsg, "registration-token-not-registered") ||
			contains(errMsg, "invalid-registration-token") {
			return fmt.Errorf("NotRegistered: %s", errMsg)
		}
		return fmt.Errorf("fcm send: %w", err)
	}

	_ = response // Successfully sent
	return nil
}

// Platform returns "fcm"
func (p *Provider) Platform() string {
	return "fcm"
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s != "" && substr != "" &&
		(s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr))
}
