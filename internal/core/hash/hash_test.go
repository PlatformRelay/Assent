package hash_test

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/core/hash"
)

func TestCanonicalize_rejectsInvalid(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"not_json", "not-json"},
		{"trailing", `{"a":1}{"b":2}`},
		{"incomplete", `{"a":`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := hash.Canonicalize([]byte(tc.input)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestCanonicalize_boolsAndFloat(t *testing.T) {
	got, err := hash.Canonicalize([]byte(`{"ok":false,"pi":3.5,"z":0.0}`))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"ok":false,"pi":3.5,"z":0}`
	if string(got) != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestDigest_emptySchemaStillSeparates(t *testing.T) {
	body := []byte(`{"a":1}`)
	a, err := hash.Digest("", body)
	if err != nil {
		t.Fatal(err)
	}
	b, err := hash.Digest("v1alpha1", body)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("empty schemaVersion must not collide with v1alpha1")
	}
	if len(a) != 64 || len(b) != 64 {
		t.Fatalf("unexpected digest lengths: %q %q", a, b)
	}
}
