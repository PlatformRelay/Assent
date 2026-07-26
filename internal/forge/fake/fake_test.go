package fake_test

import (
	"errors"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/fake"
)

const bot = "assent-bot"

func marker() forge.Marker {
	return forge.Marker{
		Slot:       forge.Slot{Project: "p", MR: "1", Rule: "r", Effect: "comment"},
		Occurrence: "sha256:" + rep('a'),
		Decision:   "sha256:" + rep('b'),
		Artifact:   forge.Artifact{Kind: "finding-thread", SchemaVersion: "v1alpha1"},
	}
}

func rep(c byte) string {
	b := make([]byte, 64)
	for i := range b {
		b[i] = c
	}
	return string(b)
}

// TestListBotThreadsFiltersByAuthor proves the author-identity filter: a
// contributor thread is excluded, a bot thread is returned.
func TestListBotThreadsFiltersByAuthor(t *testing.T) {
	f := fake.New(bot, "s", "t", "d")
	f.SeedThread("note/1", bot, marker(), false)
	f.SeedThread("note/2", "contributor", marker(), false)

	got, err := f.ListBotThreads("p", "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "note/1" {
		t.Fatalf("expected only the bot thread, got %+v", got)
	}
	if f.ThreadCount() != 2 {
		t.Fatalf("ThreadCount must count all threads, got %d", f.ThreadCount())
	}
	if f.BotThreadCount() != 1 {
		t.Fatalf("BotThreadCount must count only bot threads, got %d", f.BotThreadCount())
	}
}

// TestCurrentHeads proves the fake returns its configured CAS state — the
// pre-write SHA-guard read Reconcile uses to fail closed before any approval.
func TestCurrentHeads(t *testing.T) {
	f := fake.New(bot, "s", "t", "d")
	src, tgt, dg, err := f.CurrentHeads("p", "1")
	if err != nil {
		t.Fatal(err)
	}
	if src != "s" || tgt != "t" || dg != "d" {
		t.Fatalf("CurrentHeads must return the configured CAS state, got %q %q %q", src, tgt, dg)
	}

	// TOCTOU seam: the AfterCurrentHeads hook fires after the read; the RETURNED
	// values are the pre-move snapshot, but the fake's state is mutated for the
	// next (MergeCAS) read. This is the mechanism the Reconcile-layer race test
	// uses to reach the atomic MergeCAS guard.
	fired := false
	f.AfterCurrentHeads = func(fk *fake.Forge) {
		fired = true
		fk.CurrentTargetSha = "moved"
	}
	src, tgt, _, _ = f.CurrentHeads("p", "1")
	if !fired {
		t.Fatal("AfterCurrentHeads hook must fire")
	}
	if src != "s" || tgt != "t" {
		t.Fatalf("CurrentHeads must return the PRE-move snapshot, got src=%q tgt=%q", src, tgt)
	}
	if f.CurrentTargetSha != "moved" {
		t.Fatalf("hook must have mutated the fake's target for the next read, got %q", f.CurrentTargetSha)
	}
}

// TestCreateThreadAndApprove proves CreateThread and Approve allocate fresh ids
// and record the writes.
func TestCreateThreadAndApprove(t *testing.T) {
	f := fake.New(bot, "s", "t", "d")

	th, err := f.CreateThread("p", "1", marker(), "body")
	if err != nil {
		t.Fatal(err)
	}
	if th.ID == "" || th.Author != bot {
		t.Fatalf("created thread must have an id and bot author, got %+v", th)
	}
	if f.BotThreadCount() != 1 {
		t.Fatalf("created thread must be listed, got %d", f.BotThreadCount())
	}

	id, err := f.Approve("p", "1")
	if err != nil {
		t.Fatal(err)
	}
	if id == "" || len(f.Approvals) != 1 {
		t.Fatalf("approve must record one approval, got id=%q approvals=%v", id, f.Approvals)
	}
}

