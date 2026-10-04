# P5-XREV — final consolidated review remediation: spec delta

**REQ prefix:** `REQ-XREV-S0n-nn`.
**Levels:** L0–L3 per ADR-0006.

This is a change delta, not a new epic: it adds requirements over existing shipped surfaces.
The authoritative evidence for every finding is the origin report
(`data/assent-xconsol-final/report.md`); citations below are that report's. Where a `Test:`
artifact is shared by several REQs, the artifact covers all of them (the "one test artifact
per REQ" rule in `openspec/config.yaml` is read as one artifact per REQ, not one REQ per
artifact — as the shipped `p5-aud2` spec also does).

---

## XREV-S01 — Consolidate the trust boundary at the engine entry [autonomous · engine-grade]

**As a** maintainer relying on the decision engine's fail-safe guarantees **I want** the
three dominating guards (reserved-class BLOCK, opaque/empty REVIEW, empty-`require` refusal)
enforced by a single engine-level entry every caller uses **so that** a new engine consumer
cannot silently re-create a partial boundary or reach a vacuous APPROVE (D2, folds D6).

**Goal:** add `aggregate.Decide(req DecideRequest)` as the one guarded engine entry. It
applies, in order: reserved-class → BLOCK; opaque or empty changeset → REVIEW; empty
`require[]` → error; otherwise the profile-aware coverage loop (empty precedence/profiles =
no write authority, the safe default, so one entry serves all three callers). Route
`run.decide`, `compare.evaluate` and `adoptertest.Evaluate`'s **decidable** path through it.
Delete the compiler-proven dead walking-skeleton chain (`Aggregate`, `failSafe`, `newCELEnv`,
`bindActivation`, `evalRule`, and the `Rule`/`Binding`/`OnFailure` types); keep the 3-arg
`Cover` (test-only callers). Retarget the classify golden test and the tokenless field scan
at the live entry.

**Operator input:** none.

**Dependencies:** none. Decision-path change; the reviewer treats it as engine-grade.

**Definition of done:** an empty-`require[]` binding evaluated through `run`, `compare` and
`adoptertest` returns an error, not APPROVE; an opaque/empty changeset returns REVIEW; a
reserved-class subject returns BLOCK — each proved through the shared entry; `go build ./...`
is green with the dead chain deleted (proving no caller remained); the classify golden and
tokenless tests pass against the new input type; and reverting the empty-`require` guard
reddens the `compare` negative test.

Requirements:

- **REQ-XREV-S01-01** — Given a binding whose `require[]` is empty, when `Decide` is invoked
  through a live entry (`run`'s `decide`, `compare.evaluate`), then it returns a non-nil error
  naming the empty `require` — never APPROVE.
  - Test: `internal/core/aggregate/decide_test.go` (engine entry);
    `internal/compare/decide_vacuity_test.go` (the live consumer)
  - Verify: `go test ./internal/core/aggregate/... ./internal/compare/...`
  - Level: L1
- **REQ-XREV-S01-02** — Given an opaque or empty changeset, when `Decide` is invoked, then it
  returns REVIEW (never a silent APPROVE), carrying the `aggregate.changeset` finding with the
  supplied subject.
  - Test: `internal/core/aggregate/decide_test.go`
  - Verify: `go test ./internal/core/aggregate/...`
  - Level: L1
- **REQ-XREV-S01-03** — Given a reserved-class (`assent-policy`) subject, when `Decide` is
  invoked, then it returns BLOCK before any predicate evaluation, dominating a would-be
  satisfiable predicate.
  - Test: `internal/core/classify/assent_policy_golden_test.go` (retargeted at `Decide`)
  - Verify: `go test ./internal/core/classify/...`
  - Level: L1
- **REQ-XREV-S01-04** — Given the dead walking-skeleton chain is deleted, when the tree is
  built, then no non-test caller references it (the deletion compiles) and the tokenless field
  scan reads the live decision-input type.
  - Test: `go build ./...`; `internal/core/decision/tokenless_test.go` (retargeted at
    `aggregate.DecideRequest`)
  - Verify: `go build ./... && go test ./internal/core/decision/...`
  - Level: L1
- **REQ-XREV-S01-05** — Given the empty-`require` guard is reverted, when the `compare`
  vacuity test runs, then it fails (non-vacuity: the test measures the guard, not the input).
  - Test: mutation performed and recorded in the story's review evidence
  - Verify: remove the empty-`require` branch from `Decide`; `go test ./internal/compare/...`
    reddens
  - Level: L1
- **REQ-XREV-S01-06** — Given `adoptertest`'s undecidable path, when it is evaluated, then it
  still returns a **bare** REVIEW with no findings (the frozen fixture shape), while its
  decidable path routes through `Decide`.
  - Test: `internal/adoptertest/evaluate_test.go` (existing opaque/empty cases stay green)
  - Verify: `go test ./internal/adoptertest/... && task dogfood-examples`
  - Level: L2

---

## XREV-S02 — Arm the matcher mirror with a parity test [autonomous]

**As a** maintainer of the `--coverage` harness **I want** the harness's rule-match decision
to share the engine's predicate **so that** a divergence cannot silently mis-credit an
obligation as covered (D3).

**Goal:** export the engine's match predicate (`aggregate.MatchesAny`, wrapping
`matchChanges`) and delete `adoptertest`'s hand-maintained `ruleMatchesAny` clone, so the
harness and engine agree by construction. Keep a differential parity test over the four match
domains.

