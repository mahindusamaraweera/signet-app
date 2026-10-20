// Package blockchain connects the application to its financial service.
package blockchain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var ErrUnavailable = errors.New("blockchain service unavailable")
var ErrRejected = errors.New("blockchain request rejected without applying changes")
var ErrConflict = errors.New("blockchain request requires reconciliation")

type Gateway struct {
	identityResolver func(context.Context, uint64) (string, error)
	identityReverse  func(context.Context, string) (uint64, error)
	environment      string
	baseURL          string
	token            string
	client           *http.Client
}

// New rejects insecure remote transports and never follows credential-bearing redirects.
func New(baseURL, token string, environment ...string) (*Gateway, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("invalid blockchain service URL")
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return nil, errors.New("blockchain service requires HTTPS")
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("blockchain service token is required")
	}
	env := ""
	if len(environment) > 0 {
		env = environment[0]
	}
	if env != "" && !regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`).MatchString(env) {
		return nil, errors.New("invalid integration environment")
	}
	return &Gateway{environment: env, baseURL: strings.TrimRight(baseURL, "/"), token: token,
		client: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

type Wallet struct {
	Balances           map[string]AssetBalance `json:"balances,omitempty"`
	Address            string                  `json:"address"`
	ChainID            uint64                  `json:"chain_id"`
	Asset              string                  `json:"asset"`
	Available          string                  `json:"available"`
	ProvisioningStatus string                  `json:"provisioning_status"`
}

type AssetBalance struct {
	Available         string `json:"available"`
	Locked            string `json:"locked"`
	PendingWithdrawal string `json:"pending_withdrawal"`
}

// Explicit authoritative eligibility is required; absence is never consent.
func (g *Gateway) CanExpireRegistration(ctx context.Context, userID uint64) (bool, error) {
	var result struct {
		CanExpire *bool `json:"can_expire"`
	}
	err := g.request(ctx, http.MethodPost, "/api/v1/integration/registration-expiry/check", "signet-app:expiry-check:"+strconv.FormatUint(userID, 10), map[string]interface{}{
		"source": "signet-app", "external_user_id": strconv.FormatUint(userID, 10),
	}, &result)
	if err != nil {
		return false, err
	}
	if result.CanExpire == nil {
		return false, ErrUnavailable
	}
	return *result.CanExpire, nil
}

// EnsureWallet uses a source-qualified identity; IDs must never be matched across databases.
func (g *Gateway) EnsureWallet(ctx context.Context, userID uint64) (*Wallet, error) {
	if userID == 0 {
		return nil, errors.New("user ID is required")
	}
	key := "signet-app:wallet:" + strconv.FormatUint(userID, 10)
	var result Wallet
	err := g.request(ctx, http.MethodPost, "/api/v1/integration/wallets", key, map[string]interface{}{
		"source": "signet-app", "external_user_id": strconv.FormatUint(userID, 10),
	}, &result)
	if err != nil {
		return nil, err
	}
	if result.Asset != "USDT" || result.ChainID == 0 || !decimalPattern.MatchString(result.Available) {
		return nil, ErrUnavailable
	}
	if result.Balances != nil {
		if len(result.Balances) != 2 {
			return nil, ErrUnavailable
		}
		for _, asset := range []string{"USDT", "SIGNET"} {
			b, exists := result.Balances[asset]
			if !exists || !decimalPattern.MatchString(b.Available) || !decimalPattern.MatchString(b.Locked) || !decimalPattern.MatchString(b.PendingWithdrawal) {
				return nil, ErrUnavailable
			}
		}
		usdt, _ := new(big.Rat).SetString(result.Balances["USDT"].Available)
		available, _ := new(big.Rat).SetString(result.Available)
		if usdt.Cmp(available) != 0 {
			return nil, ErrUnavailable
		}
	}
	if result.ProvisioningStatus == "ready" {
		if len(result.Address) != 42 || !strings.HasPrefix(result.Address, "0x") {
			return nil, ErrUnavailable
		}
		for _, c := range result.Address[2:] {
			if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
				return nil, ErrUnavailable
			}
		}
	} else if result.ProvisioningStatus != "pending" {
		return nil, ErrUnavailable
	}
	return &result, nil
}

func (g *Gateway) request(ctx context.Context, method, path, key string, body interface{}, out interface{}) error {
	var back map[string]string
	if g.identityResolver != nil {
		wire, mapping, err := g.prepareUUIDWire(ctx, body)
		if err != nil {
			return err
		}
		body = wire
		owner, _ := wire["external_user_id"].(string)
		if owner == "" {
			owner, _ = wire["external_buyer_id"].(string)
		}
		key = fmt.Sprintf("signet-app:uuid-v1:%x", sha256.Sum256([]byte(key+":"+owner)))
		back = mapping
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, g.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if g.environment != "" {
		req.Header.Set("X-Signet-App-Environment", g.environment)
	}
	if g.identityResolver != nil {
		req.Header.Set("X-Signet-Identity-Version", "uuid-v1")
	}
	req.Header.Set("Idempotency-Key", key)
	// Errors keep their sentinel (errors.Is still works) but say why, so the
	// "purchase intent N: …" worker log explains an activation_sync_unavailable.
	res, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %s %s: %w", ErrUnavailable, method, path, err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnprocessableEntity {
		return requestFailure(res, ErrRejected, method, path)
	}
	if res.StatusCode == http.StatusConflict {
		return requestFailure(res, errors.Join(ErrUnavailable, ErrConflict), method, path)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return requestFailure(res, ErrUnavailable, method, path)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 65537))
	if err != nil {
		return fmt.Errorf("%w: %s %s: reading response: %v", ErrUnavailable, method, path, err)
	}
	if len(data) > 65536 {
		return fmt.Errorf("%w: %s %s: response larger than 64 KiB", ErrUnavailable, method, path)
	}
	if back != nil {
		data, err = normalizeUUIDResponse(data, back)
		if err != nil {
			return ErrUnavailable
		}
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%w: %s %s: invalid JSON response: %v", ErrUnavailable, method, path, err)
	}
	return nil
}

// errorSnippet returns the start of an error response body (the service's
// own reason), single-line and bounded, for logs.
func errorSnippet(body io.Reader) string {
	data, _ := io.ReadAll(io.LimitReader(body, 300))
	s := strings.Join(strings.Fields(string(data)), " ")
	if s == "" {
		return "(empty body)"
	}
	return s
}
