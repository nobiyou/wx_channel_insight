package insightai

import (
	"strings"
	"testing"
)

func TestSecretCodecRoundTripsAndRejectsWrongKey(t *testing.T) {
	codec, err := NewSecretCodec("00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatalf("new codec: %v", err)
	}
	ciphertext, nonce, err := codec.Encrypt("server-only-secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if ciphertext == "server-only-secret" || strings.TrimSpace(nonce) == "" {
		t.Fatalf("expected opaque ciphertext and nonce")
	}
	opened, err := codec.Decrypt(ciphertext, nonce)
	if err != nil || opened != "server-only-secret" {
		t.Fatalf("decrypt: value=%q err=%v", opened, err)
	}
	wrong, err := NewSecretCodec("ffeeddccbbaa99887766554433221100ffeeddccbbaa99887766554433221100")
	if err != nil {
		t.Fatalf("new wrong codec: %v", err)
	}
	if _, err := wrong.Decrypt(ciphertext, nonce); err == nil {
		t.Fatal("expected wrong key to fail authentication")
	}
}

func TestSecretCodecRequiresExactly32Bytes(t *testing.T) {
	if _, err := NewSecretCodec("short"); err == nil {
		t.Fatal("expected short key to be rejected")
	}
}
