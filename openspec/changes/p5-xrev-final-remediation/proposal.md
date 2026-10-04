# P5-XREV — final consolidated review remediation

**Change ID:** `p5-xrev-final-remediation`
**Origin:** `data/assent-xconsol-final/report.md` (final reconciled review of the two
independent consolidation passes `assent-xconsol-a` and `assent-xconsol-b`), verified at
`d835de4` on `main`.
**Vehicle:** spec-first, test-driven fix lanes plus one doc-truth batch. Each story closes
findings the report states **with its own reproduction**; the reproductions are lifted into
the `Verify:` lines below. No story invents scope beyond its finding.

---

## Problem

The consolidated review confirmed **19 defects (D1–D19) plus one non-defect (N1)**. This
change implements the report's **"shippable now with clear specs"** set — its recommended
actions 1–3 (minus D1, see below) plus the doc-truth batch — **and the cheap hygiene
one-liners** (the report's item 5, an intentional small extension beyond the shippable set)
— and records the rest as explicit follow-ups (Non-goals).

**D1 is deliberately deferred.** The report's fix option (ii) ("ship run-consumable packs
now") is **insufficient to deliver the walkthrough's promise**, and this change's own review
found the reason the report missed: the `assent run` path does not reconstruct governed
collection entries. `changeSetForGoverned` uses document-mode `change.Diff`
(`cmd/assent/run.go:491-496`) and `evaldecode.BuildEvaluationInput` leaves `EvalChange.Entry`
nil (`internal/evaldecode/evaldecode.go:125-142`), so every `entry.*` predicate binds the
scalar fallback (`internal/core/aggregate/evaluate.go:252-256,288`). The packs' `ownership`
rule — required by every pack — uses `entry.owner in facts.author.groups.value`, so the run
path routes it to `predicate.error` → REVIEW for all three packs regardless of the filenames.
Option (ii) therefore removes the *hard errors* but still lands on permanent REVIEW, not the
"merges, nobody was interrupted" the walkthrough promises. The durable fix is the DEM-S00
routing lane **plus** entry reconstruction on the run path (E1-S05/E1-S08 territory); it is a
feature, not a stopgap, and is deferred whole. D5 (the `--config` documentation) is
independent of D1 and is folded into the doc-truth batch.

## Scope

- **D2 (folds D6)** — one engine-level guarded `Decide` entry carrying the three dominating
  fail-safe guards; route `run`, `compare` and `adoptertest`'s decidable path through it;
  delete the compiler-proven dead walking-skeleton chain and its types.
- **D3** — export the engine's match predicate and delete `adoptertest`'s hand-maintained
  clone, with a differential parity test.
- **D4, D5, D10 (vision half), D12, D13, D14** — one doc-truth batch, plus truth-lag pins so
  the README maturity table and the `--config` docs can no longer silently contradict the
  tree.
- **D16, D17, D18** — cheap hygiene one-liners.

## Non-goals (deliberate follow-ups)

- **D1 — the documented adoption path.** Deferred whole (rationale above). It needs
  (a) DEM-S00 `(class, environment)` routing, (b) collection-entry reconstruction on the
  `run` path, and (c) facts resolution from the shipped config; it is its own spec-first lane.
  The report's option (ii) is recorded as insufficient, not implemented.
- **D7** (`failf` helper sweep), **D8** (`orchestrate` decomposition — rides the E10
  port-lift), **D11** (helper-clone cross-link), **D10 doctor half** (retire the env-only
  MET), **D15** (GitLab pagination generics) — the report assigns these to "ride the next
  touch"; none is opened as its own lane here.
- **D9** (decision-log split) and **D19** (open-questions table split) — process debt the
  report gives its own lane; out of scope.
- **N1** — no action (chosen property).
- Do not change frozen schemas, do not raise `COVERAGE_MIN`, do not retry/re-order forge
  calls.

## Counterpoints considered

- **D2 and the frozen adopter-test fixture format (ADR-0014).** `adoptertest`'s undecidable
  path returns a **bare** REVIEW with no findings, and the shipped fixtures document that
  (`examples/packs/infra-vars/.assent/tests/vars/tf-opaque/expect.yaml`). A `Decide` that
  synthesized the `aggregate.changeset` finding for that path would change a frozen public
  contract. So `Decide` returns the finding-bearing form (what `run` needs) and `adoptertest`
  keeps its bare-REVIEW early return for the undecidable case only; its **decidable** path
  routes through `Decide`, which is where the empty-`require` guard matters. The deliberate
  exception is recorded in the design.
- **D2 dead-code deletion.** Deleting the exported `Rule`/`Binding`/`OnFailure` types and
  `Aggregate` is compiler-proven safe (zero non-test callers) but touches the `tokenless_test`
  field scan and the classify golden test, which are retargeted at the live entry. The 3-arg
  `Cover` is **kept**: it has ~80 test call sites and no production caller; removing it is a
  separate mechanical lane. The alternative — leaving the whole dead copy — preserves the
  false assurance D2 is about, so the walking-skeleton chain is deleted.
- **D3 deletion vs parity gate.** Deleting the clone is preferred over keeping a second
  implementation plus a gate; the differential parity test is retained to catch a future
  divergence.
