package forge_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/fake"
)

// scriptedForge wraps the in-memory fake to inject the forge-side FAILURES the
// clear-slot path must fail closed on. The fake is a happy-path substrate: it
// never errors from ListBotThreads and its ResolveThread always succeeds, so the
// error arms of reconcileClearSlot are unreachable through it alone.
//
// It EMBEDS *fake.Forge rather than reimplementing the port, so a port change
// (new method) does not silently drop this stub out of forge.Forge — and so the
// non-scripted behaviour stays the real fake's, not a second hand-rolled model
// that could drift from it.
type scriptedForge struct {
	*fake.Forge

	// listErrAt is the 1-based ListBotThreads call ordinal that fails; 0 = never.
	// Ordinal 1 is reconcileClearSlot's own listing, ordinal 2 the post-write
	// rescan (P3-E5 step 9) — the two distinct list sites in this path.
	listErrAt int
	listCalls int

	// resolveErr, when set, is returned by ResolveThread (the forge refuses the
	// write). resolveSilentNoop instead returns SUCCESS while leaving the thread
	// OPEN — a forge that lies about having resolved, which is exactly the
	// partial clear the rescan exists to catch.
	resolveErr        error
	resolveSilentNoop bool
}

var errForgeUnavailable = errors.New("forge unavailable (503)")

func (s *scriptedForge) ListBotThreads(project, mr string) ([]forge.Thread, error) {
	s.listCalls++
	if s.listErrAt != 0 && s.listCalls == s.listErrAt {
		return nil, errForgeUnavailable
	}
	return s.Forge.ListBotThreads(project, mr)
}

func (s *scriptedForge) ResolveThread(project, mr, id string) error {
	if s.resolveErr != nil {
		return s.resolveErr
	}
	if s.resolveSilentNoop {
		return nil
	}
	return s.Forge.ResolveThread(project, mr, id)
}

// static assertion that the scripted stub is still a full Forge.
var _ forge.Forge = (*scriptedForge)(nil)

// otherSlotMarker is a marker for a DIFFERENT, healthy slot on the same MR. It is
// seeded in every case so each assertion also proves the clear is slot-scoped:
// clearing one slot must never resolve another slot's open thread.
func otherSlotMarker() forge.Marker {
	m := reviewMarker()
	m.Slot.Rule = "placement/region-allowlist"
	return m
}

// openForSlot counts UNRESOLVED bot threads for a slot, reading through the
// embedded fake directly so a scripted list error cannot distort the assertion.
func openForSlot(t *testing.T, f *scriptedForge, slot forge.Slot) int {
	t.Helper()
	threads, err := f.Forge.ListBotThreads(proj, mrIID)
	if err != nil {
		t.Fatalf("assertion listing failed: %v", err)
	}
	n := 0
	for _, th := range threads {
		if th.Marker.Slot == slot && !th.Resolved {
			n++
		}
	}
	return n
}

