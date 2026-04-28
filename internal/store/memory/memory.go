package memory

import (
	"context"
	"sync"
	"time"

	"github.com/sigilauth/relay/internal/push"
)

type Store struct {
	mu     sync.RWMutex
	tokens map[string]*push.PushToken
}

func New() *Store {
	return &Store{
		tokens: make(map[string]*push.PushToken),
	}
}

func (s *Store) RegisterDevice(ctx context.Context, fingerprint, token, platform string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	if existing, ok := s.tokens[fingerprint]; ok {
		existing.Token = token
		existing.Platform = platform
		existing.UpdatedAt = now
		return nil
	}

	s.tokens[fingerprint] = &push.PushToken{
		Fingerprint:      fingerprint,
		Token:            token,
		Platform:         platform,
		RegisteredAt:     now,
		UpdatedAt:        now,
		LastDeliveredAt:  nil,
		DeliveryFailures: 0,
	}
	return nil
}

func (s *Store) GetPushToken(ctx context.Context, fingerprint string) (*push.PushToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	token, ok := s.tokens[fingerprint]
	if !ok {
		return nil, nil
	}

	copy := *token
	if copy.LastDeliveredAt != nil {
		val := *copy.LastDeliveredAt
		copy.LastDeliveredAt = &val
	}
	return &copy, nil
}

func (s *Store) EvictToken(ctx context.Context, fingerprint string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.tokens, fingerprint)
	return nil
}

func (s *Store) IncrementFailures(ctx context.Context, fingerprint string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if token, ok := s.tokens[fingerprint]; ok {
		token.DeliveryFailures++
	}
	return nil
}

func (s *Store) ResetFailures(ctx context.Context, fingerprint string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if token, ok := s.tokens[fingerprint]; ok {
		token.DeliveryFailures = 0
		now := time.Now().Unix()
		token.LastDeliveredAt = &now
	}
	return nil
}

func (s *Store) GetStaleTokens(ctx context.Context, failureThreshold int, staleDays int) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().AddDate(0, 0, -staleDays).Unix()
	var stale []string

	for fp, token := range s.tokens {
		if token.DeliveryFailures >= failureThreshold || token.UpdatedAt < cutoff {
			stale = append(stale, fp)
		}
	}

	return stale, nil
}
