package conformance

import "github.com/PlatformRelay/assent/internal/forge"

// observe.go declares the PORT-LEVEL OBSERVATION SURFACE for the forge
// conformance suite (E10-S01, REQ-E10-S01-04).
//
// Why this file exists at all. Before extraction, the cases asserted directly on
// `*fake.Forge` internals — `sha_guard_test.go` read `f.Merges` / `f.Approvals`,
// and `reconciliation_test.go:220` type-asserted to `*fake.Forge` to read
// `NoteBody`. A `Factory` returning only a `forge.Forge` is therefore half a
// contract: the port says what a backend can be ASKED to do, and says nothing
// about what a case may OBSERVE afterwards.
//
// The cheap way to close that gap is to delete the assertions the port cannot
// express — keep only "Reconcile returned ErrSHAMoved" and drop "and it recorded
// zero merge attempts". That is exactly how a conformance suite silently stops
// proving the SHA-guard: a backend that returns the right error while merging
// anyway would pass. REQ-E10-S01-04 forbids that resolution, so the surface below
// is derived from what the cases ALREADY read, not from what is convenient to
// implement:
//
//	f.Merges       -> MergeAttempts()   f.Approvals    -> Approvals()
//	ff.NoteBody    -> NoteBody()        f.IsResolved   -> IsResolved()
//	f.ThreadCount  -> ThreadCount()
//
// Everything else the cases read (ListBotThreads, ListBotNotes) is already on
// `forge.Forge` and is deliberately NOT duplicated here.
//
// One assertion is STRENGTHENED rather than preserved, and that is intentional.
// `reconciliation_test.go:220` guarded its "summary must be updated in place"
// check behind `if ff, ok := f.(*fake.Forge); ok` — on the GitLab backend `ok` is
// false, the body never ran, and the check silently proved nothing. It was an
// assertion that could not fail for half the suite. `NoteBody` is on this
// interface so both adapters must answer it and the check becomes universal.
// (The GitLab harness already stores note bodies, so this costs it nothing.)

// Observer is the recorded-writes view of a backend, for assertions the
// `forge.Forge` port cannot express. Both adapters implement it in full; there is
// no optional method and no "unsupported" return, because an adapter allowed to
// opt out of an observation is an adapter allowed to opt out of the proof.
type Observer interface {
	// MergeAttempts is the number of times MergeCAS was INVOKED, whether or not
	// it merged. MergesPerformed is the number that actually merged. Both are on
	// this interface because the two SHA-guard cases need DIFFERENT answers from
	// them, and a single counter cannot express either honestly:
	//
	//	target advanced -> the CurrentHeads pre-check fails closed BEFORE any
	//	                   forge mutation, so MergeCAS is never called at all:
	//	                   attempts 0, performed 0.
	//	source moved    -> the pre-check passes, the head moves inside the TOCTOU
	//	                   window, and the ATOMIC CAS guard refuses:
	//	                   attempts 1, performed 0.
	//
	// Collapsing these to "no merge happened" is precisely the weakening
	// REQ-E10-S01-04 exists to prevent: it would pass a backend that skipped the
	// pre-check entirely and leaned on the CAS, and it would equally pass one
	// that never reached the CAS guard at all. The pre-existing suite asserted
	// only `len(f.Merges) == 0` — the performed count — so which of the two
	// guards actually fired was never established on either case.
	MergeAttempts() int

	// MergesPerformed is the number of merges that actually completed. It must
	// be zero in every fail-closed case.
	MergesPerformed() int

	// Approvals is the number of approvals recorded on the MR.
	Approvals() int

	// ThreadsCreated, ThreadsResolved, NotesCreated and NotesUpdated are WRITE-CALL
	// counters. They exist because the pre-extraction GitLab subtests asserted on
	// exactly these (`h.createCalls`, `h.resolveCalls`, `h.noteCreateCalls`,
	// `h.noteUpdateCalls`) while the fake subtests could not, so "a rerun must not
	// POST a new discussion" was proven on ONE backend only. Putting them on the
	// shared surface makes those assertions universal instead of adapter-local —
	// a strengthening, and the direction REQ-E10-S01-04 requires.
	ThreadsCreated() int
	ThreadsResolved() int
	NotesCreated() int
	NotesUpdated() int

	// BotThreadCount is the number of bot-authored threads, and
	// OpenBotThreadCount the number of those still unresolved. Both are derivable
	// from `forge.Forge.ListBotThreads`, but duplicate-repair asserts on the OPEN
	// count specifically — "exactly one open occupant remains per slot" — which
	// the port's list cannot answer without re-implementing the filter in every
	// case body.
	BotThreadCount() int
	OpenBotThreadCount() int

	// NoteBody returns the full rendered body of the bot note with this id, or
	// "" if there is no such note. Used to prove a summary was updated IN PLACE
	// rather than re-created.
	NoteBody(id string) string

	// IsResolved reports whether the thread with this id is resolved.
	IsResolved(id string) bool

	// ThreadCount is the total number of threads on the MR, bot-authored or
	// not. Contrast `forge.Forge.ListBotThreads`, which is author-filtered: the
	// spoofing cases need BOTH, because "the contributor thread still exists but
	// is invisible to the bot filter" is a different claim from "no thread
	// exists".
	ThreadCount() int
}

