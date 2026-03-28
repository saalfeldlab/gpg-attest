package main

import (
	"flag"
	"log"
	"net/http"
	"os/exec"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	httpSwagger "github.com/swaggo/http-swagger/v2"

	"gpg-attest.org/server/internal/api"
	"gpg-attest.org/server/internal/store"

	_ "gpg-attest.org/server/docs"
)

// @title           gpg-attest-server API
// @version         0.1.0
// @description     Transparency log for GPG-signed attestations on digital content.
// @description     Stores entries in a Trillian Merkle tree, indexes by artifact SHA-256 via Redis,
// @description     and signs each entry with the server's GPG key.
// @BasePath        /api/v1
func main() {
	addr := flag.String("addr", ":8081", "listen address")
	trillianAddr := flag.String("trillian", "localhost:8090", "Trillian log server gRPC address")
	redisAddr := flag.String("redis", "localhost:6379", "Redis address")
	treeID := flag.Int64("tree-id", 0, "Trillian tree ID")
	gpgKeyID := flag.String("gpg-keyid", "", "GPG key ID, fingerprint, or email for server timestamp signing")
	flag.Parse()

	if *gpgKeyID == "" {
		log.Fatal("--gpg-keyid is required")
	}
	if *treeID == 0 {
		log.Fatal("--tree-id is required")
	}

	// Validate the GPG key exists in the keyring.
	if err := exec.Command("gpg", "--list-keys", *gpgKeyID).Run(); err != nil {
		log.Fatalf("GPG key %q not found in keyring: %v", *gpgKeyID, err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: *redisAddr})

	conn, err := grpc.NewClient(*trillianAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("trillian connect: %v", err)
	}
	defer conn.Close()

	s := store.New(rdb, conn, *treeID, *gpgKeyID)
	h, err := api.New(s, *gpgKeyID)
	if err != nil {
		log.Fatalf("api init: %v", err)
	}

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.Handle("GET /swagger/", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	log.Printf("gpg-attest-server listening on %s (tree %d)", *addr, *treeID)
	if err := http.ListenAndServe(*addr, loggingMiddleware(mux)); err != nil {
		log.Fatal(err)
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Microsecond))
	})
}

