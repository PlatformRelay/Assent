package hash_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/hash"
)

type vectorFile struct {
	Algorithm        string   `json:"algorithm"`
	Canonicalization string   `json:"canonicalization"`
	DomainSeparation string   `json:"domainSeparation"`
	Vectors          []vector `json:"vectors"`
}

type vector struct {
	Name          string `json:"name"`
	Group         string `json:"group"`
	SchemaVersion string `json:"schemaVersion"`
	Input         string `json:"input"`
	ExpectedHash  string `json:"expectedHash"`
}

func loadVectors(t *testing.T) vectorFile {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(file), "vectors.json")
	raw, err := os.ReadFile(path) //nolint:gosec // fixed vectors.json beside this test file
	if err != nil {
		t.Fatalf("read vectors.json: %v", err)
	}
	var vf vectorFile
	if err := json.Unmarshal(raw, &vf); err != nil {
		t.Fatalf("parse vectors.json: %v", err)
	}
	if len(vf.Vectors) < 6 {
		t.Fatalf("vectors.json must have ≥6 cases, got %d", len(vf.Vectors))
	}
	return vf
}

// TestCanonicalHash exercises the checked-in vector table (REQ-P3-E2-S03-01..03).
func TestCanonicalHash(t *testing.T) {
	vf := loadVectors(t)

	t.Run("stable", func(t *testing.T) {
		// REQ-P3-E2-S03-01: key-order / whitespace / numeric variants → identical hash.
		var stable []vector
		for _, v := range vf.Vectors {
			if v.Group == "stable" {
				stable = append(stable, v)
			}
		}
		if len(stable) < 2 {
			t.Fatal("need ≥2 stable vectors")
		}
		var ref string
		for _, v := range stable {
			got, err := hash.Digest(v.SchemaVersion, []byte(v.Input))
			if err != nil {
				t.Fatalf("%s: Digest: %v", v.Name, err)
			}
			if got != v.ExpectedHash {
				t.Errorf("%s: got %s, want %s", v.Name, got, v.ExpectedHash)
			}
			if ref == "" {
				ref = got
			} else if got != ref {
				t.Errorf("%s: stable group must share one hash; got %s want %s", v.Name, got, ref)
			}
		}
	})

	t.Run("domain_separation", func(t *testing.T) {
		// REQ-P3-E2-S03-02: same content, two schema versions → different hashes.
		var domain []vector
		for _, v := range vf.Vectors {
			if v.Group == "domain_separation" {
				domain = append(domain, v)
			}
		}
		if len(domain) < 2 {
			t.Fatal("need ≥2 domain_separation vectors")
		}
		hashes := make(map[string]string, len(domain))
		for _, v := range domain {
			got, err := hash.Digest(v.SchemaVersion, []byte(v.Input))
			if err != nil {
				t.Fatalf("%s: Digest: %v", v.Name, err)
			}
			if got != v.ExpectedHash {
				t.Errorf("%s: got %s, want %s", v.Name, got, v.ExpectedHash)
			}
			if prev, ok := hashes[got]; ok {
				t.Errorf("domain collision: %s and %s both hash to %s", prev, v.Name, got)
			}
			hashes[got] = v.Name
		}
		if domain[0].Input != domain[1].Input {
			t.Fatal("domain_separation pair must share identical input bytes")
		}
		if domain[0].SchemaVersion == domain[1].SchemaVersion {
			t.Fatal("domain_separation pair must use distinct schema versions")
		}
	})

	t.Run("all_vectors", func(t *testing.T) {
		// REQ-P3-E2-S03-03: every row's recomputed hash matches the checked-in expected value.
		for _, v := range vf.Vectors {
			got, err := hash.Digest(v.SchemaVersion, []byte(v.Input))
			if err != nil {
				t.Fatalf("%s: Digest: %v", v.Name, err)
			}
			if got != v.ExpectedHash {
				t.Errorf("%s: got %s, want %s", v.Name, got, v.ExpectedHash)
			}
		}
	})
}
