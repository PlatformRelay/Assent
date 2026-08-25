package conformance

import "time"

// ids.go holds the fixed identities and the frozen clock the conformance cases
// run under. Moved verbatim out of `helpers_test.go` by E10-S01: the fixtures and
// replay bodies that reference these are now importable non-test Go, so their
// constants must be too.
//
// The clock is FIXED, not injected from the wall: hard rule 7 forbids wall-clock
// dependence in the decision path, and a conformance suite that drifted with the
// clock could not be a golden.

const (
	botID = "assent-bot"
	proj  = "platform/orders-service"
	mrIID = "482"
)

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func testClock() fixedClock {
	return fixedClock{t: time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)}
}
