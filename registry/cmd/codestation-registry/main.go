package main

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"codestation/registry/internal/catalog"
	"codestation/registry/internal/httpapi"
)

// main configures the production catalog store and starts the HTTP server.
func main() {
	listen := flag.String("listen", env("CODESTATION_LISTEN", ":8080"), "HTTP listen address")
	data := flag.String("data", env("CODESTATION_DATA", "./data"), "catalog data directory")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	store, closeStore, err := configureStore(*data)
	if err != nil {
		logger.Error("storage initialization failed", "error", err)
		os.Exit(1)
	}
	if closeStore != nil {
		defer closeStore()
	}
	handler := httpapi.New(store, logger, os.Getenv("CODESTATION_ADMIN_TOKEN"))
	if _, writable := store.(catalog.AdminStore); writable {
		signing, signingErr := loadSigningConfig()
		if signingErr != nil {
			logger.Error("signing initialization failed", "error", signingErr)
			os.Exit(1)
		}
		handler = httpapi.NewWithSigning(store, logger, os.Getenv("CODESTATION_ADMIN_TOKEN"), signing)
	}
	server := &http.Server{Addr: *listen, Handler: handler.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Minute, WriteTimeout: 5 * time.Minute, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	logger.Info("registry listening", "address", *listen, "data", *data)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("registry stopped", "error", err)
		os.Exit(1)
	}
}

func loadSigningConfig() (httpapi.SigningConfig, error) {
	keyID := os.Getenv("CODESTATION_SIGNING_KEY_ID")
	keyFile := os.Getenv("CODESTATION_SIGNING_PRIVATE_KEY_FILE")
	if keyID == "" || keyFile == "" {
		return httpapi.SigningConfig{}, fmt.Errorf("CODESTATION_SIGNING_KEY_ID and CODESTATION_SIGNING_PRIVATE_KEY_FILE are required")
	}
	raw, err := os.ReadFile(keyFile)
	if err != nil {
		return httpapi.SigningConfig{}, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return httpapi.SigningConfig{}, fmt.Errorf("signing key is not PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return httpapi.SigningConfig{}, err
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return httpapi.SigningConfig{}, fmt.Errorf("signing key is not Ed25519")
	}
	return httpapi.SigningConfig{KeyID: keyID, PrivateKey: key}, nil
}

func configureStore(data string) (catalog.Store, func(), error) {
	metadataProvider := os.Getenv("CODESTATION_METADATA_PROVIDER")
	objectProvider := os.Getenv("CODESTATION_OBJECT_STORAGE_PROVIDER")
	legacyProvider := os.Getenv("CODESTATION_DATABASE_PROVIDER")
	if metadataProvider == "" && objectProvider == "" {
		switch {
		case legacyProvider == "sqlite":
			metadataProvider, objectProvider = "sqlite", "local"
		case legacyProvider == "postgres" || legacyProvider == "" && os.Getenv("CODESTATION_DATABASE_URL") != "":
			metadataProvider, objectProvider = "postgres", "s3"
		case legacyProvider == "" || legacyProvider == "filesystem":
			return catalog.NewFileStore(data), nil, nil
		default:
			return nil, nil, fmt.Errorf("unsupported CODESTATION_DATABASE_PROVIDER %q", legacyProvider)
		}
	}
	if metadataProvider == "" {
		if os.Getenv("CODESTATION_DATABASE_URL") != "" {
			metadataProvider = "postgres"
		} else {
			metadataProvider = "sqlite"
		}
	}
	if objectProvider == "" {
		if os.Getenv("CODESTATION_S3_ENDPOINT") != "" {
			objectProvider = "s3"
		} else {
			objectProvider = "local"
		}
	}

	var metadata catalog.MetadataStore
	closers := []func() error{}
	switch metadataProvider {
	case "postgres":
		databaseURL := os.Getenv("CODESTATION_DATABASE_URL")
		if databaseURL == "" {
			return nil, nil, fmt.Errorf("CODESTATION_DATABASE_URL is required for postgres")
		}
		postgres, err := catalog.NewPostgresMetadataStore(context.Background(), databaseURL)
		if err != nil {
			return nil, nil, err
		}
		metadata = postgres
		closers = append(closers, func() error { postgres.Close(); return nil })
	case "sqlite":
		databasePath := env("CODESTATION_SQLITE_PATH", filepath.Join(data, "registry.db"))
		sqliteStore, err := catalog.NewSQLiteMetadataStore(databasePath)
		if err != nil {
			return nil, nil, err
		}
		metadata = sqliteStore
		closers = append(closers, sqliteStore.Close)
	default:
		return nil, nil, fmt.Errorf("unsupported CODESTATION_METADATA_PROVIDER %q", metadataProvider)
	}

	var objects catalog.ObjectStore
	switch objectProvider {
	case "local":
		objectsPath := env("CODESTATION_LOCAL_OBJECTS_PATH", env("CODESTATION_OBJECTS_PATH", filepath.Join(data, "objects")))
		local, err := catalog.NewLocalObjectStore(objectsPath)
		if err != nil {
			closeAll(closers)
			return nil, nil, err
		}
		objects = local
	case "s3":
		s3, err := catalog.NewS3ObjectStore(catalog.S3Config{Endpoint: os.Getenv("CODESTATION_S3_ENDPOINT"), AccessKey: os.Getenv("CODESTATION_S3_ACCESS_KEY"), SecretKey: os.Getenv("CODESTATION_S3_SECRET_KEY"), Bucket: os.Getenv("CODESTATION_S3_BUCKET"), UseTLS: env("CODESTATION_S3_USE_TLS", "true") == "true", Region: os.Getenv("CODESTATION_S3_REGION")})
		if err != nil {
			closeAll(closers)
			return nil, nil, err
		}
		objects = s3
	default:
		closeAll(closers)
		return nil, nil, fmt.Errorf("unsupported CODESTATION_OBJECT_STORAGE_PROVIDER %q", objectProvider)
	}
	store := catalog.NewCompositeStore(metadata, objects, closers...)
	return store, func() { _ = store.Close() }, nil
}

func closeAll(closers []func() error) {
	for _, closeProvider := range closers {
		_ = closeProvider()
	}
}

// env returns a configured environment value or its fallback.
func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
