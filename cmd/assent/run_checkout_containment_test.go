package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// --- fixture helpers -------------------------------------------------------

// requireSymlinks lives in run_provider_symlink_test.go — one definition serves
// the whole package. Everything below models a MERGE-REQUEST HEAD that ships a
// mode-120000 blob, which `git checkout` materialises as a real POSIX symlink.

// linkIn plants a symlink at <root>/<side>/<rel> pointing at target. target may
// be absolute (off-tree), relative, or dangling — every shape a contributor can
// commit.
func linkIn(t *testing.T, root, side, rel, target string) {
	t.Helper()
	p := filepath.Join(root, side, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
	}
	if err := os.Symlink(target, p); err != nil {
		t.Fatalf("symlink %s -> %s: %v", p, target, err)
	}
}

// hostFile writes a file OUTSIDE any checkout tree — the off-tree host content a
// containment breach would pull into the decision.
func hostFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write host file %s: %v", p, err)
	}
	return p
}

// --- Defect 1: silently truncated tree -------------------------------------

// TestCheckoutDanglingSymlinkCannotHideAssentPolicyEdit is the P0 reproduction.
//
// `collectTree` tolerated `fs.ErrNotExist` for the WHOLE walk, not just for a
// missing root. A dangling symlink named to sort BEFORE `.assent` therefore
// aborted the walk on its first entry and returned a silently truncated map with
// a nil error: the `.assent/**` add vanished from the changed-file set, the
// D-042 self-vouch guard starved, and BLOCK became APPROVE.
//
// The control subtest is load-bearing: it proves the fixture would BLOCK on its
// own merits, so a non-APPROVE verdict in the second subtest is the guard
// working and not a broken fixture.
func TestCheckoutDanglingSymlinkCannotHideAssentPolicyEdit(t *testing.T) {
	requireSymlinks(t)

	newCheckout := func(t *testing.T) string {
		t.Helper()
		return writeCheckout(t, map[string][2]string{
			"topics/orders.yaml": {"partitions: 12\n", "partitions: 24\n"},
			// The smuggled policy add: absent on base, present on head.
			".assent/newpack.yaml": {"", "threshold: 1\n"},
		})
	}

	t.Run("control: no symlink -> BLOCK", func(t *testing.T) {
		f := newFakeGitLab(t)
		f.baseFile = "partitions: 12\n"
		f.headFile = "partitions: 24\n"

		var out bytes.Buffer
		code := runRun(runArgs("--arm", "--checkout", newCheckout(t)), env("tok"), fixedClock(), &out, &out, f.factory())
		if code != 0 {
			t.Fatalf("exit = %d, want 0\n%s", code, out.String())
		}
		if !strings.Contains(out.String(), `"decision":"BLOCK"`) {
			t.Fatalf("control fixture must BLOCK on the smuggled `.assent` add:\n%s", out.String())
		}
	})

	t.Run("dangling symlink sorting before .assent must never APPROVE", func(t *testing.T) {
		f := newFakeGitLab(t)
		f.baseFile = "partitions: 12\n"
		f.headFile = "partitions: 24\n"

		checkout := newCheckout(t)
		// `.aaa` < `.assent`: WalkDir reaches it first, os.ReadFile ENOENTs, and
		// the pre-fix tolerance swallowed the abort.
		linkIn(t, checkout, "head", ".aaa", filepath.Join(t.TempDir(), "does-not-exist"))

		var out bytes.Buffer
		code := runRun(runArgs("--arm", "--checkout", checkout), env("tok"), fixedClock(), &out, &out, f.factory())
		if strings.Contains(out.String(), `"decision":"APPROVE"`) {
			t.Fatalf("a dangling symlink must not erase `.assent/**` dominance (exit=%d):\n%s", code, out.String())
		}
		if f.approvals != 0 || f.merges != 0 {
			t.Fatalf("truncated changed-file set must not approve/merge: approvals=%d merges=%d", f.approvals, f.merges)
		}
		// Fail LOUD, not silently: the run must name the offending entry rather
		// than proceed on a partial tree.
		if code == 0 {
			t.Fatalf("expected a non-zero exit (refusal), got 0:\n%s", out.String())
		}
		if !strings.Contains(out.String(), ".aaa") {
			t.Fatalf("the refusal must name the offending entry `.aaa`:\n%s", out.String())
		}
	})
}

