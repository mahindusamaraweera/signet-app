package handlers

import (
	"strings"
	"testing"
)

func TestNewResetTokensAreRandomHashOnlyAndCannotUseStoredDigest(t *testing.T) {
	a, err := newPasswordResetToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := newPasswordResetToken()
	if err != nil {
		t.Fatal(err)
	}
	stored, err := passwordResetLookup(a)
	if err != nil || a == b || stored == a || !strings.HasPrefix(stored, "sha256:") {
		t.Fatal("unsafe reset token")
	}
	if _, err := passwordResetLookup(stored); err == nil {
		t.Fatal("database digest accepted as bearer")
	}
	if _, err := passwordResetLookup("rt1_bad"); err == nil {
		t.Fatal("malformed new token accepted")
	}
	legacy, err := passwordResetLookup("legacy-reset-token")
	if err != nil || legacy != "legacy-reset-token" {
		t.Fatal("legacy lookup changed")
	}
}
