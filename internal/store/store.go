package store

import (
	"context"
	"github.com/sigilauth/relay/internal/push"
)

// Store defines the persistence interface for device push tokens.
// Implementations must be safe for concurrent access.
type Store interface {
	// RegisterDevice stores or updates a device's push token.
	// Last-write-wins semantics for concurrent updates.
	RegisterDevice(ctx context.Context, fingerprint, token, platform string) error

	// GetPushToken retrieves a device's push token by fingerprint.
	// Returns nil if not found.
	GetPushToken(ctx context.Context, fingerprint string) (*push.PushToken, error)

	// EvictToken removes a device's push token.
	// Used when token is invalid or device unregisters.
	EvictToken(ctx context.Context, fingerprint string) error

	// IncrementFailures increments the delivery failure count.
	// Used to track consecutive failures for eviction.
	IncrementFailures(ctx context.Context, fingerprint string) error

	// ResetFailures resets the delivery failure count to zero.
	// Called after successful delivery.
	ResetFailures(ctx context.Context, fingerprint string) error

	// GetStaleTokens returns fingerprints with >N consecutive failures
	// or not updated in >days. For cleanup job.
	GetStaleTokens(ctx context.Context, failureThreshold int, staleDays int) ([]string, error)
}