// TestCollectTreeNeverReturnsTruncatedTreeWithNilError pins Defect 1 at the
// helper: a walk that cannot complete must surface an error. Returning a partial
// map with err == nil is the fail-open — every caller then treats the missing
// paths as "not in the MR".
func TestCollectTreeNeverReturnsTruncatedTreeWithNilError(t *testing.T) {
	requireSymlinks(t)

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".assent"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".assent", "pack.yaml"), []byte("threshold: 1\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "gone"), filepath.Join(root, ".aaa")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	got, err := collectTree(root)
	if err == nil {
		t.Fatalf("collectTree returned err=nil on an unwalkable tree; got %d entries %v", len(got), keysOf(got))
	}
	if got != nil {
		t.Fatalf("an errored collectTree must not hand back a partial tree: %v", keysOf(got))
	}
}

// enoentOnOpenFS lists every file its base FS lists, but refuses to OPEN one of
// them with fs.ErrNotExist — a directory entry that vanishes between ReadDir and
// open. That is the whole Defect-1 class with no symlink involved, and it is the
// only way to produce it deterministically once reads are contained.
type enoentOnOpenFS struct {
	base fs.FS
	deny string
}

func (e enoentOnOpenFS) Open(name string) (fs.File, error) {
	if name == e.deny {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return e.base.Open(name)
}

// TestCollectFSPropagatesMidWalkNotExist is the falsifiable pin on Defect 1: the
// tolerance belongs to the ROOT ALONE. Restoring `!errors.Is(err, fs.ErrNotExist)`
// on the walk's error reds this test — a truncated map with a nil error.
func TestCollectFSPropagatesMidWalkNotExist(t *testing.T) {
	// `.aaa.yaml` sorts before `.assent/`, so the walk aborts before ever
	// reaching the policy file — the sort-order trick, reproduced hermetically.
	base := fstest.MapFS{
		".aaa.yaml":            {Data: []byte("x\n")},
		".assent/newpack.yaml": {Data: []byte("threshold: 1\n")},
		"topics/orders.yaml":   {Data: []byte("partitions: 24\n")},
	}

	got, err := collectFS(enoentOnOpenFS{base: base, deny: ".aaa.yaml"}, "head")
	if err == nil {
		t.Fatalf("a mid-walk ENOENT must propagate; got err=nil and %d entries %v", len(got), keysOf(got))
	}
	if got != nil {
		t.Fatalf("an errored walk must not hand back a partial tree: %v", keysOf(got))
	}

	// Control: the same FS without the denial collects everything, so the red
	// above is the denial and not a broken fixture.
	all, err := collectFS(base, "head")
	if err != nil {
		t.Fatalf("clean walk: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("clean walk must collect all 3 entries, got %v", keysOf(all))
	}
}

// TestCollectTreeMissingRootIsStillAnEmptyTree keeps the ONE tolerance the fix
// must preserve: a whole side that does not exist is an all-adds/all-deletes
// checkout, not an error.
func TestCollectTreeMissingRootIsStillAnEmptyTree(t *testing.T) {
	got, err := collectTree(filepath.Join(t.TempDir(), "no-such-side"))
	if err != nil {
		t.Fatalf("missing root must be an empty tree, got err=%v", err)
	}
	if len(got) != 0 {
		t.Fatalf("missing root must be EMPTY, got %v", keysOf(got))
	}
}

// TestCollectFSGuardsAreIndependentlyFalsifiable separates collectFS's two
// refusals, which otherwise MASK each other.
//
// Every symlink is also a non-regular entry — `fs.WalkDir`'s DirEntry comes from
// the Lstat-based ReadDir, on `(*os.Root).FS()` and on fstest.MapFS alike, so a
// symlink never reads back as dir or regular. The non-regular refusal is
// therefore a strict BEHAVIOURAL SUPERSET of the symlink refusal: delete the
// symlink arm and the tree is still refused, still exit 1, still zero forge
// writes. Only the WORDING changes, and nothing in the suite read the wording —
// so the pair was a conjunction gate, green unless BOTH arms were deleted.
//
// The honest fix is to pin what actually differs, and to be clear about what
// kind of pin it is:
//
//   - the symlink arm is pinned as a MESSAGE CONTRACT, not as behaviour. Its
//     wording is a documented promise (docs/usage/cli.md tells adopters the run
//     fails with an error naming the path and the side, and says the cause is a
//     symlink) and a contributor-facing one (GUIDELINES: error text that can
//     reach an MR must be understandable). "refusing non-regular file" does not
//     tell an adopter their repository contains a symlink.
//   - the non-regular arm is pinned BEHAVIOURALLY: a named pipe is not a
//     symlink, so with that arm deleted it is simply collected and the walk
//     succeeds.
//
// Mutation-proven both ways: deleting only the symlink arm reds the first
// subtest; deleting only the non-regular arm reds the second.
func TestCollectFSGuardsAreIndependentlyFalsifiable(t *testing.T) {
	t.Run("a symlink is refused AS A SYMLINK, not as a generic non-regular entry", func(t *testing.T) {
		// The target exists, so with both arms gone the link would be silently
		// FOLLOWED — the fixture models substitution, not a dangling stub.
		fsys := fstest.MapFS{
			"topics/orders.yaml":      {Data: []byte("partitions: 24\n")},
			"LICENSES/Apache-2.0.txt": {Data: []byte("Apache License\n")},
			"LICENSE":                 {Data: []byte("LICENSES/Apache-2.0.txt"), Mode: fs.ModeSymlink},
		}

		got, err := collectFS(fsys, "head")
		if err == nil {
			t.Fatalf("a symlink in the tree must be refused; got %d entries %v", len(got), keysOf(got))
		}
		if got != nil {
			t.Fatalf("a refused walk must not hand back a partial tree: %v", keysOf(got))
		}
		if !strings.Contains(err.Error(), "reached through a symlink") {
			t.Fatalf("the refusal must say the entry is a SYMLINK — an adopter reads this to learn why\ntheir repository cannot be judged with --checkout; got: %v", err)
		}
		if !strings.Contains(err.Error(), `"LICENSE"`) {
			t.Fatalf("the refusal must name the offending path: %v", err)
		}
		if strings.Contains(err.Error(), "non-regular") {
			t.Fatalf("the non-regular guard answered for the symlink guard — the two are masking each other again: %v", err)
		}
	})

	t.Run("a non-regular, non-symlink entry is refused", func(t *testing.T) {
		// A named pipe carries no symlink bit, so ONLY the non-regular arm can
		// refuse it. Reading it on a real filesystem would block the run forever.
		fsys := fstest.MapFS{
			"topics/orders.yaml": {Data: []byte("partitions: 24\n")},
			"run/queue.pipe":     {Data: []byte("not repository content\n"), Mode: fs.ModeNamedPipe},
		}

		got, err := collectFS(fsys, "head")
		if err == nil {
			t.Fatalf("a non-regular entry must be refused, not collected; got %d entries %v", len(got), keysOf(got))
		}
		if got != nil {
			t.Fatalf("a refused walk must not hand back a partial tree: %v", keysOf(got))
		}
		if !strings.Contains(err.Error(), "non-regular") {
			t.Fatalf("the refusal must name the reason: %v", err)
		}
		if !strings.Contains(err.Error(), `"run/queue.pipe"`) {
			t.Fatalf("the refusal must name the offending path: %v", err)
		}
	})

	t.Run("control: the same tree without either entry collects cleanly", func(t *testing.T) {
		fsys := fstest.MapFS{"topics/orders.yaml": {Data: []byte("partitions: 24\n")}}
		got, err := collectFS(fsys, "head")
		if err != nil {
			t.Fatalf("clean walk: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("clean walk must collect the regular file, got %v", keysOf(got))
		}
	})
}

// TestCheckoutSideSymlinkIsLstatted pins openCheckoutSide's DELIBERATE choice of
// `os.Lstat` over `os.Stat` (checkout.go), which had no test.
//
// `os.Stat` follows the link, so a side that is a DANGLING symlink stats as
// ErrNotExist — indistinguishable from "this side does not exist", which is the
// one absence this package tolerates (an all-adds/all-deletes checkout). A broken
// checkout would then read as an EMPTY head, and every file in base/ would be
// enumerated as a whole-file DELETE: a silent mass delete offered for judgment.
// Measured with the mutation in place: `files=[.assent/p.yaml] err=<nil>`.
//
// `os.Lstat` sees the link itself, so the side is "present" and `os.OpenRoot`
// — which does follow it — turns the broken target into a hard error.
//
// The legitimate-polarity subtest is required, not decorative: ADR-0008
// Amendment 2 promises the side directories may THEMSELVES be symlinks (they are
// operator-provisioned; only what is beneath them is contributor content).
// Without it the first subtest would pass just as well for an implementation
// that rejected every symlinked side.
func TestCheckoutSideSymlinkIsLstatted(t *testing.T) {
	requireSymlinks(t)

	t.Run("a DANGLING side symlink is a broken checkout, not an empty side", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "base", ".assent"), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, "base", ".assent", "p.yaml"), []byte("threshold: 1\n"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := os.Symlink(filepath.Join(t.TempDir(), "never-created"), filepath.Join(root, "head")); err != nil {
			t.Fatalf("symlink: %v", err)
		}

		files, err := dirCheckout{root: root}.ChangedFiles()
		if err == nil {
			t.Fatalf("a dangling head/ must be a hard error, not an empty side minting deletes; got files=%v", files)
		}
		if files != nil {
			t.Fatalf("an errored enumeration must not hand back a partial set: %v", files)
		}
	})

	t.Run("legitimate polarity: a side symlinked to a REAL directory still reads", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "base", "topics"), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, "base", "topics", "orders.yaml"), []byte("partitions: 12\n"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		realHead := filepath.Join(t.TempDir(), "materialised-head")
		if err := os.MkdirAll(filepath.Join(realHead, "topics"), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(realHead, "topics", "orders.yaml"), []byte("partitions: 24\n"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := os.Symlink(realHead, filepath.Join(root, "head")); err != nil {
			t.Fatalf("symlink: %v", err)
		}

		co := dirCheckout{root: root}
		files, err := co.ChangedFiles()
		if err != nil {
			t.Fatalf("an operator-provisioned symlinked SIDE must still be read: %v", err)
		}
		if len(files) != 1 || files[0] != "topics/orders.yaml" {
			t.Fatalf("changed set = %v, want [topics/orders.yaml]", files)
		}
		base, head, err := co.FileContents("topics/orders.yaml")
		if err != nil {
			t.Fatalf("FileContents through a symlinked side: %v", err)
		}
		if string(base) != "partitions: 12\n" || string(head) != "partitions: 24\n" {
			t.Fatalf("read base=%q head=%q through the symlinked side", base, head)
		}
	})
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// --- Defect 2: the governed document can be substituted --------------------

// TestCheckoutRefusesSymlinkedGovernedFile is the P1 reproduction. The head tree
// is the merge-request head — contributor-authored content under judgment. A
// head-side file symlink at a legitimate in-repo path made an OFF-TREE HOST FILE
// the document under judgment: the repo's own head says `partitions: 12`
// (unchanged), the host file says 20, and the run APPROVEd on the host file.
//
// The legitimate-polarity subtest is the pair that stops this passing for the
// wrong reason: the identical path with a REAL file holding the identical bytes
// must still be read and still APPROVE.
func TestCheckoutRefusesSymlinkedGovernedFile(t *testing.T) {
	requireSymlinks(t)

	t.Run("head-side file symlink is refused", func(t *testing.T) {
		f := newFakeGitLab(t)
		f.baseFile = "partitions: 12\n"
		f.headFile = "partitions: 12\n" // the repo's own head: UNCHANGED

		host := hostFile(t, "host-owned.yaml", "partitions: 20\n")
		checkout := writeCheckout(t, map[string][2]string{
			"topics/orders.yaml": {"partitions: 12\n", ""}, // base real, head planted below
		})
		linkIn(t, checkout, "head", "topics/orders.yaml", host)

		var out bytes.Buffer
		code := runRun(runArgs("--arm", "--checkout", checkout), env("tok"), fixedClock(), &out, &out, f.factory())
		if strings.Contains(out.String(), `"decision":"APPROVE"`) {
			t.Fatalf("an off-tree host file must never become the governed document (exit=%d):\n%s", code, out.String())
		}
		if f.approvals != 0 || f.merges != 0 {
			t.Fatalf("substituted governed document must not approve/merge: approvals=%d merges=%d", f.approvals, f.merges)
		}
		if code == 0 {
			t.Fatalf("expected a non-zero exit (refusal), got 0:\n%s", out.String())
		}
		if !strings.Contains(out.String(), "symlink") || !strings.Contains(out.String(), "topics/orders.yaml") {
			t.Fatalf("the refusal must name the symlink and the path:\n%s", out.String())
		}
	})

	t.Run("legitimate polarity: a real file at the same path still decides", func(t *testing.T) {
		f := newFakeGitLab(t)
		f.baseFile = "partitions: 12\n"
		f.headFile = "partitions: 20\n"

		checkout := writeCheckout(t, map[string][2]string{
			"topics/orders.yaml": {"partitions: 12\n", "partitions: 20\n"},
		})

		var out bytes.Buffer
		code := runRun(runArgs("--arm", "--checkout", checkout), env("tok"), fixedClock(), &out, &out, f.factory())
		if code != 0 {
			t.Fatalf("exit = %d, want 0\n%s", code, out.String())
		}
		if !strings.Contains(out.String(), `"decision":"APPROVE"`) {
			t.Fatalf("a REAL in-tree file must still be read and still APPROVE:\n%s", out.String())
		}
		if f.approvals != 1 || f.merges != 1 {
			t.Fatalf("armed APPROVE must approve+merge: approvals=%d merges=%d", f.approvals, f.merges)
		}
	})
}

// TestCheckoutSymlinkedGovernedFileCannotExfiltrate — the breach is not only a
// wrong verdict: off-tree bytes were rendered VERBATIM into the thread posted on
// the merge request, a forge-facing exfiltration surface. No sentinel byte from
// the host file may reach stdout or the posted thread.
func TestCheckoutSymlinkedGovernedFileCannotExfiltrate(t *testing.T) {
	requireSymlinks(t)

	const sentinel = "31337"

	f := newFakeGitLab(t)
	// A partitions SHRINK fails the frozen `new >= old` assert -> challenge ->
	// REVIEW, which is the decision that POSTS a thread rendering the matched
	// change. That render is the forge-facing surface the host bytes reached.
	f.baseFile = "partitions: 65536\n"
	f.headFile = "partitions: 65536\n" // the repo's own head: UNCHANGED

	host := hostFile(t, "host-owned.yaml", "partitions: "+sentinel+"\n")
	checkout := writeCheckout(t, map[string][2]string{
		"topics/orders.yaml": {"partitions: 65536\n", ""},
	})
	linkIn(t, checkout, "head", "topics/orders.yaml", host)

	var out bytes.Buffer
	_ = runRun(runArgs("--checkout", checkout), env("tok"), fixedClock(), &out, &out, f.factory())

	if strings.Contains(out.String(), sentinel) {
		t.Fatalf("off-tree host bytes leaked to stdout:\n%s", out.String())
	}
	if strings.Contains(f.lastThreadBody, sentinel) {
		t.Fatalf("off-tree host bytes leaked into the posted thread:\n%s", f.lastThreadBody)
	}
	if f.discussionsPosted != 0 {
		t.Fatalf("a refused checkout read must post nothing to the forge: %d threads", f.discussionsPosted)
	}
}

// TestCheckoutRefusesDirectorySymlinkOnGovernedPath — the escape does not need
// the FINAL component: a directory symlink anywhere on the path relocates the
// whole subtree off-tree.
func TestCheckoutRefusesDirectorySymlinkOnGovernedPath(t *testing.T) {
	requireSymlinks(t)

	f := newFakeGitLab(t)
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 12\n"

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "orders.yaml"), []byte("partitions: 20\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	checkout := writeCheckout(t, map[string][2]string{
		"topics/orders.yaml": {"partitions: 12\n", ""},
	})
	// head/topics is a symlink to an off-tree directory.
	if err := os.MkdirAll(filepath.Join(checkout, "head"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(checkout, "head", "topics")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	var out bytes.Buffer
	code := runRun(runArgs("--arm", "--checkout", checkout), env("tok"), fixedClock(), &out, &out, f.factory())
	if strings.Contains(out.String(), `"decision":"APPROVE"`) {
		t.Fatalf("a directory symlink on the path must not relocate the governed document (exit=%d):\n%s", code, out.String())
	}
	if f.approvals != 0 || f.merges != 0 {
		t.Fatalf("must not approve/merge: approvals=%d merges=%d", f.approvals, f.merges)
	}
	if code == 0 {
		t.Fatalf("expected a non-zero exit (refusal), got 0:\n%s", out.String())
	}
	// Pre-fix this path failed CLOSED by accident — `os.ReadFile` on the
	// symlinked directory returned EISDIR ("is a directory"). That is luck, not
	// containment: the governed subject had ALREADY been substituted from
	// off-tree at run.go's FileContents override. Pin the explicit refusal.
	if !strings.Contains(out.String(), "symlink") {
		t.Fatalf("the refusal must be an explicit containment refusal, not an incidental read error:\n%s", out.String())
	}
}

// --- the presence signal the containment fix must NOT break ----------------

// TestFileContentsKeepsNilAbsentDistinctFromEmpty pins the EFE-S03 / E1-S08
// presence signal through the stable `dirCheckout.FileContents` contract:
//
//	nil            = ABSENT  (one-sided lifecycle -> FileEvent)
//	non-nil len 0  = PRESENT but empty (stays a value diff / opaque)
//
// A containment fix that collapsed absent and empty — or that answered a refused
// symlink with "absent" — would be a fail-open dressed as a fix, so the symlink
// arm asserts an ERROR and explicitly asserts it is not the absent answer.
func TestFileContentsKeepsNilAbsentDistinctFromEmpty(t *testing.T) {
	requireSymlinks(t)

	present := "enabled: true\n"
	empty := ""

	t.Run("absent on head is nil (whole side missing)", func(t *testing.T) {
		root := writeCheckoutPresence(t, map[string][2]*string{
			"topics/orders.yaml": {&present, nil},
		})
		base, head, err := dirCheckout{root: root}.FileContents("topics/orders.yaml")
		if err != nil {
			t.Fatalf("FileContents: %v", err)
		}
		if base == nil {
			t.Fatalf("base must be PRESENT (non-nil)")
		}
		if head != nil {
			t.Fatalf("absent head must be nil, got non-nil len=%d", len(head))
		}
	})

	// The production shape: a real checkout ALWAYS has base/ and head/, so the
	// absence is of the FILE, not of the side. A fixture that omits the whole
	// side short-circuits before the per-file read and leaves that branch
	// untested — which is how a collapse of absent into empty would slip through.
	t.Run("absent on head is nil (side exists, file does not)", func(t *testing.T) {
		sibling := "keep: true\n"
		root := writeCheckoutPresence(t, map[string][2]*string{
			"topics/orders.yaml": {&present, nil},
			"topics/keep.yaml":   {&sibling, &sibling}, // makes head/ exist
		})
		base, head, err := dirCheckout{root: root}.FileContents("topics/orders.yaml")
		if err != nil {
			t.Fatalf("FileContents: %v", err)
		}
		if base == nil {
			t.Fatalf("base must be PRESENT (non-nil)")
		}
		if head != nil {
			t.Fatalf("a file absent from an EXISTING side must still be nil, got len=%d", len(head))
		}
	})

	t.Run("empty-but-present on head is non-nil and zero-length", func(t *testing.T) {
		root := writeCheckoutPresence(t, map[string][2]*string{
			"topics/orders.yaml": {&present, &empty},
		})
		_, head, err := dirCheckout{root: root}.FileContents("topics/orders.yaml")
		if err != nil {
			t.Fatalf("FileContents: %v", err)
		}
		if head == nil {
			t.Fatalf("an empty-but-present file must be non-nil (absent and empty must not collapse)")
		}
		if len(head) != 0 {
			t.Fatalf("empty file must be zero-length, got %q", head)
		}
	})

	// The containment fix must change no LEGITIMATE input. A rooted subject
	// (`--subject file:/topics/orders.yaml`) names the same repo-relative path and
	// decided fine before containment landed, because filepath.Join cleaned the
	// slash away; `fs.ValidPath` rejects it. Measured on origin/main's reader:
	// exit=0 APPROVE. `anchorFromSubject` normalises the same way.
	t.Run("a rooted subject names the same file", func(t *testing.T) {
		root := writeCheckoutPresence(t, map[string][2]*string{
			"topics/orders.yaml": {&present, &present},
		})
		plain, _, err := dirCheckout{root: root}.FileContents("topics/orders.yaml")
		if err != nil {
			t.Fatalf("plain: %v", err)
		}
		rooted, _, err := dirCheckout{root: root}.FileContents("/topics/orders.yaml")
		if err != nil {
			t.Fatalf("a rooted subject must still resolve: %v", err)
		}
		if !bytes.Equal(plain, rooted) {
			t.Fatalf("rooted subject read %q, plain read %q", rooted, plain)
		}
	})

	t.Run("a ..-traversing subject is still refused", func(t *testing.T) {
		root := writeCheckoutPresence(t, map[string][2]*string{
			"topics/orders.yaml": {&present, &present},
		})
		if _, _, err := (dirCheckout{root: root}).FileContents("../../../etc/hosts"); err == nil {
			t.Fatalf("a ..-traversing subject must be refused")
		}
	})

	t.Run("a symlinked candidate is an ERROR, never silent-absent", func(t *testing.T) {
		root := writeCheckoutPresence(t, map[string][2]*string{
			"topics/orders.yaml": {&present, nil},
		})
		linkIn(t, root, "head", "topics/orders.yaml", hostFile(t, "host.yaml", "partitions: 20\n"))

		_, head, err := dirCheckout{root: root}.FileContents("topics/orders.yaml")
		if err == nil {
			t.Fatalf("a symlinked candidate must be refused; got head=%q", head)
		}
		if head != nil {
			t.Fatalf("a refusal must not hand back content: %q", head)
		}
		if !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("the refusal must say why: %v", err)
		}
	})
}

// TestLiveCheckoutMintsFileEventWithBothSidesPresent is the ENTRY-POINT half of
// the `nil = absent` pin, in the production shape.
//
// The existing EFE-S03 suite fixtures a one-sided governed file by omitting the
// whole side directory, so it never exercises the per-file absence a real
// checkout produces. A sibling identical on both sides makes base/ and head/
// both exist (and, being unchanged, stays out of the changed-file set, so the
// fold is untouched). If absence stopped reading back as nil, the whole-file ADD
// would no longer be minted and this would fall to fail-safe REVIEW.
func TestLiveCheckoutMintsFileEventWithBothSidesPresent(t *testing.T) {
	f := newFakeGitLab(t)
	f.mergePolicy = mergePolicyFileEvents
	f.rulesetBinding = rulesetBindingReviewedDeletion
	f.baseFile = "enabled: true\n"
	f.headFile = "enabled: true\n"

	present := "enabled: true\n"
	sibling := "keep: true\n"
	checkout := writeCheckoutPresence(t, map[string][2]*string{
		"topics/orders.yaml": {nil, &present},      // ADD: absent base, present head
		"topics/keep.yaml":   {&sibling, &sibling}, // makes BOTH sides exist
	})

	var out bytes.Buffer
	code := runRun(runArgs("--checkout", checkout), env("tok"), fixedClock(), &out, &out, f.factory())
	if code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), `"decision":"APPROVE"`) {
		t.Fatalf("a per-file absence in an EXISTING side must still mint the whole-file ADD:\n%s", out.String())
	}
}