// TestMergeCASAcceptsExactPins proves MergeCAS merges when all three pins match,
// rejects on any drift, and refuses a second merge.
func TestMergeCASAcceptsExactPins(t *testing.T) {
	f := fake.New(bot, "s", "t", "d")
	m := forge.DesiredMerge{SourceSha: "s", TargetSha: "t", MergeResultDigest: "d"}

	id, err := f.MergeCAS("p", "1", m)
	if err != nil {
		t.Fatalf("exact-pin merge must succeed: %v", err)
	}
	if id == "" || len(f.Merges) != 1 {
		t.Fatalf("merge must be recorded, got id=%q merges=%v", id, f.Merges)
	}

	// Second merge rejected (idempotent guard).
	if _, err := f.MergeCAS("p", "1", m); err == nil {
		t.Fatal("a second merge must be rejected")
	}
}

// TestResolveThreadIdempotentAndCounts proves the duplicate-repair fake hooks
// (S12-03): ResolveThread flips Resolved in place, is idempotent (unknown or
// already-resolved id is a no-op that never errors and creates nothing), and the
// OpenBotThreadCount / IsResolved assertion surfaces reflect the resolved state.
func TestResolveThreadIdempotentAndCounts(t *testing.T) {
	f := fake.New(bot, "s", "t", "d")
	f.SeedThread("note/8001", bot, marker(), false)
	f.SeedThread("note/8003", bot, marker(), false)

	before := f.ThreadCount()
	if f.OpenBotThreadCount() != 2 {
		t.Fatalf("two open bot threads expected, got %d", f.OpenBotThreadCount())
	}

	if err := f.ResolveThread("p", "1", "note/8003"); err != nil {
		t.Fatalf("ResolveThread: %v", err)
	}
	if !f.IsResolved("note/8003") || f.IsResolved("note/8001") {
		t.Fatalf("only note/8003 must be resolved; got 8003=%v 8001=%v",
			f.IsResolved("note/8003"), f.IsResolved("note/8001"))
	}
	if f.OpenBotThreadCount() != 1 {
		t.Fatalf("one open bot thread must remain after resolve, got %d", f.OpenBotThreadCount())
	}

	// Idempotent: resolving the same id again, or an unknown id, is a no-op with
	// no error and no new thread.
	if err := f.ResolveThread("p", "1", "note/8003"); err != nil {
		t.Fatalf("re-resolve must be a no-op, got %v", err)
	}
	if err := f.ResolveThread("p", "1", "note/does-not-exist"); err != nil {
		t.Fatalf("resolving an unknown id must be a no-op, got %v", err)
	}
	if f.ThreadCount() != before {
		t.Fatalf("resolve must never create a thread: before=%d after=%d", before, f.ThreadCount())
	}
	// IsResolved on an unknown id reports false (not an occupant).
	if f.IsResolved("note/does-not-exist") {
		t.Fatal("IsResolved on an unknown id must be false")
	}
}

// TestMergeCASRejectsEachMovedAxis proves each of the three CAS axes is checked
// independently: moving source, target, or digest alone rejects with ErrSHAMoved.
func TestMergeCASRejectsEachMovedAxis(t *testing.T) {
	base := forge.DesiredMerge{SourceSha: "s", TargetSha: "t", MergeResultDigest: "d"}
	cases := []struct {
		name      string
		s, tg, dg string
	}{
		{"source-moved", "s2", "t", "d"},
		{"target-moved", "s", "t2", "d"},
		{"digest-moved", "s", "t", "d2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := fake.New(bot, tc.s, tc.tg, tc.dg)
			_, err := f.MergeCAS("p", "1", base)
			if !errors.Is(err, forge.ErrSHAMoved) {
				t.Fatalf("expected ErrSHAMoved, got %v", err)
			}
			if len(f.Merges) != 0 {
				t.Fatalf("no merge must be recorded on rejection, got %d", len(f.Merges))
			}
		})
	}
}
