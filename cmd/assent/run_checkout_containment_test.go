package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// --- fixture helpers -------------------------------------------------------

// requireSymlinks skips on platforms where an unprivileged process cannot
// create a symlink. Everything below models a MERGE-REQUEST HEAD that ships a
// mode-120000 blob, which `git checkout` materialises as a real POSIX symlink.
func requireSymlinks(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on windows")
	}
}

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

	t.Run("absent on head is nil", func(t *testing.T) {
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
