package blockchain

import (
	"strings"
	"testing"
)

func TestLinkVerifierIsEncryptedAndBoundToIdentityAndState(t *testing.T) {
	secret := []byte(strings.Repeat("s", 32))
	verifier, err := LinkSecret()
	if err != nil {
		t.Fatal(err)
	}
	state, err := LinkSecret()
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := SealLinkVerifier(secret, 42, state, verifier)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sealed, verifier) {
		t.Fatal("plaintext verifier exposed")
	}
	plain, err := OpenLinkVerifier(secret, 42, state, sealed)
	if err != nil || plain != verifier {
		t.Fatalf("proof recovery failed: %v", err)
	}
	if _, err = OpenLinkVerifier(secret, 43, state, sealed); err == nil {
		t.Fatal("wrong user accepted")
	}
	if _, err = OpenLinkVerifier(secret, 42, "different", sealed); err == nil {
		t.Fatal("wrong state accepted")
	}
	if _, err = SealLinkVerifier([]byte("weak"), 42, state, verifier); err == nil {
		t.Fatal("weak secret accepted")
	}
	if len(LinkChallenge(verifier)) != 43 {
		t.Fatal("invalid S256 challenge")
	}
}
