# P5-E3 — Policy surface: `assent lint` hard errors + generated rule catalogue

**Problem**: E2 shipped and hardened the *decision* half of the pipeline — the frozen-contract loader
(`internal/core/policy`, strict decode + target-ref loading), the CEL leaf primitive
(`aggregate/evaluate.go:evalLeaf`), the `all`/`any`/`not` walker (`aggregate/asserttree.go`), the
multi-obligation coverage loop (`aggregate/coverage.go`), fact tri-state fail-safe, points/threshold,
`require-review`, phase, and profile resolution — all proven against the D-016 golden. But every
protection E2 lands is *decision-time*: a malformed or unsafe **policy** (a `require`d obligation no
rule proves, a rule routing the reserved `assent-policy` class to `vouch`, a controlling provider
configured `failure: open`, a `when` referencing an out-of-scope identifier, a rule with no test) is
only caught — if at all — when an MR triggers evaluation, and several are caught only as an opaque
strict-decode `error` from the loader that names a JSON path, not the offending rule. The `assent
lint` hard-error list (`docs/planning/lint-hard-errors.md`, authoritative) exists precisely because
ADR-0017's adversarial reviews found that letting the decision layer catch a policy defect leaves a
window where the policy itself is already unsafe in a way lint could have caught **before any MR**.
E3 is where that human-facing `assent lint` pass lands — a new `assent lint` subcommand over the
`.assent/**` authoring surface, plus the **generated rule catalogue** (D-017 B10), the single source
for generated docs and lint (no second handwritten registry). E3 needs **no** live infra: pure Go +
a CLI subcommand over fixtures already in `examples/**` and `schemas/**`.

