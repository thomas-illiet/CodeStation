package packagefile

import (
	"path/filepath"
	"testing"
)

func TestRepositoryExamplesUseValidDescriptors(t *testing.T) {
	examples, err := filepath.Glob(filepath.Join("..", "..", "..", "examples", "packages", "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(examples) == 0 {
		t.Fatal("no package examples found")
	}
	for _, example := range examples {
		example := example
		t.Run(filepath.Base(example), func(t *testing.T) {
			descriptor, err := Load(example)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateSources(example, descriptor); err != nil {
				t.Fatal(err)
			}
		})
	}
}
