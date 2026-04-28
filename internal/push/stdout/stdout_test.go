package stdout

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

func TestSend(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	ctx := context.Background()
	p := New("apns")

	payload := []byte(`{"challenge":"abc123"}`)
	err := p.Send(ctx, "device-token-12345", payload)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "push_dispatched") {
		t.Errorf("Expected output to contain 'push_dispatched', got: %q", output)
	}
	if !strings.Contains(output, "apns") {
		t.Errorf("Expected output to contain 'apns', got: %q", output)
	}
	if !strings.Contains(output, "device-token-12345") {
		t.Errorf("Expected output to contain token, got: %q", output)
	}

	var event map[string]interface{}
	outputBytes := []byte(output)

	start := strings.Index(output, "{")
	if start == -1 {
		t.Fatalf("No JSON found in output: %q", output)
	}

	end := strings.LastIndex(output, "}")
	if end == -1 || end < start {
		t.Fatalf("Invalid JSON structure in output: %q", output)
	}

	jsonPart := outputBytes[start : end+1]
	if err := json.Unmarshal(jsonPart, &event); err != nil {
		t.Fatalf("Failed to parse JSON from output: %v, output: %q", err, output)
	}

	if event["event"] != "push_dispatched" {
		t.Errorf("Expected event=push_dispatched, got %v", event["event"])
	}
	if event["would_send_via"] != "apns" {
		t.Errorf("Expected would_send_via=apns, got %v", event["would_send_via"])
	}
}

func TestPlatform(t *testing.T) {
	p := New("fcm")
	if p.Platform() != "fcm" {
		t.Errorf("Expected platform=fcm, got %s", p.Platform())
	}
}

func TestDefaultPlatform(t *testing.T) {
	p := New("")
	if p.Platform() != "mock" {
		t.Errorf("Expected default platform=mock, got %s", p.Platform())
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"exactly-ten!", 12, "exactly-ten!"},
		{"this-is-very-long-token", 10, "this-is-ve..."},
	}

	for _, tc := range tests {
		result := truncate(tc.input, tc.maxLen)
		if result != tc.expected {
			t.Errorf("truncate(%q, %d) = %q; want %q", tc.input, tc.maxLen, result, tc.expected)
		}
	}
}
