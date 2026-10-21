package media

import (
	"bytes"
	"io"
	"testing"
)

func TestDecryptReaderRoundTripsEncryptedPrefix(t *testing.T) {
	plain := bytes.Repeat([]byte("video-prefix-"), 12000)
	key := uint64(1409865593)

	ciphertext, err := io.ReadAll(NewDecryptReader(bytes.NewReader(plain), key, 0, EncryptedPrefixSize))
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	decoded, err := io.ReadAll(NewDecryptReader(bytes.NewReader(ciphertext), key, 0, EncryptedPrefixSize))
	if err != nil {
		t.Fatalf("decrypt fixture: %v", err)
	}
	if !bytes.Equal(decoded, plain) {
		t.Fatalf("decrypted content does not match original prefix")
	}
}

func TestDecryptReaderStartsKeystreamAtRangeOffset(t *testing.T) {
	plain := bytes.Repeat([]byte("range-aware-video-"), 12000)
	key := uint64(647717145)
	start := uint64(97)

	ciphertext, err := io.ReadAll(NewDecryptReader(bytes.NewReader(plain), key, 0, EncryptedPrefixSize))
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	decoded, err := io.ReadAll(NewDecryptReader(bytes.NewReader(ciphertext[start:]), key, start, EncryptedPrefixSize))
	if err != nil {
		t.Fatalf("decrypt range fixture: %v", err)
	}
	if !bytes.Equal(decoded, plain[start:]) {
		t.Fatalf("range decryption does not match original suffix")
	}
}
