package push

import (
	"context"
	"testing"
)

// MockPushProvider is a test double for PushProvider
type MockPushProvider struct {
	SendFunc     func(ctx context.Context, token string, payload []byte) error
	PlatformFunc func() string
}

func (m *MockPushProvider) Send(ctx context.Context, token string, payload []byte) error {
	if m.SendFunc != nil {
		return m.SendFunc(ctx, token, payload)
	}
	return nil
}

func (m *MockPushProvider) Platform() string {
	if m.PlatformFunc != nil {
		return m.PlatformFunc()
	}
	return "mock"
}

func TestMockPushProvider(t *testing.T) {
	mock := &MockPushProvider{
		SendFunc: func(ctx context.Context, token string, payload []byte) error {
			return nil
		},
		PlatformFunc: func() string {
			return "test-platform"
		},
	}

	ctx := context.Background()
	err := mock.Send(ctx, "test-token", []byte("test-payload"))
	if err != nil {
		t.Errorf("Send() error = %v, want nil", err)
	}

	platform := mock.Platform()
	if platform != "test-platform" {
		t.Errorf("Platform() = %q, want %q", platform, "test-platform")
	}
}
