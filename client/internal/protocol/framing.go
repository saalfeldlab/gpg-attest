package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

const maxMessageSize = 1 << 20 // 1 MB — Chrome's enforced limit

// ReadRequest reads one length-prefixed JSON message from r and decodes it into a Request.
func ReadRequest(r io.Reader) (*Request, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(lenBuf[:])
	if size > maxMessageSize {
		return nil, fmt.Errorf("message too large: %d bytes (max %d)", size, maxMessageSize)
	}
	msgBuf := make([]byte, size)
	if _, err := io.ReadFull(r, msgBuf); err != nil {
		return nil, err
	}
	var req Request
	if err := json.Unmarshal(msgBuf, &req); err != nil {
		return nil, fmt.Errorf("invalid JSON request: %w", err)
	}
	return &req, nil
}

// WriteResponse encodes resp as JSON and writes it with a 4-byte LE length prefix to w.
func WriteResponse(w io.Writer, resp *Response) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(data)))
	if _, err := w.Write(lenBuf[:]); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
