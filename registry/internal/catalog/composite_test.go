package catalog

import (
	"errors"
	"testing"
	"time"
)

type metadataStub struct {
	manifest  PackageVersion
	insertErr error
	deleted   bool
}

func (s *metadataStub) List(string, string, string, int, int) (Page, error) { return Page{}, nil }
func (s *metadataStub) Get(_, _, _ string) (PackageVersion, error) {
	if s.manifest.Name == "" || s.deleted {
		return PackageVersion{}, ErrNotFound
	}
	return s.manifest, nil
}
func (s *metadataStub) Versions(string, string) ([]PackageVersion, error) { return nil, ErrNotFound }
func (s *metadataStub) InsertMetadata(manifest PackageVersion) error {
	if s.insertErr != nil {
		return s.insertErr
	}
	s.manifest = manifest
	return nil
}
func (s *metadataStub) DeleteMetadata(string, string, string) error {
	s.deleted = true
	return nil
}

type objectStub struct {
	put, deleted bool
}

func (s *objectStub) ReadObject(string) ([]byte, string, error) { return nil, "", ErrNotFound }
func (s *objectStub) PutObjects(PackageVersion, []byte, []byte, []byte) error {
	s.put = true
	return nil
}
func (s *objectStub) DeleteObjects(PackageVersion) error {
	s.deleted = true
	return nil
}

func TestCompositeStoreRollsBackObjectsOnMetadataFailure(t *testing.T) {
	metadata := &metadataStub{insertErr: errors.New("database unavailable")}
	objects := &objectStub{}
	store := NewCompositeStore(metadata, objects)
	manifest := PackageVersion{Namespace: "core", Name: "tool", Version: "1.0.0", PublishedAt: time.Now()}
	if err := store.Publish(manifest, nil, nil, nil); err == nil {
		t.Fatal("expected metadata error")
	}
	if !objects.put || !objects.deleted {
		t.Fatalf("object rollback: put=%v deleted=%v", objects.put, objects.deleted)
	}
}
