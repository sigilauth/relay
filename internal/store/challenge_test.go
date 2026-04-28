package store

import (
	"context"
	"testing"
	"time"
)

func TestChallengeStore_CreateAndConsume(t *testing.T) {
	cs := NewChallengeStore()
	defer cs.Close()

	ctx := context.Background()

	challenge, err := cs.CreateChallenge(ctx)
	if err != nil {
		t.Fatalf("CreateChallenge failed: %v", err)
	}

	if challenge.ID == "" {
		t.Error("Expected non-empty challenge ID")
	}

	if len(challenge.Nonce) != challengeByteLength {
		t.Errorf("Expected nonce length %d, got %d", challengeByteLength, len(challenge.Nonce))
	}

	if challenge.NonceBase64() == "" {
		t.Error("Expected non-empty base64 nonce")
	}

	consumed, err := cs.ConsumeChallenge(ctx, challenge.ID)
	if err != nil {
		t.Fatalf("ConsumeChallenge failed: %v", err)
	}

	if consumed.ID != challenge.ID {
		t.Errorf("Expected challenge ID %s, got %s", challenge.ID, consumed.ID)
	}

	_, err = cs.ConsumeChallenge(ctx, challenge.ID)
	if err == nil {
		t.Error("Expected error when consuming challenge twice")
	}
}

func TestChallengeStore_NonExistent(t *testing.T) {
	cs := NewChallengeStore()
	defer cs.Close()

	ctx := context.Background()

	_, err := cs.ConsumeChallenge(ctx, "non-existent-id")
	if err == nil {
		t.Error("Expected error for non-existent challenge")
	}
}

func TestChallengeStore_Cleanup(t *testing.T) {
	cs := NewChallengeStore()
	defer cs.Close()

	ctx := context.Background()

	challenge, _ := cs.CreateChallenge(ctx)

	cs.mu.Lock()
	challenge.CreatedAt = time.Now().UTC().Add(-10 * time.Minute)
	cs.mu.Unlock()

	time.Sleep(2 * cleanupInterval)

	_, err := cs.ConsumeChallenge(ctx, challenge.ID)
	if err == nil {
		t.Error("Expected expired challenge to be cleaned up")
	}
}
