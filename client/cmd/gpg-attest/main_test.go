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