package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testHandler returns a Handler with no backing store — sufficient for
// request-validation tests that reject before touching the store.
func testHandler() *Handler {
	return &Handler{}
}

func TestCreateEntry_BodyTooLarge(t *testing.T) {
	h := testHandler()
	// 100 KB + 1 byte exceeds the MaxBytesReader limit.
	body := strings.NewReader(strings.Repeat("x", 100*1024+1))
	req := httptest.NewRequest("POST", "/api/v1/entries", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.createEntry(rec, req)

	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 400 or 413, got %d", rec.Code)
	}
}

func TestValidateHash(t *testing.T) {
	tests := []struct {
		name string
		hash string
		ok   bool
	}{
		{"valid sha256", "sha256:" + strings.Repeat("ab", 32), true},
		{"uppercase hex", "sha256:" + strings.Repeat("AB", 32), true},
		{"too short", "sha256:abcd", false},
		{"too long", "sha256:" + strings.Repeat("ab", 33), false},
		{"not hex", "sha256:" + strings.Repeat("zz", 32), false},
		{"missing prefix", strings.Repeat("ab", 32), false},
		{"unsupported algo", "md5:" + strings.Repeat("ab", 16), false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHash(tt.hash)
			if tt.ok && err != nil {
				t.Errorf("expected valid, got error: %v", err)
			}
			if !tt.ok && err == nil {
				t.Errorf("expected error for hash %q", tt.hash)
			}
		})
	}
}

func TestCreateEntry_InvalidHash(t *testing.T) {
	h := testHandler()
	body := `{"artifact_hash":"sha256:tooshort","category":"authenticity","verdict":"authentic","signer_keyid":"AABB","signature":"sig"}`
	req := httptest.NewRequest("POST", "/api/v1/entries", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.createEntry(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateEntry_SignatureTooLarge(t *testing.T) {
	h := testHandler()
	validHash := "sha256:" + strings.Repeat("ab", 32)
	bigSig := strings.Repeat("A", 8*1024+1)
	body := `{"artifact_hash":"` + validHash + `","category":"authenticity","verdict":"authentic","signer_keyid":"AABB","signature":"` + bigSig + `"}`
	req := httptest.NewRequest("POST", "/api/v1/entries", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.createEntry(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateEntry_SignerKeyIDTooLong(t *testing.T) {
	h := testHandler()
	validHash := "sha256:" + strings.Repeat("ab", 32)
	longKeyID := strings.Repeat("A", 257)
	body := `{"artifact_hash":"` + validHash + `","category":"authenticity","verdict":"authentic","signer_keyid":"` + longKeyID + `","signature":"sig"}`
	req := httptest.NewRequest("POST", "/api/v1/entries", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.createEntry(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListEntries_HashValidation(t *testing.T) {
	tests := []struct {
		name  string
		query string
		code  int
	}{
		{"missing hash", "", http.StatusBadRequest},
		{"invalid hash", "hash=not-a-hash", http.StatusBadRequest},
		{"too short hash", "hash=sha256:abcd", http.StatusBadRequest},
	}
	h := testHandler()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := "/api/v1/entries"
			if tt.query != "" {
				path += "?" + tt.query
			}
			req := httptest.NewRequest("GET", path, nil)
			rec := httptest.NewRecorder()

			h.listEntries(rec, req)

			if rec.Code != tt.code {
				t.Errorf("expected %d, got %d: %s", tt.code, rec.Code, rec.Body.String())
			}
		})
	}
}
