package withdrawalsecurity

import (
	"strings"
	"testing"
)

func TestEnrollmentEncryptionBindsUserEmailAndVersion(t *testing.T) {
	key := []byte(strings.Repeat("k", 32))
	secret, err := newAuthenticatorSecret()
	if err != nil {
		t.Fatal(err)
	}
	if len(secret) != 32 {
		t.Fatal("invalid secret length")
	}
	sealed, err := encryptEnrollment(key, 42, 1, "user@example.com", secret)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sealed, secret) {
		t.Fatal("plaintext secret leaked")
	}
	recovered, err := decryptEnrollment(key, 42, 1, "user@example.com", sealed)
	if err != nil || recovered != secret {
		t.Fatal("secret recovery failed")
	}
	for _, test := range []struct {
		key           []byte
		user, version uint64
		email, sealed string
	}{
		{key, 43, 1, "user@example.com", sealed}, {key, 42, 2, "user@example.com", sealed},
		{key, 42, 1, "other@example.com", sealed}, {[]byte(strings.Repeat("x", 32)), 42, 1, "user@example.com", sealed},
		{key, 42, 1, "user@example.com", "malformed"},
	} {
		if _, err := decryptEnrollment(test.key, test.user, test.version, test.email, test.sealed); err == nil {
			t.Fatal("altered enrollment accepted")
		}
	}
	second, _ := encryptEnrollment(key, 42, 1, "user@example.com", secret)
	if second == sealed {
		t.Fatal("nonce reused")
	}
	if secretFingerprint(key, secret) == secretFingerprint(key, secret+"X") {
		t.Fatal("fingerprint not bound to secret")
	}
}
