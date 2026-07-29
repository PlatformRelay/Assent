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
	// tree. A side that is absent (a whole-file add/delete) yields nil bytes,
	// which the differ treats as opaque (fail-safe) rather than a silent no-op.
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
// missing side returns nil (not an error): the differ maps nil/empty content to
// an opaque diff, which is the fail-safe outcome for a whole-file add/delete.
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
func collectTree(root string) (map[string][]byte, error) {
	out := map[string][]byte{}
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
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
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
// The per-file change LISTS are intentionally NOT concatenated into the
// aggregator's predicate input: bindActivation binds scalar old/new only for a
// single change, so a multi-file union would leave predicates unbound and, worse,
// could let an unrelated file's scalars leak into the governed subject's
// predicate. The governed subject's own single-file ChangeSet stays the rule-eval
// input; this fold only strengthens the class and the opaque flag, both of which
// the aggregator honours BEFORE any predicate runs (reserved-class + opaque
// short-circuits) — so every security case is decided without rule evaluation.
func foldCheckout(co localCheckout) (checkoutFold, error) {
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
