package api

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gpg-attest.org/server/internal/store"
)

var allowedVerdicts = map[string]bool{
	"false": true, "suspect": true, "plausible": true, "trusted": true, "verified": true,
}

// Handler handles HTTP requests for the transparency log API.
type Handler struct {
	store  *store.Store
	pubPEM string
}

// New creates a Handler and derives the PEM public key from privKey.
func New(s *store.Store, privKey ed25519.PrivateKey) *Handler {
	pub := privKey.Public().(ed25519.PublicKey)
	der, _ := x509.MarshalPKIXPublicKey(pub)
	pubPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	return &Handler{store: s, pubPEM: pubPEM}
}

// RegisterRoutes registers all API routes on mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/entries", h.createEntry)
	mux.HandleFunc("GET /api/v1/entries", h.listEntries)
	mux.HandleFunc("GET /api/v1/entries/{uuid}", h.getEntry)
	mux.HandleFunc("GET /api/v1/publickey", h.publicKey)
	mux.HandleFunc("GET /api/v1/loginfo", h.logInfo)
}

// CreateRequest is the request body for creating a new attestation entry.
type CreateRequest struct {
	ArtifactHash string `json:"artifact_hash" example:"sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"`
	Verdict      string `json:"verdict" example:"trusted" enums:"false,suspect,plausible,trusted,verified"`
	SignerKeyID  string `json:"signer_keyid" example:"3AA5C34371567BD2"`
	Signature    string `json:"signature" example:"LS0tLS1CRUdJTi..."`
}

// createEntry creates a new attestation entry.
// @Summary      Submit a verdict entry
// @Description  Append a signed verdict attestation to the transparency log. The server assigns uuid, log_index, server_timestamp, and server_signature.
// @Tags         entries
// @Accept       json
// @Produce      json
// @Param        entry  body      CreateRequest  true  "Attestation entry to submit"
// @Success      201    {object}  store.Entry
// @Failure      400    {string}  string  "invalid request"
// @Failure      500    {string}  string  "internal error"
// @Router       /entries [post]
func (h *Handler) createEntry(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(req.ArtifactHash, "sha256:") || len(req.ArtifactHash) <= 7 {
		http.Error(w, "artifact_hash must have sha256: prefix", http.StatusBadRequest)
		return
	}
	if !allowedVerdicts[req.Verdict] {
		http.Error(w, "verdict must be one of: false, suspect, plausible, trusted, verified", http.StatusBadRequest)
		return
	}
	if req.SignerKeyID == "" {
		http.Error(w, "signer_keyid is required", http.StatusBadRequest)
		return
	}
	if req.Signature == "" {
		http.Error(w, "signature is required", http.StatusBadRequest)
		return
	}

	id, err := newUUID()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	e := &store.Entry{
		UUID:            id,
		ArtifactHash:    req.ArtifactHash,
		Verdict:         req.Verdict,
		SignerKeyID:     req.SignerKeyID,
		Signature:       req.Signature,
		ServerTimestamp: time.Now().UTC().Format(time.RFC3339),
	}

	if err := h.store.Append(r.Context(), e); err != nil {
		http.Error(w, "failed to append entry", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(e) //nolint:errcheck
}

// listEntries retrieves all entries for an artifact hash.
// @Summary      List entries by artifact hash
// @Description  Retrieve all attestation entries matching the given artifact hash.
// @Tags         entries
// @Produce      json
// @Param        hash   query     string  true  "Artifact hash (e.g. sha256:abcdef...)"
// @Success      200    {array}   store.Entry
// @Failure      400    {string}  string  "hash query parameter is required"
// @Failure      500    {string}  string  "failed to retrieve entries"
// @Router       /entries [get]
func (h *Handler) listEntries(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("hash")
	if hash == "" {
		http.Error(w, "hash query parameter is required", http.StatusBadRequest)
		return
	}
	entries, err := h.store.GetByHash(r.Context(), hash)
	if err != nil {
		http.Error(w, "failed to retrieve entries", http.StatusInternalServerError)
		return
	}
	if entries == nil {
		entries = []*store.Entry{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries) //nolint:errcheck
}

// getEntry retrieves a single entry by UUID.
// @Summary      Get entry by UUID
// @Description  Retrieve a single attestation entry by its UUID.
// @Tags         entries
// @Produce      json
// @Param        uuid   path      string  true  "Entry UUID"
// @Success      200    {object}  store.Entry
// @Failure      404    {string}  string  "not found"
// @Failure      500    {string}  string  "failed to retrieve entry"
// @Router       /entries/{uuid} [get]
func (h *Handler) getEntry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("uuid")
	e, err := h.store.GetByUUID(r.Context(), id)
	if err != nil {
		http.Error(w, "failed to retrieve entry", http.StatusInternalServerError)
		return
	}
	if e == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(e) //nolint:errcheck
}

// publicKey returns the server's Ed25519 public key.
// @Summary      Get server public key
// @Description  Returns the server's Ed25519 public key in PEM (PKIX) format for verifying server timestamps.
// @Tags         server
// @Produce      plain
// @Success      200  {string}  string  "PEM-encoded public key"
// @Router       /publickey [get]
func (h *Handler) publicKey(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, h.pubPEM)
}

// LogInfoResponse is the response body for the log info endpoint.
type LogInfoResponse struct {
	TreeSize int64  `json:"tree_size" example:"42"`
	RootHash string `json:"root_hash" example:"abc123def456"`
}

// logInfo returns the current Merkle tree status.
// @Summary      Get log info
// @Description  Returns the current Trillian Merkle tree size and root hash.
// @Tags         server
// @Produce      json
// @Success      200  {object}  LogInfoResponse
// @Failure      500  {string}  string  "failed to get log info"
// @Router       /loginfo [get]
func (h *Handler) logInfo(w http.ResponseWriter, r *http.Request) {
	treeSize, rootHash, err := h.store.LogInfo(r.Context())
	if err != nil {
		http.Error(w, "failed to get log info", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(LogInfoResponse{TreeSize: treeSize, RootHash: rootHash}) //nolint:errcheck
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
