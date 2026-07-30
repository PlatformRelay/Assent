package change

import (
	"fmt"
	"sort"
)

// Input resource ceilings (E1-S07, ADR-0003 Amendment 2). These are PURE, parse-time comparisons
// against the input bytes and the already-parsed value tree — no wall-clock deadline (a deadline
// needs a clock the purity gate bars from this package; it is fenced to a cmd/assent-tier story).
// A breach yields an opaque ChangeSet whose reason names the breached ceiling — never a crash,
// a partial diff, or a silent skip. Enforcement lives in ONE place (withLimits wrapping every
// producer), so the YAML, JSON, and HCL adapters inherit identical limits instead of three copies.
//
// Defaults are generous for legitimate self-service config (which is KB-sized, shallow, and has
// hundreds of entries) while bounding a maliciously or accidentally oversized/deep/exploded input.
const (
	// maxInputBytes caps the raw input size, checked BEFORE parsing so an oversized document is
	// refused without allocating a parse tree for it.
	maxInputBytes = 1 << 20 // 1 MiB
	// maxDepth caps value-tree nesting depth.
	maxDepth = 64
	// maxNodeCount caps the total number of value-tree nodes (mappings + sequences + scalars).
	maxNodeCount = 20000
	// maxPreParseNesting caps raw bracket-nesting depth in a PRE-PARSE byte scan, refusing a
	// deeply-nested input BEFORE the recursive parser runs. This is load-bearing for fail-CLOSED:
	// a recursive-descent parser (notably hclsyntax.ParseConfig) blows the goroutine stack — a
	// fatal, unrecoverable crash — on a crafted sub-maxInputBytes brace bomb, so the post-parse
	// depth ceiling (maxDepth, on the built tree) can never fire. This coarse byte-level scan
	// pre-empts that. It is intentionally set FAR above maxDepth: it is not the semantic depth
	// limit (maxDepth is, enforced on the parsed tree) but a crude crash-guard that must not
	// false-positive on a legitimate string value that happens to contain brackets, while sitting
	// far below any stack-exhaustion threshold. A breach fails safe (opaque), never open.
	maxPreParseNesting = 1000
)

// withLimits wraps a treeProducer with the shared input ceilings: a pre-parse byte-size check and
// a post-parse depth/node-count walk. Any breach returns a non-empty reason (routed to opaque).
// This is the single enforcement point every format adapter inherits (E1-S07).
func withLimits(p treeProducer) treeProducer {
	return func(data []byte) (*vnode, string) {
		if len(data) > maxInputBytes {
			return nil, fmt.Sprintf("input size %d bytes exceeds the max input size ceiling (%d bytes)", len(data), maxInputBytes)
		}
		// Pre-parse nesting guard: MUST run before the (recursive-descent) producer, because a
		// crafted sub-maxInputBytes brace bomb blows the parser's goroutine stack — a fatal,
		// unrecoverable crash — before any post-parse tree ceiling could fire. This coarse
		// byte-level scan refuses such input to opaque first, so every format (incl. HCL, whose
		// parser has no internal nesting cap) fails CLOSED rather than crashing.
		if reason := preParseNestingReason(data); reason != "" {
			return nil, reason
		}
		tree, reason := p(data)
		if reason != "" {
			return nil, reason
		}
		if reason := checkNodeCeilings(tree); reason != "" {
			return nil, reason
		}
		return tree, ""
	}
}

// preParseNestingReason scans the raw bytes for bracket-nesting depth ({[(  vs  }])), returning a
// non-empty reason the moment depth exceeds maxPreParseNesting. It runs BEFORE any parser, so a
// brace bomb is refused before it can drive a recursive parser into stack exhaustion. It is a
// crude crash-guard, not the semantic depth limit (maxDepth, enforced on the parsed tree): it does
// not track string context, so brackets inside a string value count too — deliberately, since the
// ceiling is far above any legitimate config and the failure direction (opaque) is safe. Pure and
// O(n) with an early exit.
func preParseNestingReason(data []byte) string {
	depth := 0
	for _, b := range data {
		switch b {
		case '{', '[', '(':
			depth++
			if depth > maxPreParseNesting {
				return fmt.Sprintf("input bracket-nesting depth exceeds the max pre-parse nesting ceiling (%d) — refused before parse to prevent stack exhaustion", maxPreParseNesting)
			}
		case '}', ']', ')':
			if depth > 0 {
				depth--
			}
		}
	}
	return ""
}

// checkNodeCeilings walks the value tree once, failing closed if nesting depth exceeds maxDepth or
// the total node count exceeds maxNodeCount. The walk is deterministic (mapping keys visited in
// sorted order) so the breach reason is byte-stable across runs. It is the generalized form of the
// pre-S07 per-format ad hoc guards; combined with the differ's outright rejection of YAML
// aliases/anchors (which stops an alias/anchor expansion bomb before it ever reaches this walk),
// the two jointly close the resource-exhaustion class — depth, count, and alias expansion.
func checkNodeCeilings(root *vnode) string {
	count := 0
	var walk func(n *vnode, depth int) string
	walk = func(n *vnode, depth int) string {
		if depth > maxDepth {
			return fmt.Sprintf("nesting depth exceeds the max depth ceiling (%d)", maxDepth)
		}
		count++
		if count > maxNodeCount {
			return fmt.Sprintf("value-tree node count exceeds the max entry-count ceiling (%d)", maxNodeCount)
		}
		switch n.kind {
		case vMapping:
			keys := make([]string, 0, len(n.fields))
			for k := range n.fields {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if r := walk(n.fields[k], depth+1); r != "" {
					return r
				}
			}
		case vSequence:
			for _, e := range n.elems {
				if r := walk(e, depth+1); r != "" {
					return r
				}
			}
		}
		return ""
	}
	return walk(root, 1)
}
