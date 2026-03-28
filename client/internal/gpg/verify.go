package gpg

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// VerifyResult holds the outcome of a GPG signature verification.
type VerifyResult struct {
	Valid       bool   // cryptographic signature is correct (GOODSIG or REVKEYSIG)
	Fingerprint string // full fingerprint of the signing key (from VALIDSIG)
	SignedAt    string // RFC3339 signature creation time (from VALIDSIG)
	KeyRevoked  bool   // signing key is revoked (REVKEYSIG)
	KeyExpired  bool   // signing key is expired (EXPKEYSIG)
	KeyMissing  bool   // public key not in keyring (ERRSIG + NO_PUBKEY)
}

// Verify checks an ASCII-armored detached PGP signature against payload.
// It parses gpg's --status-fd output for machine-readable results.
func Verify(signature string, payload []byte) (*VerifyResult, error) {
	sigFile, err := os.CreateTemp("", "gpg-verify-sig-*.asc")
	if err != nil {
		return nil, fmt.Errorf("create sig temp file: %w", err)
	}
	defer os.Remove(sigFile.Name())
	if _, err := sigFile.WriteString(signature); err != nil {
		sigFile.Close()
		return nil, fmt.Errorf("write sig: %w", err)
	}
	sigFile.Close()

	payloadFile, err := os.CreateTemp("", "gpg-verify-payload-*")
	if err != nil {
		return nil, fmt.Errorf("create payload temp file: %w", err)
	}
	defer os.Remove(payloadFile.Name())
	if _, err := payloadFile.Write(payload); err != nil {
		payloadFile.Close()
		return nil, fmt.Errorf("write payload: %w", err)
	}
	payloadFile.Close()

	cmd := exec.Command("gpg", "--batch", "--verify", "--status-fd", "2", sigFile.Name(), payloadFile.Name())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	cmd.Run() // exit code is non-zero for bad/revoked sigs; we parse status-fd instead

	return parseStatusFd(stderr.String()), nil
}

// parseStatusFd extracts verification results from gpg --status-fd output.
func parseStatusFd(output string) *VerifyResult {
	r := &VerifyResult{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "[GNUPG:] ") {
			continue
		}
		parts := strings.Fields(line[9:]) // strip "[GNUPG:] " prefix
		if len(parts) == 0 {
			continue
		}
		switch parts[0] {
		case "GOODSIG":
			r.Valid = true
		case "BADSIG":
			r.Valid = false
		case "REVKEYSIG":
			r.Valid = true // cryptographically valid, but key is revoked
			r.KeyRevoked = true
		case "EXPKEYSIG":
			r.Valid = true // cryptographically valid, but key is expired
			r.KeyExpired = true
		case "ERRSIG":
			r.Valid = false
			// field 6 (0-indexed from parts[1]) is reason code; 9 = missing key
			if len(parts) >= 7 && parts[6] == "9" {
				r.KeyMissing = true
			}
		case "NO_PUBKEY":
			r.KeyMissing = true
		case "VALIDSIG":
			// VALIDSIG <fpr> <created_date> <sig_timestamp> ...
			// parts[1] = fingerprint, parts[2] = creation date (ISO or epoch), parts[3] = sig timestamp (epoch)
			if len(parts) >= 4 {
				r.Fingerprint = parts[1]
				if ts, err := strconv.ParseInt(parts[3], 10, 64); err == nil {
					r.SignedAt = time.Unix(ts, 0).UTC().Format(time.RFC3339)
				}
			}
		}
	}
	return r
}

// GetKeyRevocationDate checks whether a key has been self-revoked and returns the revocation date.
// It parses `gpg --list-sigs --with-colons` for `rev` records with sig class 20x (key revocation).
// The fingerprint may be a master key or subkey — GPG returns the master key's revocation
// records for both.
func GetKeyRevocationDate(fingerprint string) (revoked bool, revokedAt string, err error) {
	cmd := exec.Command("gpg", "--list-sigs", "--with-colons", fingerprint)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return false, "", fmt.Errorf("gpg --list-sigs: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ":")
		if len(fields) < 11 || fields[0] != "rev" {
			continue
		}
		// Self-revocation: sig class 20x (key revocation, not 30x cert revocation)
		if !strings.HasPrefix(fields[10], "20") {
			continue
		}
		if ts, parseErr := strconv.ParseInt(fields[5], 10, 64); parseErr == nil {
			return true, time.Unix(ts, 0).UTC().Format(time.RFC3339), nil
		}
	}

	return false, "", nil
}

// GetCertRevocationDate checks whether a specific user has revoked their
// certification on a subject key. It parses `gpg --list-sigs --with-colons`
// for `rev` records where field[12] (revoker fingerprint) matches revokerFingerprint.
// Returns the earliest such revocation timestamp.
func GetCertRevocationDate(subjectFingerprint string, revokerFingerprint string) (revoked bool, revokedAt string, err error) {
	cmd := exec.Command("gpg", "--list-sigs", "--with-colons", subjectFingerprint)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return false, "", fmt.Errorf("gpg --list-sigs: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	var earliest int64
	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ":")
		if len(fields) < 13 || fields[0] != "rev" {
			continue
		}
		// field[12] is the full fingerprint of the revoker
		if fields[12] != revokerFingerprint {
			continue
		}
		ts, parseErr := strconv.ParseInt(fields[5], 10, 64)
		if parseErr != nil {
			continue
		}
		if earliest == 0 || ts < earliest {
			earliest = ts
		}
	}

	if earliest > 0 {
		return true, time.Unix(earliest, 0).UTC().Format(time.RFC3339), nil
	}
	return false, "", nil
}
