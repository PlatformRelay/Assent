package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
)

// AUD-S08 / D-122: the DecisionRecord is emitted BEFORE forge.Reconcile, and the
// `--emit` write is atomic (same-directory `<path>.tmp` + os.Rename). The new
// invariant: NO forge write without a schema-valid, durably-emitted record — an
// emit failure aborts the run with ZERO forge writes.
//
// Every test here drives runRun end to end against the real *gitlab.Client
// pointed at the fakeGitLab httptest cassette (the production entry point), not
// emitRecord directly: the ordering only exists on the path production takes.

// errReconcileHardFail is a forge transport-style failure — deliberately NOT one
// of forge.ErrArmingRefused / ErrSHAMoved / ErrIncompletePreconditions, so
// isFailClosed does not swallow it and the run exits non-zero (REQ-AUD-S08-02's
// durability polarity needs the HARD-failure branch, not the advisory one).
var errReconcileHardFail = errors.New("forge exploded mid-reconcile")

// hardFailingForge wraps the real client and makes the MUTATING reconcile calls
// fail. Reads (ListBotThreads, CurrentHeads, Snapshot, Resolve, FileAtRef, GetMR)
// are inherited untouched, so orchestrate reaches Reconcile normally and only the
// write blows up — the "record exists, forge actions may be partial" scenario.
type hardFailingForge struct {
	forgePort
}

func (h hardFailingForge) CreateThread(string, string, forge.Marker, string) (forge.Thread, error) {
	return forge.Thread{}, errReconcileHardFail
}

func (h hardFailingForge) UpsertComment(string, string, forge.Marker, string) (forge.Note, error) {
	return forge.Note{}, errReconcileHardFail
}

func (h hardFailingForge) Approve(string, string) (string, error) {
	return "", errReconcileHardFail
}

func (h hardFailingForge) MergeCAS(string, string, forge.DesiredMerge) (string, error) {
	return "", errReconcileHardFail
}

// hardFailingFactory yields the fake's real client wrapped so every reconcile
// WRITE hard-fails.
func hardFailingFactory(f *fakeGitLab) func(string, string, string) forgePort {
	inner := f.factory()
	return func(endpoint, token, botAuthor string) forgePort {
		return hardFailingForge{forgePort: inner(endpoint, token, botAuthor)}
	}
}

// approving configures the fake for the APPROVE + armed-merge polarity (a
// partitions GROW proves `non-destructive`; the default premium project JSON
// makes the forge probe arm-eligible).
func approving(f *fakeGitLab) {
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 24\n"
}

// reviewing configures the fake for the REVIEW polarity (a partitions SHRINK
// fails the obligation → challenge → REVIEW → exactly one thread).
func reviewing(f *fakeGitLab) {
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 6\n"
}

// assertNoForgeWrites is the REQ-AUD-S08-01 teeth: not one mutating call.
func assertNoForgeWrites(t *testing.T, f *fakeGitLab) {
	t.Helper()
	if f.discussionsPosted != 0 || f.notesPosted != 0 || f.notesUpdated != 0 || f.approvals != 0 || f.merges != 0 {
		t.Errorf("emit failure must produce ZERO forge writes, got threads=%d notes=%d noteEdits=%d approvals=%d merges=%d",
			f.discussionsPosted, f.notesPosted, f.notesUpdated, f.approvals, f.merges)
	}
}

