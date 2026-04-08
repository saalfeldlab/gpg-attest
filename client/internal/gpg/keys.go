package gpg

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"gpg-attest.org/client/internal/protocol"
)

// ListKeys returns all non-revoked/expired public keys from the user's GnuPG keystore.
// It runs `gpg --list-keys --with-colons` and parses the colon-delimited output.
func ListKeys() ([]protocol.KeyInfo, error) {
	cmd := exec.Command("gpg", "--list-keys", "--with-colons")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gpg --list-keys: %w: %s", err, stderr.String())
	}

	return parseColons(stdout.Bytes()), nil
}

// ListSecretKeys returns all non-revoked/expired secret keys from the user's GnuPG keystore.
// It runs `gpg --list-secret-keys --with-colons` and parses the colon-delimited output.
func ListSecretKeys() ([]protocol.KeyInfo, error) {
	cmd := exec.Command("gpg", "--list-secret-keys", "--with-colons")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gpg --list-secret-keys: %w: %s", err, stderr.String())
	}

	return parseColons(stdout.Bytes()), nil
}

// parseColons parses gpg --with-colons output into KeyInfo entries.
// It only considers pub records; sub records are skipped.
func parseColons(data []byte) []protocol.KeyInfo {
	type pending struct {
		validity   string
		canSign    bool
		fpr        string
		uid        string
		ownertrust string
	}

	var results []protocol.KeyInfo
	var cur *pending

	flush := func() {
		if cur == nil {
			return
		}
		// Skip revoked (r), expired (e), or invalid (i) keys
		switch cur.validity {
		case "r", "e", "i":
			cur = nil
			return
		}
		if cur.fpr == "" {
			cur = nil
			return
		}
		results = append(results, protocol.KeyInfo{
			Fingerprint: cur.fpr,
			UID:         cur.uid,
			CanSign:     cur.canSign,
			Trust:       cur.ownertrust,
		})
		cur = nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Split(line, ":")
		if len(fields) < 2 {
			continue
		}
		recType := fields[0]

		switch recType {
		case "pub", "sec":
			flush()
			cur = &pending{}
			if len(fields) > 1 {
				cur.validity = fields[1]
			}
			// field[8] (index 8) holds ownertrust: u=ultimate, f=full, m=marginal, n=never, -/o/q=undefined
			if len(fields) > 8 {
				cur.ownertrust = fields[8]
			}
			// field[11] (index 11) holds key capabilities: s/S means can sign.
			// GPG --with-colons format (0-indexed): type:validity:bits:algo:keyid:created:expires:hash:ownertrust:uid:sigclass:caps
			if len(fields) > 11 {
				caps := strings.ToLower(fields[11])
				cur.canSign = strings.ContainsAny(caps, "sS")
			}

		case "fpr":
			if cur != nil && len(fields) > 9 && cur.fpr == "" {
				cur.fpr = fields[9]
			}

		case "uid":
			if cur != nil && len(fields) > 9 && cur.uid == "" {
				// Skip revoked/expired UIDs
				if validity := fields[1]; validity == "r" || validity == "e" || validity == "i" {
					continue
				}
				cur.uid = fields[9]
			}

		case "sub", "ssb":
			// Skip subkey records — only primary keys are reported
		}
	}
	flush()

	return results
}
