package store

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	challengeTTL        = 5 * time.Minute
	cleanupInterval     = 1 * time.Minute
	challengeByteLength = 32
)

type Challenge struct {
	ID        string
	Nonce     []byte
	CreatedAt time.Time
}

type ChallengeStore struct {
	mu         sync.RWMutex
	challenges map[string]*Challenge
	stopChan   chan struct{}
}

func NewChallengeStore() *ChallengeStore {
	cs := &ChallengeStore{
		challenges: make(map[string]*Challenge),
		stopChan:   make(chan struct{}),
	}
	go cs.cleanup()
	return cs
}

func (cs *ChallengeStore) CreateChallenge(ctx context.Context) (*Challenge, error) {
	nonce := make([]byte, challengeByteLength)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate challenge nonce: %w", err)
	}

	challenge := &Challenge{
		ID:        uuid.New().String(),
		Nonce:     nonce,
		CreatedAt: time.Now().UTC(),
	}

	cs.mu.Lock()
	cs.challenges[challenge.ID] = challenge
	cs.mu.Unlock()

	return challenge, nil
}

func (cs *ChallengeStore) ConsumeChallenge(ctx context.Context, challengeID string) (*Challenge, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	challenge, exists := cs.challenges[challengeID]
	if !exists {
		return nil, fmt.Errorf("challenge not found or already consumed")
	}

	if time.Since(challenge.CreatedAt) > challengeTTL {
		delete(cs.challenges, challengeID)
		return nil, fmt.Errorf("challenge expired")
	}

	delete(cs.challenges, challengeID)
	return challenge, nil
}

func (cs *ChallengeStore) cleanup() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cs.mu.Lock()
			now := time.Now().UTC()
			for id, challenge := range cs.challenges {
				if now.Sub(challenge.CreatedAt) > challengeTTL {
					delete(cs.challenges, id)
				}
			}
			cs.mu.Unlock()
		case <-cs.stopChan:
			return
		}
	}
}

func (cs *ChallengeStore) Close() {
	close(cs.stopChan)
}

func (c *Challenge) NonceBase64() string {
	return base64.StdEncoding.EncodeToString(c.Nonce)
}
