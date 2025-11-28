package crypto

import (
	"bytes"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	handler, err := NewGPGHandler("testpassphrase")
	if err != nil {
		t.Fatalf("Failed to create GPG handler: %v", err)
	}

	plaintext := []byte("Hello, encrypted world!")

	ciphertext, err := handler.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	if bytes.Equal(plaintext, ciphertext) {
		t.Error("Ciphertext should not equal plaintext")
	}

	decrypted, err := handler.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("Decrypted text doesn't match original. Got %s, want %s", decrypted, plaintext)
	}
}

func TestEncryptDecryptLargeData(t *testing.T) {
	handler, err := NewGPGHandler("testpassphrase")
	if err != nil {
		t.Fatalf("Failed to create GPG handler: %v", err)
	}

	// 1MB of data
	plaintext := make([]byte, 1024*1024)
	for i := range plaintext {
		plaintext[i] = byte(i % 256)
	}

	ciphertext, err := handler.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	decrypted, err := handler.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Error("Decrypted data doesn't match original")
	}
}

func TestExportKeys(t *testing.T) {
	handler, err := NewGPGHandler("testpassphrase")
	if err != nil {
		t.Fatalf("Failed to create GPG handler: %v", err)
	}

	pubKey, err := handler.ExportPublicKey()
	if err != nil {
		t.Fatalf("Failed to export public key: %v", err)
	}

	if pubKey == "" {
		t.Error("Public key should not be empty")
	}

	privKey, err := handler.ExportPrivateKey()
	if err != nil {
		t.Fatalf("Failed to export private key: %v", err)
	}

	if privKey == "" {
		t.Error("Private key should not be empty")
	}
}
