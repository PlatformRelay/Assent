package conformance

import "github.com/PlatformRelay/assent/internal/forge"

// portcount.go counts PORT-LEVEL calls by decorating any `forge.Forge`.
//
// Counting at the port rather than inside each adapter is what makes the
// observation surface adapter-independent: a new backend gets merge/approve/
// thread counters for free, and cannot report them dishonestly, because it never
// implements them. That matters more than it first appears — the two counts the
// SHA-guard cases turn on are NOT observable at the transport layer for GitLab:
// its `MergeCAS` re-reads the heads and refuses client-side, so a moved head
// produces ZERO merge HTTP requests. An httptest harness counting PUTs would
// report 0 attempts where the port was called once, and the case asserting "the
// CAS guard was reached and refused" would be measuring the wrong thing.
//
// UpsertComment is deliberately NOT counted here: create-vs-update is one port
// method with two outcomes, and only the backend knows which occurred. Those two
// stay adapter-reported.
type countingPort struct {
	forge.Forge

	mergeAttempts   int
	mergesPerformed int
	approvals       int
	threadsCreated  int
	threadsResolved int
}

func newCountingPort(inner forge.Forge) *countingPort {
	return &countingPort{Forge: inner}
}

func (c *countingPort) MergeCAS(project, mr string, m forge.DesiredMerge) (string, error) {
	// Incremented BEFORE delegating: a refused CAS is still an attempt. Counting
	// after a successful return would make the two indistinguishable.
	c.mergeAttempts++
	id, err := c.Forge.MergeCAS(project, mr, m)
	if err == nil {
		c.mergesPerformed++
	}
	return id, err
}

func (c *countingPort) Approve(project, mr string) (string, error) {
	id, err := c.Forge.Approve(project, mr)
	if err == nil {
		c.approvals++
	}
	return id, err
}

func (c *countingPort) CreateThread(project, mr string, marker forge.Marker, body string) (forge.Thread, error) {
	th, err := c.Forge.CreateThread(project, mr, marker, body)
	if err == nil {
		c.threadsCreated++
	}
	return th, err
}

func (c *countingPort) ResolveThread(project, mr, id string) error {
	err := c.Forge.ResolveThread(project, mr, id)
	if err == nil {
		c.threadsResolved++
	}
	return err
}

// static assertion that the decorator still satisfies the port it wraps.
var _ forge.Forge = (*countingPort)(nil)
