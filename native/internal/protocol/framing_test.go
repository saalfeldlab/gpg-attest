package protocol

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"
	"testing"
)

// encodeRequest manually builds a length-prefixed JSON message for test inputs.
func encodeRequest(t *testing.T, json string) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(json)))
	buf.Write(lenBuf[:])
	buf.WriteString(json)
	return &buf
}

func TestReadRequest_valid(t *testing.T) {
	buf := encodeRequest(t, `{"id":"1","op":"get_version"}`)
	req, err := ReadRequest(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.ID != "1" || req.Op != "get_version" {
		t.Errorf("got %+v, want id=1 op=get_version", req)
	}
}

func TestReadRequest_allFields(t *testing.T) {
	buf := encodeRequest(t, `{"id":"42","op":"sign","key_id":"DEADBEEF","payload":"aGVsbG8="}`)
	req, err := ReadRequest(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.KeyID != "DEADBEEF" || req.Payload != "aGVsbG8=" {
		t.Errorf("got %+v", req)
	}
}

func TestReadRequest_tooLarge(t *testing.T) {
	var buf bytes.Buffer
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], maxMessageSize+1)
	buf.Write(lenBuf[:])
	_, err := ReadRequest(&buf)
	if err == nil {
		t.Fatal("expected error for oversized message, got nil")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("expected 'too large' in error, got: %v", err)
	}
}

func TestReadRequest_truncatedBody(t *testing.T) {
	var buf bytes.Buffer
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], 100) // claims 100 bytes
	buf.Write(lenBuf[:])
	buf.WriteString(`{"id":"1"}`) // only 10 bytes
	_, err := ReadRequest(&buf)
	if err == nil || err == io.EOF {
		// io.ReadFull returns io.ErrUnexpectedEOF for short reads
		t.Fatalf("expected error for truncated body, got: %v", err)
	}
}

func TestReadRequest_invalidJSON(t *testing.T) {
	buf := encodeRequest(t, `not json`)
	_, err := ReadRequest(buf)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestReadRequest_emptyStdin(t *testing.T) {
	_, err := ReadRequest(bytes.NewReader(nil))
	if err != io.EOF {
		t.Errorf("expected io.EOF for empty reader, got: %v", err)
	}
}

func TestWriteResponse_roundTrip(t *testing.T) {
	resp := &Response{
		ID:      "99",
		OK:      true,
		Version: "v1.2.3",
	}
	var buf bytes.Buffer
	if err := WriteResponse(&buf, resp); err != nil {
		t.Fatalf("WriteResponse: %v", err)
	}

	// Verify length prefix matches actual JSON body length.
	raw := buf.Bytes()
	if len(raw) < 4 {
		t.Fatal("output too short")
	}
	bodyLen := binary.LittleEndian.Uint32(raw[:4])
	if int(bodyLen) != len(raw)-4 {
		t.Errorf("length prefix %d does not match body length %d", bodyLen, len(raw)-4)
	}
}

func TestWriteResponse_errorResponse(t *testing.T) {
	resp := &Response{ID: "1", OK: false, Error: "something went wrong"}
	var buf bytes.Buffer
	if err := WriteResponse(&buf, resp); err != nil {
		t.Fatalf("WriteResponse: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected non-empty output")
	}
}

// BenchmarkFramingRoundTrip measures the cost of encoding and decoding one message.
func BenchmarkFramingRoundTrip(b *testing.B) {
	resp := &Response{ID: "1", OK: true, Version: "v0.0.1"}
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		_ = WriteResponse(&buf, resp)
		_, _ = ReadRequest(&buf) // will fail JSON decode but exercises framing path
	}
}
