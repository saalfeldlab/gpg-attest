package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter enforces two layers of rate limiting on incoming requests:
//
//   - Global: caps total server throughput (each request spawns a GPG
//     subprocess + Redis pipeline, so this is the real DoS protection).
//   - Per-IP: prevents a single source from monopolizing the global budget.
//
// Per-IP state is a map of IP → limiter, swept every sweepInterval to evict
// entries that have not been seen recently.  Each entry is ~36 bytes (IP string
// + token bucket), so even millions of unique IPs stay manageable.
type RateLimiter struct {
	global  *rate.Limiter
	perIP   map[string]*ipEntry
	mu      sync.Mutex
	ipRate  rate.Limit
	ipBurst int
	sweep   time.Duration
}

type ipEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewRateLimiter creates a RateLimiter and starts a background goroutine that
// evicts stale per-IP entries every sweepInterval.
func NewRateLimiter(globalRate rate.Limit, globalBurst int, ipRate rate.Limit, ipBurst int, sweepInterval time.Duration) *RateLimiter {
	rl := &RateLimiter{
		global:  rate.NewLimiter(globalRate, globalBurst),
		perIP:   make(map[string]*ipEntry),
		ipRate:  ipRate,
		ipBurst: ipBurst,
		sweep:   sweepInterval,
	}
	go rl.sweepLoop()
	return rl
}

func (rl *RateLimiter) sweepLoop() {
	ticker := time.NewTicker(rl.sweep)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-rl.sweep)
		for ip, entry := range rl.perIP {
			if entry.lastSeen.Before(cutoff) {
				delete(rl.perIP, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) getIPLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	entry, ok := rl.perIP[ip]
	if !ok {
		entry = &ipEntry{limiter: rate.NewLimiter(rl.ipRate, rl.ipBurst)}
		rl.perIP[ip] = entry
	}
	entry.lastSeen = time.Now()
	return entry.limiter
}

// ClientIP returns the address of the originating client, used for per-IP rate
// limiting and request logging. The server runs behind a reverse proxy
// (Traefik), so the connecting peer (r.RemoteAddr) is always the proxy and
// would collapse every client into a single bucket. The originating client is
// the left-most entry of X-Forwarded-For, which Traefik sets and sanitizes: it
// overwrites client-supplied values arriving from sources outside its
// forwardedHeaders.trustedIPs. When the header is absent or its first entry is
// not a valid IP, it falls back to the connecting peer address.
//
// Only deploy this behind a proxy that sanitizes X-Forwarded-For. Exposed
// directly, a client could set the header to pick its own rate-limit bucket.
func ClientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		first := strings.TrimSpace(strings.SplitN(forwarded, ",", 2)[0])
		if ip := net.ParseIP(first); ip != nil {
			return ip.String()
		}
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// Middleware returns an http.Handler that rejects requests exceeding either
// rate limit with 429 Too Many Requests.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.global.Allow() {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		if !rl.getIPLimiter(ClientIP(r)).Allow() {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
