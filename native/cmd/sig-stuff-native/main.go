package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"

	"sig-stuff.dev/native/internal/gpg"
	"sig-stuff.dev/native/internal/protocol"
	"sig-stuff.dev/native/internal/version"
)

func main() {
	// All diagnostic output must go to stderr — any stray stdout write corrupts the NMH channel.
	log.SetOutput(os.Stderr)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmsgprefix)
	log.SetPrefix("sig-stuff-native: ")

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
			return errResp(req.ID, fmt.Sprintf("list_keys: %v", err))
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
			return errResp(req.ID, fmt.Sprintf("sign: %v", err))
		}
		return &protocol.Response{
			ID:        req.ID,
			OK:        true,
			Signature: sig,
		}

	default:
		return errResp(req.ID, fmt.Sprintf("unknown op: %q", req.Op))
	}
}

func errResp(id, msg string) *protocol.Response {
	return &protocol.Response{ID: id, OK: false, Error: msg}
}
