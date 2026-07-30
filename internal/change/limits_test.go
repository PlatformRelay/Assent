package change

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// A tiny valid document per format, used as the (small) base side while the head breaches a ceiling.
func smallDoc(ext string) string {
	switch ext {
	case "json":
		return `{"k":1}`
	case "tfvars":
		return "k = 1\n"
	default:
		return "k: 1\n"
	}
}

func extFile(ext string) string { return "f." + ext }

// oversized returns a syntactically-valid document > maxInputBytes for the given format (a single
// huge string value), so the pre-parse size ceiling fires regardless of format.
func oversized(ext string) string {
	big := strings.Repeat("x", maxInputBytes+16)
	switch ext {
	case "json":
		return `{"k":"` + big + `"}`
	case "tfvars":
		return `k = "` + big + `"` + "\n"
	default:
		return "k: " + big + "\n"
	}
}

// deeplyNested returns a valid document nested `depth` levels for the given format.
func deeplyNested(ext string, depth int) string {
	switch ext {
	case "json":
		return strings.Repeat(`{"a":`, depth) + "1" + strings.Repeat("}", depth)
	case "tfvars":
		return "a = " + strings.Repeat("{ a = ", depth) + "1" + strings.Repeat(" }", depth) + "\n"
	default: // yaml
		var b strings.Builder
		for i := 0; i < depth; i++ {
			b.WriteString(strings.Repeat("  ", i))
			b.WriteString("a:\n")
		}
		b.WriteString(strings.Repeat("  ", depth))
		b.WriteString("v: 1\n")
		return b.String()
	}
}

// manyEntries returns a valid FLAT document with n top-level scalar entries for the given format.
func manyEntries(ext string, n int) string {
	var b strings.Builder
	switch ext {
	case "json":
		b.WriteString("{")
		for i := 0; i < n; i++ {
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, `"k%d":%d`, i, i)
		}
		b.WriteString("}")
	case "tfvars":
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, "k%d = %d\n", i, i)
		}
	default: // yaml
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, "k%d: %d\n", i, i)
		}
	}
	return b.String()
}

var limitFormats = []string{"yaml", "json", "tfvars"}

// REQ-E1-S07-01 — an over-size input fails opaque with a reason naming the size ceiling, for all
// three formats (one shared fixture-generation helper).
func TestOversizeInputFailsSafe(t *testing.T) {
	for _, ext := range limitFormats {
		t.Run(ext, func(t *testing.T) {
			cs, err := Diff(extFile(ext), []byte(smallDoc(ext)), []byte(oversized(ext)))
			assertCeilingOpaque(t, cs, err, "size")
		})
	}
}

// REQ-E1-S07-02 — an over-depth input fails opaque naming the depth ceiling, all three formats.
func TestExcessiveDepthFailsSafe(t *testing.T) {
	for _, ext := range limitFormats {
		t.Run(ext, func(t *testing.T) {
			cs, err := Diff(extFile(ext), []byte(smallDoc(ext)), []byte(deeplyNested(ext, maxDepth+5)))
			assertCeilingOpaque(t, cs, err, "depth")
		})
	}
}

// REQ-E1-S07-03 — an over-count input fails opaque naming the entry-count ceiling, all three formats.
func TestExcessiveEntryCountFailsSafe(t *testing.T) {
	for _, ext := range limitFormats {
		t.Run(ext, func(t *testing.T) {
			cs, err := Diff(extFile(ext), []byte(smallDoc(ext)), []byte(manyEntries(ext, maxNodeCount+2)))
			assertCeilingOpaque(t, cs, err, "count")
		})
	}
}

// REQ-E1-S07-04 — the shipped YAML billion-laughs alias bomb stays opaque (the alias/anchor
// rejection stops it before expansion), and a wide-but-shallow node explosion that is NOT an alias
// bomb is caught by the entry-count ceiling — the two guards jointly close the expansion class.
// JSON/HCL have no alias construct (documented), so the alias arm is YAML-only.
func TestAliasExpansionFailsSafe(t *testing.T) {
	t.Run("yaml billion-laughs stays opaque", func(t *testing.T) {
		base := "partitions: 12\n"
		bomb := "anchors: &a [\"x\",\"x\",\"x\",\"x\",\"x\",\"x\",\"x\",\"x\",\"x\"]\nb: &b [*a,*a,*a,*a,*a,*a,*a,*a,*a]\nc: &c [*b,*b,*b,*b,*b,*b,*b,*b,*b]\npartitions: &d [*c,*c,*c,*c,*c,*c,*c,*c,*c]\n"
		cs, err := Diff("f.yaml", []byte(base), []byte(bomb))
		if !cs.Opaque || err == nil || !errors.Is(err, ErrOpaque) {
			t.Fatalf("billion-laughs must stay opaque, got opaque=%v err=%v changes=%+v", cs.Opaque, err, cs.Changes)
		}
	})
	t.Run("wide-but-shallow explosion caught by entry-count", func(t *testing.T) {
		// A flat, alias-free document with too many entries: not depth, not an alias bomb — only
		// the node-count ceiling closes it.
		cs, err := Diff("f.json", []byte(smallDoc("json")), []byte(manyEntries("json", maxNodeCount+2)))
		assertCeilingOpaque(t, cs, err, "count")
	})
}

