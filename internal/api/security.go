package api

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// SecurityPolicy configures request-boundary protections for the HTTP server.
// A zero value disables rate limiting and trusts no forwarded headers.
type SecurityPolicy struct {
	TrustedProxies    []netip.Prefix
	RateLimitRequests int
	RateLimitWindow   time.Duration
}

// securityHeaders applies hardening headers to every response. HSTS is only
// emitted for TLS-terminated requests.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP resolves the caller address. Forwarded headers are honored only
// when the direct peer is a configured trusted proxy.
func clientIP(r *http.Request, trustedProxies []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil {
		return host
	}
	if !isTrustedProxy(peer, trustedProxies) {
		return peer.String()
	}
	forwarded := r.Header.Values("X-Forwarded-For")
	for i := len(forwarded) - 1; i >= 0; i-- {
		for _, candidate := range strings.Split(forwarded[i], ",") {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				continue
			}
			address, err := netip.ParseAddr(strings.Trim(candidate, "[]"))
			if err != nil {
				return candidate
			}
			if !isTrustedProxy(address, trustedProxies) {
				return address.String()
			}
		}
	}
	return peer.String()
}

func isTrustedProxy(address netip.Addr, trustedProxies []netip.Prefix) bool {
	for _, prefix := range trustedProxies {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

type rateLimiter struct {
	mu       sync.Mutex
	requests int
	window   time.Duration
	buckets  map[string]*tokenBucket
	lastGC   time.Time
}

type tokenBucket struct {
	tokens float64
	last   time.Time
}

func newRateLimiter(requests int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		requests: requests, window: window, buckets: make(map[string]*tokenBucket), lastGC: time.Now(),
	}
}

func (l *rateLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.lastGC) > l.window*2 {
		for k, bucket := range l.buckets {
			if now.Sub(bucket.last) > l.window*2 {
				delete(l.buckets, k)
			}
		}
		l.lastGC = now
	}
	bucket, ok := l.buckets[key]
	if !ok {
		l.buckets[key] = &tokenBucket{tokens: float64(l.requests) - 1, last: now}
		return true
	}
	elapsed := now.Sub(bucket.last).Seconds()
	bucket.tokens += elapsed * (float64(l.requests) / l.window.Seconds())
	if bucket.tokens > float64(l.requests) {
		bucket.tokens = float64(l.requests)
	}
	bucket.last = now
	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens--
	return true
}

func rateLimitHandler(next http.Handler, policy SecurityPolicy) http.Handler {
	if policy.RateLimitRequests <= 0 || policy.RateLimitWindow <= 0 {
		return next
	}
	limiter := newRateLimiter(policy.RateLimitRequests, policy.RateLimitWindow)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/") || r.URL.Path == "/api/v1/status" {
			next.ServeHTTP(w, r)
			return
		}
		if !limiter.allow(clientIP(r, policy.TrustedProxies), time.Now()) {
			writeAPIError(w, http.StatusTooManyRequests, "rate_limited", "too many requests")
			return
		}
		next.ServeHTTP(w, r)
	})
}
