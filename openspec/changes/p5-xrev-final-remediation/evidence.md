# P5-XREV — review evidence

This file records the independent reviews and the mutation controls required by the
change's `Verify:` lines (S01-05, S02-02, S04-01, S04-02). All work is on branch
`fm/assent-impl-findings`; the base is `d835de4` (`main`).

## Spec review (one, before implementation)

A fresh reviewer that did not author the spec read the proposal/spec/design/tasks against
the origin report and the code. Findings addressed before implementation:

- **D1 (adoption path) is not fixable by the report's option (ii).** The reviewer verified
  the `assent run` path does not reconstruct governed collection entries
  (`changeSetForGoverned` uses document-mode `change.Diff`; `evaldecode.BuildEvaluationInput`
  leaves `EvalChange.Entry` nil; `bindLeafActivation` falls back to the scalar), so every
  pack's `entry.owner` rule routes to `predicate.error` → REVIEW regardless of filenames.
  D1 was therefore deferred whole, with the rationale recorded in the proposal.
- **The `Decide` design originally claimed `compare` would enforce all three guards.** Fixed:
  `compare` structurally carries neither class nor opacity, so it reaches only the
  empty-changeset and empty-require guards; the design and the `Decide` doc now say so.
- **Deleting the 3-arg `Cover` would break ~80 test call sites.** Fixed: `Cover` is kept (and
  marked deprecated for production callers); only the compiler-proven dead walking-skeleton
  chain is deleted.
- **A single `Decide` signature for three callers.** Fixed: `DecideRequest` carries the union
  (policy, binding, input, approval, ceiling, precedence, profiles); `Decide` always finishes
  at `CoverWithProfile`, whose empty-profile default is byte-identical to the old entries.
- **Pins that would redden before their fix landed, and an absence-only pin with no control.**
  Fixed: the walkthrough pin moved with D1's deferral; the E10/E11+D-012 pin gained a
  synthetic-pairing positive control.

## Implementation reviews (three, independent, fresh context)

Three reviewers (adversarial-correctness, security/trust-boundary, spec-compliance) read the
full diff `d835de4..HEAD` and the origin report. Convergent real findings, all fixed in
`:recycle: fix(review): address three independent implementation reviews`:

- **REQ-XREV-S02-02 did not match the shipped test.** The spec promised an "inlined
  independent predicate + deliberate-mismatch control"; the shipped test compares
  `MatchesAny` to `Cover`, which use the same `matchChanges`. The spec/design/tasks were
  amended to the sound mechanism actually shipped (a per-domain expectation table as the
  semantic oracle, plus engine agreement), and a structural guard
  (`TestHarnessUsesSharedMatcher`) was added against a future re-clone.
- **The credential field scan was weakened.** `"auth"` had been removed (to stop
  `MR.Author`/`IsAuthor` false positives). Fixed: `"auth"` restored with an exact-name
  exemption for the two metadata fields, `"creds"` added, and the control extended to prove
  `AuthHeader`/`Creds` are caught while `Author`/`IsAuthor` are not.
- **`Decide` overclaimed.** Its doc said no consumer could reach a vacuous APPROVE; the
  reserved-class and opaque guards read caller-supplied signals (the frozen input carries
  neither). Fixed: the caveat is stated; the empty-require error also restores the
  `(class, environment)` context.
- **`buildtag_test` accepted any always-false constraint.** Fixed: it now asserts the
  constraint is exactly `ignore` (a GOOS tag would otherwise pass).
- **Stale comments naming deleted symbols, and the four unguarded `Cover*` exports.** Fixed:
  comments corrected; the four entries carry deprecation notes pointing at `Decide`.
- **Missing tests.** Added `TestEvaluateEmptyRequireFailsClosed` (adoptertest DoD) and the
  `testing/fstest` import scan.

## Re-reviews (three, after the fixes)

Three fresh reviewers verified the fix commit. Residual items, all cleared in
`:recycle: fix(review): clear residual re-review findings`:

- Added the static re-clone guard; widened the fstest scan to every non-test file in
  `cmd/assent`; corrected the last stale comments (`evaluate.go`, `main.go`) and the
  `Decide` caveat wording; pinned the empty-require error in the adoptertest test.

## Mutation controls (executed)

| REQ | Mutation | Command | Result |
| --- | --- | --- | --- |
| S01-05 | delete the empty-`require` branch in `Decide` | `go test ./internal/compare/ -run TestCompareEmptyRequireFailsClosed` | **FAIL** (`returned nil error — the vacuous APPROVE was not closed`); restored → ok |
| S02-02 | invert `MatchesAny`'s `len(matched) > 0` → `== 0` | `go test ./internal/core/aggregate/ -run TestMatchesAnyDomains` | **FAIL** (domain rows mismatch); restored → ok |
| S04-02 | remove `//go:build ignore` from `maliciousexec/main.go` | `go test ./hack/spikes/ -run TestMaliciousExecIsBuildConstrained` | **FAIL** (`carries no //go:build constraint`); restored → ok |
| S04-01 | absent/empty/doc-less tree drives `validateContractsTree` | `go test ./schemas/ -run TestValidateContractsTreeFailsClosed` | asserts a recorded `Fatalf` for each degenerate tree (the helper's `fixtureTB` has no `Skip`, so a revert to skip cannot compile) |

## Final gates

- `go build ./...`, `go vet ./...`, `golangci-lint run ./...` — clean.
- `go test ./...` — green.
- `task coverage` — 91.1% (floor 91%).
- `bash hack/docs/truthlag_pins_test.sh` — green, including the XREV-S03 pins and the
  E10/E11+D-012 positive control.
- `task dogfood-examples`, `go test ./examples/comparison/...` — green.
