package stdout

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

type Provider struct {
	platform string
}

func New(platform string) *Provider {
	if platform == "" {
		platform = "mock"
	}
	return &Provider{platform: platform}
}

func (p *Provider) Send(ctx context.Context, token string, payload []byte) error {
	event := map[string]interface{}{
		"event":         "push_dispatched",
		"token":         token,
		"payload":       json.RawMessage(payload),
		"would_send_via": p.platform,
	}

	output, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal mock push event: %w", err)
	}

	fmt.Println(string(output))
	slog.Info("Mock push dispatched", "token_prefix", truncate(token, 16), "platform", p.platform)
	return nil
}

func (p *Provider) Platform() string {
	return p.platform
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
