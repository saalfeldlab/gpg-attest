package protocol

// Request is a message sent from the browser extension to the native host.
type Request struct {
	ID      string        `json:"id"`
	Op      string        `json:"op"`
	KeyID   string        `json:"key_id,omitempty"`
	Payload string        `json:"payload,omitempty"`
	Entries        []VerifyEntry `json:"entries,omitempty"`         // for "verify" op
	VerifierKeyIDs []string      `json:"verifier_keyids,omitempty"` // user's own key fingerprints for cert revocation check
}

// VerifyEntry is a single signature to verify in a batch "verify" request.
type VerifyEntry struct {
	Signature   string `json:"signature"`     // armored PGP signature
	Payload     string `json:"payload"`       // base64-encoded data that was signed
	SignerKeyID string `json:"signer_keyid"`  // expected signer fingerprint
	Timestamp   string `json:"timestamp"`     // server_timestamp for revocation check (RFC3339)
}

// Response is a message sent from the native host back to the browser extension.
type Response struct {
	ID            string            `json:"id"`
	OK            bool              `json:"ok"`
	Signature     string            `json:"signature,omitempty"`
	Version       string            `json:"version,omitempty"`
	Error         string            `json:"error,omitempty"`
	Keys          []KeyInfo         `json:"keys,omitempty"`
	VerifyResults []VerifyResultMsg `json:"verify_results,omitempty"` // for "verify" op
	Imported      []string          `json:"imported,omitempty"`       // for "import_key" op
}

// VerifyResultMsg is the verification result for a single entry.
type VerifyResultMsg struct {
	Index       int    `json:"index"`
	Valid       bool   `json:"valid"`       // crypto valid AND (not revoked OR timestamp predates revocation)
	Fingerprint string `json:"fingerprint,omitempty"`
	KeyRevoked  bool   `json:"key_revoked"`
	KeyExpired  bool   `json:"key_expired"`
	KeyMissing  bool   `json:"key_missing"`
	TimestampOK bool   `json:"timestamp_ok"` // true if signature predates key revocation
	Error       string `json:"error,omitempty"`
}

// KeyInfo describes a GnuPG key available for signing.
type KeyInfo struct {
	Fingerprint string `json:"fingerprint"`
	UID         string `json:"uid"`
	CanSign     bool   `json:"can_sign"`
	Trust       string `json:"trust"` // GPG ownertrust: "u", "f", "m", "n", "-", "o", "q"
}
