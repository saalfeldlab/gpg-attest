package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestRateLimiter_GlobalLimit(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// Allow 2 requests total, then block.
	rl := NewRateLimiter(rate.Limit(2), 2, rate.Limit(100), 100, time.Hour)
	handler := rl.Middleware(ok)

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rec.Code)
	}
}

func TestRateLimiter_PerIPLimit(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// Global is generous, per-IP allows 1 request.
	rl := NewRateLimiter(rate.Limit(100), 100, rate.Limit(1), 1, time.Hour)
	handler := rl.Middleware(ok)

	// First request from same IP succeeds.
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Second request from same IP is blocked.
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 for same IP, got %d", rec.Code)
	}

	// Request from different IP still succeeds.
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.RemoteAddr = "5.6.7.8:12345"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req2)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for different IP, got %d", rec.Code)
	}
}

func TestRateLimiter_PerIPUsesForwardedFor(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// Per-IP allows 1 request per client.
	rl := NewRateLimiter(rate.Limit(100), 100, rate.Limit(1), 1, time.Hour)
	handler := rl.Middleware(ok)

	// All requests arrive from the same proxy connection (RemoteAddr) but carry
	// distinct clients in X-Forwarded-For, so each gets its own bucket.
	for _, client := range []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"} {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "172.27.0.5:443" // proxy pod
		req.Header.Set("X-Forwarded-For", client)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("client %s: expected 200, got %d", client, rec.Code)
		}
	}

	// A second request from an already-seen client (left-most XFF entry, extra
	// proxy hop appended) is throttled.
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "172.27.0.5:443"
	req.Header.Set("X-Forwarded-For", "1.1.1.1, 172.27.0.5")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 for repeat client, got %d", rec.Code)
	}

	// With no X-Forwarded-For, the limiter falls back to RemoteAddr.
	reqNoXFF := httptest.NewRequest("GET", "/", nil)
	reqNoXFF.RemoteAddr = "9.9.9.9:1234"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, reqNoXFF)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for new RemoteAddr fallback, got %d", rec.Code)
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		forwarded  string
		want       string
	}{
		{"forwarded client", "172.27.0.5:443", "203.0.113.9", "203.0.113.9"},
		{"left-most of several hops", "172.27.0.5:443", "203.0.113.9, 172.27.0.5", "203.0.113.9"},
		{"ipv6 client", "172.27.0.5:443", "2001:db8::1", "2001:db8::1"},
		{"no header uses peer", "198.51.100.7:51234", "", "198.51.100.7"},
		{"garbage header uses peer", "198.51.100.7:51234", "not-an-ip", "198.51.100.7"},
		{"empty first entry uses peer", "198.51.100.7:51234", ", 203.0.113.9", "198.51.100.7"},
		{"peer without port", "198.51.100.7", "", "198.51.100.7"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tc.remoteAddr
			if tc.forwarded != "" {
				req.Header.Set("X-Forwarded-For", tc.forwarded)
			}
			if got := ClientIP(req); got != tc.want {
				t.Errorf("ClientIP() = %q, want %q", got, tc.want)
			}
		})
	}
}
