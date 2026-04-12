package api

import (
	"net"
	"net/http"
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

// Middleware returns an http.Handler that rejects requests exceeding either
// rate limit with 429 Too Many Requests.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.global.Allow() {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		if ip == "" {
			ip = r.RemoteAddr
		}
		if !rl.getIPLimiter(ip).Allow() {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
