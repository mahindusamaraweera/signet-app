package handlers

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/blockchain"
	"signet-backend/internal/models"
)

var kycFixtures sync.Map
var kycDriverOnce sync.Once

type kycFixture struct {
	mu       sync.Mutex
	verified bool
	status   string
	writes   int
	previous time.Time
}
type kycDriver struct{}
type kycConn struct{ fixture *kycFixture }
type kycTx struct{}
type kycRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (kycDriver) Open(name string) (driver.Conn, error) {
	v, ok := kycFixtures.Load(name)
	if !ok {
		return nil, errors.New("unknown fixture")
	}
	return &kycConn{v.(*kycFixture)}, nil
}
func (c *kycConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unexpected prepare") }
func (c *kycConn) Close() error                        { return nil }
func (c *kycConn) Begin() (driver.Tx, error)           { return kycTx{}, nil }
func (kycTx) Commit() error                            { return nil }
func (kycTx) Rollback() error                          { return nil }
func (r *kycRows) Columns() []string                   { return r.columns }
func (r *kycRows) Close() error                        { return nil }
func (r *kycRows) Next(out []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(out, r.values[r.index])
	r.index++
	return nil
}
func (c *kycConn) QueryContext(_ context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	if strings.HasPrefix(q, "SELECT * FROM users WHERE id") {
		return &kycRows{columns: []string{"id", "password", "role", "email"}, values: [][]driver.Value{{int64(42), "stored-password-hash", "user", "test@example.test"}}}, nil
	}
	if strings.HasPrefix(q, "SELECT tier_user_id FROM tier_account_links") {
		return &kycRows{columns: []string{"tier_user_id"}, values: [][]driver.Value{{"30"}}}, nil
	}
	if strings.HasPrefix(q, "SELECT provider_checked_at") {
		c.fixture.mu.Lock()
		defer c.fixture.mu.Unlock()
		values := [][]driver.Value{}
		if !c.fixture.previous.IsZero() {
			values = append(values, []driver.Value{c.fixture.previous})
		}
		return &kycRows{columns: []string{"provider_checked_at"}, values: values}, nil
	}
	if strings.Contains(q, "SELECT p.telegram_link") {
		return &kycRows{columns: []string{"telegram_link"}}, nil
	}
	return nil, errors.New("unexpected query (vesting must not be needed): " + q)
}
func (c *kycConn) ExecContext(_ context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	if !strings.HasPrefix(q, "INSERT INTO tier_identity_verifications") {
		return nil, errors.New("unexpected write")
	}
	c.fixture.mu.Lock()
	defer c.fixture.mu.Unlock()
	c.fixture.status = args[2].Value.(string)
	c.fixture.verified = args[3].Value.(bool)
	c.fixture.previous = args[6].Value.(time.Time)
	c.fixture.writes++
	return driver.RowsAffected(1), nil
}
func TestSharedTierKycPersistsApprovalAndRevocationWithoutVesting(t *testing.T) {
	kycDriverOnce.Do(func() { sql.Register("tier-kyc-fixture", kycDriver{}) })
	fixture := &kycFixture{}
	kycFixtures.Store(t.Name(), fixture)
	defer kycFixtures.Delete(t.Name())
	raw, err := sql.Open("tier-kyc-fixture", t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	db := sqlx.NewDb(raw, "mysql")
	verified := true
	remoteTierID := "30"
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["external_user_id"] != "42" {
			t.Error("request user was not authenticated owner")
		}
		status := "rejected"
		var until *time.Time
		if verified {
			status = "approved"
			value := time.Now().UTC().Add(30 * time.Minute)
			until = &value
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"external_user_id": "42", "linked": true, "tier_user_id": remoteTierID, "status": status, "verified": verified, "withdrawal_required": true, "environment": "production", "level": "SIGNET", "checked_at": time.Now().UTC(), "valid_until": until, "verification_path": "/identity-verification?account=11111111-1111-4111-8111-111111111111"})
	}))
	defer service.Close()
	gateway, _ := blockchain.New(service.URL, "test-token")
	authentication := auth.New(db, strings.Repeat("secret", 8), time.Hour, false)
	cookieResponse := httptest.NewRecorder()
	if err := authentication.IssueSession(cookieResponse, &models.User{ID: 42, Password: "stored-password-hash", Role: "user"}); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	RegisterKycRoutes(router, &app.Deps{DB: db, Auth: authentication, Blockchain: gateway})
	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		req.AddCookie(cookieResponse.Result().Cookies()[0])
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	if rec := get("/api/v1/kyc/show?user_id=43"); rec.Code != 200 {
		t.Fatalf("approval: %d %s", rec.Code, rec.Body.String())
	}
	if !fixture.verified || fixture.writes != 1 {
		t.Fatal("linked approval not recorded")
	}
	verified = false
	if rec := get("/api/v1/kyc/show"); rec.Code != 200 {
		t.Fatalf("revocation: %s", rec.Body.String())
	}
	if fixture.verified || fixture.status != "rejected" {
		t.Fatal("revocation did not clear mirror")
	}
	remoteTierID = "31"
	if rec := get("/api/v1/kyc/show"); rec.Code != 503 {
		t.Fatal("mismatched link was accepted")
	}
	if fixture.writes != 2 {
		t.Fatal("mismatched identity changed stored status")
	}
	for _, path := range []string{"/api/v1/kyc/create", "/api/v1/kyc/verified", "/api/v1/kyc/1/edit"} {
		if get(path).Code != 410 {
			t.Fatal("legacy workflow remains reachable")
		}
	}
	req := httptest.NewRequest("POST", "/api/v1/kyc/1/verify", nil)
	req.AddCookie(cookieResponse.Result().Cookies()[0])
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != 410 {
		t.Fatal("manual approval remains enabled")
	}
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/storage/kyc/nic_front/private.png", nil))
	if rec.Code != 404 {
		t.Fatal("legacy documents exposed")
	}
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/kyc/show", nil))
	if rec.Code != 401 {
		t.Fatal("anonymous status exposed")
	}
}
