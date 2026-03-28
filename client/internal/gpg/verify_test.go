package gpg

import (
	"strings"
	"testing"
)

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
	const subjectFpr = "7F5B5CB8B2F24155A9AC345E3EB81DCD4E9F8B7F"
	const revokerFpr = "D781B9DF3744931B5015A72E8E1323F3A105D1B7"

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
	const subjectFpr = "D781B9DF3744931B5015A72E8E1323F3A105D1B7"
	const revokerFpr = "7F5B5CB8B2F24155A9AC345E3EB81DCD4E9F8B7F"

	revoked, _, err := GetCertRevocationDate(subjectFpr, revokerFpr)
	if err != nil {
		t.Fatalf("GetCertRevocationDate failed: %v", err)
	}
	if revoked {
		t.Error("expected cert NOT to be revoked")
	}
}

func TestGetKeyRevocationDate_revokedServerKey(t *testing.T) {
	const serverFpr = "E185BC21E2DF31CE0C0934CA01C10B03BA05D4D6"
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
	const subkeyFpr = "7D14CFACA16C8BD47704BC8555FE9FCB63E2A1B6"
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
