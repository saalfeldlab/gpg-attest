package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/google/trillian"
	"github.com/google/trillian/types"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
)

// Entry is a signed verdict stored in the transparency log.
type Entry struct {
	UUID            string `json:"uuid"`
	LogIndex        int64  `json:"log_index"`
	ArtifactHash    string `json:"artifact_hash"`
	Verdict         string `json:"verdict"`
	SignerKeyID     string `json:"signer_keyid"`
	Signature       string `json:"signature"`
	ServerTimestamp string `json:"server_timestamp"`
	ServerSignature string `json:"server_signature"`
}

// sigPayload is marshaled to produce the canonical bytes the server signs.
// Fields are declared in alphabetical JSON-key order so encoding/json produces
// sorted output (it marshals struct fields in declaration order).
type sigPayload struct {
	ArtifactHash    string `json:"artifact_hash"`
	LogIndex        int64  `json:"log_index"`
	ServerTimestamp string `json:"server_timestamp"`
	Signature       string `json:"signature"`
	SignerKeyID     string `json:"signer_keyid"`
	UUID            string `json:"uuid"`
	Verdict         string `json:"verdict"`
}

// Store manages entries in Redis (operational) and Trillian (audit trail).
type Store struct {
	rdb      *redis.Client
	tlog     trillian.TrillianLogClient
	treeID   int64
	gpgKeyID string
}

// New creates a Store.
func New(rdb *redis.Client, conn *grpc.ClientConn, treeID int64, gpgKeyID string) *Store {
	return &Store{
		rdb:      rdb,
		tlog:     trillian.NewTrillianLogClient(conn),
		treeID:   treeID,
		gpgKeyID: gpgKeyID,
	}
}

// Append atomically assigns log_index and server_signature to e, then persists it.
func (s *Store) Append(ctx context.Context, e *Entry) error {
	// Allocate a sequential log index (INCR is atomic; first call returns 1 → index 0).
	idx, err := s.rdb.Incr(ctx, "counter").Result()
	if err != nil {
		return fmt.Errorf("redis INCR: %w", err)
	}
	e.LogIndex = idx - 1

	// Compute server signature over canonical payload (server_signature absent).
	p := sigPayload{
		ArtifactHash:    e.ArtifactHash,
		LogIndex:        e.LogIndex,
		ServerTimestamp: e.ServerTimestamp,
		Signature:       e.Signature,
		SignerKeyID:     e.SignerKeyID,
		UUID:            e.UUID,
		Verdict:         e.Verdict,
	}
	payload, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal signing payload: %w", err)
	}
	sig, err := gpgSign(s.gpgKeyID, payload)
	if err != nil {
		return fmt.Errorf("gpg sign: %w", err)
	}
	e.ServerSignature = sig

	// Persist full entry to Redis.
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}
	idxStr := strconv.FormatInt(e.LogIndex, 10)
	pipe := s.rdb.Pipeline()
	pipe.Set(ctx, "entry:"+idxStr, string(data), 0)
	pipe.Set(ctx, "uuid:"+e.UUID, idxStr, 0)
	pipe.LPush(ctx, "idx:"+e.ArtifactHash, idxStr)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis pipeline: %w", err)
	}

	// Queue leaf to Trillian for tamper-evident audit trail (fire-and-forget).
	leafData := data
	go func() {
		req := &trillian.QueueLeafRequest{
			LogId: s.treeID,
			Leaf:  &trillian.LogLeaf{LeafValue: leafData},
		}
		s.tlog.QueueLeaf(context.Background(), req) //nolint:errcheck
	}()

	return nil
}

// GetByIndex returns the entry at the given log index.
func (s *Store) GetByIndex(ctx context.Context, n int64) (*Entry, error) {
	data, err := s.rdb.Get(ctx, "entry:"+strconv.FormatInt(n, 10)).Bytes()
	if err == redis.Nil {
		return nil, fmt.Errorf("entry %d not found", n)
	}
	if err != nil {
		return nil, fmt.Errorf("redis GET: %w", err)
	}
	var e Entry
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return &e, nil
}

// GetByUUID returns the entry with the given UUID, or nil if not found.
func (s *Store) GetByUUID(ctx context.Context, uuid string) (*Entry, error) {
	idxStr, err := s.rdb.Get(ctx, "uuid:"+uuid).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis GET uuid: %w", err)
	}
	n, err := strconv.ParseInt(idxStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse index: %w", err)
	}
	return s.GetByIndex(ctx, n)
}

// GetByHash returns all entries for the given artifact hash.
func (s *Store) GetByHash(ctx context.Context, hash string) ([]*Entry, error) {
	indices, err := s.rdb.LRange(ctx, "idx:"+hash, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("redis LRANGE: %w", err)
	}
	entries := make([]*Entry, 0, len(indices))
	for _, idxStr := range indices {
		n, err := strconv.ParseInt(idxStr, 10, 64)
		if err != nil {
			continue
		}
		e, err := s.GetByIndex(ctx, n)
		if err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// LogInfo returns the current Trillian tree size and root hash.
func (s *Store) LogInfo(ctx context.Context) (treeSize int64, rootHash string, err error) {
	req := &trillian.GetLatestSignedLogRootRequest{LogId: s.treeID}
	resp, err := s.tlog.GetLatestSignedLogRoot(ctx, req)
	if err != nil {
		return 0, "", fmt.Errorf("trillian GetLatestSignedLogRoot: %w", err)
	}
	var logRoot types.LogRootV1
	if err := logRoot.UnmarshalBinary(resp.SignedLogRoot.LogRoot); err != nil {
		return 0, "", fmt.Errorf("unmarshal log root: %w", err)
	}
	return int64(logRoot.TreeSize), fmt.Sprintf("%x", logRoot.RootHash), nil
}

// gpgSign produces an ASCII-armored detached PGP signature over payload.
func gpgSign(keyID string, payload []byte) (string, error) {
	cmd := exec.Command("gpg", "--batch", "--detach-sign", "--armor", "--local-user", keyID)
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("%s", errMsg)
	}
	return strings.TrimSpace(stdout.String()), nil
}
