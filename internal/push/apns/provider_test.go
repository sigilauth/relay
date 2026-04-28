package apns

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewProvider(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.p8")

	pemContent := `-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgVcB/UNPxalR9zDdO
jR7hYr4dCFqbfGVPy0Tt95jMEt6hRANCAASN3F3XnHFKQQqW6KGmJC4xHGqBl6qX
w/fGcmkGVJMqLt8VqHEbjk0R7pUNMAJi9Ue4VvB8S/kjGZGPJCGqEWVj
-----END PRIVATE KEY-----`

	if err := os.WriteFile(keyPath, []byte(pemContent), 0600); err != nil {
		t.Fatalf("Failed to write test key: %v", err)
	}

	tests := []struct {
		name       string
		keyPath    string
		keyID      string
		teamID     string
		production bool
		wantErr    bool
	}{
		{
			name:       "Valid key development",
			keyPath:    keyPath,
			keyID:      "ABC123DEFG",
			teamID:     "DEF123GHIJ",
			production: false,
			wantErr:    false,
		},
		{
			name:       "Valid key production",
			keyPath:    keyPath,
			keyID:      "ABC123DEFG",
			teamID:     "DEF123GHIJ",
			production: true,
			wantErr:    false,
		},
		{
			name:       "Invalid key path",
			keyPath:    "/nonexistent/key.p8",
			keyID:      "ABC123DEFG",
			teamID:     "DEF123GHIJ",
			production: false,
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider, err := NewProvider(tc.keyPath, tc.keyID, tc.teamID, tc.production)
			if (err != nil) != tc.wantErr {
				t.Errorf("NewProvider() error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && provider == nil {
				t.Error("Expected non-nil provider")
			}
			if !tc.wantErr && provider.Platform() != "apns" {
				t.Errorf("Platform() = %v, want apns", provider.Platform())
			}
		})
	}
}

func TestProvider_Send(t *testing.T) {
	t.Skip("Requires valid APNs credentials and device token for live testing")

	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.p8")

	pemContent := `-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgVcB/UNPxalR9zDdO
jR7hYr4dCFqbfGVPy0Tt95jMEt6hRANCAASN3F3XnHFKQQqW6KGmJC4xHGqBl6qX
w/fGcmkGVJMqLt8VqHEbjk0R7pUNMAJi9Ue4VvB8S/kjGZGPJCGqEWVj
-----END PRIVATE KEY-----`

	os.WriteFile(keyPath, []byte(pemContent), 0600)

	provider, err := NewProvider(keyPath, "ABC123DEFG", "DEF123GHIJ", false)
	if err != nil {
		t.Fatalf("NewProvider() failed: %v", err)
	}

	ctx := context.Background()
	payload := []byte(`{"aps":{"alert":"Test"}}`)
	deviceToken := "00000000000000000000000000000000"

	err = provider.Send(ctx, deviceToken, payload)

	t.Logf("Send() returned: %v", err)
}

func TestProvider_Platform(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.p8")

	pemContent := `-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgVcB/UNPxalR9zDdO
jR7hYr4dCFqbfGVPy0Tt95jMEt6hRANCAASN3F3XnHFKQQqW6KGmJC4xHGqBl6qX
w/fGcmkGVJMqLt8VqHEbjk0R7pUNMAJi9Ue4VvB8S/kjGZGPJCGqEWVj
-----END PRIVATE KEY-----`

	os.WriteFile(keyPath, []byte(pemContent), 0600)

	provider, err := NewProvider(keyPath, "ABC123DEFG", "DEF123GHIJ", false)
	if err != nil {
		t.Fatalf("NewProvider() failed: %v", err)
	}

	if got := provider.Platform(); got != "apns" {
		t.Errorf("Platform() = %v, want apns", got)
	}
}