**Operator input:** none.

**Dependencies:** none.

**Definition of done:** `adoptertest/coverage.go` no longer carries its own match switch;
`go test ./internal/adoptertest/... ./internal/core/aggregate/...` is green; inlining a
diverging predicate in the parity test reddens it.

Requirements:

- **REQ-XREV-S02-01** — Given the same rule match and change set, when the harness and the
  engine evaluate coverage, then they select the same matched set (one shared predicate).
  - Test: `internal/adoptertest/match_parity_test.go`
  - Verify: `go test ./internal/adoptertest/...`
  - Level: L1
- **REQ-XREV-S02-02** — Given the shared predicate's semantics are changed (a match domain
  inverted), when the harness test suite runs, then it fails. The mutation control is the
  per-domain expectation table (`TestMatchesAnyDomains`), which independently encodes the
  expected match result for every domain and both fail-closed error branches — inverting a
  domain in `MatchesAny`/`matchChanges` reddens it. `TestMatchesAnyParityWithEngine` separately
  proves the shared export's selection agrees with what the engine actually evaluated through
  `aggregate.Cover`, so the export cannot decouple from `matchChanges`. (A two-implementation
  parity oracle is deliberately NOT used: the clone is deleted, so there is no second
  implementation to diverge from; the per-domain table is the semantic oracle.)
  A static guard (`TestHarnessUsesSharedMatcher`) additionally fails if
  `adoptertest/coverage.go` stops calling `aggregate.MatchesAny` or reintroduces a
  local match predicate, closing the re-clone regression path.
  - Test: `internal/adoptertest/match_parity_test.go` (per-domain expectation table + engine
    agreement); `internal/core/aggregate/matches_any_test.go` (engine-package copy);
    `internal/adoptertest/no_clone_test.go` (structural guard)
  - Verify: `go test ./internal/adoptertest/... ./internal/core/aggregate/...`
  - Level: L1

---

## XREV-S03 — Doc-truth batch [autonomous]

**As a** maintainer who relies on the docs as the next session's context **I want** the
README, C4, vision, internal README, `--config` docs and provenance comments to match the
tree **so that** stale status text stops misleading (D4, D5, D10-vision, D12, D13, D14).

**Goal:** fix the README maturity table (Rego row → Planned with the D-141 qualifier; E10 row
cites D-140; the `task check` comment names the real gate), the `assent run --config` flag
help + `docs/usage/cli.md` row (the Config drives fact resolution; posture validation is the
side effect), the vision `hack/kind/` claim (link, not claim), `internal/README.md`'s
purity-enforcement line, the C4 `internal/core/hash` row, and the `go.mod`/rego provenance
comments. Extend `hack/docs/truthlag_pins_test.sh` with pins that redden when the old text
returns, each with a positive control.

**Operator input:** none.

**Dependencies:** none.

**Definition of done:** each corrected line reads true against the tree at HEAD; each new pin
has a control proving it can fail; `bash hack/docs/truthlag_pins_test.sh` passes.

Requirements:

- **REQ-XREV-S03-01** — Given the README maturity table, when it is read, then the Rego row
  is Planned with the D-141 qualifier, the E10/GitHub row cites D-140, and the `task check`
  comment names the real gate; a pin reddens if the old rows return.
  - Test: `hack/docs/truthlag_pins_test.sh` (README-vs-meta-plan pin + control)
  - Verify: `bash hack/docs/truthlag_pins_test.sh`
  - Level: L1
