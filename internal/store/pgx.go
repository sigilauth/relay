package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sigilauth/relay/internal/crypto"
	"github.com/sigilauth/relay/internal/push"
)

type PgxStore struct {
	pool      *pgxpool.Pool
	encryptor *crypto.TokenEncryptor
}

func NewPgxStore(ctx context.Context, connString string, encryptor *crypto.TokenEncryptor) (*PgxStore, error) {
	if encryptor == nil {
		return nil, fmt.Errorf("token encryptor is required")
	}

	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &PgxStore{
		pool:      pool,
		encryptor: encryptor,
	}, nil
}

// Close closes the connection pool
func (s *PgxStore) Close() {
	s.pool.Close()
}

func (s *PgxStore) RegisterDevice(ctx context.Context, fingerprint, token, platform string) error {
	encryptedToken, err := s.encryptor.Encrypt(token)
	if err != nil {
		return fmt.Errorf("encrypt token: %w", err)
	}

	query := `
		INSERT INTO device_push_tokens (fingerprint, push_token, platform, registered_at, updated_at)
		VALUES ($1, $2, $3, now(), now())
		ON CONFLICT (fingerprint)
		DO UPDATE SET
			push_token = EXCLUDED.push_token,
			platform = EXCLUDED.platform,
			updated_at = now(),
			delivery_failures = 0
	`

	_, err = s.pool.Exec(ctx, query, fingerprint, encryptedToken, platform)
	if err != nil {
		return fmt.Errorf("register device: %w", err)
	}

	return nil
}

func (s *PgxStore) GetPushToken(ctx context.Context, fingerprint string) (*push.PushToken, error) {
	query := `
		SELECT fingerprint, push_token, platform,
		       extract(epoch from registered_at)::bigint,
		       extract(epoch from updated_at)::bigint,
		       extract(epoch from last_delivered_at)::bigint,
		       delivery_failures
		FROM device_push_tokens
		WHERE fingerprint = $1
	`

	var pt push.PushToken
	var encryptedToken string
	var lastDeliveredAt *int64

	err := s.pool.QueryRow(ctx, query, fingerprint).Scan(
		&pt.Fingerprint,
		&encryptedToken,
		&pt.Platform,
		&pt.RegisteredAt,
		&pt.UpdatedAt,
		&lastDeliveredAt,
		&pt.DeliveryFailures,
	)

	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("get push token: %w", err)
	}

	decryptedToken, err := s.encryptor.Decrypt(encryptedToken)
	if err != nil {
		return nil, fmt.Errorf("decrypt token: %w", err)
	}

	pt.Token = decryptedToken
	pt.LastDeliveredAt = lastDeliveredAt

	return &pt, nil
}

// EvictToken removes a device's push token
func (s *PgxStore) EvictToken(ctx context.Context, fingerprint string) error {
	query := `DELETE FROM device_push_tokens WHERE fingerprint = $1`

	_, err := s.pool.Exec(ctx, query, fingerprint)
	if err != nil {
		return fmt.Errorf("evict token: %w", err)
	}

	return nil
}

// IncrementFailures increments the delivery failure count
func (s *PgxStore) IncrementFailures(ctx context.Context, fingerprint string) error {
	query := `
		UPDATE device_push_tokens
		SET delivery_failures = delivery_failures + 1
		WHERE fingerprint = $1
	`

	_, err := s.pool.Exec(ctx, query, fingerprint)
	if err != nil {
		return fmt.Errorf("increment failures: %w", err)
	}

	return nil
}

// ResetFailures resets the delivery failure count to zero
func (s *PgxStore) ResetFailures(ctx context.Context, fingerprint string) error {
	query := `
		UPDATE device_push_tokens
		SET delivery_failures = 0,
		    last_delivered_at = now()
		WHERE fingerprint = $1
	`

	_, err := s.pool.Exec(ctx, query, fingerprint)
	if err != nil {
		return fmt.Errorf("reset failures: %w", err)
	}

	return nil
}

// GetStaleTokens returns fingerprints with >N consecutive failures or not updated in >days
func (s *PgxStore) GetStaleTokens(ctx context.Context, failureThreshold int, staleDays int) ([]string, error) {
	query := `
		SELECT fingerprint
		FROM device_push_tokens
		WHERE delivery_failures >= $1
		   OR updated_at < now() - make_interval(days => $2)
	`

	rows, err := s.pool.Query(ctx, query, failureThreshold, staleDays)
	if err != nil {
		return nil, fmt.Errorf("get stale tokens: %w", err)
	}
	defer rows.Close()

	var fingerprints []string
	for rows.Next() {
		var fp string
		if err := rows.Scan(&fp); err != nil {
			return nil, fmt.Errorf("scan fingerprint: %w", err)
		}
		fingerprints = append(fingerprints, fp)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return fingerprints, nil
}
