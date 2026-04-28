package pictogram

import (
	"encoding/binary"
	"fmt"
)

// EmojiList is the canonical 64-emoji list per test-vectors/pictogram.json
var EmojiList = [64]string{
	"apple", "banana", "grapes", "orange", "lemon", "cherry", "strawberry", "kiwi",
	"carrot", "corn", "broccoli", "mushroom", "pepper", "avocado", "onion", "peanut",
	"pizza", "burger", "taco", "donut", "cookie", "cake", "cupcake", "popcorn",
	"car", "taxi", "bus", "rocket", "plane", "helicopter", "sailboat", "bicycle",
	"dog", "cat", "fish", "butterfly", "bee", "fox", "lion", "elephant",
	"tree", "sunflower", "cactus", "clover", "blossom", "rainbow", "star", "moon",
	"house", "mountain", "peak", "volcano", "island", "moai", "tent", "castle",
	"key", "bell", "books", "guitar", "anchor", "crown", "diamond", "fire",
}

// Derive computes a 5-emoji pictogram from a fingerprint.
// Extracts 5 × 6-bit indices from the first 4 bytes (32 bits, using first 30).
// Per protocol-spec §3.6 and test-vectors/pictogram.json.
func Derive(fingerprintBytes []byte) ([]string, error) {
	if len(fingerprintBytes) < 4 {
		return nil, fmt.Errorf("fingerprint too short: need at least 4 bytes, got %d", len(fingerprintBytes))
	}

	// Read first 4 bytes as big-endian uint32
	firstFourBytes := binary.BigEndian.Uint32(fingerprintBytes[:4])

	// Extract 5 × 6-bit indices from the first 30 bits
	indices := [5]int{
		int((firstFourBytes >> 26) & 0x3F), // bits 0-5
		int((firstFourBytes >> 20) & 0x3F), // bits 6-11
		int((firstFourBytes >> 14) & 0x3F), // bits 12-17
		int((firstFourBytes >> 8) & 0x3F),  // bits 18-23
		int((firstFourBytes >> 2) & 0x3F),  // bits 24-29
	}

	pictogram := make([]string, 5)
	for i, idx := range indices {
		pictogram[i] = EmojiList[idx]
	}

	return pictogram, nil
}

// Speakable joins a pictogram array into a space-separated string.
// Per D10: JSON uses spaces, URLs use hyphens.
func Speakable(pictogram []string) string {
	if len(pictogram) == 0 {
		return ""
	}

	result := pictogram[0]
	for i := 1; i < len(pictogram); i++ {
		result += " " + pictogram[i]
	}
	return result
}
