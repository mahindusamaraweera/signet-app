// Command api is the Signet backend entrypoint.
package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/blockchain"
	"signet-backend/internal/config"
	"signet-backend/internal/db"
	"signet-backend/internal/handlers"
	"signet-backend/internal/jobs"
	"signet-backend/internal/registrationverification"
	"signet-backend/internal/securitymail"
	"signet-backend/internal/storage"
	"signet-backend/internal/wallet"
	"signet-backend/internal/withdrawalsecurity"
)

func main() {
	cfg := config.Load()
	wallet.ConfigurePendingCommissionGuard(cfg.BlockchainEnabled && cfg.BlockchainTopupEnabled)

	conn, err := db.Connect(cfg)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer conn.Close()

	authSvc := auth.New(conn, cfg.SessionSecret, cfg.SessionLifetime, cfg.AppEnv == "production")
	store, err := storage.New(context.Background(), cfg)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}
	log.Printf("storage: KYC files → %s", store.Name())
	// Not fatal: a transient S3 problem at boot shouldn't take the whole
	// API down, but a misconfiguration must be visible in the log.
	checkCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := storage.Check(checkCtx, store); err != nil {
		log.Printf("storage: WARNING: %v — KYC uploads will fail until this is fixed", err)
	}
	cancel()
	deps := &app.Deps{DB: conn, Auth: authSvc, Cfg: cfg, Storage: store}
	deps.Registration, err = registrationverification.NewService(conn.DB, []byte(cfg.SessionSecret), cfg.AppURL, securitymail.New(cfg))
	if err != nil {
		log.Printf("registration verification unavailable: check application URL and signing key configuration")
	}
	deps.WithdrawalSecurity, err = withdrawalsecurity.NewService(conn.DB, []byte(cfg.SessionSecret))
	if err != nil {
		log.Printf("withdrawal security unavailable: check signing key configuration")
	}
	if cfg.BlockchainEnabled {
		deps.Blockchain, err = blockchain.New(cfg.BlockchainServiceURL, cfg.BlockchainServiceToken)
		if err != nil {
			log.Fatalf("blockchain configuration: %v", err)
		}
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(corsMiddleware(cfg.FrontendOrigin))

	r.Get("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok"}`))
	})

	handlers.RegisterAuthRoutes(r, deps)
	handlers.RegisterBlockchainRoutes(r, deps)
	handlers.RegisterWithdrawalCallbackRoutes(r, deps)
	handlers.RegisterBlockchainVestingRoutes(r, deps)
	handlers.RegisterIncomeCapRoutes(r, deps)
	handlers.RegisterTierAccountLinkRoutes(r, deps)
	handlers.RegisterGoogleAuthRoutes(r, deps)
	handlers.RegisterTokenRoutes(r, deps)
	handlers.RegisterDashboardRoutes(r, deps)
	handlers.RegisterPackageRoutes(r, deps)
	handlers.RegisterCountriesRoutes(r, deps)
	handlers.RegisterKycRoutes(r, deps)
	handlers.RegisterGeneologyRoutes(r, deps)
	handlers.RegisterMiningRoutes(r, deps)
	handlers.RegisterRocRoutes(r, deps)
	handlers.RegisterSalaryRoutes(r, deps)
	handlers.RegisterDirectShareRoutes(r, deps)
	handlers.RegisterLeaderExecutiveRoutes(r, deps)
	handlers.RegisterUserRoutes(r, deps)
	handlers.RegisterUserParentMapsLogRoutes(r, deps)
	handlers.RegisterEarnLogRoutes(r, deps)
	handlers.RegisterAPIV1Routes(r, deps)

	if cfg.EnableScheduler {
		if cfg.BlockchainEnabled {
			jobs.StartDepositNotifications(conn, deps.Blockchain, securitymail.New(cfg))
			handlers.StartAutomaticPurchases(deps)
			handlers.StartRegistrationReopening(deps)
			handlers.StartCommissionPreparation(deps)
			handlers.StartCommissionDelivery(deps)
			handlers.StartWithdrawalSubmission(deps)
			handlers.StartWithdrawalCancellation(deps)
		}
		jobs.StartScheduler(conn, cfg)
		if cfg.RegistrationExpiryEnabled {
			jobs.StartRegistrationExpiry(conn, deps.Blockchain, cfg.BlockchainEnabled)
		}
	}

	log.Printf("Signet API listening on :%s (env=%s, db=%s@%s)", cfg.Port, cfg.AppEnv, cfg.DBDatabase, cfg.DBHost)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatal(err)
	}
}

func corsMiddleware(origin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
