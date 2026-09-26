# P5-RVW — six-review remediation: empty `require` and unmatched edits never arm APPROVE

**Epic ID / REQ prefix:** `RVW` / `REQ-RVW-Snn-nn`. Cross-cutting remediation epic (same
naming class as `AUD` / `AUD2` / `PCS` / `SEC-SC`). Does **not** consume E10–E14.

**Problem**: a schema-valid `RulesetBinding` with an empty or absent `require:` decides
**APPROVE with zero findings** for every governed change its block rules do not fire on. The
obligation layer iterates `bind.Require`, so an empty list makes it vacuous — there is no
positive vouch at all, and the run still arms approve/merge once the forge-probed arming
preconditions are met. The repo already states the invariant it is missing in two places:

- `GUIDELINES.md` §Safety-1: "an empty, broken, or non-matching policy set never auto-merges
  anything. Every change must be positively vouched."
- `internal/core/decision/record.go` P3 seam note (S03 review F3): the CLI wiring "must
  guarantee `require` is non-empty before an APPROVE is armed."

Nothing enforces it. The frozen v1alpha1 schema does not put `require` in the binding's
`required` list and carries no `minItems`, and its description plus D-021 bless the shape as
"Absent or empty ⇒ no required obligations (vacuously covered)". `assent lint` checks nothing
for an empty `require` (`internal/lint/coverage.go`'s obligation-coverage loop iterates the
list, so an empty list checks nothing). `cmd/assent/run.go`'s `selectBinding` checks the
binding *count* only. The result is a gate that stays clean on a document the schema accepts
and lint passes, while silently converting to a rubber stamp.

