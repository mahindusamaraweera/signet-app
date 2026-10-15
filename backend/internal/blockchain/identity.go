package blockchain

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var localOperationIdentity = regexp.MustCompile(`^(topup-plan:)?[1-9][0-9]{0,19}$`)

var uuidIdentity = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// UUID resolvers are installed once, before serving requests. Numeric user IDs
// remain local foreign keys; they are never the custody customer identity.
func (g *Gateway) UseUUIDIdentity(resolve func(context.Context, uint64) (string, error), reverse func(context.Context, string) (uint64, error)) {
	g.identityResolver = resolve
	g.identityReverse = reverse
}

func OperationUUID(owner, kind, local string) (string, error) {
	if !uuidIdentity.MatchString(owner) {
		return "", fmt.Errorf("invalid customer UUID")
	}
	namespace, err := hex.DecodeString(strings.ReplaceAll(owner, "-", ""))
	if err != nil {
		return "", err
	}
	h := sha1.New()
	h.Write(namespace)
	h.Write([]byte("signet-app:" + kind + ":" + local))
	b := h.Sum(nil)[:16]
	b[6] = (b[6] & 15) | 0x50
	b[8] = (b[8] & 63) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

func (g *Gateway) customerUUID(ctx context.Context, local string) (string, error) {
	if uuidIdentity.MatchString(local) {
		return local, nil
	}
	id, err := strconv.ParseUint(local, 10, 64)
	if err != nil || id == 0 || strconv.FormatUint(id, 10) != local || g.identityResolver == nil {
		return "", ErrRejected
	}
	uuid, err := g.identityResolver(ctx, id)
	if err != nil || !uuidIdentity.MatchString(uuid) {
		return "", fmt.Errorf("%w: missing/invalid UUID for local user %d", ErrUnavailable, id)
	}
	return uuid, nil
}

// Only recognized identity fields are transformed. Prices, native custody IDs,
// cap accounting, receipt hashes and opaque signed tokens are never rewritten.
func (g *Gateway) prepareUUIDWire(ctx context.Context, body interface{}) (map[string]interface{}, map[string]string, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, nil, err
	}
	var root map[string]interface{}
	if err = decodeIdentityJSON(encoded, &root); err != nil {
		return nil, nil, err
	}
	back := map[string]string{}
	resolvedCustomers := map[string]string{}
	bindings := map[string]string{}
	if previous, ok := root["identity_bindings"].(map[string]interface{}); ok {
		for uuid, value := range previous {
			legacy, ok := value.(string)
			if !ok || !uuidIdentity.MatchString(uuid) {
				return nil, nil, ErrRejected
			}
			id, err := strconv.ParseUint(legacy, 10, 64)
			if err != nil || id == 0 || strconv.FormatUint(id, 10) != legacy {
				return nil, nil, ErrRejected
			}
			bindings[uuid] = legacy
		}
	}
	identityFields := map[string]bool{"external_user_id": true, "external_buyer_id": true, "external_recipient_id": true, "admin_id": true}
	var identities func(interface{}) error
	identities = func(node interface{}) error {
		switch n := node.(type) {
		case map[string]interface{}:
			for key, value := range n {
				if identityFields[key] {
					local, ok := value.(string)
					if !ok {
						return ErrRejected
					}
					if key == "external_recipient_id" && (local == "1" || local == "treasury") {
						n[key] = "treasury"
						if local == "1" {
							back[key+"|treasury"] = "1"
						}
						continue
					}
					wire, found := resolvedCustomers[local]
					if !found {
						var e error
						wire, e = g.customerUUID(ctx, local)
						if e != nil {
							return e
						}
						resolvedCustomers[local] = wire
					}
					n[key] = wire
					if local != wire {
						back[key+"|"+wire] = local
						if previous, ok := bindings[wire]; ok && previous != local {
							return fmt.Errorf("%w: duplicate source customer UUID", ErrRejected)
						}
						bindings[wire] = local
					}
				} else if key != "signature" {
					if e := identities(value); e != nil {
						return e
					}
				}
			}
		case []interface{}:
			for _, value := range n {
				if e := identities(value); e != nil {
					return e
				}
			}
		}
		return nil
	}
	if err = identities(root); err != nil {
		return nil, nil, err
	}
	owner, _ := root["external_user_id"].(string)
	if owner == "" {
		owner, _ = root["external_buyer_id"].(string)
	}
	kinds := map[string]string{"external_purchase_id": "purchase", "external_outbox_id": "outbox", "external_earn_log_id": "earn-log", "placement_log_id": "placement", "expired_placement_log_id": "placement", "replacement_placement_log_id": "placement", "new_placement_log_id": "placement"}
	var operations func(interface{}) error
	operations = func(node interface{}) error {
		switch n := node.(type) {
		case map[string]interface{}:
			for key, value := range n {
				if kind, ok := kinds[key]; ok {
					local, ok := value.(string)
					if !ok {
						return ErrRejected
					}
					if uuidIdentity.MatchString(local) {
						continue
					}
					if !localOperationIdentity.MatchString(local) {
						return ErrRejected
					}
					if strings.HasPrefix(local, "topup-plan:") {
						kind = "topup-plan"
					}
					wire, e := OperationUUID(owner, kind, local)
					if e != nil {
						return e
					}
					n[key] = wire
					back[key+"|"+wire] = local
				} else {
					if e := operations(value); e != nil {
						return e
					}
				}
			}
		case []interface{}:
			for _, v := range n {
				if e := operations(v); e != nil {
					return e
				}
			}
		}
		return nil
	}
	if err = operations(root); err != nil {
		return nil, nil, err
	}
	root["identity_bindings"] = bindings
	return root, back, nil
}

func normalizeUUIDResponse(raw []byte, back map[string]string) ([]byte, error) {
	var body interface{}
	if err := decodeIdentityJSON(raw, &body); err != nil {
		return nil, err
	}
	var walk func(interface{})
	walk = func(node interface{}) {
		switch n := node.(type) {
		case map[string]interface{}:
			for key, value := range n {
				if text, ok := value.(string); ok {
					if local, found := back[key+"|"+text]; found {
						n[key] = local
					}
				} else {
					walk(value)
				}
			}
		case []interface{}:
			for _, value := range n {
				walk(value)
			}
		}
	}
	walk(body)
	return json.Marshal(body)
}

func decodeIdentityJSON(raw []byte, target interface{}) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return decoder.Decode(target)
}
