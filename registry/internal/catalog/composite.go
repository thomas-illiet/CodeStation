package catalog

import "errors"

// CompositeStore combines any metadata provider with any object provider.
type CompositeStore struct {
	metadata MetadataStore
	objects  ObjectStore
	closers  []func() error
}

// NewCompositeStore composes independently configured metadata and object stores.
func NewCompositeStore(metadata MetadataStore, objects ObjectStore, closers ...func() error) *CompositeStore {
	return &CompositeStore{metadata: metadata, objects: objects, closers: closers}
}

func (s *CompositeStore) List(query, category, sortBy string, page, pageSize int) (Page, error) {
	return s.metadata.List(query, category, sortBy, page, pageSize)
}

func (s *CompositeStore) Get(namespace, name, version string) (PackageVersion, error) {
	return s.metadata.Get(namespace, name, version)
}

func (s *CompositeStore) Versions(namespace, name string) ([]PackageVersion, error) {
	return s.metadata.Versions(namespace, name)
}

func (s *CompositeStore) ReadObject(path string) ([]byte, string, error) {
	return s.objects.ReadObject(path)
}

// Publish writes objects first and rolls them back if the metadata insert fails.
func (s *CompositeStore) Publish(manifest PackageVersion, archive, readme, icon []byte) error {
	if _, err := s.metadata.Get(manifest.Namespace, manifest.Name, manifest.Version); err == nil {
		return ErrConflict
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	if err := s.objects.PutObjects(manifest, archive, readme, icon); err != nil {
		return err
	}
	if err := s.metadata.InsertMetadata(manifest); err != nil {
		_ = s.objects.DeleteObjects(manifest)
		return err
	}
	return nil
}

// Remove deletes metadata first, then its now-unreferenced immutable objects.
func (s *CompositeStore) Remove(namespace, name, version string) error {
	manifest, err := s.metadata.Get(namespace, name, version)
	if err != nil {
		return err
	}
	if err := s.metadata.DeleteMetadata(namespace, name, version); err != nil {
		return err
	}
	return s.objects.DeleteObjects(manifest)
}

// Close releases all configured provider resources.
func (s *CompositeStore) Close() error {
	var result error
	for i := len(s.closers) - 1; i >= 0; i-- {
		result = errors.Join(result, s.closers[i]())
	}
	return result
}
