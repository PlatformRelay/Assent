package conformance

import (
	"fmt"
	"testing"
)

// tb.go defines the testing surface the conformance case bodies assert through.
//
// Why an interface and not `*testing.T`. This repo's most frequently found defect
// is "this assertion cannot fail", and a conformance suite is the worst possible
// place for it: a case that silently stops proving its property still reports
// PASS, and the catalog keeps claiming coverage. The only way to know a case can
// fail is to RUN it against a backend that violates the property and observe it
// go red — and that is impossible with `*testing.T` hard-wired in, because
// `t.Fatalf` fails the very test doing the checking. The proof and the failure
// become indistinguishable.
//
// With TB, `TestEveryCaseCanFail` runs each case against a deliberately
// sabotaged backend and requires a failure. A case whose assertions were
// weakened into unfailability reds THAT test instead of passing quietly.

// TB is the subset of *testing.T the case bodies use.
type TB interface {
	Helper()
	Cleanup(func())
	Fatal(args ...any)
	Fatalf(format string, args ...any)
	Run(name string, fn func(TB)) bool
}

// tbT adapts *testing.T to TB for real runs.
type tbT struct{ *testing.T }

func (t tbT) Run(name string, fn func(TB)) bool {
	return t.T.Run(name, func(sub *testing.T) { fn(tbT{sub}) })
}

// abortFatal unwinds a recorder's Fatalf. Fatalf must STOP the case the way
// testing.T does; returning normally would let execution continue past a failed
// assertion and reach ones that assume it held.
type abortFatal struct{ msg string }

// failRecorder is a TB that records failure instead of reporting it, so a case
// can be run for the purpose of proving it CAN fail.
type failRecorder struct {
	failed   bool
	msg      string
	cleanups []func()
}

func (r *failRecorder) Helper()           {}
func (r *failRecorder) Cleanup(fn func()) { r.cleanups = append(r.cleanups, fn) }
func (r *failRecorder) Fatalf(f string, a ...any) {
	r.failed = true
	if r.msg == "" {
		r.msg = fmt.Sprintf(f, a...)
	}
	panic(abortFatal{msg: r.msg})
}

// Run executes a subcase, absorbing its abort so sibling subcases still run —
// mirroring testing.T, where one failing subtest does not abandon the rest.
func (r *failRecorder) Run(_ string, fn func(TB)) bool {
	before := r.failed
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				if _, ok := rec.(abortFatal); !ok {
					panic(rec)
				}
			}
		}()
		fn(r)
	}()
	return r.failed == before
}

func (r *failRecorder) runCleanups() {
	for i := len(r.cleanups) - 1; i >= 0; i-- {
		r.cleanups[i]()
	}
	r.cleanups = nil
}

// runAndRecord runs one case against a factory and reports whether it failed.
func runAndRecord(c Case, f Factory) (failed bool, msg string) {
	r := &failRecorder{}
	defer r.runCleanups()
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				if _, ok := rec.(abortFatal); !ok {
					panic(rec)
				}
			}
		}()
		c.Run(r, f)
	}()
	return r.failed, r.msg
}

// Fatal mirrors testing.T.Fatal: record and abort.
func (r *failRecorder) Fatal(a ...any) { r.Fatalf("%s", fmt.Sprint(a...)) }
