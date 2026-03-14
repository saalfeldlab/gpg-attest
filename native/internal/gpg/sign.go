package gpg

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Sign produces an ASCII-armored detached PGP signature for payload using keyID.
// keyID may be a fingerprint, key ID, or email address understood by gpg --local-user.
// gpg-agent handles passphrase prompting so that hardware tokens work transparently.
func Sign(keyID string, payload []byte) (string, error) {
	cmd := exec.Command(
		"gpg",
		"--batch",
		"--detach-sign",
		"--armor",
		"--local-user", keyID,
	)
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("gpg sign: %s", errMsg)
	}

	return stdout.String(), nil
}
