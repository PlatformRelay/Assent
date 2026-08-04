# P5-E6 — Adopter test harness: `assent test` (+ `assent compare` seed)

**Problem**: E2 shipped the decision engine (`aggregate.Cover`/`CoverWithApproval`/`CoverWithPhaseCeiling`,
the frozen `policy.*` loader, fact tri-state, points, `require-review`, phase, profiles) and E3 shipped
the *static* authoring surface (`assent lint` + tests-per-rule *presence* + the rule catalogue). But the
one contract an adopter reaches for first — "prove my policy decides the way I think it does, without
wiring a live forge" — is still unexecutable. ADR-0014 freezes that contract (the `.assent/tests/**`
directory-case + inline `cases.yaml` fixture format, now schema-frozen at
`schemas/testfixture/v1alpha1/test-expectation.schema.json`), and E3-S06 asserts every rule *has* a test
file, but **nothing runs one**. The load-bearing gap is documented verbatim in
`cmd/assent/lint_corpus_test.go` (the E3-S08 exit gate): the on-disk `.assent/tests/**` fixtures are the
**flat authored shape** and do **not** map onto an `aggregate.EvaluationInput` as-is, for two concrete
reasons E3 reported rather than fabricated around — (1) **fact shape**: `facts.yaml` carries the authored
form (`author: {groups: [...]}`), not the resolved-fact **envelope** (`{state: resolved, value: ...}`)
`aggregate.factsToCEL`/`Fact` require; (2) **entry-tree binding**: `aggregate.bindLeafActivation` binds
`entry`/`oldEntry` to a *single change's* New/Old (a documented approximation), so an entry-scoped `files`
rule and a scalar `valueChanges` leaf rule cannot both be satisfied over one shared changeset. E3-S08
therefore fell back to a per-obligation single-rule `Cover` gate and **explicitly deferred the full
base/head→whole-pack replay to E6**. E6 owns that mapping and turns the frozen fixture format into a
runnable, dogfooded harness so the example packs gate *themselves* in CI.

**Scope**: an `assent test` subcommand (`cmd/assent/test.go`, the thin filesystem+command shell) over a
pure `internal/adoptertest/**` library that (S01) discovers directory cases under
`.assent/tests/<pack>/<case>/`, strict-decodes `expect.yaml` against the **frozen**
`test-expectation.schema.json`, maps `facts.yaml` (authored) → the resolved-fact envelope, diffs
`base/`↔`head/` with the **production** `change.Diff`/`DiffEntries`, and evaluates the pack via
`aggregate.Cover` to assert the expected `decision`; (S02) reconstructs the per-EntryRef **entry tree**
and threads `mr.yaml`→`aggregate.MR` + stubbed `ApprovalEvidence`→`aggregate.ApprovalContext`, so a
**whole multi-rule pack** replays over one `EvaluationInput` — **this closes the E3-S08 full-replay
deferral**; (S03) the expectation matcher (`findings` must-contain, `exact` closed-list, `absent`,
`score`, `message~`) with an `exact` safety default and fail-closed on any assertion it cannot evaluate;
(S04) the expected/actual **diff UX** with a ready-to-copy actual block; (S05) the `--update`
golden-refresh flow; (S06) the inline `cases.yaml` shorthand; (S07) `--coverage` with a per-rule
**both-polarity** requirement; (S08) the exit gate — every shipped `examples/packs/**/.assent/tests/**`
pack green, a deliberately-broken pack failing with the diff UX, and an `examples/` CI dogfood job. A
final seed story (S09) lands the smallest end-to-end `assent compare` (`cmd/assent/compare.go`) over the
frozen `schemas/comparison/**` + `replay-bundle` contracts; the full promotion-gate suite runner is
recommended as its own epic (see Judgment calls (f)).

**Non-goals** (fenced to their owning epic): **live provider fact resolution** — host/`max_age`/tokenless
fetch is **E5**; E6 reads `facts.yaml` **statically** and stubs the envelope, never calls a provider.
**Live forge halves** — fetching real `ApprovalEvidence`, arm/revoke, reconciliation — **E4/E8**; E6
injects a *stubbed* `ApprovalContext`. **Rendered-comment goldens** (`expect_comment.md`, `assent render`
vs `PresentationModel`) — **E8**; `message~` stays a discouraged substring assertion and is **not**
counted by `--coverage` (ADR-0014 amendment, frozen in the schema description). **Rego backend** —
`examples/policies/rego/**` stays `# locked: D-012`, excluded from the corpus (**E11**). **The full
`PolicyComparisonSuite` runner** (all six delta kinds, all five promotion gates, the `acceptedDeltas`
allowlist) — S09 seeds one honest end-to-end path; the remainder is a fast-follow cluster / its own epic.
**Authoring a new fixture schema** — the format is already frozen (`test-expectation.schema.json`); E6
**reuses it as the strict-decode authority**, never forks it.

**ADRs**: 0014 (adopter test format — the contract E6 executes; §"Runner semantics"
`assent test`/`--update`/`--coverage`/determinism; the 2026-07-21 amendment on
safety-vs-presentation assertions), 0006 (dogfooding — `examples/` packs keep their fixtures green), 0012
(hints-style ready-to-copy actual block), 0017 §2 (obligation coverage / satisfied-is-silent), §3
(`require-review` satisfied only by forge-proven `ApprovalEvidence`), §5 (entryRef subjects — the entry
tree S02 reconstructs), 0018 §2 (profiles — `assent compare` baseline↔candidate). **Reuse, do not
re-implement**: `internal/core/policy` (S01 strict loader; the `loadCatalogueInput(dir)` whole-pack reader
`assent catalogue`/`lint_corpus_test.go` already use), `internal/change` (`Diff`/`DiffEntries` — the
production differ ADR-0014 mandates), `internal/core/classify` (`Classify` — reserved `assent-policy`
domination), `internal/core/aggregate`
(`Cover`/`CoverWithApproval`/`CoverWithPhaseCeiling`/`CoverWithProfile`, `EvaluationInput`/`EvalChange`/`Fact`
envelope, `ApprovalContext`/`ApprovalEvidence`, `Result`/`Finding`), `cmd/assent/evaldecode.go`
(`buildEvaluationInput`/`decodeCanonical`/`subjectOf` — the change→typed-EvalChange decoder),
`schemas/testfixture/v1alpha1/test-expectation.schema.json` (the frozen expect/cases contract),
`schemas/comparison/v1alpha1/{comparison-record,comparison-suite}.schema.json` +
`schemas/decision/v1alpha1/replay-bundle.schema.json` (S09). **New**: `cmd/assent/{test.go,compare.go}`,
`internal/adoptertest/**`, minimal fixtures under `internal/adoptertest/testdata/**` (single-rule S01
case; `mr.yaml`+approval-stub S02 case; a deliberately-broken pack for S08).

