package handlers

import (
	"testing"

	"signet-backend/internal/app"
	"signet-backend/internal/blockchain"
	"signet-backend/internal/config"
)

func TestIncomeCapTopupAvailabilityRequiresEveryGate(t *testing.T) {
	gateway, err := blockchain.New("https://custody.example", "test")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		deps *app.Deps
		want bool
	}{
		{"missing dependencies", nil, false},
		{"missing configuration", &app.Deps{Blockchain: gateway}, false},
		{"missing gateway", &app.Deps{Cfg: &config.Config{BlockchainEnabled: true, BlockchainTopupEnabled: true}}, false},
		{"blockchain disabled", &app.Deps{Blockchain: gateway, Cfg: &config.Config{BlockchainTopupEnabled: true}}, false},
		{"topup disabled", &app.Deps{Blockchain: gateway, Cfg: &config.Config{BlockchainEnabled: true}}, false},
		{"enabled", &app.Deps{Blockchain: gateway, Cfg: &config.Config{BlockchainEnabled: true, BlockchainTopupEnabled: true}}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := incomeCapTopupEnabled(test.deps); got != test.want {
				t.Fatalf("availability = %v, want %v", got, test.want)
			}
		})
	}
}
