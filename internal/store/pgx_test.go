package store

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func setupTestDB(t *testing.T) (Store, func()) {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("relay_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("Failed to start postgres container: %v", err)
	}

	connString, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("Failed to get connection string: %v", err)
	}

	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	migrationSQL, err := os.ReadFile("../../migrations/001_initial_schema.up.sql")
	if err != nil {
		t.Fatalf("Failed to read migration: %v", err)
	}

	if _, err := conn.Exec(ctx, string(migrationSQL)); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}
	conn.Close(ctx)

	store, err := NewPgxStore(ctx, connString)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	cleanup := func() {
		store.Close()
		container.Terminate(ctx)
	}

	return store, cleanup
}

func TestStore_RegisterDevice(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	tests := []struct {
		name        string
		fingerprint string
		token       string
		platform    string
		wantErr     bool
	}{
		{
			name:        "Register new APNs device",
			fingerprint: "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2",
			token:       "apns-token-123",
			platform:    "apns",
			wantErr:     false,
		},
		{
			name:        "Register new FCM device",
			fingerprint: "b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3",
			token:       "fcm-token-789",
			platform:    "fcm",
			wantErr:     false,
		},
		{
			name:        "Update existing device (last-write-wins)",
			fingerprint: "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2",
			token:       "apns-token-456-updated",
			platform:    "apns",
			wantErr:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := store.RegisterDevice(ctx, tc.fingerprint, tc.token, tc.platform)
			if (err != nil) != tc.wantErr {
				t.Errorf("RegisterDevice() error = %v, wantErr %v", err, tc.wantErr)
			}

			if !tc.wantErr {
				pt, err := store.GetPushToken(ctx, tc.fingerprint)
				if err != nil {
					t.Fatalf("GetPushToken() after register failed: %v", err)
				}
				if pt == nil {
					t.Fatal("Expected token to be registered")
				}
				if pt.Token != tc.token {
					t.Errorf("Token = %v, want %v", pt.Token, tc.token)
				}
				if pt.Platform != tc.platform {
					t.Errorf("Platform = %v, want %v", pt.Platform, tc.platform)
				}
			}
		})
	}
}

func TestStore_GetPushToken(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	fp := "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2"
	store.RegisterDevice(ctx, fp, "apns-token-123", "apns")

	tests := []struct {
		name        string
		fingerprint string
		wantNil     bool
		wantErr     bool
	}{
		{
			name:        "Get existing token",
			fingerprint: fp,
			wantNil:     false,
			wantErr:     false,
		},
		{
			name:        "Get non-existent token",
			fingerprint: "nonexistent",
			wantNil:     true,
			wantErr:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pt, err := store.GetPushToken(ctx, tc.fingerprint)
			if (err != nil) != tc.wantErr {
				t.Errorf("GetPushToken() error = %v, wantErr %v", err, tc.wantErr)
			}
			if (pt == nil) != tc.wantNil {
				t.Errorf("GetPushToken() nil = %v, wantNil %v", pt == nil, tc.wantNil)
			}
		})
	}
}

func TestStore_EvictToken(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	fp := "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2"
	store.RegisterDevice(ctx, fp, "apns-token-123", "apns")

	tests := []struct {
		name        string
		fingerprint string
		wantErr     bool
	}{
		{
			name:        "Evict existing token",
			fingerprint: fp,
			wantErr:     false,
		},
		{
			name:        "Evict non-existent token (idempotent)",
			fingerprint: "nonexistent",
			wantErr:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := store.EvictToken(ctx, tc.fingerprint)
			if (err != nil) != tc.wantErr {
				t.Errorf("EvictToken() error = %v, wantErr %v", err, tc.wantErr)
			}

			if !tc.wantErr && tc.fingerprint == fp {
				pt, err := store.GetPushToken(ctx, tc.fingerprint)
				if err != nil {
					t.Fatalf("GetPushToken() after evict failed: %v", err)
				}
				if pt != nil {
					t.Error("Token should have been evicted")
				}
			}
		})
	}
}

