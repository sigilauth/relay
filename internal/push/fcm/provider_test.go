package fcm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewProvider(t *testing.T) {
	tmpDir := t.TempDir()
	saPath := filepath.Join(tmpDir, "service-account.json")

	validSA := `{
  "type": "service_account",
  "project_id": "test-project",
  "private_key_id": "abc123",
  "private_key": "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC7VJTUt9Us8cKj\nMzEfYyjiWA4R4/M2bS1+fWIcPm15j9IK7vQDLGNR6NmBPYrB7CjYRqGCzKLqTVZJ\nDqbTOm8E9i0xrpIkFJNp8tXhTM4X/5TxQ8Ie8pB2H3S3Y1lhpVvb8+rHOKCxcQDO\njYxM8fMQs0Ly9xnBQkGwKpLCKxBPBPpYLPX6GUr7RY8uQWG1V0Sj3qMXy7jCRdXx\nvW3adQsjJqmqLeHg9w6MkLBm6QsmFr/PxFMGqKNKqPQFSKGM4i9Y4KNmKNfGp4dO\nBPL6kXnDH0S8D1mDLqHhc9RLqDwFcMJ3yNL8LiXqQNMxXlVnHH8DYGe3r7WYnqBa\ncXAgMBAAECggEAKlVdNKZJJkNQfVbzCKCq5T8VZvlmSKBL9+XZlV0M3YZkJKRHb2J0\noXU7L8L3Q1bUCJKnP0U7VQJDKnqLCfJwLjCOkF/MG5h0Q7WN3jN+vQKBgQDLTJR+\nXZvV9yJ7xLpKJGCLgYOLvJBxkJqVVKqCqWxX2vB+8MQF9FhNLqPR1L3j0ZfWBqXN\nMhJLgRdJ5qKQqL7vN+fH9rKRHbHqYqYp2CQyNQpFLhQJKLqGCRLpLqPh/gKBgQDf\nLRXLvLqYH9rQNJqL7vN+fH9rKRHbHqYqYp2CQyNQpFLhQJKLqGCRLpLqPh\n-----END PRIVATE KEY-----\n",
  "client_email": "test@test-project.iam.gserviceaccount.com",
  "client_id": "123456789",
  "auth_uri": "https://accounts.google.com/o/oauth2/auth",
  "token_uri": "https://oauth2.googleapis.com/token"
}`

	os.WriteFile(saPath, []byte(validSA), 0600)

	tests := []struct {
		name    string
		saPath  string
		wantErr bool
	}{
		{
			name:    "Valid service account",
			saPath:  saPath,
			wantErr: false,
		},
		{
			name:    "Invalid path",
			saPath:  "/nonexistent/sa.json",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			provider, err := NewProvider(ctx, tc.saPath)
			if (err != nil) != tc.wantErr {
				t.Errorf("NewProvider() error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && provider == nil {
				t.Error("Expected non-nil provider")
			}
			if !tc.wantErr && provider.Platform() != "fcm" {
				t.Errorf("Platform() = %v, want fcm", provider.Platform())
			}
		})
	}
}

func TestProvider_Send(t *testing.T) {
	t.Skip("Requires valid FCM credentials and device token for live testing")
}

func TestProvider_Platform(t *testing.T) {
	tmpDir := t.TempDir()
	saPath := filepath.Join(tmpDir, "service-account.json")

	validSA := `{
  "type": "service_account",
  "project_id": "test-project",
  "private_key_id": "abc123",
  "private_key": "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC7VJTUt9Us8cKj\nMzEfYyjiWA4R4/M2bS1+fWIcPm15j9IK7vQDLGNR6NmBPYrB7CjYRqGCzKLqTVZJ\nDqbTOm8E9i0xrpIkFJNp8tXhTM4X/5TxQ8Ie8pB2H3S3Y1lhpVvb8+rHOKCxcQDO\njYxM8fMQs0Ly9xnBQkGwKpLCKxBPBPpYLPX6GUr7RY8uQWG1V0Sj3qMXy7jCRdXx\nvW3adQsjJqmqLeHg9w6MkLBm6QsmFr/PxFMGqKNKqPQFSKGM4i9Y4KNmKNfGp4dO\nBPL6kXnDH0S8D1mDLqHhc9RLqDwFcMJ3yNL8LiXqQNMxXlVnHH8DYGe3r7WYnqBa\ncXAgMBAAECggEAKlVdNKZJJkNQfVbzCKCq5T8VZvlmSKBL9+XZlV0M3YZkJKRHb2J0\noXU7L8L3Q1bUCJKnP0U7VQJDKnqLCfJwLjCOkF/MG5h0Q7WN3jN+vQKBgQDLTJR+\nXZvV9yJ7xLpKJGCLgYOLvJBxkJqVVKqCqWxX2vB+8MQF9FhNLqPR1L3j0ZfWBqXN\nMhJLgRdJ5qKQqL7vN+fH9rKRHbHqYqYp2CQyNQpFLhQJKLqGCRLpLqPh/gKBgQDf\nLRXLvLqYH9rQNJqL7vN+fH9rKRHbHqYqYp2CQyNQpFLhQJKLqGCRLpLqPh\n-----END PRIVATE KEY-----\n",
  "client_email": "test@test-project.iam.gserviceaccount.com",
  "client_id": "123456789",
  "auth_uri": "https://accounts.google.com/o/oauth2/auth",
  "token_uri": "https://oauth2.googleapis.com/token"
}`

	os.WriteFile(saPath, []byte(validSA), 0600)

	ctx := context.Background()
	provider, err := NewProvider(ctx, saPath)
	if err != nil {
		t.Fatalf("NewProvider() failed: %v", err)
	}

	if got := provider.Platform(); got != "fcm" {
		t.Errorf("Platform() = %v, want fcm", got)
	}
}

func Test_contains(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		substr string
		want   bool
	}{
		{
			name:   "Contains at start",
			s:      "registration-token-not-registered",
			substr: "registration-token-not-registered",
			want:   true,
		},
		{
			name:   "Contains at end",
			s:      "Error: registration-token-not-registered",
			substr: "registration-token-not-registered",
			want:   true,
		},
		{
			name:   "Does not contain",
			s:      "Some other error",
			substr: "registration-token-not-registered",
			want:   false,
		},
		{
			name:   "Empty string",
			s:      "",
			substr: "test",
			want:   false,
		},
		{
			name:   "Empty substr",
			s:      "test",
			substr: "",
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := contains(tc.s, tc.substr); got != tc.want {
				t.Errorf("contains() = %v, want %v", got, tc.want)
			}
		})
	}
}
