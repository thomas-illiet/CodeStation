package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var ErrConflict = errors.New("conflict")

type ProductionConfig struct {
	DatabaseURL string
	S3Endpoint  string
	S3AccessKey string
	S3SecretKey string
	S3Bucket    string
	S3UseTLS    bool
	S3Region    string
}

type PostgresS3Store struct {
	db     *pgxpool.Pool
	object *minio.Client
	bucket string
}

// NewPostgresMetadataStore connects an independent PostgreSQL metadata provider.
func NewPostgresMetadataStore(ctx context.Context, databaseURL string) (*PostgresS3Store, error) {
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := db.Ping(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &PostgresS3Store{db: db}, nil
}

// NewPostgresS3Store connects and validates the production PostgreSQL and S3 backends.
func NewPostgresS3Store(ctx context.Context, config ProductionConfig) (*PostgresS3Store, error) {
	db, err := pgxpool.New(ctx, config.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := db.Ping(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	client, err := minio.New(config.S3Endpoint, &minio.Options{Creds: credentials.NewStaticV4(config.S3AccessKey, config.S3SecretKey, ""), Secure: config.S3UseTLS, Region: config.S3Region})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("configure object store: %w", err)
	}
	return &PostgresS3Store{db: db, object: client, bucket: config.S3Bucket}, nil
}

// Close releases database connections owned by the store.
func (s *PostgresS3Store) Close() { s.db.Close() }

// List queries, filters and paginates published package summaries.
func (s *PostgresS3Store) List(query, category, sortBy string, page, pageSize int) (Page, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pattern := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	filter := `(retired = false AND ($1 = '%%' OR lower(namespace || ' ' || name || ' ' || (manifest->>'summary') || ' ' || (manifest->>'publisher')) LIKE $1) AND ($2 = '' OR (manifest->'categories') ? $2))`
	var total int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM (SELECT DISTINCT ON (namespace,name) namespace FROM package_versions WHERE `+filter+` ORDER BY namespace,name,published_at DESC) latest`, pattern, category).Scan(&total); err != nil {
		return Page{}, err
	}
	order := "name ASC"
	if sortBy == "updated" {
		order = "published_at DESC"
	}
	rows, err := s.db.Query(ctx, `SELECT DISTINCT ON (namespace,name) manifest FROM package_versions WHERE `+filter+` ORDER BY namespace,name,published_at DESC`, pattern, category)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	items := make([]PackageSummary, 0, total)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return Page{}, err
		}
		var v PackageVersion
		if err := json.Unmarshal(raw, &v); err != nil {
			return Page{}, err
		}
		items = append(items, PackageSummary{Namespace: v.Namespace, Name: v.Name, Summary: v.Summary, Publisher: v.Publisher, Categories: v.Categories, LatestVersion: v.Version, IconURL: "/v1/packages/" + v.Namespace + "/" + v.Name + "/" + v.Version + "/icon", UpdatedAt: v.PublishedAt})
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	sort.Slice(items, func(i, j int) bool {
		if order == "published_at DESC" {
			return items[i].UpdatedAt.After(items[j].UpdatedAt)
		}
		return items[i].Name < items[j].Name
	})
	start := (page - 1) * pageSize
	if start > len(items) {
		start = len(items)
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	pages := 0
	if total > 0 {
		pages = (total + pageSize - 1) / pageSize
	}
	return Page{Items: items[start:end], Page: page, PageSize: pageSize, Total: total, TotalPages: pages}, nil
}

// Get returns a specific package release or the current latest release.
func (s *PostgresS3Store) Get(namespace, name, version string) (PackageVersion, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var raw []byte
	query := `SELECT manifest FROM package_versions WHERE namespace=$1 AND name=$2 AND version=$3`
	args := []any{namespace, name, version}
	if version == "" || version == "latest" {
		query = `SELECT manifest FROM package_versions WHERE namespace=$1 AND name=$2 AND retired=false ORDER BY published_at DESC LIMIT 1`
		args = []any{namespace, name}
	}
	if err := s.db.QueryRow(ctx, query, args...).Scan(&raw); errors.Is(err, pgx.ErrNoRows) {
		return PackageVersion{}, ErrNotFound
	} else if err != nil {
		return PackageVersion{}, err
	}
	var result PackageVersion
	if err := json.Unmarshal(raw, &result); err != nil {
		return PackageVersion{}, err
	}
	return result, nil
}

// Versions lists every published release of a package.
func (s *PostgresS3Store) Versions(namespace, name string) ([]PackageVersion, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := s.db.Query(ctx, `SELECT manifest FROM package_versions WHERE namespace=$1 AND name=$2 ORDER BY published_at DESC`, namespace, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []PackageVersion{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var item PackageVersion
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, ErrNotFound
	}
	return result, nil
}

// ReadObject fetches immutable bytes and their content type from S3.
func (s *PostgresS3Store) ReadObject(path string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	object, err := s.object.GetObject(ctx, s.bucket, path, minio.GetObjectOptions{})
	if err != nil {
		return nil, "", err
	}
	defer object.Close()
	info, err := object.Stat()
	if minio.ToErrorResponse(err).StatusCode == 404 {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	data, err := io.ReadAll(io.LimitReader(object, 536870913))
	if err != nil {
		return nil, "", err
	}
	if len(data) > 536870912 {
		return nil, "", fmt.Errorf("object exceeds maximum size")
	}
	contentType := info.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return data, contentType, nil
}

// Publish atomically records metadata after uploading immutable package objects.
func (s *PostgresS3Store) Publish(manifest PackageVersion, archive, readme, icon []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	exists, err := s.object.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("check object bucket: %w", err)
	}
	if !exists {
		if err := s.object.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("create object bucket: %w", err)
		}
	}
	objects := []struct {
		path, contentType string
		data              []byte
	}{
		{manifest.Archive.Path, "application/gzip", archive},
		{manifest.Readme.Path, "text/markdown; charset=utf-8", readme},
		{manifest.Icon.Path, "image/svg+xml", icon},
	}
	for _, object := range objects {
		if _, err := s.object.StatObject(ctx, s.bucket, object.path, minio.StatObjectOptions{}); err == nil {
			return ErrConflict
		} else if minio.ToErrorResponse(err).StatusCode != http.StatusNotFound {
			return fmt.Errorf("check object %s: %w", object.path, err)
		}
	}
	for _, object := range objects {
		if _, err := s.object.PutObject(ctx, s.bucket, object.path, bytes.NewReader(object.data), int64(len(object.data)), minio.PutObjectOptions{ContentType: object.contentType}); err != nil {
			return fmt.Errorf("upload object %s: %w", object.path, err)
		}
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO package_versions(namespace,name,version,manifest,published_at) VALUES($1,$2,$3,$4,$5)`, manifest.Namespace, manifest.Name, manifest.Version, raw, manifest.PublishedAt)
	if pgErr := new(pgconn.PgError); errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrConflict
	}
	return err
}

