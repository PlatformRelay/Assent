package conformance

import (
	"errors"

	"github.com/PlatformRelay/assent/internal/change"
	"github.com/PlatformRelay/assent/internal/forge"
)

// cases_addressing.go holds the addressing & sentinel cases S00 minted
// (docs/planning/github-addressing-model.md, Q1/Q4 conformance tables):
//
//	fork-head-unchanged-file-no-lifecycle   — unchanged governed file ⇒ NO lifecycle event
//	fork-head-genuine-delete-detected       — positive control: a real delete still mints KindDelete
//	forbidden-never-renders-as-absent       — a refused read is never absence
//	absent-file-still-renders-as-absent     — positive control: a real 404 is still absence
//	own-markers-recognised-identity         — the marker filter matches the authenticated identity
//
// They run through the MR-relative accessors (forge.RunPort.FileAtBase /
// FileAtHead) and the port identity (ADR-0021 items 5 and 7). The GitHub
// factory joins these rows in E10-S06/S07; until then the catalog rows carry
// the cited deferral.

// addressingConfig is the Config the addressing cases build their backend with.
func addressingConfig() Config {
	return Config{
		Project:                  proj,
		MR:                       mrIID,
		BotAuthor:                botID,
		CurrentSourceSHA:         "srcSHA",
		CurrentTargetSHA:         "tgtSHA",
		CurrentMergeResultDigest: "sha256:digest",
		ForkMR:                   true,
		GovernedPath:             "topics/orders.yaml",
	}
}

// governedContent is the byte content every addressing case shares; unchanged
// governed files seed the SAME bytes on both sides.
var governedContent = []byte("kind: Topic\nname: orders.events.v1\n")

// oneSidedLifecycle is the judged-pair predicate the run path uses to mint
// whole-file FileEvents (changeSetForGoverned → change.OneSidedLifecycle). The
// addressing cases assert on it directly: an unchanged pair must be NOT
// one-sided, a base-present/head-absent pair MUST be (KindDelete, true).
func oneSidedLifecycle(base, head []byte) (change.Kind, bool) {
	return change.OneSidedLifecycle(base, head)
}

// caseForkHeadUnchangedFileNoLifecycle is
// TestConformanceForkHeadUnchangedFileNoLifecycle (S00 Q1): a FORK MR whose
// governed file is unchanged yields NO lifecycle event — the fabricated-DELETE
// defect. The port never names how the backend reaches the fork's head; the
// case reads the two MR-relative sides and asserts the judged pair produces no
// one-sided lifecycle.
func caseForkHeadUnchangedFileNoLifecycle(t TB, f Factory) {
	cfg := addressingConfig()
	b := f(t, cfg)
	b.Fixture.SeedFile(cfg.GovernedPath, FileSideBase, governedContent)
	b.Fixture.SeedFile(cfg.GovernedPath, FileSideHead, governedContent)

	base, err := b.Port.FileAtBase(cfg.Project, cfg.MR, cfg.GovernedPath)
	if err != nil {
		t.Fatalf("FileAtBase: %v", err)
	}
	head, err := b.Port.FileAtHead(cfg.Project, cfg.MR, cfg.GovernedPath)
	if err != nil {
		t.Fatalf("FileAtHead must reach the fork's head content, got: %v", err)
	}
	if string(base) != string(governedContent) || string(head) != string(governedContent) {
		t.Fatalf("unchanged governed file must read identically on both sides (base=%d bytes, head=%d bytes)",
			len(base), len(head))
	}
	if _, ok := oneSidedLifecycle(base, head); ok {
		t.Fatalf("unchanged governed file minted a lifecycle event — the fabricated-DELETE defect (S00 Q1)")
	}
	// A read-only addressing case writes nothing; the observation surface makes
	// that load-bearing rather than incidental.
	if got := b.Observer.MergeAttempts() + b.Observer.MergesPerformed() + b.Observer.Approvals() + b.Observer.ThreadsCreated(); got != 0 {
		t.Fatalf("a governed-file read must perform zero forge writes, got %d", got)
	}
}

