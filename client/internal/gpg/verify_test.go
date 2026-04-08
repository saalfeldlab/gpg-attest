package gpg

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

// lookupFingerprint returns the primary key fingerprint for the given email.
func lookupFingerprint(t *testing.T, email string) string {
	t.Helper()
	keys, err := ListKeys()
	if err != nil {
		t.Fatalf("ListKeys() error: %v", err)
	}
	for _, k := range keys {
		if strings.Contains(k.UID, email) {
			return k.Fingerprint
		}
	}
	t.Fatalf("key for %s not found", email)
	return ""
}

// lookupFingerprintRaw returns the primary key fingerprint for the given email
// by parsing gpg output directly, including revoked keys that ListKeys() filters out.
func lookupFingerprintRaw(t *testing.T, email string) string {
	t.Helper()
	cmd := exec.Command("gpg", "--list-keys", "--with-colons", email)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("gpg --list-keys %s: %v: %s", email, err, stderr.String())
	}
	inPub := false
	for _, line := range strings.Split(stdout.String(), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) < 2 {
			continue
		}
		if fields[0] == "pub" {
			inPub = true
			continue
		}
		if fields[0] == "fpr" && inPub && len(fields) > 9 {
			return fields[9]
		}
		if fields[0] == "sub" || fields[0] == "ssb" {
			inPub = false
		}
	}
	t.Fatalf("no fingerprint found for %s", email)
	return ""
}

// lookupSubkeyFingerprint returns the first subkey fingerprint for the given email.
func lookupSubkeyFingerprint(t *testing.T, email string) string {
	t.Helper()
	cmd := exec.Command("gpg", "--list-keys", "--with-colons", email)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("gpg --list-keys %s: %v: %s", email, err, stderr.String())
	}
	inSub := false
	for _, line := range strings.Split(stdout.String(), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) < 2 {
			continue
		}
		if fields[0] == "sub" || fields[0] == "ssb" {
			inSub = true
			continue
		}
		if fields[0] == "fpr" && inSub && len(fields) > 9 {
			return fields[9]
		}
		if fields[0] == "pub" || fields[0] == "sec" {
			inSub = false
		}
	}
	t.Fatalf("no subkey fingerprint found for %s", email)
	return ""
}

func TestVerify_validSignature(t *testing.T) {
	payload := []byte("test payload for verify")
	sig, err := Sign("test@gpg-attest.org", payload)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}

	result, err := Verify(sig, payload)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if !result.Valid {
		t.Error("expected Valid=true for a good signature")
	}
	if result.Fingerprint == "" {
		t.Error("expected non-empty Fingerprint")
	}
	if result.SignedAt == "" {
		t.Error("expected non-empty SignedAt")
	}
	if result.KeyRevoked {
		t.Error("expected KeyRevoked=false")
	}
	if result.KeyExpired {
		t.Error("expected KeyExpired=false")
	}
	if result.KeyMissing {
		t.Error("expected KeyMissing=false")
	}
}

func TestVerify_badSignature(t *testing.T) {
	payload := []byte("original payload")
	sig, err := Sign("test@gpg-attest.org", payload)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}

	// Tamper with the payload
	result, err := Verify(sig, []byte("tampered payload"))
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if result.Valid {
		t.Error("expected Valid=false for tampered payload")
	}
}

func TestVerify_missingKey(t *testing.T) {
	// Create a signature string that looks valid but references a nonexistent key.
	// We'll use a bogus armored signature block.
	bogusSig := `-----BEGIN PGP SIGNATURE-----

iQEzBAABCAAdFiEEAAAAAAAAAAAAAAAAAAAAAAAAAAAABQJnxwS5AAoJEAAAAAAAA
AAAAAAD/0test
=test
-----END PGP SIGNATURE-----`

	result, err := Verify(bogusSig, []byte("test"))
	if err != nil {
		t.Fatalf("verify returned error: %v", err)
	}
	// Should fail verification (bad/missing key)
	if result.Valid {
		t.Error("expected Valid=false for bogus signature")
	}
}

func TestGetKeyRevocationDate_notRevoked(t *testing.T) {
	// The test key should not be revoked
	revoked, revokedAt, err := GetKeyRevocationDate("test@gpg-attest.org")
	if err != nil {
		// Try to get fingerprint first
		t.Skipf("could not check key: %v", err)
	}
	if revoked {
		t.Errorf("expected test key not to be revoked, got revokedAt=%s", revokedAt)
	}
}

func TestParseStatusFd_goodSig(t *testing.T) {
	output := `[GNUPG:] SIG_ID abc123 2026-03-27 1711570255
[GNUPG:] GOODSIG AABBCCDD11223344 Test Signer <test@gpg-attest.org>
[GNUPG:] VALIDSIG AABBCCDD1122334455667788AABBCCDD11223344 2026-03-27 1711570255 0 4 0 1 8 00 AABBCCDD1122334455667788AABBCCDD11223344`

	r := parseStatusFd(output)
	if !r.Valid {
		t.Error("expected Valid=true")
	}
	if r.Fingerprint != "AABBCCDD1122334455667788AABBCCDD11223344" {
		t.Errorf("unexpected fingerprint: %s", r.Fingerprint)
	}
	if r.KeyRevoked || r.KeyExpired || r.KeyMissing {
		t.Error("expected no key issues")
	}
}