// TestReconcileClearSlotBranches — REQ-AUD-S13-02 (TEST-05).
//
// Drives the PRODUCTION entry point (forge.Reconcile) down the clear-slot path
// and walks every branch of reconcileClearSlot. Each ERROR branch is asserted at
// BOTH polarities: the refusal AND the neighbouring success that proves the
// refusal was caused by the injected failure and not by the fixture being inert.
//
// The fail-closed teeth, asserted on every erroring case:
//   - ZERO operations on the returned receipt (an empty-ops receipt is not even
//     representable in the frozen schema, so a refusal must never fabricate one);
//   - the slot's open thread is STILL OPEN (no half-write left behind);
//   - the unrelated healthy slot is untouched.
func TestReconcileClearSlotBranches(t *testing.T) {
	slot := reviewMarker().Slot
	otherSlot := otherSlotMarker().Slot

	desired := forge.DesiredReviewState{
		Project:   proj,
		MR:        mrIID,
		ClearSlot: &slot,
	}

	cases := []struct {
		name string
		// seed prepares the forge state and the failure script.
		seed func(f *scriptedForge)
		// wantErrIs is a sentinel the error must match (nil when none expected).
		wantErrIs error
		// wantErrSub is a substring the error message must carry ("" when none).
		wantErrSub string
		// wantOpID is the receipt operation's targetId on the success polarity.
		wantOpID string
		// wantOpenAfter is how many threads for the cleared slot remain OPEN.
		wantOpenAfter int
	}{
		{
			// Branch: ListBotThreads error. The slot cannot be inspected, so the
			// clear cannot be proven — refuse rather than assume nothing is open.
			name: "list_error_fails_closed",
			seed: func(f *scriptedForge) {
				f.SeedThread("note/9001", botID, reviewMarker(), false)
				f.listErrAt = 1
			},
			wantErrSub:    "list bot threads",
			wantOpenAfter: 1,
		},
		{
			// Polarity for the branch above AND for the resolve/rescan branches
			// below: the ordinary single-open-thread clear succeeds.
			name: "single_open_thread_resolved",
			seed: func(f *scriptedForge) {
				f.SeedThread("note/9001", botID, reviewMarker(), false)
			},
			wantOpID:      "note/9001",
			wantOpenAfter: 0,
		},
		{
			// Branch: no bot thread for the slot at all. There is nothing to
			// reference, so no schema-valid receipt exists — fail closed with the
			// typed sentinel rather than invent an operation that did not happen.
			name: "no_thread_for_slot_unsupported",
			seed: func(_ *scriptedForge) {
				// Only the OTHER slot has a thread; the cleared slot has none.
			},
			wantErrIs:     forge.ErrUnsupportedDecision,
			wantOpenAfter: 0,
		},
		{
			// Branch: idempotent clear. The slot's thread is ALREADY resolved, so
			// zero writes occur, yet the receipt still references the thread so it
			// validates against the frozen schema (operations minItems:1).
			name: "already_clear_is_idempotent",
			seed: func(f *scriptedForge) {
				f.SeedThread("note/9001", botID, reviewMarker(), true)
			},
			wantOpID:      "note/9001",
			wantOpenAfter: 0,
		},
		{
			// Branch: the idempotent arm ALSO rescans. If that rescan listing
			// fails, the already-clear state is unconfirmed — refuse.
			name: "already_clear_rescan_list_error",
			seed: func(f *scriptedForge) {
				f.SeedThread("note/9001", botID, reviewMarker(), true)
				f.listErrAt = 2 // the post-write rescan listing
			},
			wantErrSub:    "rescan list bot threads",
			wantOpenAfter: 0,
		},
		{
			// Branch: ResolveThread refuses. No receipt, and the thread stays open.
			name: "resolve_error_fails_closed",
			seed: func(f *scriptedForge) {
				f.SeedThread("note/9001", botID, reviewMarker(), false)
				f.resolveErr = errForgeUnavailable
			},
			wantErrSub:    "resolve no-longer-desired note/9001",
			wantOpenAfter: 1,
		},
		{
			// Branch: PARTIAL CLEAR. ResolveThread reports success but the thread
			// is still open on the forge. Success must never be reported without
			// forge confirmation — the rescan catches the lie.
			name: "partial_clear_rescan_mismatch",
			seed: func(f *scriptedForge) {
				f.SeedThread("note/9001", botID, reviewMarker(), false)
				f.resolveSilentNoop = true
			},
			wantErrIs:     forge.ErrRescanFailed,
			wantOpenAfter: 1,
		},
		{
			// Branch: two open threads occupy the cleared slot. Which to resolve is
			// undecidable here, so refuse with ZERO writes rather than guess.
			name: "duplicate_open_threads_fail_closed",
			seed: func(f *scriptedForge) {
				f.SeedThread("note/9001", botID, reviewMarker(), false)
				f.SeedThread("note/9003", botID, reviewMarker(), false)
			},
			wantErrSub:    "duplicate open threads",
			wantOpenAfter: 2,
		},
	}
	// Positive control: a table that lost its cases would sweep an empty set and
	// pass vacuously.
	if len(cases) != 8 {
		t.Fatalf("table lost cases: have %d, want 8", len(cases))
	}
	// Both polarities must actually be present: at least one refusal and at least
	// one success, else "both-polarity" would be a claim the table cannot back.
	var errCases, okCases int
	for _, tc := range cases {
		if tc.wantErrIs != nil || tc.wantErrSub != "" {
			errCases++
		} else {
			okCases++
		}
	}
	if errCases < 5 || okCases < 2 {
		t.Fatalf("table must keep both polarities: %d error cases, %d success cases", errCases, okCases)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &scriptedForge{Forge: fake.New(botID, "src", "tgt", "sha256:merge")}
			// A healthy, unrelated slot on the same MR — must survive every case.
			f.SeedThread("note/9500", botID, otherSlotMarker(), false)
			tc.seed(f)

			receipt, err := forge.Reconcile(f, testClock(), desired, forge.Preconditions{})

			switch {
			case tc.wantErrIs != nil:
				if !errors.Is(err, tc.wantErrIs) {
					t.Fatalf("err = %v, want errors.Is %v", err, tc.wantErrIs)
				}
			case tc.wantErrSub != "":
				if err == nil {
					t.Fatalf("want an error containing %q, got success with receipt %+v", tc.wantErrSub, receipt)
				}
				if !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Fatalf("err = %v, want it to contain %q", err, tc.wantErrSub)
				}
			default:
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}

			if tc.wantErrIs != nil || tc.wantErrSub != "" {
				// Fail-closed: a refusal fabricates no operation.
				if len(receipt.Operations) != 0 {
					t.Fatalf("a refusal must return ZERO operations, got %+v", receipt.Operations)
				}
			} else {
				if len(receipt.Operations) != 1 {
					t.Fatalf("want exactly one operation, got %+v", receipt.Operations)
				}
				op := receipt.Operations[0]
				if op.TargetID != tc.wantOpID {
					t.Fatalf("operation targetId = %q, want %q", op.TargetID, tc.wantOpID)
				}
				// The receipt must satisfy the frozen schema — this is the reason
				// the idempotent arm references a thread instead of returning empty.
				raw, mErr := json.Marshal(receipt)
				if mErr != nil {
					t.Fatalf("marshal receipt: %v", mErr)
				}
				if vErr := validateReceipt(t, raw); vErr != nil {
					t.Fatalf("receipt does not validate against the frozen schema: %v", vErr)
				}
			}

			if got := openForSlot(t, f, slot); got != tc.wantOpenAfter {
				t.Fatalf("open threads for the cleared slot = %d, want %d", got, tc.wantOpenAfter)
			}
			// Slot scoping: the unrelated healthy slot is never touched.
			if got := openForSlot(t, f, otherSlot); got != 1 {
				t.Fatalf("the unrelated slot's open thread must survive, open = %d want 1", got)
			}
		})
	}
}
