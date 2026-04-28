package pictogram

import (
	"encoding/hex"
	"reflect"
	"testing"
)

// Test vectors from api/test-vectors/pictogram.json
func TestDerive(t *testing.T) {
	tests := []struct {
		name                       string
		fingerprintHex             string
		expectedPictogram          []string
		expectedPictogramSpeakable string
	}{
		{
			name:                       "Example from protocol-spec §11.4",
			fingerprintHex:             "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2",
			expectedPictogram:          []string{"tree", "rocket", "mushroom", "orange", "moai"},
			expectedPictogramSpeakable: "tree rocket mushroom orange moai",
		},
		{
			name:                       "All zeros fingerprint",
			fingerprintHex:             "0000000000000000000000000000000000000000000000000000000000000000",
			expectedPictogram:          []string{"apple", "apple", "apple", "apple", "apple"},
			expectedPictogramSpeakable: "apple apple apple apple apple",
		},
		{
			name:                       "All 0xFF fingerprint (max indices)",
			fingerprintHex:             "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			expectedPictogram:          []string{"fire", "fire", "fire", "fire", "fire"},
			expectedPictogramSpeakable: "fire fire fire fire fire",
		},
		// Note: Skipping "Sequential indices test" from test vectors - appears to have incorrect expected values
		// The hex 0x04104104 produces indices [1,1,1,1,1] not [1,0,16,4,4] as claimed
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fingerprintBytes, err := hex.DecodeString(tc.fingerprintHex)
			if err != nil {
				t.Fatalf("Failed to decode fingerprint hex: %v", err)
			}

			pictogram, err := Derive(fingerprintBytes)
			if err != nil {
				t.Fatalf("Derive() error = %v", err)
			}

			if !reflect.DeepEqual(pictogram, tc.expectedPictogram) {
				t.Errorf("Derive() pictogram = %v, want %v", pictogram, tc.expectedPictogram)
			}

			speakable := Speakable(pictogram)
			if speakable != tc.expectedPictogramSpeakable {
				t.Errorf("Speakable() = %q, want %q", speakable, tc.expectedPictogramSpeakable)
			}
		})
	}
}

func TestDerive_ErrorCases(t *testing.T) {
	tests := []struct {
		name             string
		fingerprintBytes []byte
		wantErr          bool
	}{
		{
			name:             "Empty fingerprint",
			fingerprintBytes: []byte{},
			wantErr:          true,
		},
		{
			name:             "Too short (3 bytes)",
			fingerprintBytes: []byte{0x01, 0x02, 0x03},
			wantErr:          true,
		},
		{
			name:             "Exactly 4 bytes (minimum valid)",
			fingerprintBytes: []byte{0x00, 0x00, 0x00, 0x00},
			wantErr:          false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Derive(tc.fingerprintBytes)
			if (err != nil) != tc.wantErr {
				t.Errorf("Derive() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestSpeakable(t *testing.T) {
	tests := []struct {
		name      string
		pictogram []string
		want      string
	}{
		{
			name:      "5-emoji pictogram",
			pictogram: []string{"apple", "banana", "plane", "car", "dog"},
			want:      "apple banana plane car dog",
		},
		{
			name:      "Empty pictogram",
			pictogram: []string{},
			want:      "",
		},
		{
			name:      "Single emoji",
			pictogram: []string{"fire"},
			want:      "fire",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Speakable(tc.pictogram)
			if got != tc.want {
				t.Errorf("Speakable() = %q, want %q", got, tc.want)
			}
		})
	}
}
