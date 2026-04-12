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