- **REQ-XREV-S03-02** — Given a user-facing status surface (`README.md`,
  `docs/vision.md`, `docs/architecture/c4-container.md`, `docs/architecture/c4-context.md`),
  when it pairs `E10` or `E11` with `D-012` without also naming the later unlock
  (`D-140`/`D-141`), then the truth-lag gate fails; a synthetic temp-file pairing reddens the
  detector (positive control, D-124/D-167). Historical mentions in ADRs/openspec specs that
  record the epic *was* locked under `D-012` are out of scope by design.
  - Test: `hack/docs/truthlag_pins_test.sh` (pairing detector + temp-file control)
  - Verify: `bash hack/docs/truthlag_pins_test.sh`
  - Level: L1
- **REQ-XREV-S03-03** — Given `docs/vision.md`, when the `hack/kind/` sentence is read, then
  it links to the directory rather than claiming a working cluster setup.
  - Test: `hack/docs/truthlag_pins_test.sh` (vision claim pin)
  - Verify: `bash hack/docs/truthlag_pins_test.sh`
  - Level: L1
- **REQ-XREV-S03-04** — Given `internal/README.md` and `docs/architecture/c4-container.md`,
  when the purity-enforcement and `internal/core/hash` rows are read, then they name the
  shipped gates (`depguard` + `purity_test.go`) and the landed `compare` importer.
  - Test: `hack/docs/truthlag_pins_test.sh`
  - Verify: `bash hack/docs/truthlag_pins_test.sh`
  - Level: L1
- **REQ-XREV-S03-05** — Given `go.mod` and `examples/policies/rego/bounded_change.rego`,
  when their provenance comments are read, then neither asserts a superseded state (D-003
  placement; "gated until the Phase-4 adoption gate").
  - Test: `hack/docs/truthlag_pins_test.sh`
  - Verify: `bash hack/docs/truthlag_pins_test.sh`
  - Level: L1
- **REQ-XREV-S03-06** — Given `assent run --help` and `docs/usage/cli.md`, when the `--config`
  entry is read, then both state the Config is the fact-resolution input (posture validation
  is the side effect); a pin compares the two and reddens on drift.
  - Test: `hack/docs/truthlag_pins_test.sh` (help-vs-cli.md pin)
  - Verify: `bash hack/docs/truthlag_pins_test.sh`
  - Level: L1

---

## XREV-S04 — Cheap hygiene one-liners [autonomous]

**As a** maintainer **I want** the three low-cost hygiene defects closed **so that** a
fixture gate cannot silently green (D16), the public module path stops exposing a
`go install`-able exfiltrator (D17), and the shipped binary stops linking `testing/fstest`
(D18).

**Goal:** make the three vacuity `t.Skip`s in `schemas/fixtures_validate_test.go` fail on a
missing/empty/no-doc fixture tree (parameterising the walk root so the failure is testable);
add `//go:build ignore` to `hack/spikes/provider/maliciousexec/main.go`; replace
`testing/fstest` in `cmd/assent/provider_host.go` with a tiny in-repo `fs.FS` adapter.

**Operator input:** none.

**Dependencies:** none.

**Definition of done:** each fix is proved by its own test; `go build ./...` and `go test
./schemas/... ./cmd/assent/... ./hack/spikes/...` are green; the exfiltrator no longer
resolves under the public module path.

Requirements:

- **REQ-XREV-S04-01** — Given `examples/contracts/` is absent, empty, or has no
  apiVersion/kind doc, when the fixtures gate runs, then it FAILS rather than skipping.
  - Test: `schemas/fixtures_validate_test.go` (walk root parameterised; temp-tree cases)
  - Verify: `go test ./schemas/...`
  - Level: L1
- **REQ-XREV-S04-02** — Given the exfiltrator spike, when the tree is listed, then
  `hack/spikes/provider/maliciousexec` carries a `//go:build ignore` constraint and is
  excluded from `go list ./...` / `go test ./...`.
  - Test: `hack/spikes/buildtag_test.go` (parses the file's build constraint)
  - Verify: `go test ./hack/spikes/...`
  - Level: L1
- **REQ-XREV-S04-03** — Given `cmd/assent/provider_host.go`, when the binary's imports are
  read, then it does not import `testing/fstest`, and the resource-owner map still loads.
  - Test: `cmd/assent/no_fstest_test.go` (AST import scan of every non-test file in the package)
  - Verify: `go test ./cmd/assent/... && go vet ./cmd/assent/...`
  - Level: L1
