package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PlatformRelay/assent/internal/change"
	"github.com/PlatformRelay/assent/internal/core/classify"
	"github.com/PlatformRelay/assent/internal/forge"
)

// localCheckout is the E1-S08 mechanism seam (ADR-0008 §4). It yields the MR's
// full changed-file set and each changed file's base+head bytes from the LOCAL
// CHECKOUT ONLY — never a forge API call. This is a hard fence, not a style
// choice: ADR-0008 §4 forbids API-only file fetching, so the new changed-file
// enumeration this story adds must read the working tree, not issue a
// `client.FileAtRef`-style call per enumerated file. The production
// implementation (dirCheckout) reads a directory; a test double is a fixture
// directory, keeping every read on the filesystem.
type localCheckout interface {
	// ChangedFiles returns the deterministic, SORTED set of repo-relative paths
	// the MR's head changed relative to its base — read from the local tree.
	ChangedFiles() ([]string, error)
	// FileContents returns one changed file's base and head bytes from the local
	// tree. A side that is absent (a whole-file add/delete) yields nil bytes —
	// the EFE-S03 presence signal (nil-vs-non-nil, never empty bytes). An
	// empty-but-present file yields a non-nil (possibly zero-length) slice.
	FileContents(path string) (base, head []byte, err error)
}

// dirCheckout is the filesystem-backed localCheckout: base/ and head/ subtrees
// under a root directory. The changed set is every relative path whose base and
// head bytes differ (including present-on-one-side-only). All reads are
// os.ReadFile/WalkDir on the given root — no network, no forge API, satisfying
// the ADR-0008 §4 fence.
type dirCheckout struct{ root string }

func (d dirCheckout) baseDir() string { return filepath.Join(d.root, "base") }
func (d dirCheckout) headDir() string { return filepath.Join(d.root, "head") }

// ChangedFiles walks both subtrees and returns the sorted set of relative paths
// whose base and head contents differ. Sorting makes the enumeration order
// deterministic (REQ-E1-S08-04: byte-identical double-run).
func (d dirCheckout) ChangedFiles() ([]string, error) {
	base, err := collectTree(d.baseDir())
	if err != nil {
		return nil, err
	}
	head, err := collectTree(d.headDir())
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for p := range base {
		seen[p] = struct{}{}
	}
	for p := range head {
		seen[p] = struct{}{}
	}
	var changed []string
	for p := range seen {
		// bytes.Equal(nil, x) is false for a present-on-one-side file, so an
		// add/delete counts as changed; identical content on both sides does not.
		if !bytes.Equal(base[p], head[p]) {
			changed = append(changed, p)
		}
	}
	sort.Strings(changed)
	return changed, nil
}

// FileContents reads one file's base and head bytes from the local subtrees. A
// missing side returns nil (not an error): nil is ABSENT (one-sided lifecycle →
// FileEvent); a present empty file returns non-nil []byte{} and stays a value
// diff / opaque — never a fabricated delete (EFE-S03).
func (d dirCheckout) FileContents(path string) ([]byte, []byte, error) {
	base, err := readIfPresent(filepath.Join(d.baseDir(), filepath.FromSlash(path)))
	if err != nil {
		return nil, nil, err
	}
	head, err := readIfPresent(filepath.Join(d.headDir(), filepath.FromSlash(path)))
	if err != nil {
		return nil, nil, err
	}
	return base, head, nil
}

