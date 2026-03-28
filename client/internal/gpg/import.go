package gpg

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// ImportKey imports an ASCII-armored PGP public key into the local keyring.
// Returns the fingerprints of imported keys. Idempotent — re-importing is a no-op.
func ImportKey(armoredKey []byte) ([]string, error) {
	cmd := exec.Command("gpg", "--batch", "--import", "--status-fd", "2")
	cmd.Stdin = bytes.NewReader(armoredKey)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return nil, fmt.Errorf("gpg import: %s", errMsg)
	}

	return parseImportStatus(stderr.String()), nil
}

// parseImportStatus extracts fingerprints from gpg --import --status-fd output.
// Looks for IMPORT_OK lines: [GNUPG:] IMPORT_OK <reason> <fingerprint>
func parseImportStatus(output string) []string {
	var fingerprints []string
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "[GNUPG:] IMPORT_OK ") {
			continue
		}
		parts := strings.Fields(line)
		// [GNUPG:] IMPORT_OK <reason> <fingerprint>
		if len(parts) >= 4 {
			fpr := parts[3]
			if !seen[fpr] {
				seen[fpr] = true
				fingerprints = append(fingerprints, fpr)
			}
		}
	}
	return fingerprints
}