**Executability**: **every story `[autonomous]`** — a pure case-loader/assembler/matcher library over the
E2/E1 primitives plus a `cmd/assent` shell that walks `.assent/tests/**` and prints results. No network,
no forge, no provider, no token: facts stubbed from authored text into the envelope, `ApprovalEvidence`
injected. `internal/adoptertest` sits under `internal/` (not `internal/core`), so — exactly like
`cmd/assent/evaldecode.go` — it may import `internal/change` and `internal/core/aggregate` while keeping
`internal/core` change-/forge-free; the only I/O (the case-directory walk) lives in `cmd/assent/test.go`,
the sanctioned boundary. Each case runs **twice**, byte-identical (ADR-0014 determinism, the golden L0
gate); no clock/env/random in the decision path.

**Judgment calls (decide-and-log)**:
(a) **Reuse the frozen fixture schema — DECIDED.** `schemas/testfixture/v1alpha1/test-expectation.schema.json`
already freezes both `expect.yaml` (`#/$defs/expectation`) and inline `cases.yaml` (`#/$defs/casesFile`)
as one `oneOf` contract. E6 strict-decodes against it (reuse frozen JSON schemas as the loader's
strict-decode authority) and authors **no** new schema — S01/S06.
(b) **`path` finding assertion — DECIDED: error-as-unsupported (safe fallback), engine field-add deferred.**
The frozen `finding` schema pins `path` (JSON Pointer, a structured safety assertion **counted by
`--coverage`**), but `aggregate.Finding` carries **no `Path` field** — it cannot be matched today. The
matcher **fails closed** on a `path` assertion (erroring "path assertion unsupported", never silently
passing — a silent pass is a silent-approve of a wrong decision). Threading `path` onto the emitted
`Finding` is a **decision-path change** (`aggregate.Finding` is produced in `internal/core`); per the
advisor (2026-08-04) it does **not** ride in as a matcher line-item — if wanted it is its **own** small,
fail-safety-reviewed engine lane (E6-S02-adjacent or a fast-follow), not folded into S03. S03 ships the
safe fallback; the fixtures simply do not use `findings[].path` until that engine lane lands (log an OQ).
(c) **Both-polarity supersedes the retired-vouch clause (decide-and-log, S07).** ADR-0014 scopes the "≥1
case where the predicate does **not** hold" requirement to `vouch` rules — but `effect: vouch` is
**retired** (the frozen finding enum rejects it). The operator brief requires both-polarity for **every**
rule; the on-disk fixtures already carry a `negative/` sibling per case. **Recommend**: adopt every-rule
both-polarity as the successor to the moot vouch-scoped clause; log the divergence.
(d) **Two corpora / filename reconciliation (decide-and-log, S08).** The seed manifest
(`archetype-goldens.md`) points at `examples/archetypes/<archetype>/expected.yaml`; the adopter packs live
at `examples/packs/**/.assent/tests/**/expect.yaml`. Filenames differ (`expected.yaml` vs `expect.yaml`)
and only the latter is what `assent test` discovers (a repo's `.assent/tests/**`). **Recommend**: E6's
exit gate runs the **adopter format** (`examples/packs/**/.assent/tests/**`, `expect.yaml`,
schema-conformant); the `examples/archetypes/**` manifest stays the P3-E3 golden seed (feeding S08's
manifest cross-check, not a second discovery root). Log whether `archetypes/` is also driven or left to
its internal golden test.
(e) **`--update` must preserve authored comments (S05).** Real `expect.yaml` files carry explanatory
comments (`# partitions 12 -> 16 ...`); a naive re-marshal clobbers them and defeats the review-by-diff
purpose that is `--update`'s only justification. S05 rewrites the expectation block in place, preserving
surrounding comments, and refuses to run under a dirty/`--update` combination that would mask a real
regression (overwrite safety).
(f) **`assent compare` scope (decide-and-log / RECOMMEND own epic).** `ReplayBundle` carries a
**pre-built** `evaluationInput` + `pins`, so `assent compare` reuses `CoverWithProfile` + the frozen
`comparison-suite`/`comparison-record` schemas and shares **zero** code with the S01/S02 base/head
assembler. A one-delta/one-gate slice is not independently *valuable* (you cannot promotion-gate on one of
six kinds). **Recommend** the full suite runner (six-kind taxonomy classifier, five-gate table,
`acceptedDeltas` allowlist, exit-code contract) becomes its own epic; S09 lands only the smallest honest
end-to-end path to de-risk the reuse and name the remainder.

**Dependency order**: **S01** (scaffold + directory-case loader + facts→envelope + single-case decision —
anchor, closes the *fact-shape* half of the E3-S08 deferral) → **S02** (entry-tree reconstruction +
`mr.yaml`/approval seam + whole-pack replay — **closes the full E3-S08 replay deferral**) → **S03**
(matcher; dep S01+S02) → {**S04** diff UX (dep S03), **S05** `--update` (dep S03), **S06** inline
`cases.yaml` (dep S01)} → **S07** `--coverage` both-polarity (dep S02+S03) → **S08** exit gate (dep
S01–S07; E3 lane-C conformant packs already landed). **S09** `assent compare` seed depends only on the E2
loader + `CoverWithProfile` + the frozen comparison/replay schemas — parallelizable with the whole `assent
test` chain, sequenced last. **First slice: S01.** **The story that closes the E3-S08 full-replay
deferral: S02.**

---

## E6-S01 — `assent test` scaffold + directory-case loader + facts→envelope mapping + single-case decision assertion [autonomous]

**As a** policy author **I want** `assent test` to run one `.assent/tests/<pack>/<case>/` directory case —
diffing my `base/`↔`head/` with the real differ, stubbing my `facts.yaml`, and evaluating my pack — and
tell me whether it reached the `decision` my `expect.yaml` pins **so that** I can prove a policy decides
correctly before wiring any forge.

**Goal**: an `assent test <repo>` subcommand (`cmd/assent/test.go`) dispatched from `main.go` that
discovers directory cases under `.assent/tests/**`, reads each case's files into memory (the FS walk —
the only I/O — lives in `cmd/assent`), and hands them to a new pure `internal/adoptertest` package that:
(1) strict-decodes `expect.yaml` against the **frozen** `test-expectation.schema.json`
(`#/$defs/expectation`) — reuse, not a new schema; (2) maps the **authored** `facts.yaml`
(`author: {groups: [...]}`) into the resolved-fact **envelope** `map[string]map[string]aggregate.Fact`
(`{State:"resolved", Value:...}`) the engine binds — the exact fact-shape translation
`lint_corpus_test.go` documented as deferred; (3) diffs `base/`↔`head/` with the production `change.Diff`
and builds a typed `aggregate.EvaluationInput` via the reused `buildEvaluationInput`/`decodeCanonical`;
(4) loads the pack via `loadCatalogueInput` and evaluates through `aggregate.Cover`, asserting the produced
`Decision` equals `expect.yaml`'s `decision`. Scope the demo to a **minimal single-rule/leaf fixture**
under `internal/adoptertest/testdata/**` — whole multi-rule shipped packs need S02's entry tree and are
out of scope here.

