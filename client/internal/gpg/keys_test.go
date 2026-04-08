package gpg

import (
	"os/exec"
	"strings"
	"testing"
)

func TestListKeys_returnsKeys(t *testing.T) {
	keys, err := ListKeys()
	if err != nil {
		t.Fatalf("ListKeys() error: %v", err)
	}
	if len(keys) == 0 {
		t.Fatal("expected at least one key, got none")
	}
}

func TestListKeys_includesPublicKeys(t *testing.T) {
	keys, err := ListKeys()
	if err != nil {
		t.Fatalf("ListKeys() error: %v", err)
	}

	const wantUID = "Test Signer <test@gpg-attest.org>"
	for _, k := range keys {
		if k.UID == wantUID {
			if k.Fingerprint == "" {
				t.Error("test key has empty fingerprint")
			}
			return
		}
	}
	t.Errorf("test key %q not found in %d keys returned by ListKeys()", wantUID, len(keys))
}

func TestListSecretKeys_returnsKeys(t *testing.T) {
	keys, err := ListSecretKeys()
	if err != nil {
		t.Fatalf("ListSecretKeys() error: %v", err)
	}
	if len(keys) == 0 {
		t.Fatal("expected at least one key, got none")
	}
}

func TestListSecretKeys_testKeyPresent(t *testing.T) {
	keys, err := ListSecretKeys()
	if err != nil {
		t.Fatalf("ListSecretKeys() error: %v", err)
	}

	const wantUID = "Test Signer <test@gpg-attest.org>"
	for _, k := range keys {
		if k.UID == wantUID {
			if k.Fingerprint == "" {
				t.Error("test key has empty fingerprint")
			}
			if !k.CanSign {
				t.Error("expected CanSign=true for test key")
			}
			return
		}
	}
	t.Errorf("test key %q not found in %d keys returned by ListSecretKeys()", wantUID, len(keys))
}

func TestListSecretKeys_fingerprintMatchesCLI(t *testing.T) {
	// Get the fingerprint our Go code returns for the test key.
	keys, err := ListSecretKeys()
	if err != nil {
		t.Fatalf("ListSecretKeys() error: %v", err)
	}
	var goFingerprint string
	for _, k := range keys {
		if k.UID == "Test Signer <test@gpg-attest.org>" {
			goFingerprint = k.Fingerprint
			break
		}
	}
	if goFingerprint == "" {
		t.Fatal("test key not found — run .devcontainer/setup-test-keys.sh to provision it")
	}

	// Get the fingerprint directly from the CLI (independent oracle).
	out, err := exec.Command("gpg", "--list-secret-keys", "--with-colons",
		"test@gpg-attest.org").Output()
	if err != nil {
		t.Fatalf("gpg --list-secret-keys: %v", err)
	}
	// The 'fpr' record contains the fingerprint in field 10 (0-indexed: 9).
	var cliFingerprint string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "fpr:") {
			fields := strings.Split(line, ":")
			if len(fields) > 9 {
				cliFingerprint = fields[9]
				break
			}
		}
	}
	if cliFingerprint == "" {
		t.Fatal("could not parse fingerprint from gpg --list-secret-keys --with-colons output")
	}
	if goFingerprint != cliFingerprint {
		t.Errorf("fingerprint mismatch: Go=%q CLI=%q", goFingerprint, cliFingerprint)
	}
}

func TestListKeys_supersetOfSecretKeys(t *testing.T) {
	pubKeys, err := ListKeys()
	if err != nil {
		t.Fatalf("ListKeys() error: %v", err)
	}
	secKeys, err := ListSecretKeys()
	if err != nil {
		t.Fatalf("ListSecretKeys() error: %v", err)
	}

	pubSet := make(map[string]bool)
	for _, k := range pubKeys {
		pubSet[k.Fingerprint] = true
	}
	for _, k := range secKeys {
		if !pubSet[k.Fingerprint] {
			t.Errorf("secret key %s (%s) not found in ListKeys() output", k.Fingerprint, k.UID)
		}
	}
}

func BenchmarkListKeys(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := ListKeys(); err != nil {
			b.Fatalf("ListKeys() error: %v", err)
		}
	}
}

func BenchmarkListSecretKeys(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := ListSecretKeys(); err != nil {
			b.Fatalf("ListSecretKeys() error: %v", err)
		}
	}
}