**Scope**: an `assent lint` subcommand (`cmd/assent/lint.go`) that discovers a repo's `.assent/**`
tree and ingests it **tolerantly** (accumulating *all* diagnostics rather than dying on the first
strict-decode error — lint's whole job is the malformed packs the strict loader refuses), running the
`lint-hard-errors.md` hard errors as located, actionable diagnostics that fail the run (exit
non-zero) — never warnings, never contingent on what a predicate evaluates to (S01…S06). The checks
live in a pure, testable `internal/lint` package consuming the E2-loaded `policy.*` types and reusing
E2's compile/coverage/provider-posture/profile-resolution primitives; `cmd/assent/lint.go` is the thin
filesystem+command shell. Fold in the two items E2 lanes deferred "to E3 lint" — the S03 leaf-`message`
out-of-scope-template check (S04) and the S05 non-dot `facts` reference rejection (S03) — plus the
bare-`facts.x.y`-vs-`.value` fact-model question (S03, decide-and-log). Add the generated rule
catalogue (`internal/catalogue`, S07). Land a **pack-conformance lane C** correcting `examples/packs/**`
(add the required `phase`, apply the fact-model decision) so the exit gate's archetype packs load.
Close with the exit-gate story (S08).

**Non-goals** (fenced to their owning epic): the `assent test` adopter harness / expect-matcher /
`--update` / `--coverage` diff UX — **E6** (S06 asserts *statically* that every rule has ≥1 test on
disk; it does not run them; S08's "archetype packs evaluate to expected decisions" is an internal
`aggregate.Cover` gate, not `assent test`). `assent compare` / delta taxonomy — **E6**. Provider fact
*resolution* / host / `max_age` — **E5** (S05 reads authored `config.yaml` posture statically, never
calls a provider). Forge halves (fetching `ApprovalEvidence`, arm/revoke) — **E4/E8**. Renderer /
presentation template lint — **E8**. Rego / complex-rule backend — **E11** (`examples/policies/rego/**`
stays `# locked: D-012`, excluded from every corpus). Re-implementing the loader's strict-decode
refusals — lint catches the *same* violation *earlier, tolerantly, named to the offending rule*.

**ADRs**: 0010 (`assent lint` fails packs without tests; `.assent/**` layout), 0013 (frozen closed
predicate scope), 0014 (adopter test format / tests-per-rule source), 0015 §1 (policy MRs block-by-
default, may relax only to `challenge`, never `vouch`), 0016 §2 (unknown-field refs are load-time
errors), 0017 §2 (required obligations), §5 (unkeyed lists rejected), §6 (controlling facts never
fail open), 0018 §1 (`no-implicit-enforce-phase`), §2 (`single-writer-profile`); D-017 B2/B3 (phase +
profile lint), B10 (generated catalogue). **Reuse, do not re-implement**: `internal/core/policy`
(strict loader + types — lint decodes into the same `policy.*` types tolerantly),
`policy.ValidateProviderPosture` (E2-S05 fail-open scan — **widened** here per its own TODO),
`aggregate/{evaluate.go,asserttree.go}` (E2-S02/S03 CEL compile path — S04 adds a new exported
compile-only helper in `aggregate`, never in `policy` which is deliberately off cel-go),
`aggregate/coverage.go` (E2-S04 obligation vocabulary), `aggregate/profile.go:ResolveProfile`
(E2-S09), `internal/core/classify` (reserved `assent-policy` class + `ValidateRouting`). **New**:
`cmd/assent/{lint.go,catalogue.go}`, `internal/lint/**`, `internal/catalogue/**`, fixtures under
`examples/lint-fixtures/**` + `testdata/**`.

**Executability**: **every story `[autonomous]`** — pure Go check library over the E2-loaded types +
`cmd/assent` shells that walk `.assent/**` and print diagnostics. No network/forge/provider/token;
facts + approval posture read *statically* from authored text. Check logic pure (`TestCorePurity`
respected); the only I/O (the directory walk) lives in `cmd/assent`, the sanctioned boundary.

**Judgment calls (decide-and-log)**: (a) `assent lint` is a **subcommand**, not a second binary
(recommend; log an OQ only if an operator wants a `assent-lint` alias) — S01. (b) Lint ingests
**tolerantly** (fail-many), not via the strict E2 loader (the load-bearing architectural decision —
every check plugs into the accumulator) — S01. (c) **Fact-model `.value`** — DECIDED + logged.
**Resolution: Option B (D-051, which SUPERSEDES the initial D-049 Option A)** — the fact value is at
`facts.<p>.<n>.value` (bound only for a `resolved` fact; navigation goes THROUGH `.value`),
`.state`/`.expiresAt`/`.observedAt`/`.reason`/`.sensitive` are reserved envelope escapes, and every
other third segment (or a bare `facts.<p>.<n>`) is a `facts-reference-shape` error. Option A
(auto-unwrap: bare `facts.<p>.<n>` = the value, `.value` rejected) was reversed on engineering
grounds — it needs a custom cel-go fact type in the fail-safe decision path (a fresh fail-open
surface), whereas Option B needs ZERO engine change (`factsToCEL` + S10 already bind/prove the
envelope shape) and lane C rewrites the corpus regardless, voiding Option A's "don't touch the
corpus" cost. The security-critical `facts-reference-syntax` non-dot rejection is UNCHANGED — S03,
sequenced before S04. (d) Fail-open must **WIDEN** (S05) beyond the single
archetype `ValidateProviderPosture` catches today, per its own header TODO. (e) **Lane C** (E3's
analog to E2's lane F) adds the required `phase` + applies the S03 fact-model decision to
`examples/packs/**` (all 11 authored rule files omit `phase` today) — startable once S03 is decided,
lands before S08; edits authored *examples*, not a frozen contract.

**Dependency order**: S01 (scaffold + tolerant ingestion + obligation-coverage — anchor) → {S02
structural, S05 config-posture, S06 tests-per-rule}; S03 (fact-model decision + facts-ref lint —
closes S05 non-dot) → S04 (predicate-scope + message-template — closes S03 message-scope). S07
(catalogue) depends only on the E2 loader (parallelizable). Lane C depends on S03's decision, lands
before S08. S08 (exit gate) depends on S01–S07 + lane C. **First slice: S01.**

---

## E3-S01 — `assent lint` scaffold + tolerant ingestion + obligation-coverage hard error `[autonomous]`

**As a** repo owner **I want** to run `assent lint` over my `.assent/**` packs and be told, by name,
when a `RulesetBinding` requires an obligation that no bound rule proves **so that** I catch a policy
that can never satisfy its own safety requirement before it reaches a single MR.

**Goal**: an `assent lint` subcommand (`cmd/assent/lint.go`) dispatched from `main.go` that discovers
the `.assent/**` tree and ingests it **tolerantly** into the E2 `policy.*` types via a new pure
`internal/lint` package (a tolerant decode accumulating *all* problems, not aborting on the first
strict-decode error). Establish the diagnostic model — `Diagnostic{Code, Severity(error),
Location(file+rule/binding name), Message}` + a `Report` accumulator with deterministic (canonically
sorted) ordering and a non-zero exit when any error diagnostic is present. Wire the first hard error:
**obligation coverage** — every `require[]` name is `prove.obligation`'d by ≥1 bound rule (reuses the
E2-S04 vocabulary as a *static set* check, no `EvaluationInput`). Decide-and-log the subcommand +
tolerant-ingestion decisions.

**Operator input**: yes — confirm/log (a) subcommand not binary, (b) tolerant fail-many ingestion.

**Dependencies**: E2-S01 (the `policy.*` types), E2-S04 (obligation vocabulary).

**Definition of done**: a covered binding exits 0 clean; a binding requiring an unproven obligation
exits non-zero with one `obligation-coverage` diagnostic naming the binding + obligation; a pack with
two defects (strict-schema violation + uncovered obligation) reports **both** (tolerant fail-many);
diagnostic order deterministic; `TestCorePurity` green over `internal/lint` + reuse.

Requirements:
- **REQ-E3-S01-01** — uncovered `require[]` obligation → one located `obligation-coverage` error naming binding + obligation, non-zero exit (ADR-0017 §2). Test: `internal/lint/coverage_test.go`; Verify: `go test ./internal/lint/... -run TestObligationCoverageUncovered`; Level: L0
- **REQ-E3-S01-02** — a pack with two distinct defects → both reported in one run (tolerant fail-many, never first-error abort). Test: `internal/lint/coverage_test.go`; Verify: `go test ./internal/lint/... -run TestTolerantIngestionAccumulatesAllDiagnostics`; Level: L0
- **REQ-E3-S01-03** — fully-covered pack → no `obligation-coverage` diagnostic, exits 0 (no over-fire). Test: `internal/lint/coverage_test.go`; Verify: `go test ./internal/lint/... -run TestFullyCoveredPackLintsClean`; Level: L0
- **REQ-E3-S01-04** — `assent lint <dir>` wired into `main.go`: discovers `.assent/**`, runs the check library, prints located diagnostics, correct exit code. Test: `cmd/assent/lint_test.go`; Verify: `go test ./cmd/assent/... -run TestLintCommand`; Level: L1
- **REQ-E3-S01-05** — determinism: canonically sorted, double-run byte-identical, no clock/env/net/random, `TestCorePurity` green. Test: `internal/lint/coverage_test.go`; Verify: `go test ./internal/lint/... -run TestLintReportDoubleRunStable`; Level: L0

## E3-S02 — Structural hard errors: reserved-class, no-implicit-enforce-phase, unkeyed-lists `[autonomous]`

**Goal**: over the tolerant parse, three per-document checks: (1) **reserved-class** — a binding on
the reserved `assent-policy` class binding a pack whose rules resolve to a non-`block`/non-`challenge`
outcome → `reserved-class` (ADR-0015 §1; reuses `classify` reserved-class identity + `ValidateRouting`);
(2) **no-implicit-enforce-phase** — a rule or `Pack` missing `phase` → `no-implicit-enforce-phase`
naming the offender, pointing at `off`/`observe`/`enforce` (D-017 B2, ADR-0018 §1), surfaced
tolerantly before the strict loader aborts; (3) **unkeyed-lists** — a `mode: list` entry with no
`identity.pointer` → `unkeyed-list` (ADR-0017 §5).

**Operator input**: no. **Dependencies**: E3-S01.

Requirements:
- **REQ-E3-S02-01** — reserved `assent-policy` routed to a non-block/challenge outcome → `reserved-class` error; block/challenge lints clean. Test: `internal/lint/structural_test.go`; Verify: `go test ./internal/lint/... -run TestReservedClassRouting`; Level: L0
- **REQ-E3-S02-02** — rule/Pack missing `phase` → `no-implicit-enforce-phase` naming the offender; explicit phase clean; effect/onFailure workaround still flagged. Test: `internal/lint/structural_test.go`; Verify: `go test ./internal/lint/... -run TestNoImplicitEnforcePhase`; Level: L0
- **REQ-E3-S02-03** — `mode: list` without `identity.pointer` → `unkeyed-list`; `mode: document` + keyed list clean. Test: `internal/lint/structural_test.go`; Verify: `go test ./internal/lint/... -run TestUnkeyedListEntry`; Level: L0
- **REQ-E3-S02-04** — the three structural checks compose with S01 in one deterministic run; `TestCorePurity` green. Test: `internal/lint/structural_test.go`; Verify: `go test ./internal/lint/... -run TestStructuralChecksComposeDeterministic`; Level: L0

## E3-S03 — Fact-model `.value` decision + facts-reference lint (closes S05 non-dot deferral) `[autonomous]`

**Goal**: (1) **DECISION (decide-and-log)** — resolve whether an authored `facts.<p>.<n>`
addresses the typed value directly (auto-unwrap; `.state`/`.expiresAt` reserved escapes) or must be
`.value`; the corpus inconsistency (D-016 `.state` vs pack `facts.author.groups`) is the evidence.
**DECIDED: Option B — value at `.value`** (D-051, which SUPERSEDES the initial D-049 Option A;
reversed on engineering grounds — Option A needs a custom cel-go fact type in the fail-safe decision
path, Option B needs zero engine change). No E2 engine change is required (the current `factsToCEL`
already binds the envelope shape and S10 reproduces the D-016 golden with it); lane C applies the
convention to the authored packs.
(2) **facts-reference lint** — a `facts` reference not in dot-syntax (bracket index `facts['owner']`,
interior whitespace, any form the E2-S05 `factRefRe` scan would miss) → `facts-reference-syntax` (so
the S05 posture scan is sound by construction); a reference violating the chosen convention →
`facts-reference-shape`.

**Operator input**: yes — DECIDE + log the fact-model `.value` convention. **Dependencies**: E3-S01;
precedes E3-S04; informs lane C.

Requirements:
- **REQ-E3-S03-01** — non-dot `facts[...]` reference → `facts-reference-syntax` error (closes the S05 evasion — a controlling provider referenced only via `facts['owner']` is caught at lint). Test: `internal/lint/facts_ref_test.go`; Verify: `go test ./internal/lint/... -run TestNonDotFactsReferenceRejected`; Level: L0
- **REQ-E3-S03-02** — a reference violating the chosen `.value` convention → `facts-reference-shape`; conformant clean. Test: `internal/lint/facts_ref_test.go`; Verify: `go test ./internal/lint/... -run TestFactsReferenceShapeConvention`; Level: L0
- **REQ-E3-S03-03** — reserved envelope escapes (`facts.owner.team.state`) permitted, not flagged. Test: `internal/lint/facts_ref_test.go`; Verify: `go test ./internal/lint/... -run TestEnvelopeEscapeAccessorsPermitted`; Level: L0
- **REQ-E3-S03-04** — determinism: pure token/AST checks, `TestCorePurity` green, double-run stable. Test: `internal/lint/facts_ref_test.go`; Verify: `go test ./internal/lint/... -run TestFactsRefDoubleRunStable`; Level: L0

## E3-S04 — Predicate-scope + message-template lint (closes S03 message-scope deferral) `[autonomous]`

**Goal**: the **undeclared-predicate-scope** hard error over two surfaces: (1) compile each `when`/`cel`
leaf against the frozen 11-field scope via a **new exported compile-only helper** in `aggregate`
(reusing the E2-S02 env; never adding cel-go to `policy`) → `undeclared-predicate-scope`; (2) expand
each leaf/rule `message` `{{ }}` template and apply the same scope check → `message-template-scope`
(the E2-S03 deferral, one shared activation model, ADR-0013 residual #5).

**Operator input**: no. **Dependencies**: E3-S01, E3-S03 (settled `facts` convention).

Requirements:
- **REQ-E3-S04-01** — a leaf referencing an out-of-scope top-level identifier (`input.new`) → `undeclared-predicate-scope` naming it (the pre-fix D-016 typo caught statically). Test: `internal/lint/scope_test.go`; Verify: `go test ./internal/lint/... -run TestUndeclaredPredicateScopeInLeaf`; Level: L0
- **REQ-E3-S04-02** — each of the 11 frozen fields compiles clean; a twelfth invented field does not. Test: `internal/lint/scope_test.go`; Verify: `go test ./internal/lint/... -run TestFrozenScopeExactlyElevenFields`; Level: L0
- **REQ-E3-S04-03** — a `message` `{{ }}` referencing an out-of-scope field → `message-template-scope`; in-scope clean. Test: `internal/lint/scope_test.go`; Verify: `go test ./internal/lint/... -run TestMessageTemplateScope`; Level: L0
- **REQ-E3-S04-04** — the compile helper is exported from `aggregate` (not `policy`); `policy` stays cel-go-free; `TestCorePurity` green; double-run stable. Test: `internal/lint/scope_test.go`; Verify: `go test ./internal/lint/... ./internal/core/... -run 'TestScopeDoubleRunStable|TestCorePurity'`; Level: L0

## E3-S05 — Config-posture hard errors: fail-open (widened) + single-writer-profile `[autonomous]`

**Goal**: (1) **fail-open (WIDENED)** — reuse `policy.ValidateProviderPosture` for the ownership
archetype **and widen** to the two other controlling-provider archetypes (an `Entry.identity.pointer`
provider, an approval-eligibility provider) → `fail-open` when `config.yaml` declares `failure: open`
(ADR-0017 §6); sound because S03 guarantees dot-syntax facts refs. (2) **single-writer-profile** —
reuse `aggregate.ResolveProfile` to detect, per `(environment, class)`, zero or >1 write-authoritative
profile → `single-writer-profile` (ADR-0018 §2; D-017 B3), never last-one-wins.

**Operator input**: no. **Dependencies**: E3-S01, E3-S03. Reuses E2-S05/S09.

Requirements:
- **REQ-E3-S05-01** — a require-review-proof controlling provider `failure: open` → `fail-open`; advisory provider may fail open (clean). Test: `internal/lint/posture_test.go`; Verify: `go test ./internal/lint/... -run TestFailOpenControllingProofProvider`; Level: L0
- **REQ-E3-S05-02** — an entries-identity OR approval-eligibility provider `failure: open` → `fail-open` (the widening the E2-S05 scan doesn't reach). Test: `internal/lint/posture_test.go`; Verify: `go test ./internal/lint/... -run TestFailOpenEntriesIdentityAndEligibilityProviders`; Level: L0
- **REQ-E3-S05-03** — two `writes:true` profiles covering a binding → `single-writer-profile`; zero writers with `Config.profiles` present → also error; exactly one → clean. Test: `internal/lint/posture_test.go`; Verify: `go test ./internal/lint/... -run TestSingleWriterProfileInvariant`; Level: L0
- **REQ-E3-S05-04** — determinism via the precedence table; `TestCorePurity` green; double-run stable. Test: `internal/lint/posture_test.go`; Verify: `go test ./internal/lint/... -run TestPostureChecksDoubleRunStable`; Level: L0

## E3-S06 — Tests-per-rule hard error `[autonomous]`

**Goal**: the **tests-per-rule** hard error (ADR-0010/0014): statically map each loaded rule to its
test cases on disk (`.assent/tests/**` directory-form, or inline `cases.yaml`) and emit `tests-per-rule`
for any rule with zero cases. Static *presence* only — NOT the `assent test` runner (E6): never
executes a case, loads `facts.yaml`, or diffs decisions.

**Operator input**: no (log an OQ only if the rule→test-dir mapping is ambiguous). **Dependencies**: E3-S01.

Requirements:
- **REQ-E3-S06-01** — a rule with no case directory/inline case → `tests-per-rule` naming the rule; ≥1 matching case clean. Test: `internal/lint/tests_per_rule_test.go`; Verify: `go test ./internal/lint/... -run TestRuleWithoutTestRejected`; Level: L0
- **REQ-E3-S06-02** — the topic-registry pack (each rule has a `.assent/tests/**` dir) → no `tests-per-rule` (reads the real layout). Test: `internal/lint/tests_per_rule_test.go`; Verify: `go test ./internal/lint/... -run TestFullyTestedPackLintsClean`; Level: L0
- **REQ-E3-S06-03** — a rule whose only case references a *different* rule → still flagged (presence must be for that rule). Test: `internal/lint/tests_per_rule_test.go`; Verify: `go test ./internal/lint/... -run TestTestMustReferenceTheRule`; Level: L0
- **REQ-E3-S06-04** — determinism; presence-only (no case executed); `TestCorePurity` green; double-run stable. Test: `internal/lint/tests_per_rule_test.go`; Verify: `go test ./internal/lint/... -run TestTestsPerRuleDoubleRunStable`; Level: L0

## E3-S07 — Generated rule catalogue (D-017 B10) `[autonomous]`

**Goal**: a pure `internal/catalogue` package walking the E2-loaded `policy.*` types into a
deterministic `Catalogue`: per rule/obligation a **stable ID**, `docs.url`, rollout `phase`, required
facts/capabilities, classes + matcher domains, possible finding codes/effects, deprecation metadata.
Additive-tolerant (stable-ID keyed, canonically sorted — adding a rule extends without reorder).
Surfaced via `assent catalogue` (recommend a distinct subcommand; decide-and-log). The single source
for generated docs + lint — no second handwritten registry.

**Operator input**: yes — decide-and-log the catalogue surface. **Dependencies**: E2-S01 loader only (parallelizable).

Requirements:
- **REQ-E3-S07-01** — each rule/obligation catalogued with the D-017 B10 field set, derived from packs. Test: `internal/catalogue/catalogue_test.go`; Verify: `go test ./internal/catalogue/... -run TestCatalogueFieldsFromLoadedPack`; Level: L0
- **REQ-E3-S07-02** — adding one rule → additive (existing IDs/positions unchanged). Test: `internal/catalogue/catalogue_test.go`; Verify: `go test ./internal/catalogue/... -run TestCatalogueAdditiveTolerant`; Level: L0
- **REQ-E3-S07-03** — ~~a deprecated rule surfaces its deprecation metadata~~ **DEFERRED to OQ (E3-S07 review, D-048):** the frozen v1alpha1 contract carries NO lifecycle/`deprecated` field, and `phase: off` is the rollout ENTRY state (`off→observe→enforce`, ADR-0018 §1), NOT a retirement marker — inferring deprecation from `phase` would mislabel every new rule as deprecated in generated docs. The catalogue therefore fabricates NO lifecycle metadata; it surfaces `phase`/`effectivePhase` faithfully. Deprecation metadata is deferred until a real schema `lifecycle`/`deprecated` field exists. Test asserts the honest state. Test: `internal/catalogue/catalogue_test.go`; Verify: `go test ./internal/catalogue/... -run TestNoDeprecationMetadataInV1alpha1`; Level: L0
- **REQ-E3-S07-04** — `assent catalogue` wired into `cmd/assent` emits the report, exits 0. Test: `cmd/assent/catalogue_test.go`; Verify: `go test ./cmd/assent/... -run TestCatalogueCommand`; Level: L1
- **REQ-E3-S07-05** — determinism: canonically sorted, double-run byte-identical, no clock/env/net/random, `TestCorePurity` green. Test: `internal/catalogue/catalogue_test.go`; Verify: `go test ./internal/catalogue/... -run TestCatalogueDoubleRunStable`; Level: L0

## E3-S08 — Exit gate: hard-error corpus + archetype load/evaluate + catalogue generation `[autonomous]`

**Goal**: wire S01–S07 + lane C into the exit gate. (1) **hard-error corpus** under
`examples/lint-fixtures/**`: a positive + negative fixture per hard error, a table test asserting lint
emits *exactly* the expected code(s) on each negative and clean on each positive. (2) **archetype
load+evaluate**: after lane C, assert every non-locked `examples/packs/**` pack loads (E2-S01 loader)
and, built into an `EvaluationInput` from `base`/`head`/`facts`, evaluates via `aggregate.Cover` to
its `archetype-goldens.md`/`expected.yaml` `decision` — an **internal** `Cover` gate, NOT `assent
test`/E6. (3) **catalogue** generates from the loaded packs.

**Operator input**: no. **Dependencies**: E3-S01..S07 + lane C.

Requirements:
- **REQ-E3-S08-01** — each hard-error negative fixture triggers exactly its code; each positive clean. Test: `internal/lint/exitgate_test.go`; Verify: `go test ./internal/lint/... -run TestEveryHardErrorFixtureCaught`; Level: L0
- **REQ-E3-S08-02** — every non-locked archetype pack loads + evaluates via `Cover` to its expected `decision`. Test: `internal/lint/exitgate_test.go` / `cmd/assent/lint_corpus_test.go`; Verify: `go test ./... -run TestArchetypePacksEvaluateToExpectedDecision`; Level: L1
- **REQ-E3-S08-03** — the catalogue generates completely + deterministically over the loaded packs. Test: `internal/catalogue/catalogue_test.go`; Verify: `go test ./internal/catalogue/... -run TestCatalogueGeneratesFromArchetypeCorpus`; Level: L0
- **REQ-E3-S08-04** — a pre-lane-C pack (no `phase`) fails to load with a missing-phase reason (proving lane C is required — mirrors E2's lane-F-required proof). Test: `internal/lint/exitgate_test.go`; Verify: `go test ./internal/lint/... -run TestPreConformancePackFailsToLoad`; Level: L0
- **REQ-E3-S08-05** — determinism: whole gate double-runs byte-identical; `TestCorePurity` green; `assent lint`/`catalogue` exit codes correct end-to-end. Test: `cmd/assent/lint_corpus_test.go`; Verify: `go test ./cmd/assent/... ./internal/lint/... ./internal/catalogue/... -run 'DoubleRun|ExitCode'`; Level: L1

## Lane C — pack-conformance corrections (E3's analog to E2's lane F) `[autonomous]`

Add the required `phase` to each authored `examples/packs/**` rule/pack (all 11 rule files omit it
today) and apply the E3-S03 fact-model decision to authored `facts.*` references, so E3-S08's exit
gate runs over conformant packs. Startable once E3-S03 is logged; lands before E3-S08. Edits authored
*examples*, not a frozen contract — lighter than lane F (no `🔴 DECIDED` on a frozen artifact), but
log the corpus change.
