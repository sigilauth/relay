package websocket

import (
	"context"
)

// Provider implements push.PushProvider for WebSocket delivery
type Provider struct {
	manager *Manager
}

// NewProvider creates a new WebSocket push provider
func NewProvider(manager *Manager) *Provider {
	return &Provider{
		manager: manager,
	}
}

// Send delivers a push notification via WebSocket
// Returns error if no active connection exists for the fingerprint
func (p *Provider) Send(ctx context.Context, token string, payload []byte) error {
	return p.manager.Send(ctx, token, payload)
}

// Platform returns the platform identifier for this provider
func (p *Provider) Platform() string {
	return "websocket"
}
