package blockchain

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
)

var linkTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43,128}$`)
var tierIDPattern = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)

func LinkSecret() (string, error) {
	b := make([]byte, 48)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func LinkHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func LinkChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func SealLinkVerifier(secret []byte, userID uint64, state, verifier string) (string, error) {
	if len(secret) < 32 {
		return "", errors.New("strong session secret required for account linking")
	}
	key := sha256.Sum256(secret)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(verifier), []byte(fmt.Sprintf("tier-link:%d:%s", userID, state)))
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}
func OpenLinkVerifier(secret []byte, userID uint64, state, sealed string) (string, error) {
	if len(secret) < 32 {
		return "", errors.New("strong session secret required for account linking")
	}
	key := sha256.Sum256(secret)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	data, err := base64.RawURLEncoding.DecodeString(sealed)
	if err != nil || len(data) < gcm.NonceSize() {
		return "", errors.New("invalid encrypted link proof")
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], []byte(fmt.Sprintf("tier-link:%d:%s", userID, state)))
	return string(plain), err
}

type TierLinkResult struct {
	ExternalUserID string  `json:"external_user_id"`
	TierUserID     *string `json:"tier_user_id"`
	Linked         *bool   `json:"linked"`
}

func (g *Gateway) BeginTierLink(ctx context.Context, userID uint64, state, verifier string) (string, error) {
	var result struct {
		RequestToken      string `json:"request_token"`
		AuthorizationPath string `json:"authorization_path"`
	}
	err := g.request(ctx, "POST", "/api/v1/integration/tier-link/begin", "signet-app:tier-link:"+LinkHash(state), map[string]string{
		"source": "signet-app", "external_user_id": fmt.Sprint(userID), "state": state, "pkce_challenge": LinkChallenge(verifier),
	}, &result)
	if err != nil {
		return "", err
	}
	expected := "/integration/signet-app/link?request_token=" + url.QueryEscape(result.RequestToken)
	if !linkTokenPattern.MatchString(result.RequestToken) || result.AuthorizationPath != expected {
		return "", ErrUnavailable
	}
	return g.baseURL + expected, nil
}
func (g *Gateway) TierLinkStatus(ctx context.Context, userID uint64) (*TierLinkResult, error) {
	return g.tierLinkRequest(ctx, userID, "/status", map[string]string{})
}
func (g *Gateway) ExchangeTierLink(ctx context.Context, userID uint64, code, verifier, state string) (*TierLinkResult, error) {
	return g.tierLinkRequest(ctx, userID, "/exchange", map[string]string{"code": code, "verifier": verifier, "state": state})
}
func (g *Gateway) tierLinkRequest(ctx context.Context, userID uint64, path string, body map[string]string) (*TierLinkResult, error) {
	body["source"] = "signet-app"
	body["external_user_id"] = fmt.Sprint(userID)
	var result TierLinkResult
	if err := g.request(ctx, "POST", "/api/v1/integration/tier-link"+path, "signet-app:tier-link:"+fmt.Sprint(userID), body, &result); err != nil {
		return nil, err
	}
	if result.ExternalUserID != fmt.Sprint(userID) || result.Linked == nil || (*result.Linked && (result.TierUserID == nil || !tierIDPattern.MatchString(*result.TierUserID))) {
		return nil, ErrUnavailable
	}
	return &result, nil
}
