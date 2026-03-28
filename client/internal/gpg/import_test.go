package gpg

import (
	"bytes"
	"os/exec"
	"testing"
)

func TestImportKey_success(t *testing.T) {
	// Export the test key, then re-import it (idempotent)
	cmd := exec.Command("gpg", "--export", "--armor", "test@gpg-attest.org")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		t.Fatalf("gpg export failed: %v", err)
	}

	fingerprints, err := ImportKey(stdout.Bytes())
	if err != nil {
		t.Fatalf("ImportKey failed: %v", err)
	}
	if len(fingerprints) == 0 {
		t.Error("expected at least one fingerprint returned")
	}
}

func TestImportKey_idempotent(t *testing.T) {
	cmd := exec.Command("gpg", "--export", "--armor", "test@gpg-attest.org")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		t.Fatalf("gpg export failed: %v", err)
	}
	key := stdout.Bytes()

	// Import twice — both should succeed
	_, err1 := ImportKey(key)
	_, err2 := ImportKey(key)
	if err1 != nil {
		t.Fatalf("first import failed: %v", err1)
	}
	if err2 != nil {
		t.Fatalf("second import failed: %v", err2)
	}
}
