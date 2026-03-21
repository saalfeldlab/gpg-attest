package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"gpg-attest.org/server/internal/api"
	"gpg-attest.org/server/internal/store"
)

func main() {
	addr := flag.String("addr", ":8081", "listen address")
	trillianAddr := flag.String("trillian", "localhost:8090", "Trillian log server gRPC address")
	redisAddr := flag.String("redis", "localhost:6379", "Redis address")
	treeID := flag.Int64("tree-id", 0, "Trillian tree ID")
	keyPath := flag.String("key", "", "path to Ed25519 private key (PEM PKCS8; created if missing)")
	flag.Parse()

	if *keyPath == "" {
		log.Fatal("--key is required")
	}
	if *treeID == 0 {
		log.Fatal("--tree-id is required")
	}

	privKey, err := loadOrGenerateKey(*keyPath)
	if err != nil {
		log.Fatalf("key: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: *redisAddr})

	conn, err := grpc.NewClient(*trillianAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("trillian connect: %v", err)
	}
	defer conn.Close()

	s := store.New(rdb, conn, *treeID, privKey)
	h := api.New(s, privKey)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

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

func loadOrGenerateKey(path string) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		block, _ := pem.Decode(data)
		if block == nil {
			return nil, fmt.Errorf("no PEM block in %s", path)
		}
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse key: %w", err)
		}
		privKey, ok := key.(ed25519.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("key in %s is not Ed25519", path)
		}
		return privKey, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read key file: %w", err)
	}

	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	der, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("marshal key: %w", err)
	}
	pemData := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}
	if err := os.WriteFile(path, pemData, 0600); err != nil {
		return nil, fmt.Errorf("write key: %w", err)
	}

	log.Printf("generated new Ed25519 key at %s", path)
	return privKey, nil
}
