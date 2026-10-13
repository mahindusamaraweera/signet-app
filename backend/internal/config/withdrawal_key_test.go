package config

import (
	"strings"
	"testing"
)

func TestWithdrawalAuthorizationKeyRequiresIndependentSecret(t *testing.T) {
	key := strings.Repeat("k", 32)
	for _, cfg := range []Config{
		{},
		{BlockchainWithdrawalAuthorizationKey: "weak"},
		{BlockchainWithdrawalAuthorizationKey: key, SessionSecret: key},
		{BlockchainWithdrawalAuthorizationKey: key, BlockchainServiceToken: key},
	} {
		if got, err := cfg.WithdrawalAuthorizationKey(); err == nil || got != nil {
			t.Fatal("unsafe signing configuration accepted")
		}
	}
	cfg := Config{BlockchainWithdrawalAuthorizationKey: key, SessionSecret: "session", BlockchainServiceToken: "bearer"}
	got, err := cfg.WithdrawalAuthorizationKey()
	if err != nil || string(got) != key {
		t.Fatal("independent signing key rejected")
	}
	got[0] = 'x'
	if cfg.BlockchainWithdrawalAuthorizationKey != key {
		t.Fatal("returned buffer mutated configuration")
	}
}