**Operator input**: yes — confirm/log (a) `assent test` is a subcommand, not a second binary; (b) reuse of
the frozen `test-expectation.schema.json` as the strict-decode authority (no new schema).

**Dependencies**: E2 (`policy` loader, `aggregate.Cover`/`EvaluationInput`/`Fact`), E1 (`change.Diff`),
`cmd/assent/evaldecode.go`, the frozen `test-expectation.schema.json`. Closes the **fact-shape** half of
the E3-S08 deferral.

**Definition of done**: a single-rule case whose `head` proves the obligation exits 0 asserting
`decision: APPROVE`; the same rule's failing `head` asserts the fixture's `REVIEW`/`BLOCK`; a malformed
`expect.yaml` (unknown field, bad `decision` enum) is rejected by the frozen-schema strict decode with a
located error; the authored `facts.yaml` is faithfully lifted into the resolved envelope (a `resolved`
fact exposes `.value`, an omitted fact never fabricates one); the case runs **twice** byte-identical;
determinism gate green over `internal/adoptertest`.

**Not in scope**: whole multi-rule shipped packs / entry-scoped `files` rules (S02); the
finding/`absent`/`score` matcher (S03); diff UX (S04); `--update` (S05); inline `cases.yaml` (S06);
`--coverage` (S07).

Requirements:
- **REQ-E6-S01-01** — a single-rule directory case builds an `EvaluationInput` from `base/`↔`head/` (via `change.Diff` + `buildEvaluationInput`) and its authored `facts.yaml` mapped to the resolved envelope, then `Cover` reproduces the `expect.yaml` `decision`. Test: `internal/adoptertest/case_test.go`; Verify: `go test ./internal/adoptertest/... -run TestDirectoryCaseEvaluatesToExpectedDecision`; Level: L0
- **REQ-E6-S01-02** — authored `facts.yaml` → resolved-fact envelope: a present fact becomes `{State:"resolved", Value:...}` exposing `.value`; an absent provider/name yields no `Fact` (never a fabricated resolved value that could APPROVE on an unresolved fact). Test: `internal/adoptertest/facts_test.go`; Verify: `go test ./internal/adoptertest/... -run TestAuthoredFactsMapToResolvedEnvelope`; Level: L0
- **REQ-E6-S01-03** — `expect.yaml` strict-decodes against the frozen `test-expectation.schema.json`; an unknown field or a non-`{APPROVE,REVIEW,BLOCK}` `decision` is a located rejection (reuses the frozen contract, authors no new schema). Test: `internal/adoptertest/expect_load_test.go`; Verify: `go test ./internal/adoptertest/... -run TestExpectYamlStrictDecodedAgainstFrozenSchema`; Level: L0
- **REQ-E6-S01-04** — `assent test <repo>` wired into `main.go`: discovers `.assent/tests/**`, runs each case, prints pass/fail, exits 0 all-pass / non-zero on any mismatch. Test: `cmd/assent/test_test.go`; Verify: `go test ./cmd/assent/... -run TestTestCommand`; Level: L1
- **REQ-E6-S01-05** — determinism: each case runs twice byte-identical; no clock/env/net/random; determinism gate green over `internal/adoptertest`. Test: `internal/adoptertest/case_test.go`; Verify: `go test ./internal/adoptertest/... -run TestCaseDoubleRunStable`; Level: L0

## E6-S02 — ⚠️ DECISION-PATH lane: per-EntryRef entry-object binding in `bindLeafActivation` + whole-pack assembler (closes the E3-S08 full-replay deferral) [autonomous · engine-grade review]

