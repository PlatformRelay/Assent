package conformance

import (
	"fmt"
	"sort"
	"testing"
)

// loadbearing_test.go strengthens the can-fail gate from "the case notices SOME
// violation" to "EVERY observation the case makes is load-bearing".
//
// TestEveryCaseCanFail sabotages everything at once, so a case passes it by
// noticing a single violation while its remaining assertions are dead. That is
// not a hypothetical failure mode — it is what a case looks like after someone
// weakens one assertion to accommodate a backend and leaves the rest in place.
//
// The gate below needs no hand-maintained expectations, which matters because a
// pinned list of "assertions case X makes" is a predicate over text that drifts
// from the code the moment either changes. Instead it RECORDS which observations
// a case actually calls, then corrupts them ONE AT A TIME and requires each to
// change the verdict. What is asserted is a property of the run, not a
// description of the source.

type observation string

// recordingObserver notes which observations a case actually consults.
type recordingObserver struct {
	real   Observer
	called map[observation]bool
}

func (r *recordingObserver) note(name observation) { r.called[name] = true }

func (r *recordingObserver) MergeAttempts() int {
	r.note("MergeAttempts")
	return r.real.MergeAttempts()
}
func (r *recordingObserver) MergesPerformed() int {
	r.note("MergesPerformed")
	return r.real.MergesPerformed()
}
func (r *recordingObserver) Approvals() int { r.note("Approvals"); return r.real.Approvals() }
func (r *recordingObserver) ThreadsCreated() int {
	r.note("ThreadsCreated")
	return r.real.ThreadsCreated()
}
func (r *recordingObserver) ThreadsResolved() int {
	r.note("ThreadsResolved")
	return r.real.ThreadsResolved()
}
func (r *recordingObserver) NotesCreated() int { r.note("NotesCreated"); return r.real.NotesCreated() }
func (r *recordingObserver) NotesUpdated() int { r.note("NotesUpdated"); return r.real.NotesUpdated() }
func (r *recordingObserver) ThreadCount() int  { r.note("ThreadCount"); return r.real.ThreadCount() }
func (r *recordingObserver) BotThreadCount() int {
	r.note("BotThreadCount")
	return r.real.BotThreadCount()
}
func (r *recordingObserver) OpenBotThreadCount() int {
	r.note("OpenBotThreadCount")
	return r.real.OpenBotThreadCount()
}
func (r *recordingObserver) NoteBody(id string) string {
	r.note("NoteBody")
	return r.real.NoteBody(id)
}
func (r *recordingObserver) IsResolved(id string) bool {
	r.note("IsResolved")
	return r.real.IsResolved(id)
}

// corruptObserver returns a DIFFERENT value for exactly one observation and the
// truth for every other. Ints are offset by one and bools inverted, so the
// corruption is guaranteed to differ from the real answer whatever it is — a
// fixed sentinel like 0 or "" could coincide with the true value and produce a
// false green.
// It corrupts only the FIRST read of the target. A uniform offset is invisible to
// an assertion comparing a BEFORE and an AFTER of the same observation — both
// shift by the same amount and the delta is unchanged — which would report a
// perfectly good delta assertion as dead weight. Corrupting one read makes the
// delta move.
type corruptObserver struct {
	real   Observer
	target observation
	seen   *int
}

func (c corruptObserver) hit(name observation) bool {
	if c.target != name {
		return false
	}
	*c.seen++
	return *c.seen == 1
}

func (c corruptObserver) MergeAttempts() int {
	return offsetIf(c.hit("MergeAttempts"), c.real.MergeAttempts())
}
func (c corruptObserver) MergesPerformed() int {
	return offsetIf(c.hit("MergesPerformed"), c.real.MergesPerformed())
}
func (c corruptObserver) Approvals() int {
	return offsetIf(c.hit("Approvals"), c.real.Approvals())
}
func (c corruptObserver) ThreadsCreated() int {
	return offsetIf(c.hit("ThreadsCreated"), c.real.ThreadsCreated())
}
func (c corruptObserver) ThreadsResolved() int {
	return offsetIf(c.hit("ThreadsResolved"), c.real.ThreadsResolved())
}
func (c corruptObserver) NotesCreated() int {
	return offsetIf(c.hit("NotesCreated"), c.real.NotesCreated())
}
func (c corruptObserver) NotesUpdated() int {
	return offsetIf(c.hit("NotesUpdated"), c.real.NotesUpdated())
}
func (c corruptObserver) ThreadCount() int {
	return offsetIf(c.hit("ThreadCount"), c.real.ThreadCount())
}
func (c corruptObserver) BotThreadCount() int {
	return offsetIf(c.hit("BotThreadCount"), c.real.BotThreadCount())
}
func (c corruptObserver) OpenBotThreadCount() int {
	return offsetIf(c.hit("OpenBotThreadCount"), c.real.OpenBotThreadCount())
}

func (c corruptObserver) NoteBody(id string) string {
	body := c.real.NoteBody(id)
	if c.hit("NoteBody") {
		return body + "-corrupted"
	}
	return body
}

func (c corruptObserver) IsResolved(id string) bool {
	got := c.real.IsResolved(id)
	if c.hit("IsResolved") {
		return !got
	}
	return got
}

func offsetIf(corrupt bool, v int) int {
	if corrupt {
		return v + 1
	}
	return v
}

// TestEveryObservationIsLoadBearing is the per-assertion mutation control.
func TestEveryObservationIsLoadBearing(t *testing.T) {
	for _, c := range Cases() {
		t.Run(c.ID, func(t *testing.T) {
			// 1. Discover what this case actually observes, by running it.
			called := map[observation]bool{}
			recording := func(tb TB, cfg Config) Backend {
				b := fakeFactory(tb, cfg)
				return Backend{
					Port:     b.Port,
					Fixture:  b.Fixture,
					Observer: &recordingObserver{real: b.Observer, called: called},
				}
			}
			if failed, msg := runAndRecord(c, recording); failed {
				t.Fatalf("case must pass against a conforming backend, got: %s", msg)
			}
			if len(called) == 0 {
				t.Fatal("case consulted NO observation — it cannot be proving anything " +
					"about what the forge actually did")
			}

			// 2. Corrupt each observed value ALONE; each must flip the verdict.
			var names []observation
			for n := range called {
				names = append(names, n)
			}
			sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })

			for _, name := range names {
				t.Run(string(name), func(t *testing.T) {
					seen := new(int)
					corrupting := func(tb TB, cfg Config) Backend {
						b := fakeFactory(tb, cfg)
						return Backend{
							Port:     b.Port,
							Fixture:  b.Fixture,
							Observer: corruptObserver{real: b.Observer, target: name, seen: seen},
						}
					}
					failed, _ := runAndRecord(c, corrupting)
					if !failed {
						t.Fatalf("case %q read %s but does not ASSERT on it — corrupting the "+
							"value alone left the case green, so that observation is dead "+
							"weight and the property it implies is unproven", c.ID, name)
					}
				})
			}
			t.Logf("%d load-bearing observation(s): %s", len(names), fmt.Sprint(names))
		})
	}
}