// REQ-E1-S07-05 — a fixture comfortably UNDER every ceiling parses and diffs normally (the
// ceilings do not false-positive on legitimate bounded input).
func TestUnderCeilingsParsesNormally(t *testing.T) {
	for _, ext := range limitFormats {
		t.Run(ext, func(t *testing.T) {
			var base, head string
			switch ext {
			case "json":
				base, head = `{"a":{"b":1},"c":2}`, `{"a":{"b":1},"c":3}`
			case "tfvars":
				base, head = "a = { b = 1 }\nc = 2\n", "a = { b = 1 }\nc = 3\n"
			default:
				base, head = "a:\n  b: 1\nc: 2\n", "a:\n  b: 1\nc: 3\n"
			}
			cs, err := Diff(extFile(ext), []byte(base), []byte(head))
			if err != nil || cs.Opaque {
				t.Fatalf("under-ceilings fixture must parse normally, got err=%v opaque=%v", err, cs.OpaqueReason)
			}
			if len(cs.Changes) != 1 {
				t.Fatalf("expected one change, got %+v", cs.Changes)
			}
		})
	}
}

// REQ-E1-S07-02 (crash-scale regression — the review's P0) — a crafted sub-maxInputBytes brace
// bomb must fail CLOSED (opaque), NEVER crash the process with a stack overflow. Before the
// pre-parse nesting guard, a deeply brace-nested .tfvars blew hclsyntax.ParseConfig's goroutine
// stack (a fatal, unrecoverable crash) because the depth ceiling only ran post-parse. Each format
// is exercised at a depth far past the guard and past an unguarded recursive parser's crash point,
// while staying under the size cap — proving the guard pre-empts the parser for all three formats.
func TestDeepNestingDoesNotCrash(t *testing.T) {
	const depth = 200000 // well past a recursive parser's stack-crash threshold (~175k)
	for _, ext := range limitFormats {
		t.Run(ext, func(t *testing.T) {
			var head string
			switch ext {
			case "json":
				head = strings.Repeat("[", depth) + strings.Repeat("]", depth) // ~400 KB, < 1 MiB
			case "tfvars":
				head = "x = " + strings.Repeat("[", depth) + strings.Repeat("]", depth) + "\n"
			default: // yaml flow-style nesting
				head = "x: " + strings.Repeat("[", depth) + strings.Repeat("]", depth) + "\n"
			}
			if len(head) > maxInputBytes {
				t.Fatalf("test fixture must stay under the size cap to prove the DEPTH guard fires (len %d)", len(head))
			}
			// Must return cleanly (opaque), not panic/crash the process.
			cs, err := Diff(extFile(ext), []byte(smallDoc(ext)), []byte(head))
			assertCeilingOpaque(t, cs, err, "nesting")
		})
	}
}

// REQ-E1-S07-06 — the ceilings are deterministic: a breaching input double-runs to a byte-identical
// (opaque) result with a stable reason. (Purity is proven by TestCorePurity over internal/change.)
func TestLimitsDoubleRunStable(t *testing.T) {
	head := deeplyNested("json", maxDepth+5)
	cs1, _ := Diff("f.json", []byte(smallDoc("json")), []byte(head))
	cs2, _ := Diff("f.json", []byte(smallDoc("json")), []byte(head))
	if !cs1.Opaque || !cs2.Opaque {
		t.Fatalf("both runs must be opaque")
	}
	if cs1.OpaqueReason != cs2.OpaqueReason {
		t.Errorf("opaque reason not stable: %q vs %q", cs1.OpaqueReason, cs2.OpaqueReason)
	}
}

// assertCeilingOpaque checks a ceiling breach produced a fail-safe opaque ChangeSet whose reason
// names the breached ceiling (by keyword).
func assertCeilingOpaque(t *testing.T, cs ChangeSet, err error, keyword string) {
	t.Helper()
	if !cs.Opaque {
		t.Fatalf("expected opaque (ceiling breach), got opaque=false with %d changes", len(cs.Changes))
	}
	if len(cs.Changes) != 0 {
		t.Errorf("opaque ceiling result must carry no partial changes, got %d", len(cs.Changes))
	}
	if err == nil || !errors.Is(err, ErrOpaque) {
		t.Errorf("expected error wrapping ErrOpaque, got %v", err)
	}
	if !strings.Contains(cs.OpaqueReason, keyword) {
		t.Errorf("opaque reason %q should name the %q ceiling", cs.OpaqueReason, keyword)
	}
}
