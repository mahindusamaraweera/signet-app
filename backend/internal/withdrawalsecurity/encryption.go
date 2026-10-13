package withdrawalsecurity

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
)

func encryptionCipher(key []byte) (cipher.AEAD, error) {
	if len(key) < 32 {
		return nil, errors.New("authenticator encryption requires a strong key")
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("withdrawal-authenticator-encryption-v1"))
	block, err := aes.NewCipher(mac.Sum(nil))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func enrollmentContext(userID, version uint64, email string) []byte {
	return []byte(fmt.Sprintf("withdrawal-enrollment-v1|%d|%d|%s", userID, version, email))
}

func encryptEnrollment(key []byte, userID, version uint64, email, secret string) (string, error) {
	if userID == 0 || version == 0 || email == "" || secret == "" {
		return "", errors.New("invalid authenticator enrollment")
	}
	box, err := encryptionCipher(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, box.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := box.Seal(nonce, nonce, []byte(secret), enrollmentContext(userID, version, email))
	return base64.RawStdEncoding.EncodeToString(sealed), nil
}

func decryptEnrollment(key []byte, userID, version uint64, email, encrypted string) (string, error) {
	box, err := encryptionCipher(key)
	if err != nil {
		return "", err
	}
	sealed, err := base64.RawStdEncoding.DecodeString(encrypted)
	if err != nil || len(sealed) < box.NonceSize()+box.Overhead() {
		return "", errors.New("invalid encrypted authenticator enrollment")
	}
	secret, err := box.Open(nil, sealed[:box.NonceSize()], sealed[box.NonceSize():], enrollmentContext(userID, version, email))
	if err != nil {
		return "", errors.New("authenticator enrollment integrity check failed")
	}
	return string(secret), nil
}

func secretFingerprint(key []byte, secret string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("withdrawal-authenticator-fingerprint-v1|" + secret))
	return hex.EncodeToString(mac.Sum(nil))
}

func newAuthenticatorSecret() (string, error) {
	value := make([]byte, 20)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(value), nil
}