// caseForkHeadGenuineDeleteDetected is the POSITIVE CONTROL (S00 Q1): a fork MR
// that really deletes the governed file still mints KindDelete. Without it,
// "never mint a DELETE from a fork" is satisfiable by never minting a DELETE at
// all — the unfailable-assertion shape this repo's review gate pairs with every
// forbidden case.
func caseForkHeadGenuineDeleteDetected(t TB, f Factory) {
	cfg := addressingConfig()
	b := f(t, cfg)
	b.Fixture.SeedFile(cfg.GovernedPath, FileSideBase, governedContent)
	// The base side is READ THROUGH THE PORT, so the case fails against a
	// backend that never seeds it (the inert-fixture gate) — the delete is
	// real only if the base side was actually present.
	base, err := b.Port.FileAtBase(cfg.Project, cfg.MR, cfg.GovernedPath)
	if err != nil {
		t.Fatalf("FileAtBase on the seeded base side: %v", err)
	}
	if string(base) != string(governedContent) {
		t.Fatalf("FileAtBase must serve the seeded base content, got %d bytes", len(base))
	}
	// Head side stays absent: the genuine delete. FileAtHead must report
	// forge.ErrNotFound (absence), which the run path's orAbsent mapping turns
	// into nil bytes — and THAT nil is what mints the true KindDelete.
	_, err = b.Port.FileAtHead(cfg.Project, cfg.MR, cfg.GovernedPath)
	if !errors.Is(err, forge.ErrNotFound) {
		t.Fatalf("a deleted governed file must be ABSENT on the head side, got err=%v", err)
	}
	if _, ok := oneSidedLifecycle(base, nil); !ok {
		t.Fatalf("a genuine whole-file delete must mint a lifecycle event")
	}
	if b.Observer.MergeAttempts() != 0 || b.Observer.Approvals() != 0 || b.Observer.ThreadsCreated() != 0 {
		t.Fatalf("a read-only addressing case must perform zero forge writes, got attempts=%d approvals=%d",
			b.Observer.MergeAttempts(), b.Observer.Approvals())
	}
}

// caseForbiddenNeverRendersAsAbsent is S00 Q4: a governed-file read the forge
// refuses on permission grounds yields forge.ErrUnauthorized — NEVER
// forge.ErrNotFound, never nil content, never a lifecycle event.
func caseForbiddenNeverRendersAsAbsent(t TB, f Factory) {
	cfg := addressingConfig()
	b := f(t, cfg)
	b.Fixture.SeedFile(cfg.GovernedPath, FileSideBase, governedContent)
	b.Fixture.RefuseFileRead(cfg.GovernedPath)
	for name, read := range map[string]func() ([]byte, error){
		"FileAtBase": func() ([]byte, error) { return b.Port.FileAtBase(cfg.Project, cfg.MR, cfg.GovernedPath) },
		"FileAtHead": func() ([]byte, error) { return b.Port.FileAtHead(cfg.Project, cfg.MR, cfg.GovernedPath) },
		"FileAtRef":  func() ([]byte, error) { return b.Port.FileAtRef(cfg.Project, cfg.GovernedPath, "tgtSHA") },
	} {
		_, err := read()
		switch {
		case err == nil:
			t.Fatalf("%s: a refused read must not succeed", name)
		case errors.Is(err, forge.ErrNotFound):
			t.Fatalf("%s: a forbidden read rendered as ABSENT — the absent-means-trusted collapse (S00 Q4)", name)
		case !errors.Is(err, forge.ErrUnauthorized):
			t.Fatalf("%s: a forbidden read must carry forge.ErrUnauthorized, got: %v", name, err)
		}
	}
	if b.Observer.MergeAttempts() != 0 || b.Observer.ThreadsCreated() != 0 {
		t.Fatalf("a refused read must abort with zero forge writes, got attempts=%d threads=%d",
			b.Observer.MergeAttempts(), b.Observer.ThreadsCreated())
	}
}

