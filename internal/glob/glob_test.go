package glob_test

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/glob"
)

func TestMatch(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		s       string
		want    bool
	}{
		{"double-star spans segments", "topics/prod/**.yaml", "topics/prod/orders-events.yaml", true},
		{"double-star spans nested dirs", "topics/prod/**.yaml", "topics/prod/sub/a.yaml", true},
		{"double-star with dotted name", "topics/prod/**.yaml", "topics/prod/payments.settled.v2.yaml", true},
		{"wrong extension", "topics/prod/**.yaml", "topics/prod/orders.json", false},
		{"different root", "topics/**", "catalog/services.json", false},
		{"single-star stays in segment", "topics/*.yaml", "topics/orders.yaml", true},
		{"single-star does not span slash", "topics/*.yaml", "topics/sub/orders.yaml", false},
		{"exact pointer", "/partitions", "/partitions", true},
		{"exact pointer mismatch", "/partitions", "/retentionMs", false},
		{"pointer wildcard segment", "/services/*/tier", "/services/api/tier", true},
		{"pointer wildcard rejects deep", "/services/*/tier", "/services/api/sub/tier", false},
		{"dot is literal not wildcard", "a.b", "axb", false},
		{"empty pattern matches empty", "", "", true},
		{"empty pattern rejects nonempty", "", "x", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := glob.Match(tc.pattern, tc.s); got != tc.want {
				t.Errorf("Match(%q, %q) = %v, want %v", tc.pattern, tc.s, got, tc.want)
			}
		})
	}
}