> **⚠️ This story changes `internal/core/aggregate/evaluate.go` (`bindLeafActivation`) — the fail-safe
> decision path (`TestCorePurity`-scanned, first-class security surface per `SECURITY.md`).** It is NOT
> a pure adopter-test-harness story; the advisor flagged (2026-08-04) that closing the E3-S08 deferral
> is *engine-side*, because `bindLeafActivation` currently binds `entry: toCEL(ch.New)` (the change's
> scalar `New`), not the reconstructed entry object. **Split into two lanes, each with its own fresh
> independent reviewer; the Part-A reviewer is pointed explicitly at fail-safety — a richer `entry`
> binding must never turn a REVIEW/BLOCK into a silent APPROVE.** Do the engine lane (Part A) FIRST,
> gate + review + merge it as an engine change, THEN the harness lane (Part B). Do not let Part A ride
> in under "adopter test harness".

**As a** policy author **I want** `assent test` to replay my **whole** pack — entry-scoped `files` rules
and scalar `valueChanges` leaf rules together — over one case's `base/`↔`head/` **so that** a case
exercises the same multi-rule aggregation a real MR would, not one rule in isolation.

**Goal — Part A (ENGINE, decision-path lane)**: extend `EvalChange` with an optional reconstructed
`Entry`/`OldEntry` (the whole entry object for the change's EntryRef) and change `bindLeafActivation` to
bind CEL `entry`/`oldEntry` to those **when present**, falling back to the current
`toCEL(ch.New)`/`toCEL(ch.Old)` **when absent** — so every existing evaluation is byte-identical and only a
populated entry object changes the binding. This removes the documented `evaluate.go:75` approximation.
**Fail-safety is the load-bearing property**: an ambiguous / unreconstructable / absent entry must fall to
the current safe behaviour, never fabricate an entry that could let an entry-scoped predicate bind
permissively (a fail-OPEN). TDD at the `aggregate` level (an `EvalChange` carrying an `Entry` binds
`entry.<field>`; one without it preserves the scalar binding exactly).
**Goal — Part B (HARNESS, input-side)**: in `internal/adoptertest`, reconstruct the per-EntryRef entry
tree from a case's `base/`↔`head/` via `change.DiffEntries` + the pack's `Entry` config
(`identity.pointer`) and populate the Part-A `EvalChange.Entry`/`OldEntry`; thread `mr.yaml`→`aggregate.MR`
(ADR-0014 defaults when absent); inject a **stubbed** `ApprovalEvidence` (optional case `approval.yaml`)
via `aggregate.ApprovalContext{Evidence: map[subject]*ApprovalEvidence}` so `require-review` can be
*satisfied* without a live forge; drive the whole pack via `CoverWithApproval`/`CoverWithPhaseCeiling`. Add
a minimal multi-rule fixture and a `require-review` fixture under `internal/adoptertest/testdata/**`.

**Operator input**: yes — 🔴 DECIDE + log (decisions.md) that closing the E3-S08 deferral is a
decision-path change to `bindLeafActivation`, implemented as an **additive, fail-safe-fallback** binding
(populated entry → bind object; absent → current scalar behaviour), landed as its own engine lane ahead of
the harness. Log an OQ only if the entry-key reconstruction is ambiguous for a `mode: document` pack.

**Dependencies**: E6-S01; E1 (`change.DiffEntries`, `EntryConfig`), E2 (`EvalChange`/`bindLeafActivation`,
`CoverWithApproval`/`CoverWithPhaseCeiling`, `ApprovalContext`/`ApprovalEvidence`, `MR`). **This story
closes the full E3-S08 base/head→whole-pack replay deferral** (Part A is the engine half `lint_corpus_test.go`
documented as deferred; S01 closed the fact-shape half).

**Definition of done**: **(Part A)** an `EvalChange` carrying a reconstructed `Entry` binds CEL
`entry.<field>` to the entry object; an `EvalChange` WITHOUT one preserves the exact current
`entry==new`/`oldEntry==old` scalar binding (every pre-S02 test byte-identical — a regression here is a
decision-path regression); an absent/ambiguous entry never fabricates a permissive binding (fail-safe);
`TestCorePurity` green; reviewed as an engine change pointed at fail-safety. **(Part B)** a multi-rule pack
where the case satisfies an entry-scoped ownership rule **and** a scalar bounded-change leaf rule over one
`base/`↔`head/` evaluates to `APPROVE` (both bindings correct simultaneously — the case
`lint_corpus_test.go` proved impossible under the approximation); a `mode: list` case reconstructs each
entry by `identity.pointer`; `mr.yaml` populates `aggregate.MR` (absent → defaults); a `require-review`
obligation is satisfied by a stubbed sha-matched `ApprovalEvidence` and **unsatisfied** (→ REVIEW) when the
stub is absent or sha-mismatched (fail-safe direction); double-run byte-identical.

**Not in scope**: the finding/`absent`/`score` matcher (S03); live `ApprovalEvidence` fetch or forge
reconcile (E4/E8); provider fact resolution (E5). Part A adds ONLY the optional binding + fallback — no
new decision semantics, no new effect, no points/threshold change.

Requirements:
- **REQ-E6-S02-01** *(Part A · ENGINE · decision-path)* — `EvalChange` gains optional `Entry`/`OldEntry`; `bindLeafActivation` binds CEL `entry`/`oldEntry` to them when present and to the current `toCEL(ch.New)`/`toCEL(ch.Old)` when absent; an entry-scoped predicate (`entry.<field>`) binds correctly for a populated entry. Test: `internal/core/aggregate/bindentry_test.go`; Verify: `go test ./internal/core/aggregate/... -run TestBindLeafActivationBindsEntryObjectWhenPresent`; Level: L0
- **REQ-E6-S02-02** *(Part A · ENGINE · fail-safe)* — an `EvalChange` with NO reconstructed entry preserves the exact pre-S02 scalar binding (all existing `aggregate` tests byte-identical); an absent/ambiguous entry never fabricates a permissive binding; `TestCorePurity` + the full `aggregate` suite green. Test: `internal/core/aggregate/bindentry_test.go`; Verify: `go test ./internal/core/aggregate/... && go test ./... -run TestCorePurity`; Level: L0
- **REQ-E6-S02-03** *(Part B · HARNESS)* — a whole multi-rule pack with an entry-scoped `files` rule + a scalar `valueChanges` leaf rule replays over ONE `EvaluationInput` (harness reconstructs + populates `Entry` via `change.DiffEntries`) and evaluates to the fixture decision — both bindings correct simultaneously (the documented `lint_corpus_test.go` blocker). Test: `internal/adoptertest/entrytree_test.go`; Verify: `go test ./internal/adoptertest/... -run TestWholePackReplayBindsEntryAndScalar`; Level: L0
- **REQ-E6-S02-04** *(Part B · HARNESS)* — a `mode: list` case reconstructs entries keyed by `identity.pointer`; `entry`/`oldEntry` bind the reconstructed entry object, not the raw change scalar. Test: `internal/adoptertest/entrytree_test.go`; Verify: `go test ./internal/adoptertest/... -run TestEntryTreeReconstructionKeyedByPointer`; Level: L0
- **REQ-E6-S02-05** *(Part B · HARNESS)* — a stubbed sha-matched `ApprovalEvidence` (case `approval.yaml`) satisfies a `require-review` obligation via `ApprovalContext`; an absent or sha-mismatched stub leaves it unsatisfied → REVIEW (never satisfied by absence). Test: `internal/adoptertest/approval_stub_test.go`; Verify: `go test ./internal/adoptertest/... -run TestStubbedApprovalEvidenceSatisfiesRequireReview`; Level: L0
- **REQ-E6-S02-06** *(Part B · HARNESS)* — `mr.yaml` populates `aggregate.MR` (author/labels/target); an absent `mr.yaml` applies the ADR-0014 defaults; determinism double-run stable. Test: `internal/adoptertest/mr_test.go`; Verify: `go test ./internal/adoptertest/... -run TestMrYamlPopulatesMRWithDefaults`; Level: L0
- **REQ-E6-S02-07** *(Part B · HARNESS · fail-safety — Part-A review F2 carry-forward)* — the harness reconstructs the **FULL** entry object (every field of the reconstructed entry), NEVER a partial/empty entry: the Part-A review flagged that `has(entry.<field>)` on an empty/partial map returns `false` cleanly (whereas on the old scalar binding it errored), so a `has(entry.x) ? … : true`-style predicate could take a MORE-permissive branch if `Entry` were populated partially. A `mode: list` entry whose reconstruction cannot be completed must leave `Entry` **nil** (fall back to the fail-safe scalar binding) rather than bind a partial object. Test: `internal/adoptertest/entrytree_test.go`; Verify: `go test ./internal/adoptertest/... -run TestPartialEntryNeverBindsPermissively`; Level: L0

## E6-S03 — Expectation matcher: findings must-contain / `exact` closed-list / `absent` / `score` / `message~`, fail-closed [autonomous]

**As a** policy author **I want** my `expect.yaml` findings, absences, and score arithmetic checked against
what the engine actually emitted **so that** a test pins *which* rule fired with *which* effect, not just
the coarse decision.

**Goal**: over the S02 `aggregate.Result`, implement the frozen-schema matcher: `findings[]` **must-contain
by default** (each listed finding — `rule` + `effect` (+ optional `obligation`, `path`) — must appear;
others may too); `exact: true` → the closed list (nothing else may fire); `absent[]` → named rules must
**not** fire; `score{total,threshold}` pins the arithmetic; `message~` a **discouraged** substring/regex
match on the rendered message. The matcher **fails closed** on any assertion it cannot evaluate — never
silently passes. **`path` reconciliation is DECIDED (Judgment call (b)): error-as-unsupported** — the
matcher errors on a `findings[].path` assertion ("unsupported") rather than silently passing, and S03 does
**not** add a `Path` field to `aggregate.Finding` (that is a decision-path change reserved to its own
fail-safety-reviewed engine lane). Fixtures avoid `path` until that lane lands.

**Operator input**: no (Judgment call (b) is pre-decided: error-as-unsupported; no `internal/core` change
in S03).

**Dependencies**: E6-S01, E6-S02 (whole-pack findings).

**Definition of done**: a must-contain `findings[]` passes when every listed finding fired (extra findings
allowed) and fails naming the missing one; `exact: true` fails when an unlisted finding also fired;
`absent` fails when a named rule fired; `score` mismatch fails with the expected/actual arithmetic; a
`message~` matches the rendered message as substring/regex; an assertion the matcher cannot evaluate (per
the `path` decision) **errors** (fail-closed), never passes; `exact` defaults to must-contain (an omitted
`exact` never silently closes the list); double-run stable.

**Not in scope**: rendered-comment goldens / `assent render` (E8 — `message~` stays a discouraged
substring, not counted by `--coverage`); diff UX (S04).

Requirements:
- **REQ-E6-S03-01** — must-contain (default): every `findings[]` entry (`rule`+`effect`) must fire, extras allowed; a missing listed finding fails naming it. Test: `internal/adoptertest/match_test.go`; Verify: `go test ./internal/adoptertest/... -run TestFindingsMustContainDefault`; Level: L0
- **REQ-E6-S03-02** — `exact: true` closes the list (an unlisted firing finding fails); an omitted `exact` never silently closes it (the frozen must-contain default). Test: `internal/adoptertest/match_test.go`; Verify: `go test ./internal/adoptertest/... -run TestExactClosedListVsMustContainDefault`; Level: L0
- **REQ-E6-S03-03** — `absent[]` fails when a named rule fires; `score{total,threshold}` fails on an arithmetic mismatch reporting expected vs actual. Test: `internal/adoptertest/match_test.go`; Verify: `go test ./internal/adoptertest/... -run TestAbsentAndScoreAssertions`; Level: L0
- **REQ-E6-S03-04** — an assertion the matcher cannot evaluate (per the logged `path` decision) **errors** the case (fail-closed), never silently passes. Test: `internal/adoptertest/match_test.go`; Verify: `go test ./internal/adoptertest/... -run TestUnevaluableAssertionFailsClosed`; Level: L0
- **REQ-E6-S03-05** — determinism: canonical assertion ordering, double-run byte-identical. Test: `internal/adoptertest/match_test.go`; Verify: `go test ./internal/adoptertest/... -run TestMatcherDoubleRunStable`; Level: L0

## E6-S04 — Failure UX: expected/actual diff + ready-to-copy actual block [autonomous]

**As a** policy author whose test just failed **I want** the runner to show me expected vs actual decision,
the finding diff, and a ready-to-paste actual block **so that** I can see exactly what changed and, if
intended, copy the new expectation in one step (ADR-0012 hints style).

**Goal**: on any case failure, render a deterministic, located report: the expected vs actual `decision`; a
finding-level diff (missing / unexpected / effect-mismatched); and a **ready-to-copy** `expect.yaml` actual
block (the same serialization `--update` would write, so a human can hand-copy it). Wire it into
`cmd/assent/test.go` output; keep the pure formatting in `internal/adoptertest`.

**Operator input**: no.

**Dependencies**: E6-S03.

**Definition of done**: a decision mismatch prints expected≠actual + the ready-to-copy actual block; a
finding mismatch prints which findings were missing/unexpected/effect-wrong; the actual block is valid
against the frozen `test-expectation.schema.json` (round-trips); output is deterministic (double-run
identical); a passing run prints no diff.

**Not in scope**: writing the block to disk (S05); rendered-comment goldens (E8).

Requirements:
- **REQ-E6-S04-01** — a decision mismatch renders expected vs actual + a ready-to-copy actual block that itself strict-decodes against the frozen schema. Test: `internal/adoptertest/diff_test.go`; Verify: `go test ./internal/adoptertest/... -run TestDecisionMismatchRendersCopyableActual`; Level: L0
- **REQ-E6-S04-02** — a finding mismatch enumerates missing / unexpected / effect-mismatched findings, located to the rule. Test: `internal/adoptertest/diff_test.go`; Verify: `go test ./internal/adoptertest/... -run TestFindingDiffEnumeratesDeltas`; Level: L0
- **REQ-E6-S04-03** — a passing case prints no diff; a failing case exits non-zero via `assent test`. Test: `cmd/assent/test_test.go`; Verify: `go test ./cmd/assent/... -run TestTestCommandFailureUX`; Level: L1
- **REQ-E6-S04-04** — diff output deterministic (canonical ordering, double-run byte-identical). Test: `internal/adoptertest/diff_test.go`; Verify: `go test ./internal/adoptertest/... -run TestDiffOutputDoubleRunStable`; Level: L0

## E6-S05 — `--update` golden-refresh flow with comment-preserving write + overwrite safety [autonomous]

**As a** policy author **I want** `assent test --update` to write the current actuals into `expect.yaml` for
review-by-diff **so that** maintaining goldens is cheap — without clobbering my explanatory comments or
masking a regression.

**Goal**: an `--update` flag that, per failing case, rewrites the `expectation` block of `expect.yaml` with
the produced actuals, **preserving surrounding authored comments** (the real fixtures carry
`# partitions 12 -> 16 ...` — a naive re-marshal defeats review-by-diff, Judgment call (e)). Overwrite
safety: `--update` writes only the expectation payload, leaves passing cases untouched, and the written
file re-validates against the frozen schema. Recommend + log a guard against running `--update` in CI (it
would auto-accept regressions).

**Operator input**: yes — confirm/log the `--update` overwrite-safety posture (comment preservation; CI
guard).

**Dependencies**: E6-S03 (produces the actual block S05 writes).

**Definition of done**: `--update` on a failing case writes the actual expectation and the file re-validates
against `test-expectation.schema.json`; authored comments survive the rewrite; a passing case is left
byte-identical; a subsequent normal run over the updated file passes; the write is deterministic (same
input → same bytes).

**Not in scope**: inline `cases.yaml` update (fold once S06 lands, or note as follow-up); a git-clean
precondition beyond the logged guard.

Requirements:
- **REQ-E6-S05-01** — `--update` rewrites a failing case's expectation to the actuals; the file re-validates against the frozen schema and a re-run passes. Test: `internal/adoptertest/update_test.go`; Verify: `go test ./internal/adoptertest/... -run TestUpdateWritesValidExpectation`; Level: L0
- **REQ-E6-S05-02** — authored comments around the expectation block survive `--update` (review-by-diff preserved). Test: `internal/adoptertest/update_test.go`; Verify: `go test ./internal/adoptertest/... -run TestUpdatePreservesAuthoredComments`; Level: L0
- **REQ-E6-S05-03** — a passing case is left byte-identical under `--update` (no spurious churn). Test: `internal/adoptertest/update_test.go`; Verify: `go test ./internal/adoptertest/... -run TestUpdateLeavesPassingCasesUntouched`; Level: L0
- **REQ-E6-S05-04** — `--update` write is deterministic (same actuals → byte-identical file). Test: `internal/adoptertest/update_test.go`; Verify: `go test ./internal/adoptertest/... -run TestUpdateWriteDoubleRunStable`; Level: L0

## E6-S06 — Inline `cases.yaml` shorthand support [autonomous]

**As a** policy author with many one-field cases **I want** to write them compactly in
`.assent/tests/<pack>/cases.yaml` **so that** the common "one field changed" test doesn't need a full
base/head directory tree.

**Goal**: support the inline shorthand (`#/$defs/casesFile` of the frozen schema): a `cases.yaml` holding
`cases[]` with `{name, file, base, head, facts?, expect}`, where `base`/`head` are inline resource
contents (any shape; `null` for new/deleted-file cases) and `expect` reuses the exact `#/$defs/expectation`
(single source of truth). Each inline case feeds the **same** S01/S02 assembler + S03 matcher — the inline
form is only an alternate front-end (marshal `base`/`head` to bytes → `change.Diff` → the same pipeline).
Net-new format: there is **no** `cases.yaml` on disk yet, so this story authors the first fixture.

**Operator input**: no.

**Dependencies**: E6-S01 (reuses the assembler + matcher; inline is an alternate loader).

**Definition of done**: a `cases.yaml` with multiple cases strict-decodes against `#/$defs/casesFile` and
each case evaluates through the same pipeline to its `expect.decision`; a `null` `base` (new-file case) and
a `null` `head` (deleted-file case) diff correctly via the production differ; an unknown top-level key or a
malformed inline `expect` is a located rejection; double-run stable.

**Not in scope**: `--update` for inline cases (S05 follow-up); mixing directory + inline for the same case
name (log an OQ if the precedence is ambiguous).

Requirements:
- **REQ-E6-S06-01** — a `cases.yaml` with multiple inline cases strict-decodes against `#/$defs/casesFile` and each evaluates to its `expect.decision` via the shared pipeline. Test: `internal/adoptertest/cases_file_test.go`; Verify: `go test ./internal/adoptertest/... -run TestInlineCasesFileEvaluates`; Level: L0
- **REQ-E6-S06-02** — a `null` `base` (new file) and `null` `head` (delete) diff correctly via the production differ. Test: `internal/adoptertest/cases_file_test.go`; Verify: `go test ./internal/adoptertest/... -run TestInlineNewAndDeletedFileCases`; Level: L0
- **REQ-E6-S06-03** — a malformed `cases.yaml` (unknown key, bad inline `expect`) is a located rejection against the frozen schema. Test: `internal/adoptertest/cases_file_test.go`; Verify: `go test ./internal/adoptertest/... -run TestInlineCasesFileMalformedRejected`; Level: L0
- **REQ-E6-S06-04** — inline + directory forms share one matcher/assembler (an inline `expect` and a directory `expect.yaml` with identical content produce identical results); double-run stable. Test: `internal/adoptertest/cases_file_test.go`; Verify: `go test ./internal/adoptertest/... -run TestInlineAndDirectoryFormsAgree`; Level: L0

## E6-S07 — `--coverage` per-rule both-polarity requirement + coverage report [autonomous]

**As a** repo owner **I want** `assent test --coverage` to fail unless every rule is exercised by both a
proving case and a failing case **so that** a rule can never ship with only its happy path tested (the
run-time counterpart to E3-S06's static presence check).

**Goal**: a `--coverage` mode computing, per loaded rule, whether it is the **driving** finding in ≥1
**failing** case *and* satisfied (silently proven) in ≥1 **proving** case, counting only **structured
safety assertions** (decision, rule, effect, `findings[].path`, score) — **never** `message~` (frozen
schema/ADR-0014 amendment). Emit a deterministic coverage report and fail (non-zero) any rule missing a
polarity. **Decide-and-log** (Judgment call (c)): ADR-0014 scopes the negative-polarity requirement to
`vouch` rules, but `vouch` is retired — adopt **every-rule** both-polarity (the on-disk `negative/`
siblings already supply the failing polarity); log the supersession.

**Operator input**: yes — DECIDE + log every-rule both-polarity superseding ADR-0014's retired-vouch
clause.

**Dependencies**: E6-S02 (whole-pack per-rule findings), E6-S03 (assertion counting).

**Definition of done**: a pack where every rule has a proving + a failing case reports full coverage, exits
0; a rule tested only in the proving polarity fails `--coverage` naming the missing failing case; a rule
tested only via `message~` does **not** count as covered (safety assertions only); the report is
deterministic; the every-rule decision is logged.

**Not in scope**: static tests-per-rule *presence* (that is E3-S06, already shipped); rendered-comment
coverage (E8).

Requirements:
- **REQ-E6-S07-01** — a rule with both a proving and a failing case → covered; a rule with only the proving polarity → `--coverage` fails naming the missing failing case. Test: `internal/adoptertest/coverage_test.go`; Verify: `go test ./internal/adoptertest/... -run TestBothPolarityCoverageRequired`; Level: L0
- **REQ-E6-S07-02** — `--coverage` counts only structured safety assertions (decision/rule/effect/path/score); a case asserting only `message~` does not count toward coverage. Test: `internal/adoptertest/coverage_test.go`; Verify: `go test ./internal/adoptertest/... -run TestCoverageCountsSafetyAssertionsOnly`; Level: L0
- **REQ-E6-S07-03** — `assent test --coverage` exits 0 on a fully-both-polarity pack and non-zero on an under-covered one, with a deterministic report. Test: `cmd/assent/test_test.go`; Verify: `go test ./cmd/assent/... -run TestCoverageCommandExitCodes`; Level: L1
- **REQ-E6-S07-04** — coverage report deterministic (canonical rule ordering, double-run byte-identical). Test: `internal/adoptertest/coverage_test.go`; Verify: `go test ./internal/adoptertest/... -run TestCoverageReportDoubleRunStable`; Level: L0

## E6-S08 — Exit gate: every `examples/packs/**` pack green + broken-pack diff UX + dogfood CI [autonomous]

**As a** maintainer **I want** every shipped example pack to gate itself green under `assent test` in CI,
and a deliberately-broken pack to fail with the clear diff UX **so that** the harness is proven end-to-end
and the packs are trustworthy documentation (ADR-0006 dogfooding).

**Goal**: wire S01–S07 into the exit gate. (1) **Green corpus**: every non-locked
`examples/packs/**/.assent/tests/**` case (`service-catalog`, `infra-vars`, `topic-registry` — reconciling
the D-052 pin) evaluates via the full S02 whole-pack replay to its `expect.yaml`, and `--coverage`
both-polarity passes across the corpus. (2) **Broken-pack proof**: a deliberately-broken pack fixture fails
with the S04 expected/actual diff UX and a non-zero exit (mirrors E3's negative-fixture proof). (3)
**Dogfood CI**: an `examples/` CI job runs `assent test` on every pack (packs gate themselves). (4)
**Corpus reconciliation** (Judgment call (d)): document that the gate runs the adopter `.assent/tests/**`
format (`expect.yaml`); cross-check against the `examples/archetypes/**` seed manifest (`expected.yaml`)
and log the filename/root divergence. Whole gate double-runs byte-identical.

**Operator input**: yes — confirm/log the corpus reconciliation (adopter `.assent/tests/**` as the
discovery root; archetypes manifest cross-check) and the `topic-registry`/D-052 disposition.

**Dependencies**: E6-S01..S07; E3 lane-C conformant packs (already landed).

**Definition of done**: every non-locked `examples/packs/**` case is green under `assent test`;
`--coverage` passes corpus-wide; a deliberately-broken pack fixture fails with the diff UX and non-zero
exit; the `examples/` CI job runs `assent test`; the archetype-goldens manifest cross-check holds (or the
divergence is logged); the whole gate double-runs byte-identical.

**Not in scope**: `examples/policies/rego/**` (locked D-012, excluded); `assent compare` (S09);
rendered-comment goldens (E8).

Requirements:
- **REQ-E6-S08-01** — every non-locked `examples/packs/**/.assent/tests/**` case evaluates via the whole-pack replay to its `expect.yaml` decision + findings. Test: `cmd/assent/test_corpus_test.go`; Verify: `go test ./cmd/assent/... -run TestAllExamplePacksGreenUnderAssentTest`; Level: L1
- **REQ-E6-S08-02** — `--coverage` both-polarity passes across the whole example corpus. Test: `cmd/assent/test_corpus_test.go`; Verify: `go test ./cmd/assent/... -run TestExampleCorpusBothPolarityCoverage`; Level: L1
- **REQ-E6-S08-03** — a deliberately-broken pack fixture fails with the expected/actual diff UX and a non-zero exit (proving the failure path — mirrors E3's negative-fixture gate). Test: `cmd/assent/test_corpus_test.go`; Verify: `go test ./cmd/assent/... -run TestDeliberatelyBrokenPackFailsWithDiff`; Level: L1
- **REQ-E6-S08-04** — the `examples/packs/**` corpus reconciles with the `archetype-goldens.md` manifest (expected decisions agree); the `expect.yaml`/`expected.yaml` root divergence is logged. Test: `cmd/assent/test_corpus_test.go`; Verify: `go test ./cmd/assent/... -run TestExampleCorpusReconcilesArchetypeManifest`; Level: L1
- **REQ-E6-S08-05** — determinism: whole gate double-runs byte-identical; `assent test` exit codes correct end-to-end over clean + broken corpora. Test: `cmd/assent/test_corpus_test.go`; Verify: `go test ./cmd/assent/... -run 'TestAssentTestGateDoubleRun|TestAssentTestExitCodes'`; Level: L1

## E6-S09 — `assent compare` seed: one ReplayBundle, baseline↔candidate, one delta classified, one gate [autonomous]

**As a** policy owner promoting a candidate profile **I want** `assent compare` to evaluate a baseline vs a
candidate over an immutable ReplayBundle and classify the resulting decision delta **so that** the
promotion-gate machinery is proven end-to-end on the smallest honest slice.

**Goal**: a minimal `assent compare` (`cmd/assent/compare.go`) that loads one `ReplayBundle` (frozen
`schemas/decision/v1alpha1/replay-bundle.schema.json` — it carries a **pre-built** `evaluationInput` +
`pins`, so **no** base/head assembler is needed), evaluates it under a **baseline** and a **candidate**
`PolicyProfile` via the reused `aggregate.CoverWithProfile`, classifies the decision difference as exactly
one of the closed six-kind taxonomy (`schemas/comparison/v1alpha1/comparison-record.schema.json`), applies
**one** promotion gate from the frozen `comparison-suite` table, and maps the outcome to an exit code. A
difference matching none of the six kinds is a **hard classification error** (fail-closed). This story
shares **zero** code with S01–S08 (Judgment call (f)); it de-risks the reuse and names the remainder.

**Operator input**: yes — DECIDE + log whether the full `PolicyComparisonSuite` runner (all six kinds, all
five gates, the `acceptedDeltas` allowlist, the exit-code contract) becomes its own epic (recommended) or a
fenced E6 cluster.

**Dependencies**: E2 (`policy` loader, `CoverWithProfile`, profile resolution); the frozen
`comparison-record`/`comparison-suite`/`replay-bundle` schemas. Independent of S01–S08.

**Definition of done**: one ReplayBundle evaluated under baseline + candidate profiles produces two
decisions; a difference classifies as exactly one taxonomy kind; the chosen gate passes/fails
deterministically and maps to an exit code; a difference matching no kind is a fail-closed classification
error; an `explanation-only` (wording-only) delta never trips the gate; comparison is side-effect-free;
double-run byte-identical; the full-runner scope decision is logged.

**Not in scope**: the remaining five taxonomy kinds' full classifiers, all five gates, and the
`acceptedDeltas` allowlist (the fast-follow cluster / own epic); building ReplayBundles from base/head
(they are pre-built inputs).

Requirements:
- **REQ-E6-S09-01** — one ReplayBundle evaluated under baseline + candidate profiles via `CoverWithProfile` yields two decisions; their difference classifies as exactly one closed-taxonomy kind. Test: `internal/compare/compare_test.go`; Verify: `go test ./internal/compare/... -run TestCompareClassifiesOneDelta`; Level: L0
- **REQ-E6-S09-02** — a difference matching none of the six kinds is a hard classification error (fail-closed), never an unclassified pass. Test: `internal/compare/compare_test.go`; Verify: `go test ./internal/compare/... -run TestCompareUnclassifiableDeltaFailsClosed`; Level: L0
- **REQ-E6-S09-03** — the chosen promotion gate passes/fails deterministically and maps to an exit code; an `explanation-only` delta never trips the gate. Test: `cmd/assent/compare_test.go`; Verify: `go test ./cmd/assent/... -run TestCompareGateExitCodes`; Level: L1
- **REQ-E6-S09-04** — comparison is side-effect-free and double-runs byte-identical (no clock/env/net/random). Test: `internal/compare/compare_test.go`; Verify: `go test ./internal/compare/... -run TestCompareDoubleRunStable`; Level: L0
