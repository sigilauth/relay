package replay

import (
	"sync"
	"time"
)

type NonceStore struct {
	mu      sync.RWMutex
	nonces  map[string]time.Time
	ttl     time.Duration
	stopCh  chan struct{}
	stopped bool
}

func NewNonceStore(ttl time.Duration) *NonceStore {
	store := &NonceStore{
		nonces: make(map[string]time.Time),
		ttl:    ttl,
		stopCh: make(chan struct{}),
	}
	go store.cleanup()
	return store
}

func (s *NonceStore) CheckAndInsert(nonce string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.nonces[nonce]; exists {
		return false
	}

	s.nonces[nonce] = time.Now()
	return true
}

func (s *NonceStore) cleanup() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.removeExpired()
		case <-s.stopCh:
			return
		}
	}
}

func (s *NonceStore) removeExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for nonce, timestamp := range s.nonces {
		if now.Sub(timestamp) > s.ttl {
			delete(s.nonces, nonce)
		}
	}
}

func (s *NonceStore) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.stopped {
		close(s.stopCh)
		s.stopped = true
	}
}