// TestEmitFailureBlocksForgeWrites — REQ-AUD-S08-01. An emit that cannot be
// durably written aborts the run BEFORE reconcile: non-zero exit and zero forge
// writes, at the APPROVE+armed polarity where it bites (the merge must not
// outrun its own record). Both failure shapes are covered: the tmp write itself
// failing, and the rename failing after a successful tmp write.
func TestEmitFailureBlocksForgeWrites(t *testing.T) {
	// Positive control: the SAME fixture, with a writable --emit, really does
	// approve+merge. Without this the zero-write assertions below could pass
	// because the scenario never wrote anything in the first place.
	t.Run("control_writable_emit_merges", func(t *testing.T) {
		f := newFakeGitLab(t)
		approving(f)
		emitPath := filepath.Join(t.TempDir(), "record.json")

		var out bytes.Buffer
		code := runRun(runArgs("--arm", "--emit", emitPath), env("tok"), fixedClock(), &out, &out, f.factory())
		if code != 0 {
			t.Fatalf("exit = %d, want 0\n%s", code, out.String())
		}
		if f.approvals != 1 || f.merges != 1 {
			t.Fatalf("control fixture must approve+merge: approvals=%d merges=%d\n%s", f.approvals, f.merges, out.String())
		}
		if _, err := os.Stat(emitPath); err != nil {
			t.Fatalf("control run must leave the record on disk: %v", err)
		}
	})

	// The emit directory does not exist → the tmp write fails outright.
	t.Run("unwritable_path_no_writes", func(t *testing.T) {
		f := newFakeGitLab(t)
		approving(f)
		emitPath := filepath.Join(t.TempDir(), "no-such-dir", "record.json")

		var out bytes.Buffer
		code := runRun(runArgs("--arm", "--emit", emitPath), env("tok"), fixedClock(), &out, &out, f.factory())
		if code == 0 {
			t.Fatalf("an unwritable --emit must fail the run (non-zero), got 0\n%s", out.String())
		}
		assertNoForgeWrites(t, f)
		if !strings.Contains(out.String(), "emit decision record") {
			t.Errorf("expected an emit-decision-record error:\n%s", out.String())
		}
	})

	// --emit points at an EXISTING DIRECTORY: the same-directory tmp write
	// succeeds and the rename then fails. Zero forge writes, and the tmp must
	// not survive.
	t.Run("rename_failure_no_writes_and_no_tmp", func(t *testing.T) {
		f := newFakeGitLab(t)
		approving(f)
		dir := t.TempDir()
		emitPath := filepath.Join(dir, "record.json")
		if err := os.Mkdir(emitPath, 0o750); err != nil {
			t.Fatalf("seed a directory at the --emit path: %v", err)
		}

		var out bytes.Buffer
		code := runRun(runArgs("--arm", "--emit", emitPath), env("tok"), fixedClock(), &out, &out, f.factory())
		if code == 0 {
			t.Fatalf("a --emit path that cannot be renamed into place must fail the run, got 0\n%s", out.String())
		}
		assertNoForgeWrites(t, f)
		if _, err := os.Stat(emitTempPath(emitPath)); !os.IsNotExist(err) {
			t.Errorf("the temp file %q must not survive a failed emit (stat err = %v)", emitTempPath(emitPath), err)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read emit dir: %v", err)
		}
		if len(entries) != 1 || entries[0].Name() != "record.json" {
			t.Errorf("emit dir must hold only the seeded target, got %v", entries)
		}
	})
}

// TestEmitTempIsSameDirectory pins the STRUCTURAL half of D-122's atomic write:
// the temp file is `<path>.tmp` in the SAME directory as the target, so the
// os.Rename is a same-filesystem atomic replace rather than a cross-device copy.
// This is a direct-call assertion by necessity — a successful run leaves no trace
// of the temp path, so the property is not observable end to end.
func TestEmitTempIsSameDirectory(t *testing.T) {
	for _, target := range []string{
		"/tmp/records/record.json",
		"record.json",
		"./nested/dir/decision.json",
	} {
		tmp := emitTempPath(target)
		if got, want := filepath.Dir(tmp), filepath.Dir(target); got != want {
			t.Errorf("temp dir for %q = %q, want the target's own directory %q", target, got, want)
		}
		if tmp != target+".tmp" {
			t.Errorf("temp path for %q = %q, want %q (D-122 names it <path>.tmp)", target, tmp, target+".tmp")
		}
	}
}

