package httpx

import (
	"net"
	"net/http"
	"sync"
	"time"
)

type securityBucket struct {
	start time.Time
	count int
}

// SecurityRateLimit bounds public security endpoints per direct peer. Forwarded
// headers are intentionally ignored; deployments behind proxies must enforce
// an additional trusted edge limit. Email resend limits also persist in SQL.
func SecurityRateLimit(limit int, window time.Duration) func(http.Handler) http.Handler {
	var mu sync.Mutex
	buckets := map[string]securityBucket{}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil || net.ParseIP(host) == nil || limit <= 0 || window <= 0 {
				Error(w, 429, "Security request limit reached")
				return
			}
			now := time.Now()
			mu.Lock()
			bucket, exists := buckets[host]
			if !exists && len(buckets) >= 10000 {
				for key, old := range buckets {
					if now.Sub(old.start) >= window {
						delete(buckets, key)
					}
				}
			}
			allowed := true
			if !exists && len(buckets) >= 10000 {
				allowed = false
			} else {
				if now.Sub(bucket.start) >= window {
					bucket = securityBucket{start: now}
				}
				if bucket.count >= limit {
					allowed = false
				} else {
					bucket.count++
					buckets[host] = bucket
				}
			}
			mu.Unlock()
			w.Header().Set("Cache-Control", "no-store")
			if !allowed {
				w.Header().Set("Retry-After", "60")
				Error(w, 429, "Security request limit reached")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
