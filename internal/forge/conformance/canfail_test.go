package conformance

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
)

// canfail_test.go answers the question a green conformance suite cannot answer
// about itself: would any of these cases NOTICE if the property it claims to
// prove were violated?
//
// This repo's most frequently found defect is an assertion that cannot fail, and
// three of the cases here shipped with one — the summary-updated-in-place check
// was guarded behind a `*fake.Forge` type assertion that was false on GitLab, so
// it proved nothing there while still reporting PASS. Reading the code did not
// catch that; running it against a violating backend does.

// sabotagedObserver reports zero for every observation and truthfully answers
// nothing. A case that still passes against it is asserting on nothing.
type sabotagedObserver struct{}

func (sabotagedObserver) MergeAttempts() int      { return 0 }
func (sabotagedObserver) MergesPerformed() int    { return 0 }
func (sabotagedObserver) Approvals() int          { return 0 }
func (sabotagedObserver) ThreadsCreated() int     { return 0 }
func (sabotagedObserver) ThreadsResolved() int    { return 0 }
func (sabotagedObserver) NotesCreated() int       { return 0 }
func (sabotagedObserver) NotesUpdated() int       { return 0 }
func (sabotagedObserver) NoteBody(string) string  { return "" }
func (sabotagedObserver) IsResolved(string) bool  { return false }
func (sabotagedObserver) ThreadCount() int        { return 0 }
func (sabotagedObserver) BotThreadCount() int     { return 0 }
func (sabotagedObserver) OpenBotThreadCount() int { return 0 }

// inertFixture accepts every arrangement and performs none of it: the SHA never
// moves, so a guard that should fire never gets the chance.
type inertFixture struct{ real Fixture }

func (f inertFixture) SeedThread(id, author string, m forge.Marker, resolved bool) error {
	return f.real.SeedThread(id, author, m, resolved)
}
func (f inertFixture) SeedNote(id, author string, m forge.Marker, body string) error {
	return f.real.SeedNote(id, author, m, body)
}
func (f inertFixture) Pins() forge.DesiredMerge      { return f.real.Pins() }
func (inertFixture) MoveTargetHead(string)           {}
func (inertFixture) DriftSourceHeadAfterRead(string) {}

// sabotage wraps ANY factory, so the gate runs against every backend rather than
// only the fake. That matters: the defect this file exists to catch — an
// assertion guarded behind a `*fake.Forge` type assertion — was invisible on the
// fake and dead on GitLab, so a fake-only gate would have missed the very bug
// that motivated it.
func sabotage(f Factory) Factory {
	return func(t TB, cfg Config) Backend {
		b := f(t, cfg)
		return Backend{
			Port:     b.Port,
			Fixture:  inertFixture{real: b.Fixture},
			Observer: sabotagedObserver{},
		}
	}
}

// TestEveryCaseCanFail requires EVERY catalogued case to go red against a backend
// that violates what it claims to prove. A case that passes here has stopped
// asserting: either its checks were weakened to what any backend can satisfy, or
// they were guarded behind a condition that is false in general.
//
// This is the gate REQ-E10-S01-04 needs in order to mean anything. Without it,
// "no assertion is downgraded to accommodate a backend" is a promise in a comment.
func TestEveryCaseCanFail(t *testing.T) {
	cases := Cases()
	if len(cases) == 0 {
		t.Fatal("no cases — this gate would be vacuously satisfied")
	}
	for _, be := range backends() {
		for _, c := range cases {
			t.Run(be.name+"/"+c.ID, func(t *testing.T) {
				failed, msg := runAndRecord(c, sabotage(be.f))
				if !failed {
					t.Fatalf("case %q PASSED against a sabotaged %s backend — it is asserting "+
						"nothing that distinguishes a conforming forge from a broken one "+
						"(REQ-E10-S01-04)", c.ID, be.name)
				}
				t.Logf("case %q correctly failed: %s", c.ID, msg)
			})
		}
	}
}

// TestSabotageIsDetectableNotUniversal is the POSITIVE CONTROL. Without it,
// TestEveryCaseCanFail would also be satisfied by cases that fail against
// EVERYTHING — including a conforming backend — which proves nothing about the
// assertions and everything about a broken harness.
func TestSabotageIsDetectableNotUniversal(t *testing.T) {
	for _, be := range backends() {
		for _, c := range Cases() {
			t.Run(be.name+"/"+c.ID, func(t *testing.T) {
				if failed, msg := runAndRecord(c, be.f); failed {
					t.Fatalf("case %q failed against a CONFORMING %s backend: %s", c.ID, be.name, msg)
				}
			})
		}
	}
}
