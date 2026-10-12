package handlers

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"
	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/blockchain"
	"signet-backend/internal/config"
	"signet-backend/internal/models"
)

func TestPendingRegistrationCanSignInAfterFundingDeadline(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		intents int64
		want    string
	}{
		{"deposited purchase", 1, "/register/funding/42"},
		{"no package selected", 0, "/register/step2/42"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := &commissionSQLFixture{steps: []commissionSQLStep{
				{contains: "SELECT * FROM users WHERE email", columns: []string{"id", "email", "password", "role", "status", "created_at", "email_verified_at"}, values: []driver.Value{int64(42), "test@example.com", string(hash), "user", "pending", time.Now().Add(-24 * time.Hour), time.Now().Add(-23 * time.Hour)}},
				{contains: "SELECT COUNT(*) FROM package_purchase_intents WHERE user_id=?", columns: []string{"count"}, values: []driver.Value{tc.intents}, args: []driver.Value{int64(42)}},
			}}
			db := sqlx.NewDb(sql.OpenDB(fixture), "commission-fixture")
			defer db.Close()
			d := &app.Deps{DB: db, Cfg: &config.Config{BlockchainEnabled: true}, Auth: auth.New(db, "test-secret", time.Hour, false)}
			w := httptest.NewRecorder()
			loginHandler(d)(w, httptest.NewRequest("POST", "/login", strings.NewReader(`{"email":"test@example.com","password":"test-password"}`)))
			var body struct{ Status, Redirect string }
			json.Unmarshal(w.Body.Bytes(), &body)
			if w.Code != 200 || body.Status != "success" || body.Redirect != tc.want || len(w.Result().Cookies()) == 0 || len(fixture.steps) != 0 {
				t.Fatalf("login recovery failed: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestPackageAdminCanSaveNullableMetadata(t *testing.T) {
	fixture := &commissionSQLFixture{steps: []commissionSQLStep{
		{contains: "SELECT * FROM packages WHERE id", columns: []string{"id", "name", "price", "commission", "status", "rank", "telegram_link"}, values: []driver.Value{int64(14), "1 USDT", int64(1), int64(1), "active", nil, nil}},
		{contains: "UPDATE packages SET", args: []driver.Value{"1 USDT", int64(1), int64(1), nil, nil, "active", int64(14)}},
	}}
	db := sqlx.NewDb(sql.OpenDB(fixture), "commission-fixture")
	defer db.Close()
	r := httptest.NewRequest("PUT", "/packages/14", strings.NewReader(`{"name":"1 USDT","price":1,"commission":1,"rank":"","telegram_link":null,"status":"active"}`))
	route := chi.NewRouteContext()
	route.URLParams.Add("id", "14")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, route))
	w := httptest.NewRecorder()
	packageUpdateHandler(&app.Deps{DB: db, Cfg: &config.Config{BlockchainEnabled: true}})(w, r)
	if w.Code != 200 || len(fixture.steps) != 0 {
		t.Fatalf("nullable metadata save failed: %d %s", w.Code, w.Body.String())
	}
}

func TestPurchaseCatalogMatchesScreenshotActiveTiers(t *testing.T) {
	prices := []int64{100, 1000, 2500, 5000, 10000, 25000, 50000, 100000, 500, 500000, 1000000, 1}
	var packages []models.Package
	for i, p := range prices {
		packages = append(packages, models.Package{ID: uint64(i + 2), Price: p, Status: "active"})
	}
	packages = append(packages, models.Package{ID: 99, Price: 50, Status: "active"})
	choices := purchasePackageChoices(packages, true)
	if len(choices) != 12 {
		t.Fatalf("got %d approved packages", len(choices))
	}
	for i, choice := range choices {
		if choice.ID != packages[i].ID || choice.Terms == nil || choice.Terms.Required == "" {
			t.Fatal("IDs or commercial terms lost")
		}
	}
	data, err := json.Marshal(choices)
	if err != nil || !strings.Contains(string(data), `"service_fee_usdt":"0.10"`) || !strings.Contains(string(data), `"required_usdt":"1.10"`) {
		t.Fatalf("incorrect terms JSON: %s %v", data, err)
	}
}

func TestPackageValidationRejectsUnapprovedAndFractionalPrices(t *testing.T) {
	for _, tc := range []struct {
		price   float64
		status  string
		invalid bool
	}{{50, "active", true}, {50, "deactive", false}, {1, "active", false}, {1.5, "active", true}, {0, "active", true}} {
		name, commission := "Test", float64(1)
		body := packagePayload{Name: &name, Price: &tc.price, Commission: &commission, Status: &tc.status}
		errs := validatePackagePayload(&body)
		validateCommercialPackagePayload(&app.Deps{Cfg: &config.Config{BlockchainEnabled: true}}, &body, errs)
		if (len(errs) > 0) != tc.invalid {
			t.Fatalf("price %v status %s errors %v", tc.price, tc.status, errs)
		}
	}
}

func authenticatedRecoveryRequest(t *testing.T, d *app.Deps, user *models.User, method, path, body string) *http.Request {
	t.Helper()
	w := httptest.NewRecorder()
	if err := d.Auth.IssueSession(w, user); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.AddCookie(w.Result().Cookies()[0])
	return r
}

func TestLegacyPurchaseRejectsInactivePackage(t *testing.T) {
	fixture := &commissionSQLFixture{steps: []commissionSQLStep{
		{contains: "SELECT * FROM users WHERE id", columns: []string{"id", "role", "status", "password"}, values: []driver.Value{int64(42), "user", "active", "test-hash"}},
		{contains: "SELECT parent_id FROM user_parents", err: sql.ErrNoRows},
		{contains: "SELECT * FROM users WHERE id", columns: []string{"id"}, values: []driver.Value{int64(1)}},
		{contains: "SELECT * FROM packages WHERE id = ? AND status = 'active'", err: sql.ErrNoRows, args: []driver.Value{int64(10)}},
	}}
	db := sqlx.NewDb(sql.OpenDB(fixture), "commission-fixture")
	defer db.Close()
	d := &app.Deps{DB: db, Cfg: &config.Config{}, Auth: auth.New(db, "test", time.Hour, false)}
	r := authenticatedRecoveryRequest(t, d, &models.User{ID: 42, Role: "user", Status: "active", Password: "test-hash"}, "POST", "/buy-packages", `{"package":"10"}`)
	w := httptest.NewRecorder()
	d.Auth.RequireAuth(http.HandlerFunc(buyPackagesHandler(d))).ServeHTTP(w, r)
	if !strings.Contains(w.Body.String(), "Package not found") || len(fixture.steps) != 0 {
		t.Fatalf("inactive package accepted: %s", w.Body.String())
	}
}

func TestPurchaseRetryRejectsAnotherOwnerBeforeRemoteExecution(t *testing.T) {
	fixture := &commissionSQLFixture{steps: []commissionSQLStep{{contains: "SELECT user_id FROM package_purchase_intents", columns: []string{"user_id"}, values: []driver.Value{int64(43)}}}}
	db := sqlx.NewDb(sql.OpenDB(fixture), "commission-fixture")
	defer db.Close()
	err := syncAutomaticPurchaseForUser(&app.Deps{DB: db}, 7, 42)
	if !errors.Is(err, blockchain.ErrUnavailable) || fixture.committed || len(fixture.steps) != 0 {
		t.Fatalf("owner boundary failed: %v", err)
	}
}

func TestAuthenticatedRetryUsesOwnIntentAndRequiresConfirmedSettlement(t *testing.T) {
	for _, scenario := range []string{"partial deposit", "unconfirmed completion", "already completed"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := &commissionSQLFixture{steps: []commissionSQLStep{
				{contains: "SELECT * FROM users WHERE id", columns: []string{"id", "role", "status", "password"}, values: []driver.Value{int64(42), "user", "pending", "test-hash"}},
				{contains: "SELECT id FROM package_purchase_intents WHERE user_id=?", columns: []string{"id"}, values: []driver.Value{int64(7)}, args: []driver.Value{int64(42), int64(7)}},
				{contains: "UPDATE package_purchase_intents SET last_checked_at", args: []driver.Value{int64(7)}},
				{contains: "SELECT user_id FROM package_purchase_intents", columns: []string{"user_id"}, values: []driver.Value{int64(42)}},
				{contains: "SELECT status FROM users", columns: []string{"status"}, values: []driver.Value{"pending"}},
			}}
			if scenario == "already completed" {
				fixture.steps = append(fixture.steps, commissionSQLStep{contains: "SELECT id,user_id,user_package_id", err: sql.ErrNoRows})
			} else {
				fixture.steps = append(fixture.steps,
					commissionSQLStep{contains: "SELECT id,user_id,user_package_id", columns: []string{"id", "user_id", "user_package_id", "package_price", "package_id", "status", "purchase_kind", "service_fee_usdt", "required_usdt", "bonus_percent"}, values: []driver.Value{int64(7), int64(42), int64(9), "1", int64(14), "awaiting_deposit", "first", "0.10", "1.10", int64(200)}},
					commissionSQLStep{contains: "SELECT user_id FROM user_packages", columns: []string{"user_id"}, values: []driver.Value{int64(42)}},
					commissionSQLStep{contains: "SELECT id FROM user_parents", columns: []string{"id"}, values: []driver.Value{int64(6)}},
				)
			}
			if scenario == "partial deposit" {
				fixture.steps = append(fixture.steps, commissionSQLStep{contains: "UPDATE package_purchase_intents SET status", args: []driver.Value{"awaiting_deposit", "1", "0.100000000000000000", true, "", int64(7)}})
			}
			var diagnostic interface{}
			if scenario == "unconfirmed completion" {
				diagnostic = "activation_sync_unavailable"
			}
			fixture.steps = append(fixture.steps, commissionSQLStep{contains: "UPDATE package_purchase_intents SET last_sync_error", args: []driver.Value{diagnostic, int64(7)}})
			if scenario != "unconfirmed completion" {
				status := "completed"
				if scenario == "partial deposit" {
					status = "awaiting_deposit"
				}
				fixture.steps = append(fixture.steps, commissionSQLStep{contains: "SELECT id,status,purchase_kind,available_usdt,remaining_usdt", columns: []string{"id", "status", "purchase_kind", "available_usdt", "remaining_usdt"}, values: []driver.Value{int64(7), status, "first", "1", "0.100000000000000000"}, args: []driver.Value{int64(7), int64(42)}})
			}
			db := sqlx.NewDb(sql.OpenDB(fixture), "commission-fixture")
			defer db.Close()
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var payload map[string]interface{}
				json.NewDecoder(r.Body).Decode(&payload)
				if payload["external_user_id"] != "42" || payload["external_purchase_id"] != "7" || payload["package_id"] != "14" || r.Header.Get("Idempotency-Key") != "signet-app:purchase:7" {
					t.Error("changed owner, package ID, or durable purchase identity")
				}
				if scenario == "partial deposit" {
					w.Write([]byte(`{"status":"awaiting_deposit","available_usdt":"1","deposit_seen":true,"required_usdt":"1.10","service_fee_usdt":"0.10"}`))
				} else {
					w.Write([]byte(`{"status":"completed","available_usdt":"0","deposit_seen":true,"required_usdt":"1.10","service_fee_usdt":"0.10","order_id":"remote-7","swap_confirmed":false,"vesting_allocated":false}`))
				}
			}))
			defer server.Close()
			gateway, err := blockchain.New(server.URL, "test")
			if err != nil {
				t.Fatal(err)
			}
			d := &app.Deps{DB: db, Cfg: &config.Config{BlockchainEnabled: true}, Blockchain: gateway, Auth: auth.New(db, "test", time.Hour, false)}
			r := authenticatedRecoveryRequest(t, d, &models.User{ID: 42, Role: "user", Status: "pending", Password: "test-hash"}, "POST", "/blockchain/purchase/sync", `{"user_id":43,"purchase_id":"7"}`)
			w := httptest.NewRecorder()
			d.Auth.RequireAuth(http.HandlerFunc(purchaseSyncHandler(d))).ServeHTTP(w, r)
			wantCode, wantCalls := 200, 1
			if scenario == "unconfirmed completion" {
				wantCode = 503
			}
			if scenario == "already completed" {
				wantCalls = 0
			}
			if w.Code != wantCode || calls != wantCalls || len(fixture.steps) != 0 || fixture.committed != (scenario == "partial deposit") {
				t.Fatalf("unsafe retry: HTTP %d calls %d remaining steps %d commit %v body %s", w.Code, calls, len(fixture.steps), fixture.committed, w.Body.String())
			}
		})
	}
}

func TestPurchaseRetryRequiresAuthentication(t *testing.T) {
	d := &app.Deps{Auth: auth.New(nil, "test", time.Hour, false)}
	w := httptest.NewRecorder()
	d.Auth.RequireAuth(http.HandlerFunc(purchaseSyncHandler(d))).ServeHTTP(w, httptest.NewRequest("POST", "/blockchain/purchase/sync", nil))
	if w.Code != 401 {
		t.Fatalf("anonymous retry accepted: %d", w.Code)
	}
}
