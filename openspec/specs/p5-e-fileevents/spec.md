# P5-EFE — Whole-file add/delete match domain (`match.fileEvents`)

**Epic ID / REQ prefix:** `EFE` / `REQ-EFE-S0n-nn`. (Not `E10` — E10 is Locked per D-012; D-052 names this
fast-follow **E-FILEEVENTS**, so `EFE` is the stable, collision-free prefix.)

**Problem**: The `match.fileEvents` CONTRACT is FROZEN but UNIMPLEMENTED. `policy.FileEventsMatch{Paths,Kinds}`
(`internal/core/policy/policy.go:113-117`), `Match.FileEvents` (`policy.go:98`), and the `fileEventsMatch`
`$def` (`schemas/policy/v1alpha1/merge-policy.schema.json`, `required:[paths,kinds]`, kinds ∈
`add|modify|delete|rename`) all exist — but three engine sites HARD-REJECT the domain: the loader
(`internal/core/policy/loader.go:32-33`, "deferred (E1 fast-follow)"), the engine matcher
(`internal/core/aggregate/coverage.go:451-452`), and the E6 `ruleMatchesAny` mirror
(`internal/adoptertest/coverage.go:301-302`). A whole-file add/delete is also OPAQUE at the differ today
(`internal/change/diff.go` — a nil/empty side has no diffable document → opaque → REVIEW). This epic
IMPLEMENTS the domain and closes two live gaps: **D-052** (topic-registry, `mode: document`, needs
`match.fileEvents{kinds:[delete]}` for whole-topic-delete `non-destructive`; currently PINNED as a
known-blocker by `TestExamplesPacksKnownBlockers` and EXCLUDED from the E6-S08 green corpus) and **D-061**
(service-catalog entry-removal→REVIEW diverges from the archetype manifest's file-delete→BLOCK).

**Key ground truth (de-risks the epic — no schema change):** the FROZEN `evaluation-input.schema.json`
ALREADY models a whole-file event — `change.path` is documented "empty string for a whole-file event",
`change.kind` enum includes `delete`, `entryRef` includes `"file:<path>"`. A file-event
`EvalChange{Subject:"file:x", File:"x", Path:"", Kind:"delete"}` validates against the frozen input schema
with **no schema change** (`path` has no `minLength`; `changeSet.minItems:1` is met by the single event).
Epic DoD includes **`git diff schemas/` == 0**. The discriminator between a whole-file event and a
value-level add/delete is **`path == ""`**.

**Scope**: (S01) loader accept + engine matcher + the `path==""` domain-disjointness invariant + the
mirror, engine-grade; (S02) the change-model file-event constructor + the E6 harness minting it from
one-sided presence + the **unmatched-file-delete fail-safe default** (Judgment call (a)); (S03) the
`cmd/assent` live-checkout population; (S04) unpin topic-registry (D-052) + service-catalog file-delete→BLOCK
+ reconcile the divergence (D-061); (S05) the exit gate.

**Non-goals** (fenced): **`modify` + `rename` fileEvents kinds** — REJECTED at load in v1 (Judgment call
(b)); a whole-file modify is the ordinary value-diff case (owned by `values`/`valueChanges`), and rename
detection is a later lane. **Any change to the FROZEN contract** — `FileEventsMatch`, the merge-policy
`fileEventsMatch` `$def`, and `evaluation-input.schema.json` are REUSED as-is (`git diff schemas/` == 0).
**Live forge/provider** — S03 reads a checkout's presence signal, calls no forge. **The
`evaluate.go`/`bindLeafActivation` CEL binding** — a file event binds `kind`/`file` (strings) + `path==""`
and nil `old`/`new`/`entry` under the EXISTING binding; no new scope field, no `bindLeafActivation` change
is expected (verified in S01, else it becomes its own fail-safety-reviewed sub-lane).

**ADRs**: 0017 §5 (the four-domain matcher split — `fileEvents` is the whole-file lifecycle domain,
authoritative), 0003/0011 (canonical change model), 0018 §1 (phase — the delete-guard runs `enforce`),
0007/0017 §2 (obligation coverage & the fail-open the loader guard closes), 0014 (adopter test format —
S04/S05 fixtures), 0006 (dogfooding). **Reuse, do not re-implement**: `internal/core/policy` (loader +
`FileEventsMatch`), `internal/core/aggregate` (`matchChanges`/`Cover*`), `internal/change` (`Diff`, the
`Change`/`ChangeSet` shape), `internal/evaldecode` (`BuildEvaluationInput`/`SubjectOf`), `internal/adoptertest`
(the `ruleMatchesAny` mirror), `internal/catalogue` (`catalogue.go:298` already NAMES `fileEvents` — no
change). **New**: a file-event constructor in `internal/change`; population in
`internal/evaldecode`/`internal/adoptertest`/`cmd/assent/checkout.go`; fileEvents fixtures under
`internal/adoptertest/testdata/**` and `examples/packs/topic-registry/**`.

**Executability**: **every story `[autonomous]`** — pure `internal/core` (loader + matcher), a pure
change-model constructor, pure harness population, and a `cmd/assent` checkout adapter gated with
`dirCheckout` over testdata (no live infra). TDD, deterministic, `TestCorePurity`-clean, each case
double-runs byte-identical.

**Judgment calls (decide-and-log / operator)**:
(a) **🔴 OPERATOR QUESTION (blocks S02, NOT S01) — the default decision for an UNMATCHED whole-file
delete.** The naive decomposition claimed you "cannot both emit a clean event AND preserve blanket-REVIEW
for the ungoverned case" — that is a **FALSE dichotomy** (advisor, 2026-08-04). Emitting a clean file-event
(so `fileEvents` rules CAN match) and defaulting an *unmatched* file-DELETE to REVIEW are **SEPARABLE**: the
clean event enables governance; the engine can STILL fail-safe on a delete that no `fileEvents` rule covers.
Today every whole-file delete is opaque → REVIEW. The concrete regression risk if an unmatched file-delete
were allowed to become APPROVE: a file whose **contents** are governed by `values`/`valueChanges` rules
(which by S01's `path!=""` disjointness never match a `path==""` delete) would get **ZERO** protection when
deleted wholesale — and on upgrade across EFE that case silently flips **REVIEW→APPROVE**. That is a
fail-OPEN on a destructive operation in a fail-safe-first project — a **product/behavior-contract choice**,
NOT a decide-and-log. **Default pending the operator's answer: an unmatched whole-file DELETE event → REVIEW**
(the fail-safe, additive, walk-back-able direction — an operator can later relax to APPROVE with no adopter
ever under-protected; the reverse silently under-protects now and tightening later is a breaking change in
the annoying direction). S02 implements REVIEW-default and MUST NOT bake in REVIEW→APPROVE as a silent
consequence of emitting a clean event. Raised as a 🔴 operator question at epic start.
(b) **DECIDED — loader fails CLOSED on un-emittable kinds (S01).** `coverage.go` marks a required obligation
`covered` when an enforce rule merely NAMES it, regardless of whether match selected a subject. So a required
obligation proven only by `fileEvents{kinds:[rename]}` or `{kinds:[modify]}` — kinds the caller never emits
as a `path==""` event — would be vacuously covered → APPROVE (fail-open). S01's loader accepts kinds ⊆
`{add, delete}` and **REJECTS `modify` + `rename` at load** — a fail-open-closing v1 narrowing beyond the
frozen schema, exactly as the loader today narrows by rejecting `fileEvents` wholesale. (Advisor-affirmed:
fail-closed narrowing, the safe direction; do not re-litigate.)
(c) **No frozen-schema change — DECIDED** (Key ground truth). `git diff schemas/` == 0 in the epic DoD.
(d) **Single shared file-event minter — DECIDED (S02).** `path==""` is the one shape the minter,
`matchChanges`, and the `ruleMatchesAny` mirror must agree on; a single pure constructor in `internal/change`
prevents the drift the `evaldecode` "one canonical decoder or the fail-open reopens" lesson warns about.

**Dependency order**: **S01** (loader accept + matcher + mirror + `path==""` disjointness — the engine lane,
hand-built inputs, NO semantic change to real evaluations; anchor) → **S02** (change-model constructor +
harness minting from one-sided presence + the unmatched-delete REVIEW-default — the input+fail-safety lane;
resolves Judgment call (a)) → {**S03** cmd live-checkout ∥, **S04** unpin topic-registry + service-catalog
BLOCK + reconcile} → **S05** exit gate. **First slice: S01** (dispatched immediately — it is over hand-built
inputs, triggers no semantic change, and gets engine-grade review; the D-053 Part-A/Part-B precedent — the
engine lane lands ahead of the caller population). **Decision-path / engine-grade-review stories: S01 and S02.
Closes D-052: S04. Closes D-061: S04.**

---

## EFE-S01 — ⚠️ DECISION-PATH lane: loader accept (add/delete, reject modify/rename) + engine `fileEvents` matcher + `path==""` domain disjointness + mirror [autonomous · engine-grade review]

> **⚠️ Changes `internal/core/policy/loader.go` + `internal/core/aggregate/coverage.go` (`matchChanges`) —
> the fail-safe decision path (`TestCorePurity`-scanned, first-class security surface per SECURITY.md).
> Fresh independent reviewer pointed explicitly at fail-safety: a `fileEvents` rule must NEVER turn a
> REVIEW/BLOCK into a silent APPROVE, and the loader kinds-guard (Judgment call (b)) closes a vacuous-cover
> fail-open. NO semantic change to real evaluations — the matcher operates over hand-built file-event inputs;
> no caller mints one until S02.**

**As a** policy author **I want** the engine to LOAD and MATCH a `match.fileEvents{paths,kinds}` rule against
a whole-file event **so that** a whole-file add/delete can drive a decision, over a hand-built
`EvaluationInput` — the smallest end-to-end proof of the domain through the decision path.

**Goal**: (1) **Loader accept** — remove the `loader.go:32-33` hard-reject; validate `fileEvents.kinds ⊆
{add, delete}`, REJECTING `modify` + `rename` at load (Judgment call (b)). (2) **Engine matcher** —
`matchChanges` handles `m.FileEvents`: select changes where `ch.Kind ∈ kinds` AND `ch.Path == ""` (the
whole-file discriminator) AND `ch.File` matches a `paths` glob. (3) **Domain disjointness (both directions)**
— `fileEvents` matches ONLY `path==""` events; `files`/`values`/`valueChanges` match ONLY value-level changes
(`path != ""`). (4) **Mirror** — the E6 `internal/adoptertest.ruleMatchesAny` implements the identical
fileEvents selection + `path==""` rule, in sync (S04/S05 depend on it). (5) **Repoint the pin** —
`TestExamplesPacksKnownBlockers` t.Fatalf's the instant topic-registry loads; retire/repoint it and move
topic-registry into the load+lint-clean set, stating honestly that at S01 topic-registry LOADS+LINTS but its
delete case still evaluates via opaque→REVIEW (no clean event until S02). **Verify** (per advisor) the
file-event CEL scope needs NO `evaluate.go`/`bindLeafActivation` change: `kind`/`file` bind as strings,
`path==""` binds, nil `old`/`new`/`entry` already error→REVIEW under `toCEL` — if a change IS needed, split
it into its own fail-safety-reviewed sub-lane.

**Operator input**: yes — 🔴 DECIDE + log (decisions.md) the loader kinds-narrowing to `{add,delete}` (reject
modify/rename) closing the vacuous-cover fail-open (Judgment call (b)); and that this is a decision-path change
reviewed as an engine lane ahead of the input lane (D-053 precedent).

**Dependencies**: E1 (`change.Change`/`Kind`), E2 (`policy` loader, `matchChanges`/`Cover`), E3 (the
known-blockers pin), E6 (the `ruleMatchesAny` mirror). None blocking.

**Definition of done**: a hand-built `EvaluationInput` carrying a file-event `EvalChange{Path:"", Kind:"delete",
File:"topics/x.yaml", Subject:"file:topics/x.yaml"}` + a loaded `fileEvents{paths:["topics/**/*.yaml"],
kinds:[delete]}` rule evaluates to the fixture decision; `fileEvents{kinds:[modify]}` and `{kinds:[rename]}`
are REJECTED at load with a located error; a value-level add (`path:"/foo", kind:add`) is NOT selected by a
`fileEvents{kinds:[add]}` rule, and a file-event (`path:""`) is NOT selected by a `values`/`files`/`valueChanges`
rule; the mirror agrees byte-for-byte with `matchChanges`; `TestExamplesPacksKnownBlockers` retired/repointed
and topic-registry loads+lints clean; `TestCorePurity` + the full `aggregate` suite green; `git diff schemas/`
== 0.

**Not in scope**: minting the file-event from base/head (S02); the unmatched-delete default (S02, Judgment call
(a)); the `cmd/assent` live path (S03); re-authoring topic-registry's `when`/`kinds` (S04); the exit gate (S05).

Requirements:
- **REQ-EFE-S01-01** *(ENGINE · decision-path)* — the loader accepts `fileEvents{paths,kinds}` with kinds ⊆ `{add,delete}`; a hand-built file-event `EvalChange` (`path:""`) matching a `fileEvents` rule drives the rule's onFailure/effect and reaches the expected decision via `Cover`. Test: `internal/core/aggregate/fileevents_test.go`; Verify: `go test ./internal/core/aggregate/... -run TestFileEventsMatchDrivesDecision`; Level: L0
- **REQ-EFE-S01-02** *(ENGINE · fail-safe · Judgment call (b))* — `fileEvents{kinds:[modify]}` and `{kinds:[rename]}` are REJECTED at load with a located error; `{kinds:[add,delete]}` loads. Test: `internal/core/policy/loader_fileevents_test.go`; Verify: `go test ./internal/core/policy/... -run TestFileEventsKindsFailClosed`; Level: L0
- **REQ-EFE-S01-03** *(ENGINE · disjointness both ways)* — a `fileEvents{kinds:[add]}` rule selects a `path==""` file-add but NOT a value-level add (`path:"/x"`); a `values`/`valueChanges` rule selects the value-level change but NOT the `path==""` file-event. Test: `internal/core/aggregate/fileevents_test.go`; Verify: `go test ./internal/core/aggregate/... -run TestFileEventsPathDiscriminatorDisjoint`; Level: L0
- **REQ-EFE-S01-04** *(mirror sync)* — `adoptertest.ruleMatchesAny` implements the identical `fileEvents` + `path==""` selection; a table of inputs yields identical selected/not-selected verdicts from `matchChanges` and `ruleMatchesAny`. Test: `internal/adoptertest/coverage_internal_test.go`; Verify: `go test ./internal/adoptertest/... -run TestRuleMatchesAnyMirrorsFileEvents`; Level: L0
- **REQ-EFE-S01-05** — `TestExamplesPacksKnownBlockers` retired/repointed and topic-registry LOADS + lints clean (the delete case still opaque→REVIEW until S02); `TestCorePurity` + full `aggregate` suite green; `git diff schemas/` == 0. Test: `cmd/assent/examples_packs_lint_test.go`; Verify: `go test ./cmd/assent/... -run 'TestExamplesPacks' && go test ./... -run TestCorePurity`; Level: L1

## EFE-S02 — ⚠️ Fail-safety lane: change-model file-event constructor + harness minting from one-sided presence + unmatched-delete REVIEW-default (Judgment call (a)) [autonomous · engine-grade review]

> **⚠️ Resolves Judgment call (a) — a fail-safety + behavior-contract property. Emit a clean file-event ONLY
> on unambiguous one-sided presence; every ambiguity STAYS opaque → REVIEW; and an UNMATCHED whole-file DELETE
> defaults to REVIEW (NOT APPROVE) pending the operator's answer. Fresh reviewer pointed at exactly this — a
> wrongly-minted event, or an unmatched delete silently reaching APPROVE, is a fail-OPEN on a destructive op.**

**As a** policy author **I want** `assent test` to derive a whole-file add/delete event from a case's
`base/`↔`head/` (a `null` side = the file's lifecycle) and feed it to the S01 matcher, with an unmatched
delete failing safe **so that** a fileEvents rule evaluates over a real case AND a delete no rule governs is
never silently approved.

**Goal**: (1) a single pure constructor in `internal/change` (`FileEvent(file string, kind Kind) Change`,
`Path:""`) — the one place the `path==""` shape is minted (Judgment call (d)). (2) In `internal/adoptertest`
(+ `internal/evaldecode` if the seam belongs there), detect a whole-file lifecycle from **one-sided presence**:
`null`/absent `base` + present `head` = file-ADD; present `base` + `null`/absent `head` = file-DELETE; mint the
file-event via `SubjectOf` → `file:<path>` instead of `change.Diff` (which goes opaque). (3) **The ambiguity
invariant:** a clean event ONLY when exactly one side is ABSENT (a known lifecycle); an empty-but-present side,
an unparseable side, or a both-present content-opaque case STAYS opaque → REVIEW; a rename is NOT synthesized.
(4) **The unmatched-delete fail-safe default (Judgment call (a), advisor-guided):** a whole-file DELETE event
that NO `fileEvents` rule matches evaluates to **REVIEW** (fail-safe), NOT APPROVE. This likely needs a small
decision-path escalation (an unmatched `path==""` delete event → REVIEW) — implement it as the fail-safe
default; do NOT let emitting a clean event silently flip REVIEW→APPROVE. If the operator answers "APPROVE" the
default is relaxed in a follow-up; REVIEW is the shipped default until then.

**Operator input**: yes — 🔴 the unmatched-whole-file-delete default (a) is an OPERATOR QUESTION raised at epic
start; S02 ships REVIEW-default and logs it. Confirm/log the one-sided-presence-only emission invariant.

**Dependencies**: EFE-S01 (the matcher that consumes the event); E1 (`change.Change`/`Kind`), E6
(`internal/adoptertest` case model, `internal/evaldecode`).

**Definition of done**: a harness case with a present `base` topic file + `null` `head` mints a file-DELETE
that the S01 `fileEvents{kinds:[delete]}` rule matches → the fixture decision; a `null` `base` + present `head`
mints a file-ADD (proving polarity for `kind != "delete"`); a both-present-but-content-opaque case STAYS opaque
→ REVIEW (no spurious event); an empty-but-present side is opaque, NOT a delete; a rename-shaped pair is NOT
folded into a rename event; **a whole-file DELETE matched by NO fileEvents rule → REVIEW (fail-safe default,
Judgment call (a)), never APPROVE**; the constructor is the sole `path==""` minter; double-run byte-identical;
`git diff schemas/` == 0.

**Not in scope**: the `cmd/assent` live checkout (S03); topic-registry re-authoring (S04); the exit gate (S05).

Requirements:
- **REQ-EFE-S02-01** — `change.FileEvent(file, kind)` mints a `Change{File, Path:"", Kind}`; it is the single `path==""` minter. Test: `internal/change/fileevent_test.go`; Verify: `go test ./internal/change/... -run TestFileEventConstructor`; Level: L0
- **REQ-EFE-S02-02** — a case with present `base` + `null` `head` builds a file-DELETE `EvalChange` (`path:"", kind:delete, subject:file:<path>`); present `head` + `null` `base` builds a file-ADD; the S01 matcher selects it. Test: `internal/adoptertest/fileevents_test.go`; Verify: `go test ./internal/adoptertest/... -run TestOneSidedPresenceMintsFileEvent`; Level: L0
- **REQ-EFE-S02-03** *(fail-safe · ambiguity)* — both-present-content-opaque, empty-but-present, and rename-shaped cases each STAY opaque → REVIEW; NO file-event minted. Test: `internal/adoptertest/fileevents_test.go`; Verify: `go test ./internal/adoptertest/... -run TestAmbiguousLifecycleStaysOpaque`; Level: L0
- **REQ-EFE-S02-04** *(🔴 fail-safe default · Judgment call (a))* — a whole-file DELETE event matched by NO fileEvents rule evaluates to **REVIEW** (never APPROVE); pinned so the default cannot silently regress to APPROVE. Test: `internal/adoptertest/fileevents_test.go` (+ `internal/core/aggregate` if the escalation lives there); Verify: `go test ./... -run TestUnmatchedFileDeleteFailsSafeReview`; Level: L0
- **REQ-EFE-S02-05** — determinism: the file-event pipeline double-runs byte-identical. Test: `internal/adoptertest/fileevents_test.go`; Verify: `go test ./internal/adoptertest/... -run TestFileEventDoubleRunStable`; Level: L0

## EFE-S03 — `cmd/assent` live-checkout file-lifecycle population (presence signal, not empty bytes) [autonomous]

**As a** repo operator **I want** the live `assent run` checkout to emit a whole-file add/delete event for a
truly added/deleted governed file **so that** a fileEvents rule fires on a real MR, not just in the harness.

**Goal**: extend `cmd/assent/checkout.go` to derive the file lifecycle from a **presence signal** — whether the
path exists on each side — NOT from empty bytes (empty-file vs absent-file is the ambiguity of Judgment call
(a)/(S02)). Where presence is one-sided, mint the S02 file-event via `change.FileEvent`; where presence is
unknowable or content is opaque, stay opaque → REVIEW. If `localCheckout.FileContents` returns only bytes, add
a small checkout-interface presence report (or the path stays opaque). Closes NEITHER D-052 NOR D-061 (both via
the harness path, S04) — the live-adapter completeness slice, parallelizable after the S01/S02 seam.

**Operator input**: no (Judgment calls (a)/(b) resolved in S01/S02).

**Dependencies**: EFE-S01, EFE-S02. Independent of S04/S05.

**Definition of done**: a `dirCheckout` over testdata where a governed file exists only on head (add) / only on
base (delete) mints the file-event and the run evaluates the fileEvents rule; an empty-but-present governed file
stays opaque → REVIEW (not a delete); present-both-sides opaque content stays opaque; determinism holds; no live
infra.

**Not in scope**: topic-registry/service-catalog fixtures (S04); the CI dogfood gate (S05); a live-forge checkout
(E4/E8).

Requirements:
- **REQ-EFE-S03-01** — a `dirCheckout` case with a file present only on head mints a file-ADD; present only on base mints a file-DELETE; the run selects it via a fileEvents rule. Test: `cmd/assent/run_changedset_test.go`; Verify: `go test ./cmd/assent/... -run TestLiveCheckoutMintsFileEvent`; Level: L1
- **REQ-EFE-S03-02** *(fail-safe)* — an empty-but-present or presence-unknowable governed file stays opaque → REVIEW; no file-event minted from empty bytes. Test: `cmd/assent/run_changedset_test.go`; Verify: `go test ./cmd/assent/... -run TestLiveCheckoutAmbiguousStaysOpaque`; Level: L1
- **REQ-EFE-S03-03** — the live checkout population double-runs byte-identical. Test: `cmd/assent/run_changedset_test.go`; Verify: `go test ./cmd/assent/... -run TestLiveCheckoutFileEventDoubleRun`; Level: L1

## EFE-S04 — Unpin topic-registry (closes D-052) + service-catalog file-delete→BLOCK + reconcile the divergence (closes D-061) [autonomous]

**As a** maintainer **I want** topic-registry to LOAD and EVALUATE its whole-topic-delete `non-destructive` and
the archetype manifest's file-delete→BLOCK expressed, both reconciled against the golden manifest **so that** the
two tracked gaps (D-052, D-061) are closed and can't rot.

**Goal**: (1) **D-052** — re-author topic-registry's `non-destructive.yaml` from `when:"false"` + `kinds:[delete]`
(which can NEVER earn a `--coverage` proving polarity) to a PROVABLE form: `match.fileEvents{paths,
kinds:[add,delete]}`, `prove.when: 'kind != "delete"'`, so a file-ADD proves it (proving polarity) and a
file-DELETE fails it (failing polarity → onFailure). Author the delete + add fixture cases (`base/`↔`head/` with
a `null` side) so it evaluates green under `assent test` with both polarities. (2) **D-061** — add a
service-catalog file-delete case (or companion fileEvents rule) that BLOCKs per the manifest, and update
`TestExampleCorpusReconcilesArchetypeManifest` to reconcile the logged REVIEW-vs-BLOCK divergence so it's
asserted, not silent. Keep D-052 and D-061 as SEPARATE REQs — one closure must not mask the other.

**Operator input**: yes — confirm/log the topic-registry `non-destructive` re-authoring and the reconciled
divergence (D-061(e)).

**Dependencies**: EFE-S01, EFE-S02. Independent of S03.

**Definition of done**: topic-registry loads, lints clean, and its `non-destructive` proving (file-add) + failing
(file-delete) cases evaluate to the pinned decisions under `assent test`; a service-catalog file-delete case
BLOCKs per the manifest; `TestExampleCorpusReconcilesArchetypeManifest` reconciles the REVIEW-vs-BLOCK divergence;
the D-052 known-blocker pin is fully retired (topic-registry in the green corpus); double-run byte-identical;
`git diff schemas/` == 0.

**Not in scope**: the corpus-wide `--coverage` gate + CI dogfood (S05); the live-forge path.

Requirements:
- **REQ-EFE-S04-01** *(closes D-052)* — topic-registry's re-authored `non-destructive` (`kind != "delete"`, kinds:[add,delete]) evaluates its file-DELETE case to REVIEW and its file-ADD proving case to satisfied/silent under `assent test`. Test: `cmd/assent/test_corpus_test.go`; Verify: `go test ./cmd/assent/... -run TestTopicRegistryFileEventsGreen`; Level: L1
- **REQ-EFE-S04-02** *(closes D-061)* — a service-catalog file-delete case → BLOCK per the manifest; `TestExampleCorpusReconcilesArchetypeManifest` asserts the REVIEW-vs-BLOCK divergence so it cannot rot silently. Test: `cmd/assent/test_corpus_test.go`; Verify: `go test ./cmd/assent/... -run TestExampleCorpusReconcilesArchetypeManifest`; Level: L1
- **REQ-EFE-S04-03** — the D-052 known-blocker pin is retired and topic-registry is in the load+lint+evaluate-green corpus; double-run byte-identical. Test: `cmd/assent/examples_packs_lint_test.go`; Verify: `go test ./cmd/assent/... -run 'TestExamplesPacks'`; Level: L1

## EFE-S05 — Exit gate: fileEvents create/delete fixtures + topic-registry green under `assent test --coverage` + determinism [autonomous]

**As a** maintainer **I want** the fileEvents domain proven end-to-end in the example corpus — create AND delete
→ correct decision, both-polarity `--coverage`, deterministic **so that** the domain is trustworthy documentation
and gates itself in CI (ADR-0006).

**Goal**: wire S01–S04 into the exit gate: (1) the fileEvents domain has fixtures proving a whole-file CREATE (add)
and DELETE → the correct decision (an explicit domain fixture pair beyond topic-registry); (2) topic-registry
passes `assent test` AND `--coverage` both-polarity corpus-wide; (3) the whole gate double-runs byte-identical;
(4) `internal/core` byte-unchanged beyond S01/S02, `git diff schemas/` == 0.

**Operator input**: no.

**Dependencies**: EFE-S01..S04.

**Definition of done**: a create-fixture and a delete-fixture each evaluate to the pinned decision under `assent
test`; topic-registry passes corpus-wide `--coverage` both-polarity; the whole fileEvents gate double-runs
byte-identical; `git diff schemas/` == 0 and `internal/core` unchanged beyond the S01/S02 matcher+loader+escalation.

**Not in scope**: `modify`/`rename` fileEvents kinds (rejected at load, S01); the live-forge path; rendered-comment
goldens (E8).

Requirements:
- **REQ-EFE-S05-01** — a whole-file CREATE fixture and a whole-file DELETE fixture each evaluate to the pinned decision under the whole-pack replay. Test: `cmd/assent/test_corpus_test.go`; Verify: `go test ./cmd/assent/... -run TestFileEventsCreateAndDeleteFixtures`; Level: L1
- **REQ-EFE-S05-02** — topic-registry passes `assent test --coverage` both-polarity corpus-wide (proving file-add + failing file-delete for `non-destructive`). Test: `cmd/assent/test_corpus_test.go`; Verify: `go test ./cmd/assent/... -run TestFileEventsCorpusBothPolarityCoverage`; Level: L1
- **REQ-EFE-S05-03** — determinism: the whole fileEvents gate double-runs byte-identical; `git diff schemas/` == 0. Test: `cmd/assent/test_corpus_test.go`; Verify: `go test ./cmd/assent/... -run TestFileEventsGateDoubleRun`; Level: L1