func TestParseStatusFd_revokedKey(t *testing.T) {
	output := `[GNUPG:] REVKEYSIG AABBCCDD11223344 Test Signer <test@gpg-attest.org>
[GNUPG:] VALIDSIG AABBCCDD1122334455667788AABBCCDD11223344 2026-03-27 1711570255 0 4 0 1 8 00 AABBCCDD1122334455667788AABBCCDD11223344`

	r := parseStatusFd(output)
	if !r.Valid {
		t.Error("expected Valid=true (cryptographically valid despite revocation)")
	}
	if !r.KeyRevoked {
		t.Error("expected KeyRevoked=true")
	}
}

func TestParseStatusFd_missingKey(t *testing.T) {
	output := `[GNUPG:] ERRSIG AABBCCDD11223344 1 8 00 1711570255 9
[GNUPG:] NO_PUBKEY AABBCCDD11223344`

	r := parseStatusFd(output)
	if r.Valid {
		t.Error("expected Valid=false")
	}
	if !r.KeyMissing {
		t.Error("expected KeyMissing=true")
	}
}

func TestParseStatusFd_badSig(t *testing.T) {
	output := `[GNUPG:] BADSIG AABBCCDD11223344 Test Signer <test@gpg-attest.org>`

	r := parseStatusFd(output)
	if r.Valid {
		t.Error("expected Valid=false for BADSIG")
	}
}

func TestGetCertRevocationDate_revoked(t *testing.T) {
	// revoke-nodate@test.local had its certification by test@gpg-attest.org revoked
	subjectFpr := lookupFingerprint(t, "revoke-nodate@test.local")
	revokerFpr := lookupFingerprint(t, "test@gpg-attest.org")

	revoked, revokedAt, err := GetCertRevocationDate(subjectFpr, revokerFpr)
	if err != nil {
		t.Fatalf("GetCertRevocationDate failed: %v", err)
	}
	if !revoked {
		t.Error("expected cert to be revoked")
	}
	if revokedAt == "" {
		t.Error("expected non-empty revokedAt")
	}
}

func TestGetCertRevocationDate_notRevoked(t *testing.T) {
	// test@gpg-attest.org has NOT had its certification revoked by anyone
	subjectFpr := lookupFingerprint(t, "test@gpg-attest.org")
	revokerFpr := lookupFingerprint(t, "revoke-nodate@test.local")

	revoked, _, err := GetCertRevocationDate(subjectFpr, revokerFpr)
	if err != nil {
		t.Fatalf("GetCertRevocationDate failed: %v", err)
	}
	if revoked {
		t.Error("expected cert NOT to be revoked")
	}
}

func TestGetKeyRevocationDate_revokedServerKey(t *testing.T) {
	serverFpr := lookupFingerprintRaw(t, "revoked-server@gpg-attest.org")
	revoked, revokedAt, err := GetKeyRevocationDate(serverFpr)
	if err != nil {
		t.Fatalf("GetKeyRevocationDate failed: %v", err)
	}
	if !revoked {
		t.Error("expected server key to be revoked")
	}
	if revokedAt == "" {
		t.Error("expected non-empty revokedAt")
	}
	t.Logf("server key revoked at: %s", revokedAt)
}

func TestGetKeyRevocationDate_revokedServerSubkey(t *testing.T) {
	// When gpg --verify reports a subkey fingerprint, we need to find the
	// master key's revocation via the subkey fingerprint
	subkeyFpr := lookupSubkeyFingerprint(t, "revoked-server@gpg-attest.org")
	revoked, revokedAt, err := GetKeyRevocationDate(subkeyFpr)
	if err != nil {
		t.Fatalf("GetKeyRevocationDate (subkey) failed: %v", err)
	}
	if !revoked {
		t.Error("expected subkey lookup to find master key revocation")
	}
	if revokedAt == "" {
		t.Error("expected non-empty revokedAt")
	}
	t.Logf("server key (via subkey) revoked at: %s", revokedAt)
}

func TestGetKeyRevocationDate_fingerprint(t *testing.T) {
	// Get the test key fingerprint first
	keys, err := ListKeys()
	if err != nil {
		t.Skipf("could not list keys: %v", err)
	}
	var fpr string
	for _, k := range keys {
		if strings.Contains(k.UID, "test@gpg-attest.org") {
			fpr = k.Fingerprint
			break
		}
	}
	if fpr == "" {
		t.Skip("test key not found")
	}

	revoked, _, err := GetKeyRevocationDate(fpr)
	if err != nil {
		t.Fatalf("GetKeyRevocationDate failed: %v", err)
	}
	if revoked {
		t.Error("test key should not be revoked")
	}
}
