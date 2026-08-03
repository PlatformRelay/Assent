// Package glob is the single, pure path/pointer glob matcher shared by the
// routing classifier (internal/core/classify) and the obligation-coverage loop
// (internal/core/aggregate). It supports exactly two wildcards — `*` (any run of
// non-`/` characters, within one path segment) and `**` (any run, spanning `/`
// separators) — so a file glob like `topics/prod/**.yaml` and a pointer glob
// like `/services/*/tier` share one authoritative implementation instead of two
// drifting copies.
//
// It imports nothing (no clock, randomness, environment, or network) and is a
// pure function of (pattern, s), so both callers stay inside the internal/core/**
// purity boundary. It lives in its own package because classify already imports
// aggregate (re-exporting the reserved class); putting the shared matcher in
// either of those would create an import cycle, so it sits below both.
package glob

import (
	"regexp"
	"strings"
)

// Match reports whether s matches pattern. `*` matches any run of non-`/`
// characters within a single path segment; `**` matches any run, including `/`,
// spanning segments. The pattern is translated to an anchored regexp with every
// non-wildcard character escaped, so a literal `.` in the pattern matches only a
// literal `.`. A translation that fails to compile (which cannot occur for these
// two wildcards) matches nothing — fail-closed.
func Match(pattern, s string) bool {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(".*") // ** spans path separators
				i++
			} else {
				b.WriteString("[^/]*") // * stays within one segment
			}
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return false
	}
	return re.MatchString(s)
}