// TestCheckoutSideIsASymlinkSafeRoot pins the SECOND containment layer, which
// the explicit refusal would otherwise hide: the refusal runs before the read,
// so nothing in the suite notices if the FS handed out stops being a root.
//
// `os.DirFS` is documented as not a security boundary. The residual it leaves is
// a TOCTOU: a leaf that is a real file when `Lstat` inspects it and an escaping
// symlink by the time it is opened. `(*os.Root).FS()` refuses that at the
// syscall level; `os.DirFS` reads the host file.
func TestCheckoutSideIsASymlinkSafeRoot(t *testing.T) {
	requireSymlinks(t)

	const offTreeMarker = "OFF-TREE-HOST-BYTES"
	host := hostFile(t, "host.yaml", offTreeMarker+"\n")

	side := filepath.Join(t.TempDir(), "head")
	leaf := filepath.Join(side, "topics", "orders.yaml")
	if err := os.MkdirAll(filepath.Dir(leaf), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(leaf, []byte("in-tree\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	fsys, closeSide, err := openCheckoutSide(side)
	if err != nil {
		t.Fatalf("openCheckoutSide: %v", err)
	}
	if fsys == nil {
		t.Fatalf("side exists, expected an FS")
	}
	defer closeSide()

	// Swap the leaf AFTER the side is open — past every up-front check.
	if err := os.Remove(leaf); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := os.Symlink(host, leaf); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	got, err := fs.ReadFile(fsys, "topics/orders.yaml")
	if strings.Contains(string(got), offTreeMarker) {
		t.Fatalf("off-tree host bytes were read through the checkout side: %q", got)
	}
	if err == nil {
		t.Fatalf("a post-open escape must be refused at the syscall level; read %q", got)
	}
}

// TestCheckoutEscapeAttempts is the adversarial battery: every symlink shape a
// contributor can commit must be refused on the governed path. `(*os.Root).FS()`
// alone is NOT sufficient — it happily follows a RELATIVE symlink that resolves
// back inside the root, so the in-root arms below are the ones that fail without
// an explicit refusal.
func TestCheckoutEscapeAttempts(t *testing.T) {
	requireSymlinks(t)

	present := "enabled: true\n"

	for _, tc := range []struct {
		name string
		// plant installs the attack under <root>/head; it returns nothing.
		plant func(t *testing.T, root, outside string)
	}{
		{
			name: "absolute off-tree file symlink",
			plant: func(t *testing.T, root, outside string) {
				linkIn(t, root, "head", "topics/orders.yaml", filepath.Join(outside, "orders.yaml"))
			},
		},
		{
			name: "relative ..-chain escaping the root",
			plant: func(t *testing.T, root, outside string) {
				rel, err := filepath.Rel(filepath.Join(root, "head", "topics"), filepath.Join(outside, "orders.yaml"))
				if err != nil {
					t.Fatalf("rel: %v", err)
				}
				linkIn(t, root, "head", "topics/orders.yaml", rel)
			},
		},
		{
			name: "dangling symlink at the governed path",
			plant: func(t *testing.T, root, outside string) {
				linkIn(t, root, "head", "topics/orders.yaml", filepath.Join(outside, "never-created.yaml"))
			},
		},
		{
			name: "in-root relative symlink (os.Root follows this one)",
			plant: func(t *testing.T, root, _ string) {
				if err := os.MkdirAll(filepath.Join(root, "head"), 0o750); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(root, "head", "decoy.yaml"), []byte("partitions: 99\n"), 0o600); err != nil {
					t.Fatalf("write: %v", err)
				}
				linkIn(t, root, "head", "topics/orders.yaml", "../decoy.yaml")
			},
		},
		{
			name: "symlink to a symlink",
			plant: func(t *testing.T, root, outside string) {
				linkIn(t, root, "head", "hop.yaml", filepath.Join(outside, "orders.yaml"))
				linkIn(t, root, "head", "topics/orders.yaml", "../hop.yaml")
			},
		},
		{
			name: "escaping directory symlink mid-path",
			plant: func(t *testing.T, root, outside string) {
				if err := os.MkdirAll(filepath.Join(root, "head"), 0o750); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.Symlink(outside, filepath.Join(root, "head", "topics")); err != nil {
					t.Fatalf("symlink: %v", err)
				}
			},
		},
		{
			name: "in-root directory symlink mid-path",
			plant: func(t *testing.T, root, _ string) {
				if err := os.MkdirAll(filepath.Join(root, "head", "decoy"), 0o750); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(root, "head", "decoy", "orders.yaml"), []byte("partitions: 99\n"), 0o600); err != nil {
					t.Fatalf("write: %v", err)
				}
				if err := os.Symlink("decoy", filepath.Join(root, "head", "topics")); err != nil {
					t.Fatalf("symlink: %v", err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outside := t.TempDir()
			if err := os.WriteFile(filepath.Join(outside, "orders.yaml"), []byte("partitions: 20\n"), 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}
			root := writeCheckoutPresence(t, map[string][2]*string{
				"topics/orders.yaml": {&present, nil},
			})
			tc.plant(t, root, outside)

			_, head, err := dirCheckout{root: root}.FileContents("topics/orders.yaml")
			if err == nil {
				t.Fatalf("escape not refused; FileContents returned head=%q", head)
			}
			if head != nil {
				t.Fatalf("a refusal must not hand back content: %q", head)
			}

			// The same tree must also refuse enumeration rather than silently
			// drop the path from the changed-file set.
			co := dirCheckout{root: root}
			if _, err := co.ChangedFiles(); err == nil {
				t.Fatalf("ChangedFiles must refuse a tree containing a symlink, not skip it")
			}
		})
	}
}
