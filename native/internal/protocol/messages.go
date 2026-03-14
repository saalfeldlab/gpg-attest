package protocol

// Request is a message sent from the browser extension to the native host.
type Request struct {
	ID      string `json:"id"`
	Op      string `json:"op"`
	KeyID   string `json:"key_id,omitempty"`
	Payload string `json:"payload,omitempty"`
}

// Response is a message sent from the native host back to the browser extension.
type Response struct {
	ID        string    `json:"id"`
	OK        bool      `json:"ok"`
	Signature string    `json:"signature,omitempty"`
	Version   string    `json:"version,omitempty"`
	Error     string    `json:"error,omitempty"`
	Keys      []KeyInfo `json:"keys,omitempty"`
}

// KeyInfo describes a GnuPG key available for signing.
type KeyInfo struct {
	Fingerprint string `json:"fingerprint"`
	UID         string `json:"uid"`
	CanSign     bool   `json:"can_sign"`
}