// TestEmitBeforeReconcileByteStable — REQ-AUD-S08-02. The reorder is PURE: the
// emitted bytes, the stdout ordering, the marker digests, the summary and the
// exit codes are all unchanged; and the record survives a reconcile that
// hard-fails after it.
func TestEmitBeforeReconcileByteStable(t *testing.T) {
	// The `--emit` file and the stdout emission carry byte-identical records,
	// stdout keeps its record-then-summary ordering, and the summary still
	// reports the reconcile receipt.
	t.Run("bytes_and_ordering_unchanged", func(t *testing.T) {
		stdoutFake := newFakeGitLab(t)
		approving(stdoutFake)
		var stdoutOut bytes.Buffer
		if code := runRun(runArgs("--arm"), env("tok"), fixedClock(), &stdoutOut, &stdoutOut, stdoutFake.factory()); code != 0 {
			t.Fatalf("stdout-mode exit = %d, want 0\n%s", code, stdoutOut.String())
		}
		lines := strings.Split(strings.TrimRight(stdoutOut.String(), "\n"), "\n")
		if len(lines) != 2 {
			t.Fatalf("stdout must be exactly the record line then the summary line, got %d line(s):\n%s", len(lines), stdoutOut.String())
		}
		if !strings.Contains(lines[0], `"kind":"DecisionRecord"`) {
			t.Fatalf("first stdout line must be the DecisionRecord:\n%s", lines[0])
		}
		if !strings.HasPrefix(lines[1], "decision=APPROVE") || !strings.Contains(lines[1], "forge operation(s) written") {
			t.Errorf("summary line must still report the receipt, got %q", lines[1])
		}

		fileFake := newFakeGitLab(t)
		approving(fileFake)
		emitPath := filepath.Join(t.TempDir(), "record.json")
		var fileOut bytes.Buffer
		if code := runRun(runArgs("--arm", "--emit", emitPath), env("tok"), fixedClock(), &fileOut, &fileOut, fileFake.factory()); code != 0 {
			t.Fatalf("file-mode exit = %d, want 0\n%s", code, fileOut.String())
		}
		emitted, err := os.ReadFile(emitPath) // #nosec G304 -- test-controlled temp path.
		if err != nil {
			t.Fatalf("read emitted record: %v", err)
		}
		if string(emitted) != lines[0] {
			t.Errorf("emitted file bytes differ from the stdout record bytes\nfile:   %s\nstdout: %s", emitted, lines[0])
		}
		if fileFake.merges != 1 || stdoutFake.merges != 1 {
			t.Errorf("both emit modes must still merge: file=%d stdout=%d", fileFake.merges, stdoutFake.merges)
		}
		if strings.Contains(fileOut.String(), `"kind":"DecisionRecord"`) {
			t.Errorf("the record must stay off stdout when --emit is a file:\n%s", fileOut.String())
		}
	})

	// D-122's "marker digests unaffected": the thread marker's `decision` digest
	// is sha256 of the EXACT emitted bytes, even though the marker is now built
	// after the emit call.
	t.Run("marker_decision_digest_matches_emitted_bytes", func(t *testing.T) {
		f := newFakeGitLab(t)
		reviewing(f)
		emitPath := filepath.Join(t.TempDir(), "record.json")

		var out bytes.Buffer
		if code := runRun(runArgs("--emit", emitPath), env("tok"), fixedClock(), &out, &out, f.factory()); code != 0 {
			t.Fatalf("exit = %d, want 0\n%s", code, out.String())
		}
		if f.discussionsPosted != 1 {
			t.Fatalf("REVIEW must post exactly one thread, got %d", f.discussionsPosted)
		}
		emitted, err := os.ReadFile(emitPath) // #nosec G304 -- test-controlled temp path.
		if err != nil {
			t.Fatalf("read emitted record: %v", err)
		}
		want := `"decision":"` + sha256Prefix + sha256Hex(emitted) + `"`
		if !strings.Contains(f.lastThreadBody, want) {
			t.Errorf("marker decision digest is not sha256 of the emitted bytes; want %s in:\n%s", want, f.lastThreadBody)
		}
	})

	// Durability polarity: emit succeeds, reconcile then HARD-fails. The record
	// is on disk and schema-valid — it records the decision that was reached and
	// what was intended, never a claim that the forge actions completed (that
	// lives in the summary line, which reports the error).
	t.Run("record_survives_reconcile_hard_failure", func(t *testing.T) {
		f := newFakeGitLab(t)
		approving(f)
		emitPath := filepath.Join(t.TempDir(), "record.json")

		var out bytes.Buffer
		code := runRun(runArgs("--arm", "--emit", emitPath), env("tok"), fixedClock(), &out, &out, hardFailingFactory(f))
		if code == 0 {
			t.Fatalf("a hard reconcile failure must exit non-zero, got 0\n%s", out.String())
		}
		emitted, err := os.ReadFile(emitPath) // #nosec G304 -- test-controlled temp path.
		if err != nil {
			t.Fatalf("the record must survive a failed reconcile: %v", err)
		}
		if err := validateRecord(emitted); err != nil {
			t.Errorf("the surviving record must be schema-valid: %v", err)
		}
		if !strings.Contains(out.String(), "reconcile") {
			t.Errorf("expected a reconcile error on stderr:\n%s", out.String())
		}
		if _, err := os.Stat(emitTempPath(emitPath)); !os.IsNotExist(err) {
			t.Errorf("no temp file may survive a successful emit (stat err = %v)", err)
		}
	})

	// Existing contract preserved: a fail-CLOSED reconcile refusal (forge probe
	// refuses arming) stays a clean exit 0, with the record already emitted.
	t.Run("fail_closed_refusal_still_exit_zero", func(t *testing.T) {
		f := newFakeGitLab(t)
		f.projectJSON = fakeForgeIneligibleProjectJSON
		approving(f)
		emitPath := filepath.Join(t.TempDir(), "record.json")

		var out bytes.Buffer
		code := runRun(runArgs("--arm", "--emit", emitPath), env("tok"), fixedClock(), &out, &out, f.factory())
		if code != 0 {
			t.Fatalf("an arming refusal must stay a clean exit 0, got %d\n%s", code, out.String())
		}
		if f.approvals != 0 || f.merges != 0 {
			t.Errorf("arming refusal must not approve/merge: approvals=%d merges=%d", f.approvals, f.merges)
		}
		emitted, err := os.ReadFile(emitPath) // #nosec G304 -- test-controlled temp path.
		if err != nil {
			t.Fatalf("the record must be emitted despite the refusal: %v", err)
		}
		if err := validateRecord(emitted); err != nil {
			t.Errorf("the emitted record must be schema-valid: %v", err)
		}
		if !strings.Contains(out.String(), "advisory-only") {
			t.Errorf("expected the advisory-only summary:\n%s", out.String())
		}
	})
}