**Second problem (S02, the adversarial review's C-1)**: an **unmatched edit** fails open.
`cover()` marks a required obligation *covered* as soon as any enforce-phase rule names it in
`prove.obligation` — before it computes `matchChanges` — so a rule that names the obligation
but whose `match` selects none of the governed changes still marks it covered, contributes no
finding, and the decision falls through to APPROVE with an empty finding set. A value-level
edit outside every proving rule's match scope is not positively vouched, which GUIDELINES §1
and D-142's REQ-DEM-S10-02 both intend to be REVIEW. Reproduced in
`data/assent-adv-dsk/report.md` finding C-1.

**Decision (this epic):** the invariant wins. D-184 closes the empty `require` (S01). D-185
closes the unmatched edit (S02): an additive fail-safe escalation mirroring the D-063/D-064
unmatched-whole-file-DELETE guard. The frozen schema's `minItems: 1` is deferred to its next
change window, because the frozen `schemas/**/*.json` artifacts are under a ref-relative freeze
guard (D-132) that permits no edit here.

**Not in scope:** the `minItems: 1` schema change. Making `assent compare` refuse an
empty-require candidate: the comparison tool exists to compare permissive candidates, so it is
deliberately left able to read one. `assent test` (the adopter harness) does not arm APPROVE
and is unchanged. For S02, whole-file lifecycle events: a whole-file DELETE is the D-063/D-064
guard's subject and a whole-file ADD is non-destructive (D-063), so neither is an "edit".

**Lanes:** **A** run-path guard + polarity test (`cmd/assent/run.go`, `cmd/assent/run_test.go`) ·
**B** lint hard error + fixture pair (`internal/lint`, `examples/lint-fixtures`) ·
**C** reconciliation docs/decision row. **S02** engine escalation + regression test
(`internal/core/aggregate`), with the comparison corpus baseline corrected to govern the edit
it previously auto-approved.

---

## RVW-S01 — an empty `require:` never arms APPROVE `[autonomous · engine-adjacent]`

As an adopter, I want `assent run` to refuse to decide when my binding declares no required
obligations, and `assent lint` to tell me at authoring time, so that a forgotten or deleted
`require:` key cannot silently turn my merge gate into an auto-approver.

**Depends on:** none. **Do first.**

Acceptance criteria:

- Given a `RulesetBinding` whose covering binding has `require: []` (or omits `require:`), when
  `assent run` executes against a change its block rules do not fire on, then the run exits
  **non-zero**, writes **nothing** to the forge (no approval, no merge, no thread), and prints a
  contributor-readable error naming the binding `(class, environment)` and the missing
  obligations — it never emits a DecisionRecord with `decision: APPROVE`.
- Given the same empty-require document, when `assent lint` runs, then it emits exactly one
  **hard error** `binding-require-empty` located to the binding, and exits non-zero.
- Given a binding with at least one `require:` entry, when either command runs, then the new
  guard does not fire (no false positive on the conformant corpus).
- **Adversarial polarity** — the run-path test asserts the negative directly: the empty-require
  fixture is one the old code decided `APPROVE` with zero findings; the test fails if an
  approval or merge reaches the fake forge, or if the summary contains an `APPROVE` record.
- **Both polarities of the lint fixture** — `examples/lint-fixtures/binding-require-empty/good`
  lints clean; `.../bad` emits exactly `binding-require-empty`. The pair is registered in
  `internal/lint/exitgate_test.go`'s `hardErrorCorpus`, so deleting the check reds the E3-S08
  gate by name.

**Definition of done:** run-path guard landed; `binding-require-empty` in the lint pipeline,
the hard-error table, and the E3-S08 fixture corpus; D-184 records the reconciliation and the
deferred `minItems: 1`; `task check` green.

**Not in scope:** as the epic's *Not in scope* above.

Requirements:

- **REQ-RVW-S01-01** *(run-path guard)* — an empty-require binding fails closed before any
  forge write. Test: `cmd/assent/run_test.go`; Verify:
  `go test ./cmd/assent -run TestRunEmptyRequireNeverApproves -count=1`; Level: L1
- **REQ-RVW-S01-02** *(lint hard error)* — `assent lint` emits `binding-require-empty` for an
  empty/absent `require` and nothing for a non-empty one. Test:
  `internal/lint/coverage_test.go`; Verify:
  `go test ./internal/lint -run TestBindingRequireEmpty -count=1`; Level: L0
- **REQ-RVW-S01-03** *(fixture corpus)* — the `good`/`bad` fixture pair is in the E3-S08
  corpus. Test: `internal/lint/exitgate_test.go` + `examples/lint-fixtures/binding-require-empty`;
  Verify: `go test ./internal/lint -run TestEveryHardErrorFixtureCaught -count=1`; Level: L0
- **REQ-RVW-S01-04** *(reconciliation · doc)* — D-184 supersedes the D-021 "vacuously covered"
  clause and records the deferred schema `minItems: 1`; `docs/planning/lint-hard-errors.md`
  lists the new hard error; `docs/usage/cli.md` no longer presents empty `require` as a live
  APPROVE path on the run path. Test: those files; Verify:
  `rg 'binding-require-empty' docs/planning/lint-hard-errors.md && rg 'D-184' docs/decisions/decisions.md`;
  Level: doc

---

## RVW-S02 — an unmatched edit never arms APPROVE `[autonomous · engine-adjacent]`

As an adopter, I want a value-level change that no enforcing rule selects, under a binding
that requires an obligation some rule proves, to escalate to REVIEW rather than silently
APPROVE, so that a rule whose `match` is narrower than the binding's `require` cannot turn my
merge gate into an auto-approver for every change outside its scope.

**Depends on:** none. Engine lane; decision-path adjacent, independently reviewed.

Acceptance criteria:

- Given a binding that requires an obligation some enforce-phase rule proves, and a value-level
  change (`path != ""`) that no enforce-phase rule's `match` selects, when `Cover` evaluates,
  then the decision is **at least REVIEW** and exactly one synthetic `aggregate.unmatchedEdit`
  finding (`code: change.unmatchedEdit`, effect `require-review`) is emitted for the unvouched
  subject — it is never APPROVE with zero findings.
- Given the same binding and a change an enforce-phase rule **does** select, then the
  escalation does **not** fire: a clean-true match proves the obligation (APPROVE) and a
  clean-false match earns the rule's own effect.
- **Gate** — an empty/absent `require` (D-184's run-path seam) and a `require` with no proving
  rule (the uncovered-obligation guard's subject) do not trigger this escalation.
- **Scope** — whole-file lifecycle events are excluded: an ungoverned whole-file DELETE keeps
  the D-063/D-064 `aggregate.unmatchedDelete` escalation and an ungoverned whole-file ADD is
  non-destructive, so neither emits `aggregate.unmatchedEdit`.
- **Observe does not govern** — an observe-phase rule (or an enforce rule capped to observe by
  the pack ceiling) selecting the change does NOT suppress the escalation, because its findings
  are structurally excluded from the decision (the same D-063 fail-open the delete guard names).
- **Additive** — the escalation only ever raises the decision toward REVIEW via `worse()`; it
  never relaxes a BLOCK and never changes the decision of a fully governed changeset.

**Definition of done:** the escalation lands in `internal/core/aggregate/coverage.go` with
`editGoverned` and `requiredObligationCovered`; `unmatched_edit_test.go` reproduces the
zero-finding APPROVE and proves it now REVIEWs, with both polarities and the scope/observe
cases; D-185 records the mechanism; `task check` green.

**Not in scope:** as the epic's *Not in scope* above.

Requirements:

- **REQ-RVW-S02-01** *(engine escalation)* — an unmatched value-level edit under a binding that
  requires a rule-proven obligation escalates to REVIEW with one `aggregate.unmatchedEdit`
  finding. Test: `internal/core/aggregate/unmatched_edit_test.go`; Verify:
  `go test ./internal/core/aggregate -run TestUnmatchedEditFailsSafeReview -count=1`; Level: L0
- **REQ-RVW-S02-02** *(governed polarity)* — a matched edit is untouched (clean-true → APPROVE,
  clean-false → the rule's effect). Test: `internal/core/aggregate/unmatched_edit_test.go`;
  Verify: same; Level: L0
- **REQ-RVW-S02-03** *(gate + scope + observe)* — no-require, uncovered-require, whole-file
  delete/add and observe-phase cases do not emit `aggregate.unmatchedEdit`. Test:
  `internal/core/aggregate/unmatched_edit_test.go`; Verify: same; Level: L0
- **REQ-RVW-S02-04** *(corpus correction · doc)* — the `promotion-gates` baseline profile
  governs `/retentionMs` with a permissive rule, because the `challenge-intervention-added`
  case previously depended on the fail-open (an ungoverned edit auto-approving); D-185 records
  the mechanism. Test: `examples/comparison/promotion-gates/baseline.yaml`,
  `docs/decisions/decisions.md`; Verify:
  `go test ./examples/comparison/... -run TestCompareCorpusRunsGreen -count=1`; Level: L1

