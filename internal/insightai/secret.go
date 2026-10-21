package insightai

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

const SecretKeyBytes = 32

// SecretCodec encrypts AI credentials before they cross a persistence boundary.
// The raw key is held only in memory and is never serialized or logged.
type SecretCodec struct {
	key [SecretKeyBytes]byte
}

func NewSecretCodec(raw string) (*SecretCodec, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("AI config encryption key is required")
	}
	decoded, err := hex.DecodeString(raw)
	if err != nil || len(decoded) != SecretKeyBytes {
		decoded, err = base64.RawStdEncoding.DecodeString(raw)
		if err != nil || len(decoded) != SecretKeyBytes {
			decoded, err = base64.StdEncoding.DecodeString(raw)
		}
	}
	if err != nil || len(decoded) != SecretKeyBytes {
		return nil, fmt.Errorf("AI config encryption key must decode to %d bytes", SecretKeyBytes)
	}
	codec := &SecretCodec{}
	copy(codec.key[:], decoded)
	return codec, nil
}

func (c *SecretCodec) Encrypt(value string) (ciphertext, nonce string, err error) {
	if c == nil {
		return "", "", errors.New("AI config encryption is not configured")
	}
	block, err := aes.NewCipher(c.key[:])
	if err != nil {
		return "", "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", err
	}
	nonceBytes := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonceBytes); err != nil {
		return "", "", err
	}
	sealed := gcm.Seal(nil, nonceBytes, []byte(value), nil)
	return base64.RawStdEncoding.EncodeToString(sealed), base64.RawStdEncoding.EncodeToString(nonceBytes), nil
}

func (c *SecretCodec) Decrypt(ciphertext, nonce string) (string, error) {
	if c == nil {
		return "", errors.New("AI config encryption is not configured")
	}
	block, err := aes.NewCipher(c.key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	sealed, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(ciphertext))
	if err != nil {
		return "", errors.New("invalid AI config ciphertext")
	}
	nonceBytes, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(nonce))
	if err != nil || len(nonceBytes) != gcm.NonceSize() {
		return "", errors.New("invalid AI config nonce")
	}
	opened, err := gcm.Open(nil, nonceBytes, sealed, nil)
	if err != nil {
		return "", errors.New("AI config ciphertext authentication failed")
	}
	return string(opened), nil
}