// InsertMetadata inserts one immutable package manifest.
func (s *PostgresS3Store) InsertMetadata(manifest PackageVersion) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO package_versions(namespace,name,version,manifest,published_at) VALUES($1,$2,$3,$4,$5)`, manifest.Namespace, manifest.Name, manifest.Version, raw, manifest.PublishedAt)
	if pgErr := new(pgconn.PgError); errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrConflict
	}
	return err
}

// DeleteMetadata deletes one package manifest.
func (s *PostgresS3Store) DeleteMetadata(namespace, name, version string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := s.db.Exec(ctx, `DELETE FROM package_versions WHERE namespace=$1 AND name=$2 AND version=$3`, namespace, name, version)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Remove deletes a release and its associated immutable objects.
func (s *PostgresS3Store) Remove(namespace, name, version string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	manifest, err := s.Get(namespace, name, version)
	if err != nil {
		return err
	}
	result, err := s.db.Exec(ctx, `DELETE FROM package_versions WHERE namespace=$1 AND name=$2 AND version=$3`, namespace, name, version)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	for _, path := range []string{manifest.Archive.Path, manifest.Readme.Path, manifest.Icon.Path} {
		if err := s.object.RemoveObject(ctx, s.bucket, path, minio.RemoveObjectOptions{}); err != nil {
			return fmt.Errorf("remove object %s: %w", path, err)
		}
	}
	return nil
}