// Fixture is the backend-neutral ARRANGE surface: how a case puts a backend into
// the state it wants to act on. Seeding is not part of `forge.Forge` because a
// production adapter has no business creating a contributor-authored thread.
type Fixture interface {
	// SeedThread pre-creates a thread authored by `author` carrying `marker`.
	// `author` is deliberately a parameter and not fixed to the bot: the
	// marker-spoofing cases turn on a CONTRIBUTOR posting a well-formed marker.
	SeedThread(id, author string, marker forge.Marker, resolved bool) error

	// SeedNote pre-creates a non-resolvable MR note authored by `author`.
	SeedNote(id, author string, marker forge.Marker, body string) error

	// MoveTargetHead moves the target-branch tip to `sha` IMMEDIATELY. Distinct
	// from DriftSourceHeadAfterRead below, which fires inside the TOCTOU window:
	// the target-advanced case needs the move to have ALREADY happened when
	// Reconcile takes its pre-check read, so that the pre-check is what refuses.
	MoveTargetHead(sha string)

	// Pins reports the merge pins matching the backend's CURRENT state — what an
	// evaluation would have recorded if it ran right now. Cases take their pins
	// from here rather than hardcoding them, because the pin VALUES are
	// adapter-owned: GitLab has no merge-result digest and synthesises one from
	// source+target (`gitlab.SyntheticDigest`), so a case with a literal digest
	// can only ever run against the fake. That is not a hypothetical — it is why
	// the two SHA-guard cases were fake-only before this story, despite their
	// catalog rows claiming `forge: gitlab`. Collapsing the synthetic digest onto
	// a real one is E10-S03's job; making the cases indifferent to it is this
	// story's.
	Pins() forge.DesiredMerge

	// DriftSourceHeadAfterRead makes the MR's source head change to `sha` AFTER
	// Reconcile's CurrentHeads pre-check read and BEFORE MergeCAS — the TOCTOU
	// window the source-moved case exists to exercise. A backend that cannot
	// model the window cannot host that case honestly, so this is a required
	// method rather than an optional hook.
	DriftSourceHeadAfterRead(sha string)
}

// Backend is one constructed backend under test: the port a case drives, plus
// the two surfaces it arranges and observes through.
type Backend struct {
	Port     forge.Forge
	Fixture  Fixture
	Observer Observer
}

// Config is the initial state a case asks a Factory to construct. Every field is
// explicit: a Factory must never default a SHA, because "the pins the case
// intended" versus "whatever the backend happened to start with" is precisely
// what the SHA-guard cases discriminate.
type Config struct {
	Project string
	MR      string

	// BotAuthor is the identity ListBotThreads/ListBotNotes filter on.
	BotAuthor string

	// The backend's CURRENT heads, which may deliberately differ from the pins a
	// case passes to Reconcile.
	CurrentSourceSHA         string
	CurrentTargetSHA         string
	CurrentMergeResultDigest string
}