// caseAbsentFileStillRendersAsAbsent is the POSITIVE CONTROL (S00 Q4): a
// genuine 404 inside a readable repo still yields forge.ErrNotFound, so a real
// whole-file ADD/DELETE is still detected (EFE-S03 preserved). Without this,
// "always error" would satisfy the forbidden case above.
//
// It first proves PRESENCE on the base side (the fixture seeded it), so the
// case discriminates present from absent through the same accessor — a backend
// that maps everything to ErrNotFound fails it, as does one that maps absence
// to a permission failure.
func caseAbsentFileStillRendersAsAbsent(t TB, f Factory) {
	cfg := addressingConfig()
	b := f(t, cfg)
	b.Fixture.SeedFile(cfg.GovernedPath, FileSideBase, governedContent)
	base, err := b.Port.FileAtBase(cfg.Project, cfg.MR, cfg.GovernedPath)
	if err != nil {
		t.Fatalf("FileAtBase on a SEEDED path must return the seeded content, got %v", err)
	}
	if string(base) != string(governedContent) {
		t.Fatalf("FileAtBase read %d bytes, want the seeded content", len(base))
	}
	for name, read := range map[string]func() ([]byte, error){
		"FileAtHead(unseeded)": func() ([]byte, error) { return b.Port.FileAtHead(cfg.Project, cfg.MR, cfg.GovernedPath) },
		"FileAtRef":            func() ([]byte, error) { return b.Port.FileAtRef(cfg.Project, cfg.GovernedPath, "absentSHA") },
	} {
		_, err := read()
		if !errors.Is(err, forge.ErrNotFound) {
			t.Fatalf("%s: a genuinely absent file must yield forge.ErrNotFound, got %v", name, err)
		}
		if errors.Is(err, forge.ErrUnauthorized) {
			t.Fatalf("%s: absence must not render as forbidden", name)
		}
	}
	// Read-only cases write nothing: the observation surface must agree.
	if b.Observer.MergeAttempts() != 0 || b.Observer.Approvals() != 0 || b.Observer.ThreadsCreated() != 0 {
		t.Fatalf("a read-only addressing case must not write to the forge")
	}
}

// caseOwnMarkersRecognisedIdentity is ADR-0021 item 7 (REQ-E10-S02-06): the
// marker filter matches the AUTHENTICATED identity the port exposes — not "any
// bot". An artifact authored by the exact identity the port reports must be
// visible to the bot filter; an artifact by a DIFFERENT identity (a second app,
// a contributor) must be invisible.
//
// The PAT shape is what both built-in factories model (a PAT's identity is a
// User). The App-installation shape is exercised when an App-auth adapter joins
// the suite (E10-S06+) — the catalog row carries that disposition.
func caseOwnMarkersRecognisedIdentity(t TB, f Factory) {
	cfg := addressingConfig()
	b := f(t, cfg)
	ident, err := b.Port.Identity()
	if err != nil {
		t.Fatalf("Identity: %v", err)
	}
	if ident.Login == "" || ident.Kind == "" {
		t.Fatalf("the port must expose the authenticated identity, got %+v", ident)
	}
	own := rerunChallengeMarker()
	if err := b.Fixture.SeedThread("own/1", ident.Login, own, false); err != nil {
		t.Fatalf("seed own thread: %v", err)
	}
	other := rerunChallengeMarker()
	other.Slot.Effect = "block"
	if err := b.Fixture.SeedThread("other/1", "second-bot", other, false); err != nil {
		t.Fatalf("seed other-bot marker: %v", err)
	}
	threads, err := b.Port.ListBotThreads(cfg.Project, cfg.MR)
	if err != nil {
		t.Fatalf("ListBotThreads: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("markers authored by the authenticated identity (%s) must be recognised as our own; got %d bot threads, want 1", ident.Login, len(threads))
	}
	if threads[0].Author != ident.Login {
		t.Fatalf("bot thread author %q, want the authenticated identity %q", threads[0].Author, ident.Login)
	}
	if got := b.Observer.BotThreadCount(); got != 1 {
		t.Fatalf("exactly one bot-authored thread (the own-identity one) must be visible; observer says %d", got)
	}
	if got := b.Observer.ThreadCount(); got != 2 {
		t.Fatalf("the second identity's marker-carrying thread must still exist on the MR (invisible, not deleted); got %d total threads", got)
	}
}
