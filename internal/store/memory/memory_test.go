package memory

import (
	"context"
	"testing"
	"time"
)

func TestRegisterDevice(t *testing.T) {
	s := New()
	ctx := context.Background()

	err := s.RegisterDevice(ctx, "fp1", "token1", "apns")
	if err != nil {
		t.Fatalf("RegisterDevice failed: %v", err)
	}

	token, err := s.GetPushToken(ctx, "fp1")
	if err != nil {
		t.Fatalf("GetPushToken failed: %v", err)
	}
	if token == nil {
		t.Fatal("Expected token, got nil")
	}
	if token.Token != "token1" {
		t.Errorf("Expected token1, got %s", token.Token)
	}
	if token.Platform != "apns" {
		t.Errorf("Expected apns, got %s", token.Platform)
	}
}

func TestRegisterDeviceUpdate(t *testing.T) {
	s := New()
	ctx := context.Background()

	s.RegisterDevice(ctx, "fp1", "token1", "apns")
	first, _ := s.GetPushToken(ctx, "fp1")
	firstUpdated := first.UpdatedAt

	time.Sleep(1100 * time.Millisecond)
	s.RegisterDevice(ctx, "fp1", "token2", "fcm")

	token, _ := s.GetPushToken(ctx, "fp1")
	if token.Token != "token2" {
		t.Errorf("Expected token2, got %s", token.Token)
	}
	if token.Platform != "fcm" {
		t.Errorf("Expected fcm, got %s", token.Platform)
	}
	if token.UpdatedAt <= firstUpdated {
		t.Errorf("UpdatedAt (%d) should be greater than first UpdatedAt (%d)", token.UpdatedAt, firstUpdated)
	}
}

func TestGetPushTokenNotFound(t *testing.T) {
	s := New()
	ctx := context.Background()

	token, err := s.GetPushToken(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetPushToken failed: %v", err)
	}
	if token != nil {
		t.Errorf("Expected nil token for nonexistent fingerprint")
	}
}

func TestEvictToken(t *testing.T) {
	s := New()
	ctx := context.Background()

	s.RegisterDevice(ctx, "fp1", "token1", "apns")
	s.EvictToken(ctx, "fp1")

	token, _ := s.GetPushToken(ctx, "fp1")
	if token != nil {
		t.Error("Expected nil after eviction")
	}
}

func TestIncrementFailures(t *testing.T) {
	s := New()
	ctx := context.Background()

	s.RegisterDevice(ctx, "fp1", "token1", "apns")
	s.IncrementFailures(ctx, "fp1")
	s.IncrementFailures(ctx, "fp1")

	token, _ := s.GetPushToken(ctx, "fp1")
	if token.DeliveryFailures != 2 {
		t.Errorf("Expected 2 failures, got %d", token.DeliveryFailures)
	}
}

func TestResetFailures(t *testing.T) {
	s := New()
	ctx := context.Background()

	s.RegisterDevice(ctx, "fp1", "token1", "apns")
	s.IncrementFailures(ctx, "fp1")
	s.ResetFailures(ctx, "fp1")

	token, _ := s.GetPushToken(ctx, "fp1")
	if token.DeliveryFailures != 0 {
		t.Errorf("Expected 0 failures after reset, got %d", token.DeliveryFailures)
	}
	if token.LastDeliveredAt == nil {
		t.Error("Expected LastDeliveredAt to be set after reset")
	}
}

func TestGetStaleTokens(t *testing.T) {
	s := New()
	ctx := context.Background()

	s.RegisterDevice(ctx, "fp1", "token1", "apns")
	s.RegisterDevice(ctx, "fp2", "token2", "fcm")
	s.RegisterDevice(ctx, "fp3", "token3", "apns")

	for i := 0; i < 12; i++ {
		s.IncrementFailures(ctx, "fp1")
	}

	s.tokens["fp3"].UpdatedAt = time.Now().AddDate(0, 0, -100).Unix()

	stale, err := s.GetStaleTokens(ctx, 10, 90)
	if err != nil {
		t.Fatalf("GetStaleTokens failed: %v", err)
	}

	if len(stale) != 2 {
		t.Errorf("Expected 2 stale tokens, got %d", len(stale))
	}

	staleMap := make(map[string]bool)
	for _, fp := range stale {
		staleMap[fp] = true
	}

	if !staleMap["fp1"] || !staleMap["fp3"] {
		t.Error("Expected fp1 and fp3 to be stale")
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := New()
	ctx := context.Background()

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			fp := "fp"
			s.RegisterDevice(ctx, fp, "token", "apns")
			s.GetPushToken(ctx, fp)
			s.IncrementFailures(ctx, fp)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
