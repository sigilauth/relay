package ratelimit

import (
	"testing"
	"time"
)

func TestLimiter_Allow(t *testing.T) {
	limiter := New(1.0, 1) // 1 req/sec, burst 1

	tests := []struct {
		name        string
		fingerprint string
		sleep       time.Duration
		want        bool
	}{
		{
			name:        "first request allowed",
			fingerprint: "fp1",
			sleep:       0,
			want:        true,
		},
		{
			name:        "second request immediate denied",
			fingerprint: "fp1",
			sleep:       0,
			want:        false,
		},
		{
			name:        "request after 1s allowed",
			fingerprint: "fp1",
			sleep:       1 * time.Second,
			want:        true,
		},
		{
			name:        "different fingerprint allowed",
			fingerprint: "fp2",
			sleep:       0,
			want:        true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.sleep > 0 {
				time.Sleep(tc.sleep)
			}

			got := limiter.Allow(tc.fingerprint)
			if got != tc.want {
				t.Errorf("Allow(%q) = %v, want %v", tc.fingerprint, got, tc.want)
			}
		})
	}
}

func TestLimiter_Reset(t *testing.T) {
	limiter := New(1.0, 1)

	fp := "test-fp"

	// Exhaust limit
	limiter.Allow(fp)
	if limiter.Allow(fp) {
		t.Fatal("Expected second request to be denied")
	}

	// Reset
	limiter.Reset(fp)

	// Should allow again
	if !limiter.Allow(fp) {
		t.Error("Expected request after reset to be allowed")
	}
}

func TestLimiter_ConcurrentAccess(t *testing.T) {
	limiter := New(10.0, 1)
	fp := "concurrent-fp"

	// Launch 100 goroutines trying to access
	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func() {
			limiter.Allow(fp)
			done <- true
		}()
	}

	// Wait for all to complete
	for i := 0; i < 100; i++ {
		<-done
	}

	// No panic = success (testing concurrent map access)
}
