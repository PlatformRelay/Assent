# P5-RVW — six-review remediation: empty `require` must never arm APPROVE

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

**Decision (this epic, D-184):** the invariant wins. An empty `require` is a policy-authoring
defect, and the CLI refuses to arm on it (fail closed, no forge write). The schema description
and D-021's "vacuously covered" clause are reconciled to the invariant by superseding decision
row; the schema's `minItems: 1` is deferred to its next change window, because the frozen
`schemas/**/*.json` artifacts are under a ref-relative freeze guard (D-132) that permits no
edit here.

**Not in scope:** the unmatched-edit fail-open (a rule that names an obligation but whose
`match` selects nothing still marks it covered) — that is a separate register row and is gated
on a held captain decision; it shares the `cover()` invariant but not this lane's fix. The
`minItems: 1` schema change. Making `assent compare` refuse an empty-require candidate: the
comparison tool exists to compare permissive candidates, so it is deliberately left able to
read one. `assent test` (the adopter harness) does not arm APPROVE and is unchanged.

**Lanes:** **A** run-path guard + polarity test (`cmd/assent/run.go`, `cmd/assent/run_test.go`) ·
**B** lint hard error + fixture pair (`internal/lint`, `examples/lint-fixtures`) ·
**C** reconciliation docs/decision row.

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
