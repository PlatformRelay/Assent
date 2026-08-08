package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
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
// head bytes differ (including present-on-one-side-only). Every read is a
// filesystem read under those two directories — no network, no forge API,
// satisfying the ADR-0008 §4 fence.
//
// CONTAINMENT (D-133). `head/` is the MERGE-REQUEST HEAD: content under
// judgment, authored by the contributor, and with `--checkout` the local tree is
// the SOLE authority (D-077). Git stores a symlink as a mode-120000 blob and
// `git clone` / `git worktree` / `git checkout` materialise it as a real,
// possibly dangling, POSIX symlink — so a contributor can ship one. Reads are
// therefore anchored with `os.OpenRoot` + `(*os.Root).FS()` (the same idiom as
// `builtin.OpenRepoRoot`, D-129) AND a symlinked candidate is REFUSED rather
// than followed.
//
// Both layers are needed. The root FS blocks traversal that leaves the root at
// the syscall level, but it happily follows a RELATIVE symlink that resolves
// back inside the root — verified, not assumed — so it alone cannot stop a
// head-side `topics/prod/orders.yaml -> ../decoy.yaml` from substituting the
// document under judgment. The explicit refusal covers that; the root covers
// what the refusal cannot see (a leaf swapped between Lstat and open).
//
// Refusal is an ERROR, never "absent". Silent-absent would be a fail-open: it
// would erase the path from the changed-file set and collapse into the EFE-S03
// presence signal, where nil means a whole-file lifecycle event.
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
func (d dirCheckout) FileContents(rel string) ([]byte, []byte, error) {
	base, err := readIfPresent(d.baseDir(), rel)
	if err != nil {
		return nil, nil, err
	}
	head, err := readIfPresent(d.headDir(), rel)
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
	fsys, closeRoot, err := openCheckoutSide(root)
	if err != nil {
		return nil, err
	}
	if fsys == nil {
		return out, nil // the whole side is absent
	}
	defer closeRoot()

	err = fs.WalkDir(fsys, ".", func(p string, dirent fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// A symlink is REFUSED, never followed and never skipped. Skipping would
		// drop the path from the changed-file set, which is the same fail-open the
		// truncated walk was: a head-side `.assent/**` add shipped as a symlink
		// would stop dominating.
		if dirent.Type()&fs.ModeSymlink != 0 {
			return symlinkRefusal(root, p, p)
		}
		if dirent.IsDir() {
			return nil
		}
		if !dirent.Type().IsRegular() {
			// Sockets, devices and FIFOs are not repository content. Reading a FIFO
			// would block the run forever.
			return fmt.Errorf("refusing non-regular file %q in checkout tree %s", p, root)
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		out[p] = data // fs.WalkDir paths are already slash-separated and root-relative
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// readIfPresent returns rel's bytes read from within the checkout side rooted at
// root, or nil (no error) when it does not exist.
//
// The nil-vs-empty return is a PRESENCE SIGNAL entangled with E1-S08 fold
// semantics and EFE-S03, and containment must not disturb it:
//
//	nil            = ABSENT       (one-sided lifecycle -> FileEvent)
//	non-nil len 0  = PRESENT empty (a value diff / opaque, never a delete)
//
// A symlinked candidate is therefore an ERROR, not "absent" — answering absent
// would let a substitution attempt mint a fabricated whole-file delete.
func readIfPresent(root, rel string) ([]byte, error) {
	name := path.Clean(filepath.ToSlash(rel))
	if !fs.ValidPath(name) || name == "." {
		return nil, fmt.Errorf("checkout path %q is not a valid repo-relative path", rel)
	}
	fsys, closeRoot, err := openCheckoutSide(root)
	if err != nil {
		return nil, err
	}
	if fsys == nil {
		return nil, nil // the whole side is absent
	}
	defer closeRoot()

	if err := refuseSymlinkedPath(fsys, root, name); err != nil {
		return nil, err
	}
	data, err := fs.ReadFile(fsys, name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if data == nil {
		// Defensive: PRESENT-but-empty must never read back as ABSENT.
		data = []byte{}
	}
	return data, nil
}

// openCheckoutSide opens one checkout side (base/ or head/) as a symlink-safe
// root, the same containment idiom as builtin.OpenRepoRoot (D-129). A side that
// does not exist yields (nil, nil, nil) — an all-adds/all-deletes checkout, the
// ONE absence this package tolerates. The caller must call the returned func.
//
// Containment is anchored AT base/ and head/: those two directories are
// operator-provisioned (`--checkout`), everything beneath them is contributor
// content. The side directory may itself be a symlink the operator chose;
// nothing under it may be.
func openCheckoutSide(dir string) (fs.FS, func(), error) {
	// Lstat, not Stat: a side that is itself a DANGLING symlink is a broken
	// checkout, not an empty side — OpenRoot below turns it into a hard error.
	if _, err := os.Lstat(dir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("open checkout tree %s: %w", dir, err)
	}
	return root.FS(), func() { _ = root.Close() }, nil
}

// refuseSymlinkedPath rejects name if ANY component on the way to it is a
// symlink — including the final component, and including a link that stays
// inside the root (which `(*os.Root).FS()` would follow).
//
// `os.Root`'s Lstat reports the link rather than erroring, so the refusal has to
// be explicit. A component that does not exist ends the scan: the path is
// genuinely absent, which the caller answers as the EFE-S03 nil.
func refuseSymlinkedPath(fsys fs.FS, root, name string) error {
	parts := strings.Split(name, "/")
	for i := range parts {
		prefix := strings.Join(parts[:i+1], "/")
		st, err := fs.Lstat(fsys, prefix)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil // absent, not contained-unsafe
			}
			return fmt.Errorf("stat %q in checkout tree %s: %w", prefix, root, err)
		}
		if st.Mode()&fs.ModeSymlink != 0 {
			return symlinkRefusal(root, name, prefix)
		}
	}
	return nil
}

// symlinkRefusal is the single wording for a containment refusal, so operators
// and contributors see one message whichever read tripped it.
func symlinkRefusal(root, name, at string) error {
	return fmt.Errorf(
		"refusing %q in checkout tree %s: reached through a symlink at %q — the checkout is the content under judgment, so symlinks are refused, never followed",
		name, root, at)
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
