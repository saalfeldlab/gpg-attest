package main

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gpg-attest.org/client/internal/protocol"
)

// Tests for handle() cover all validation paths that do NOT require gpg.

func TestHandle_getVersion(t *testing.T) {
	req := &protocol.Request{ID: "1", Op: "get_version"}
	resp := handle(req)
	if !resp.OK {
		t.Errorf("expected OK=true, got error: %s", resp.Error)
	}
	if resp.Version == "" {
		t.Error("expected non-empty Version")
	}
	if resp.ID != "1" {
		t.Errorf("expected ID=1, got %q", resp.ID)
	}
}

func TestHandle_signMissingKeyID(t *testing.T) {
	req := &protocol.Request{ID: "2", Op: "sign", Payload: "aGVsbG8="}
	resp := handle(req)
	if resp.OK {
		t.Error("expected OK=false when key_id is missing")
	}
	if !strings.Contains(resp.Error, "key_id") {
		t.Errorf("expected 'key_id' in error, got: %q", resp.Error)
	}
}

func TestHandle_signMissingPayload(t *testing.T) {
	req := &protocol.Request{ID: "3", Op: "sign", KeyID: "DEADBEEF"}
	resp := handle(req)
	if resp.OK {
		t.Error("expected OK=false when payload is missing")
	}
	if !strings.Contains(resp.Error, "payload") {
		t.Errorf("expected 'payload' in error, got: %q", resp.Error)
	}
}

func TestHandle_signInvalidBase64(t *testing.T) {
	req := &protocol.Request{ID: "4", Op: "sign", KeyID: "DEADBEEF", Payload: "!!!not-base64!!!"}
	resp := handle(req)
	if resp.OK {
		t.Error("expected OK=false for invalid base64 payload")
	}
	if !strings.Contains(resp.Error, "base64") {
		t.Errorf("expected 'base64' in error, got: %q", resp.Error)
	}
}

func TestHandle_unknownOp(t *testing.T) {
	req := &protocol.Request{ID: "5", Op: "frobnicate"}
	resp := handle(req)
	if resp.OK {
		t.Error("expected OK=false for unknown op")
	}
	if !strings.Contains(resp.Error, "frobnicate") {
		t.Errorf("expected op name in error, got: %q", resp.Error)
	}
	if resp.ID != "5" {
		t.Errorf("expected ID echoed back, got %q", resp.ID)
	}
}

func TestHandle_idEchoedOnError(t *testing.T) {
	req := &protocol.Request{ID: "request-xyz", Op: "unknown_op"}
	resp := handle(req)
	if resp.ID != "request-xyz" {
		t.Errorf("expected ID=%q in response, got %q", "request-xyz", resp.ID)
	}
}

func TestHandle_listKeys(t *testing.T) {
	req := &protocol.Request{ID: "10", Op: "list_keys"}
	resp := handle(req)
	if !resp.OK {
		t.Fatalf("expected OK=true, got error: %s", resp.Error)
	}
	if len(resp.Keys) == 0 {
		t.Error("expected at least one key in response")
	}
}

func TestHandle_listKeys_testKeyPresent(t *testing.T) {
	req := &protocol.Request{ID: "11", Op: "list_keys"}
	resp := handle(req)
	if !resp.OK {
		t.Fatalf("expected OK=true, got error: %s", resp.Error)
	}

	const wantUID = "Test Signer <test@gpg-attest.org>"
	for _, k := range resp.Keys {
		if k.UID == wantUID {
			return
		}
	}
	t.Errorf("test key %q not found in list_keys response (%d keys)", wantUID, len(resp.Keys))
}

func TestHandle_sign(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString([]byte("hello from gpg-attest test"))
	req := &protocol.Request{
		ID:      "12",
		Op:      "sign",
		KeyID:   "test@gpg-attest.org",
		Payload: payload,
	}
	resp := handle(req)
	if !resp.OK {
		t.Fatalf("expected OK=true, got error: %s", resp.Error)
	}
	if !strings.HasPrefix(resp.Signature, "-----BEGIN PGP SIGNATURE-----") {
		t.Errorf("expected PGP signature, got: %q", resp.Signature)
	}
}

