package handlers

import (
	"crypto/rand"
	"encoding/json"
	"math/big"
	"net/http"
	"strconv"
	"strings"
)

const upperAlnum = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// randomString ports Laravel's Str::random($n) closely enough for the
// app's purposes (referral codes, plaintext secret keys): a
// cryptographically random string drawn from mixed-case alnum. The
// original uses mixed-case; we keep that for tokens/secret keys, and
// upper-only where the original explicitly calls strtoupper(Str::random()).
func randomString(n int) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	return randomFrom(alphabet, n)
}

func randomUpperString(n int) string {
	return randomFrom(upperAlnum, n)
}

func randomFrom(alphabet string, n int) string {
	b := make([]byte, n)
	for i := range b {
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		b[i] = alphabet[idx.Int64()]
	}
	return string(b)
}

func randomHex(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	const hexdigits = "0123456789abcdef"
	out := make([]byte, nBytes*2)
	for i, c := range b {
		out[i*2] = hexdigits[c>>4]
		out[i*2+1] = hexdigits[c&0x0f]
	}
	return string(out)
}

// signetIDToNumeric strips a "SIG-00{n}" formatted id (any number of
// leading zeros after the fixed "SIG-0" prefix, case-insensitive) back to
// its numeric id, matching the original's `^SIG-0*(\d+)$` regex. If the
// input doesn't match that shape, it's returned unchanged (already numeric).
func signetIDToNumeric(raw string) string {
	upper := strings.ToUpper(strings.TrimSpace(raw))
	if !strings.HasPrefix(upper, "SIG-0") {
		return raw
	}
	rest := strings.TrimPrefix(upper, "SIG-")
	rest = strings.TrimLeft(rest, "0")
	if rest == "" {
		return "0"
	}
	for _, c := range rest {
		if c < '0' || c > '9' {
			return raw
		}
	}
	return rest
}

func parseUintParam(s string) (uint64, bool) {
	s = signetIDToNumeric(s)
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// flexString decodes a JSON string or number into a string, the way
// Laravel's $request->input() accepts either (`"812"` and `812` both give
// "812"). A plain Go string field rejects a number and stays empty.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*f = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*f = flexString(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = flexString(n.String())
	return nil
}

func decodeJSON(r *http.Request, dst interface{}) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	return dec.Decode(dst)
}

func ltrimZero(s string) string {
	trimmed := strings.TrimLeft(s, "0")
	return trimmed
}