// collectTree reads every regular file under root into a map keyed by the
// slash-normalised path relative to root. A missing root is an EMPTY tree (an
// all-adds or all-deletes checkout), not an error.
//
// The missing-root tolerance is scoped to THE ROOT ALONE, by stating it once up
// front instead of as a predicate on the walk's error. Tolerating fs.ErrNotExist
// over the whole walk returned a SILENTLY TRUNCATED map with a nil error: any
// mid-walk ENOENT (a dangling symlink is the cheap one to commit) aborted
// WalkDir on its first entry, and every path the walk had not reached yet
// simply vanished from the changed-file set. Sorted lexically, an entry named
// `.aaa` therefore erased `.assent/**` dominance and starved the D-042
// self-vouch guard — BLOCK became APPROVE. A truncated tree must never be
// returned with a nil error; below the root, every failure propagates.
func collectTree(root string) (map[string][]byte, error) {
	out := map[string][]byte{}
	// Lstat, not Stat: a root that is itself a DANGLING symlink is a broken
	// checkout, not an empty side.
	if _, err := os.Lstat(root); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return out, nil
		}
		return nil, err
	}
	err := filepath.WalkDir(root, func(p string, dirent fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if dirent.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		// #nosec G304 G122 -- operator-supplied local checkout dir walked read-only,
		// not remote/attacker input; symlink TOCTOU is out of scope for a local tree
		// the operator hands us (ADR-0008 §4 checkout), fixing it belongs to the
		// checkout-provisioning story, not this classifier read.
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = data
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// readIfPresent returns a file's bytes, or nil (no error) when it does not exist.
func readIfPresent(p string) ([]byte, error) {
	data, err := os.ReadFile(p) // #nosec G304 -- operator-supplied local checkout dir, not remote input.
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

// checkoutFold is the E1-S08 verdict the full changed-file set contributes on top
// of the governed subject's own diff: the dominating class over ALL changed paths
// and whether ANY changed file's diff was opaque.
type checkoutFold struct {
	class        classify.Class
	opaque       bool
	opaqueReason string
}

// foldCheckout enumerates the MR's full changed-file set from the local checkout,
// diffs EACH changed file, and folds the result into two safety signals:
//
//   - class: the dominating class over every changed PATH (path-scan via
//     classify.FileClass). Any `.assent/**` path -> assent-policy. The path-scan
//     (not classify.Classify over concatenated changes) is deliberate: an opaque
//     `.assent/**` file carries NO changes, so a change-list scan would miss it —
//     a policy edit could then hide behind its own opacity. The same isPolicyPath
//     predicate as the golden, so the routing outcome matches
//     TestAssentPolicyDominatesMixedChangeSet.
//   - opaque: true if ANY changed file's diff is opaque (undecidable). An opaque
//     file among many is never silently dropped — it forces the whole run
//     fail-safe (REQ-E1-S08-03).
//
// governed is the repo-relative governed-subject path (the --subject file:…).
// Only THAT path's unambiguous one-sided lifecycle skips fold-opacity (EFE-S03:
// the Diff→ChangeSet path mints its FileEvent and Cover must not short-circuit).
// A sibling whole-file add/delete stays opaque — otherwise a clean governed
// value-diff could APPROVE while another file vanishes wholesale (E1-S08-03).
//
// The per-file change LISTS are intentionally NOT concatenated into the
// aggregator's predicate input: bindActivation binds scalar old/new only for a
// single change, so a multi-file union would leave predicates unbound and, worse,
// could let an unrelated file's scalars leak into the governed subject's
// predicate. The governed subject's own single-file ChangeSet stays the rule-eval
// input; this fold only strengthens the class and the opaque flag, both of which
// the aggregator honours BEFORE any predicate runs (reserved-class + opaque
// short-circuits) — so every security case is decided without rule evaluation.
func foldCheckout(co localCheckout, governed string) (checkoutFold, error) {
	files, err := co.ChangedFiles()
	if err != nil {
		return checkoutFold{}, fmt.Errorf("list changed files: %w", err)
	}
	fold := checkoutFold{class: classify.ClassUnclassified}
	var reasons []string
	for _, path := range files {
		if classify.FileClass(path) == classify.ClassAssentPolicy {
			fold.class = classify.ClassAssentPolicy
		}
		base, head, err := co.FileContents(path)
		if err != nil {
			return checkoutFold{}, fmt.Errorf("read %q from checkout: %w", path, err)
		}
		// Governed-subject one-sided presence is a CLEAN whole-file lifecycle
		// (EFE-S03), not fold-opaque — Cover must see the minted FileEvent.
		// Sibling one-sided presence stays opaque (fail-safe; E1-S08-03).
		if path == governed {
			if _, ok := change.OneSidedLifecycle(base, head); ok {
				continue
			}
		}
		// Diff each changed file to detect an opaque (undecidable) diff. Diff only
		// ever errors via its opaque path, so an ErrOpaque is fail-safe, not a hard
		// error. The change list itself is not carried into rule-eval (see doc).
		cs, diffErr := change.Diff(path, base, head)
		if diffErr != nil {
			if !errors.Is(diffErr, change.ErrOpaque) {
				return checkoutFold{}, fmt.Errorf("diff %q from checkout: %w", path, diffErr)
			}
			fold.opaque = true
			reasons = append(reasons, path+": "+cs.OpaqueReason)
			continue
		}
		if cs.Opaque {
			fold.opaque = true
			reasons = append(reasons, path+": "+cs.OpaqueReason)
		}
	}
	// files is sorted, so reasons are deterministic (REQ-E1-S08-04).
	fold.opaqueReason = strings.Join(reasons, "; ")
	return fold, nil
}

// foldSnapshotPaths applies the E4-S06 path-only classifier fold over forge
// Snapshot changed files when --checkout is unset. Any `.assent/**` path in the
// Snapshot set dominates to assent-policy (BLOCK).
//
// It also folds the ADR-0020 / D-119 COMPLETENESS signal. Here the Snapshot's
// path list is the SOLE `.assent/**` detector, so an enumeration that cannot
// prove itself complete is epistemically identical to an opaque diff: it is
// folded opaque, which the decide() short-circuit turns into fail-safe REVIEW
// (`changeset.undecidable`) — an auditable record and a resolvable thread, and
// never an approve/merge. Byte-level opacity of individual files remains the
// checkout fold's job; paths alone cannot see it.
//
// Ordering note: the class fold is NOT skipped when the enumeration is
// incomplete. A `.assent/**` path that IS visible in the partial list must
// still dominate to BLOCK (GUARD 1 over the gap-degrade), which decide()
// enforces by checking the reserved class before the opaque short-circuit.
func foldSnapshotPaths(snap forge.Snapshot) checkoutFold {
	fold := checkoutFold{class: classify.ClassUnclassified}
	for _, path := range snap.ChangedFiles {
		if classify.FileClass(path) == classify.ClassAssentPolicy {
			fold.class = classify.ClassAssentPolicy
		}
	}
	if reason := snap.EnumerationOpaqueReason(); reason != "" {
		fold.opaque = true
		// The NORMATIVE reason, verbatim (ADR-0020 §4). The caller must not
		// re-prefix it.
		fold.opaqueReason = reason
	}
	return fold
}