func TestHandle_sign_verifiable(t *testing.T) {
	rawPayload := []byte("hello from gpg-attest test")
	payload := base64.StdEncoding.EncodeToString(rawPayload)
	req := &protocol.Request{
		ID: "12", Op: "sign", KeyID: "test@gpg-attest.org", Payload: payload,
	}
	resp := handle(req)
	if !resp.OK {
		t.Fatalf("expected OK=true, got error: %s", resp.Error)
	}
	if !strings.HasPrefix(resp.Signature, "-----BEGIN PGP SIGNATURE-----") {
		t.Fatalf("expected PGP signature, got: %q", resp.Signature)
	}

	// Write payload and signature to temp files, then ask gpg to verify.
	dir := t.TempDir()
	payloadFile := filepath.Join(dir, "payload")
	sigFile := filepath.Join(dir, "payload.asc")
	if err := os.WriteFile(payloadFile, rawPayload, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sigFile, []byte(resp.Signature), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("gpg", "--verify", sigFile, payloadFile).CombinedOutput()
	if err != nil {
		t.Fatalf("gpg --verify failed: %v\noutput: %s", err, out)
	}
}

func TestHandle_verify(t *testing.T) {
	// Sign a payload first
	rawPayload := []byte("verify test payload")
	payload64 := base64.StdEncoding.EncodeToString(rawPayload)
	signReq := &protocol.Request{
		ID: "20", Op: "sign", KeyID: "test@gpg-attest.org", Payload: payload64,
	}
	signResp := handle(signReq)
	if !signResp.OK {
		t.Fatalf("sign failed: %s", signResp.Error)
	}

	// Now verify it
	verifyReq := &protocol.Request{
		ID: "21",
		Op: "verify",
		Entries: []protocol.VerifyEntry{
			{
				Signature:   signResp.Signature,
				Payload:     payload64,
				SignerKeyID: "test@gpg-attest.org",
				Timestamp:   "2026-03-27T22:00:00Z",
			},
		},
	}
	verifyResp := handle(verifyReq)
	if !verifyResp.OK {
		t.Fatalf("verify op failed: %s", verifyResp.Error)
	}
	if len(verifyResp.VerifyResults) != 1 {
		t.Fatalf("expected 1 result, got %d", len(verifyResp.VerifyResults))
	}
	r := verifyResp.VerifyResults[0]
	if !r.Valid {
		t.Errorf("expected Valid=true, got error: %s", r.Error)
	}
	if r.Fingerprint == "" {
		t.Error("expected non-empty Fingerprint")
	}
	if r.KeyRevoked {
		t.Error("expected KeyRevoked=false")
	}
}

func TestHandle_verify_emptyEntries(t *testing.T) {
	req := &protocol.Request{ID: "22", Op: "verify"}
	resp := handle(req)
	if resp.OK {
		t.Error("expected OK=false when entries is empty")
	}
}

func TestHandle_verify_badPayload(t *testing.T) {
	req := &protocol.Request{
		ID: "23",
		Op: "verify",
		Entries: []protocol.VerifyEntry{
			{
				Signature:   "-----BEGIN PGP SIGNATURE-----\ntest\n-----END PGP SIGNATURE-----",
				Payload:     "!!!invalid-base64!!!",
				SignerKeyID: "test@gpg-attest.org",
			},
		},
	}
	resp := handle(req)
	if !resp.OK {
		t.Fatal("verify op should return OK=true with per-entry errors")
	}
	if len(resp.VerifyResults) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.VerifyResults))
	}
	if resp.VerifyResults[0].Error == "" {
		t.Error("expected error for invalid base64")
	}
}

func TestHandle_verify_certRevoked_timestampBefore(t *testing.T) {
	// Sign a payload with revoke-nodate@test.local (whose cert from test@ was revoked)
	rawPayload := []byte("cert revocation test")
	payload64 := base64.StdEncoding.EncodeToString(rawPayload)
	signReq := &protocol.Request{
		ID: "40", Op: "sign", KeyID: "revoke-nodate@test.local", Payload: payload64,
	}
	signResp := handle(signReq)
	if !signResp.OK {
		t.Fatalf("sign failed: %s", signResp.Error)
	}

	// Verify with a timestamp BEFORE the cert revocation — should be valid
	verifyReq := &protocol.Request{
		ID: "41",
		Op: "verify",
		Entries: []protocol.VerifyEntry{
			{
				Signature:   signResp.Signature,
				Payload:     payload64,
				SignerKeyID: "revoke-nodate@test.local",
				Timestamp:   "2020-01-01T00:00:00Z", // well before revocation
			},
		},
		VerifierKeyIDs: []string{"D781B9DF3744931B5015A72E8E1323F3A105D1B7"}, // test@gpg-attest.org
	}
	verifyResp := handle(verifyReq)
	if !verifyResp.OK {
		t.Fatalf("verify failed: %s", verifyResp.Error)
	}
	r := verifyResp.VerifyResults[0]
	if !r.Valid {
		t.Errorf("expected Valid=true (timestamp predates cert revocation), error: %s", r.Error)
	}
	if !r.TimestampOK {
		t.Error("expected TimestampOK=true")
	}
}

func TestHandle_verify_certRevoked_timestampAfter(t *testing.T) {
	rawPayload := []byte("cert revocation test after")
	payload64 := base64.StdEncoding.EncodeToString(rawPayload)
	signReq := &protocol.Request{
		ID: "42", Op: "sign", KeyID: "revoke-nodate@test.local", Payload: payload64,
	}
	signResp := handle(signReq)
	if !signResp.OK {
		t.Fatalf("sign failed: %s", signResp.Error)
	}

	// Verify with a timestamp AFTER the cert revocation — should be invalid
	verifyReq := &protocol.Request{
		ID: "43",
		Op: "verify",
		Entries: []protocol.VerifyEntry{
			{
				Signature:   signResp.Signature,
				Payload:     payload64,
				SignerKeyID: "revoke-nodate@test.local",
				Timestamp:   "2099-01-01T00:00:00Z", // well after revocation
			},
		},
		VerifierKeyIDs: []string{"D781B9DF3744931B5015A72E8E1323F3A105D1B7"},
	}
	verifyResp := handle(verifyReq)
	if !verifyResp.OK {
		t.Fatalf("verify failed: %s", verifyResp.Error)
	}
	r := verifyResp.VerifyResults[0]
	if r.Valid {
		t.Error("expected Valid=false (timestamp is after cert revocation)")
	}
}

func TestHandle_importKey(t *testing.T) {
	// Export the test key and re-import via handler
	exportOut, err := exec.Command("gpg", "--export", "--armor", "test@gpg-attest.org").Output()
	if err != nil {
		t.Fatalf("gpg export failed: %v", err)
	}

	req := &protocol.Request{
		ID:      "30",
		Op:      "import_key",
		Payload: base64.StdEncoding.EncodeToString(exportOut),
	}
	resp := handle(req)
	if !resp.OK {
		t.Fatalf("import_key failed: %s", resp.Error)
	}
	if len(resp.Imported) == 0 {
		t.Error("expected at least one imported fingerprint")
	}
}

func TestHandle_importKey_missingPayload(t *testing.T) {
	req := &protocol.Request{ID: "31", Op: "import_key"}
	resp := handle(req)
	if resp.OK {
		t.Error("expected OK=false when payload is missing")
	}
}