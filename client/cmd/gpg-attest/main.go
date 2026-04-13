package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"

	"gpg-attest.org/client/internal/gpg"
	"gpg-attest.org/client/internal/protocol"
	"gpg-attest.org/client/internal/version"
)

func main() {
	// All diagnostic output must go to stderr — any stray stdout write corrupts the NMH channel.
	log.SetOutput(os.Stderr)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmsgprefix)
	log.SetPrefix("gpg-attest: ")

	in := bufio.NewReader(os.Stdin)
	out := os.Stdout

	log.Printf("started (version %s)", version.Version)

	for {
		req, err := protocol.ReadRequest(in)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				log.Printf("stdin closed, exiting")
				return
			}
			log.Printf("read error: %v", err)
			return
		}

		resp := handle(req)
		if err := protocol.WriteResponse(out, resp); err != nil {
			log.Printf("write error: %v", err)
			return
		}
	}
}

func handle(req *protocol.Request) *protocol.Response {
	switch req.Op {
	case "get_version":
		return &protocol.Response{
			ID:      req.ID,
			OK:      true,
			Version: version.Version,
		}

	case "list_keys":
		keys, err := gpg.ListKeys()
		if err != nil {
			log.Printf("list_keys error: %v", err)
			return errResp(req.ID, "list_keys failed")
		}
		return &protocol.Response{
			ID:   req.ID,
			OK:   true,
			Keys: keys,
		}

	case "list_secret_keys":
		keys, err := gpg.ListSecretKeys()
		if err != nil {
			log.Printf("list_secret_keys error: %v", err)
			return errResp(req.ID, "list_secret_keys failed")
		}
		return &protocol.Response{
			ID:   req.ID,
			OK:   true,
			Keys: keys,
		}

	case "sign":
		if req.KeyID == "" {
			return errResp(req.ID, "sign: key_id is required")
		}
		if req.Payload == "" {
			return errResp(req.ID, "sign: payload is required")
		}
		payload, err := base64.StdEncoding.DecodeString(req.Payload)
		if err != nil {
			return errResp(req.ID, fmt.Sprintf("sign: invalid base64 payload: %v", err))
		}
		sig, err := gpg.Sign(req.KeyID, payload)
		if err != nil {
			log.Printf("sign error: %v", err)
			return errResp(req.ID, "sign failed")
		}
		return &protocol.Response{
			ID:        req.ID,
			OK:        true,
			Signature: sig,
		}

	case "verify":
		if len(req.Entries) == 0 {
			return errResp(req.ID, "verify: entries is required")
		}
		results := make([]protocol.VerifyResultMsg, len(req.Entries))
		for i, entry := range req.Entries {
			results[i].Index = i
			payload, err := base64.StdEncoding.DecodeString(entry.Payload)
			if err != nil {
				results[i].Error = fmt.Sprintf("invalid base64 payload: %v", err)
				continue
			}
			vr, err := gpg.Verify(entry.Signature, payload)
			if err != nil {
				results[i].Error = "verify failed"
				continue
			}
			results[i].Fingerprint = vr.Fingerprint
			results[i].KeyRevoked = vr.KeyRevoked
			results[i].KeyExpired = vr.KeyExpired
			results[i].KeyMissing = vr.KeyMissing

			selfRevoked := vr.KeyRevoked
			selfTimestampOK := false
			if selfRevoked && entry.Timestamp != "" {
				revoked, revokedAt, _ := gpg.GetKeyRevocationDate(vr.Fingerprint)
				if revoked && revokedAt != "" {
					selfTimestampOK = entry.Timestamp < revokedAt
				}
			}

			// Check certification revocation: has the verifying user revoked
			// their certification on this signer's key?
			certRevoked := false
			certTimestampOK := false
			if vr.Valid && vr.Fingerprint != "" && entry.Timestamp != "" {
				for _, verifierFpr := range req.VerifierKeyIDs {
					revoked, revokedAt, _ := gpg.GetCertRevocationDate(vr.Fingerprint, verifierFpr)
					if revoked && revokedAt != "" {
						certRevoked = true
						certTimestampOK = entry.Timestamp < revokedAt
						break
					}
				}
			}

			results[i].TimestampOK = (!selfRevoked || selfTimestampOK) && (!certRevoked || certTimestampOK)
			results[i].Valid = vr.Valid && (!selfRevoked || selfTimestampOK) && (!certRevoked || certTimestampOK)
		}
		return &protocol.Response{
			ID:            req.ID,
			OK:            true,
			VerifyResults: results,
		}

	case "import_key":
		if req.Payload == "" {
			return errResp(req.ID, "import_key: payload is required")
		}
		keyData, err := base64.StdEncoding.DecodeString(req.Payload)
		if err != nil {
			return errResp(req.ID, fmt.Sprintf("import_key: invalid base64 payload: %v", err))
		}
		fingerprints, err := gpg.ImportKey(keyData)
		if err != nil {
			log.Printf("import_key error: %v", err)
			return errResp(req.ID, "import_key failed")
		}
		return &protocol.Response{
			ID:       req.ID,
			OK:       true,
			Imported: fingerprints,
		}

	default:
		return errResp(req.ID, fmt.Sprintf("unknown op: %q", req.Op))
	}
}

func errResp(id, msg string) *protocol.Response {
	return &protocol.Response{ID: id, OK: false, Error: msg}
}
