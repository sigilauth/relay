package crypto

import (
	"strings"
	"testing"
)

func TestTokenEncryptor_EncryptDecrypt(t *testing.T) {
	encryptor, err := NewTokenEncryptor("test-key-material-for-encryption")
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	plaintext := "apns-token-1234567890abcdef"

	ciphertext, err := encryptor.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	if ciphertext == plaintext {
		t.Error("Ciphertext should not equal plaintext")
	}

	if strings.Contains(ciphertext, plaintext) {
		t.Error("Ciphertext should not contain plaintext")
	}

	decrypted, err := encryptor.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decryption failed: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("Decrypted text does not match original. Got %s, want %s", decrypted, plaintext)
	}
}

func TestTokenEncryptor_DifferentCiphertexts(t *testing.T) {
	encryptor, err := NewTokenEncryptor("test-key-material")
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	plaintext := "same-token"

	ct1, _ := encryptor.Encrypt(plaintext)
	ct2, _ := encryptor.Encrypt(plaintext)

	if ct1 == ct2 {
		t.Error("Same plaintext should produce different ciphertexts (nonce randomization)")
	}

	dec1, _ := encryptor.Decrypt(ct1)
	dec2, _ := encryptor.Decrypt(ct2)

	if dec1 != plaintext || dec2 != plaintext {
		t.Error("Both ciphertexts should decrypt to original plaintext")
	}
}

func TestTokenEncryptor_InvalidCiphertext(t *testing.T) {
	encryptor, err := NewTokenEncryptor("test-key-material")
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	_, err = encryptor.Decrypt("invalid-base64!!!")
	if err == nil {
		t.Error("Expected error for invalid base64")
	}

	_, err = encryptor.Decrypt("YWJjZGVm")
	if err == nil {
		t.Error("Expected error for too-short ciphertext")
	}
}

func TestTokenEncryptor_EmptyKey(t *testing.T) {
	_, err := NewTokenEncryptor("")
	if err == nil {
		t.Error("Expected error for empty key material")
	}
}

func TestTokenEncryptor_DifferentKeys(t *testing.T) {
	enc1, _ := NewTokenEncryptor("key1")
	enc2, _ := NewTokenEncryptor("key2")

	plaintext := "test-token"
	ciphertext, _ := enc1.Encrypt(plaintext)

	_, err := enc2.Decrypt(ciphertext)
	if err == nil {
		t.Error("Decryption with different key should fail")
	}
}