func TestStore_IncrementFailures(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	fp := "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2"
	store.RegisterDevice(ctx, fp, "apns-token-123", "apns")

	t.Run("Increment failures 10 times", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			if err := store.IncrementFailures(ctx, fp); err != nil {
				t.Fatalf("IncrementFailures() iteration %d failed: %v", i+1, err)
			}
		}

		stale, err := store.GetStaleTokens(ctx, 10, 90)
		if err != nil {
			t.Fatalf("GetStaleTokens() failed: %v", err)
		}

		found := false
		for _, staleFp := range stale {
			if staleFp == fp {
				found = true
				break
			}
		}
		if !found {
			t.Error("Token with 10 failures should be in stale list")
		}
	})
}

func TestStore_ResetFailures(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	fp := "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2"
	store.RegisterDevice(ctx, fp, "apns-token-123", "apns")

	for i := 0; i < 5; i++ {
		store.IncrementFailures(ctx, fp)
	}

	t.Run("Reset failures after success", func(t *testing.T) {
		if err := store.ResetFailures(ctx, fp); err != nil {
			t.Fatalf("ResetFailures() failed: %v", err)
		}

		stale, err := store.GetStaleTokens(ctx, 5, 90)
		if err != nil {
			t.Fatalf("GetStaleTokens() failed: %v", err)
		}

		for _, staleFp := range stale {
			if staleFp == fp {
				t.Error("Token should not be stale after reset")
			}
		}
	})
}

func TestStore_GetStaleTokens(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	fp1 := "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2"
	fp2 := "b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3"
	fp3 := "c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4"

	store.RegisterDevice(ctx, fp1, "token-1", "apns")
	store.RegisterDevice(ctx, fp2, "token-2", "apns")
	store.RegisterDevice(ctx, fp3, "token-3", "fcm")

	for i := 0; i < 10; i++ {
		store.IncrementFailures(ctx, fp1)
	}

	t.Run("Get tokens with 10+ failures", func(t *testing.T) {
		stale, err := store.GetStaleTokens(ctx, 10, 90)
		if err != nil {
			t.Fatalf("GetStaleTokens() failed: %v", err)
		}

		found := false
		for _, staleFp := range stale {
			if staleFp == fp1 {
				found = true
			}
			if staleFp == fp2 || staleFp == fp3 {
				t.Errorf("fp2/fp3 should not be stale, got %s", staleFp)
			}
		}
		if !found {
			t.Error("fp1 with 10 failures should be in stale list")
		}
	})
}

func TestStore_ConcurrentUpdates(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	fp := "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2"

	t.Run("Concurrent re-register", func(t *testing.T) {
		var wg sync.WaitGroup
		iterations := 10

		for i := 0; i < iterations; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				token := "token-" + string(rune('0'+id))
				if err := store.RegisterDevice(ctx, fp, token, "apns"); err != nil {
					t.Errorf("RegisterDevice() concurrent call %d failed: %v", id, err)
				}
			}(i)
		}

		wg.Wait()

		pt, err := store.GetPushToken(ctx, fp)
		if err != nil {
			t.Fatalf("GetPushToken() after concurrent updates failed: %v", err)
		}
		if pt == nil {
			t.Fatal("Expected token to exist after concurrent updates")
		}
	})
}

func TestStore_UpdateResetsFailures(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	fp := "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2"
	store.RegisterDevice(ctx, fp, "token-1", "apns")

	for i := 0; i < 5; i++ {
		store.IncrementFailures(ctx, fp)
	}

	store.RegisterDevice(ctx, fp, "token-2", "apns")

	stale, err := store.GetStaleTokens(ctx, 5, 90)
	if err != nil {
		t.Fatalf("GetStaleTokens() failed: %v", err)
	}

	for _, staleFp := range stale {
		if staleFp == fp {
			t.Error("Re-registering should reset failures counter")
		}
	}
}

func TestStore_LastDeliveredAt(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	fp := "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2"
	store.RegisterDevice(ctx, fp, "token-1", "apns")

	pt1, _ := store.GetPushToken(ctx, fp)
	if pt1.LastDeliveredAt != nil {
		t.Error("LastDeliveredAt should be nil for new device")
	}

	time.Sleep(100 * time.Millisecond)
	store.ResetFailures(ctx, fp)

	pt2, _ := store.GetPushToken(ctx, fp)
	if pt2.LastDeliveredAt == nil {
		t.Error("LastDeliveredAt should be set after ResetFailures")
	}
}
