package catalog

import "testing"

// TestCompareSemverReleaseSortsAfterPrerelease protects release ordering semantics.
func TestCompareSemverReleaseSortsAfterPrerelease(t *testing.T) {
	if compareSemver("1.0.0", "1.0.0-rc.1") <= 0 {
		t.Fatal("release must sort after prerelease")
	}
	if compareSemver("2.0.0", "1.99.99") <= 0 {
		t.Fatal("major version comparison failed")
	}
}
