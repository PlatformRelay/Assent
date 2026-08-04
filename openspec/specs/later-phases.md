# Later phases — epics only (stories authored in Phase 3, per meta-plan "contracts first")

Statuses: **Planned** (unlocks with its phase) · **Locked (D-012)** (needs a named consumer)
· **Unlocked (D-017)** (named consumer committed; contract in Phase 3, implementation
sequenced after the Phase-4 adoption gate). See
[named-consumer-compat.md](../../docs/planning/named-consumer-compat.md).

## Phase 3 — Contracts first

### P3-E1 — JSON Schemas + the strict contract fixture — Planned
Freeze the serialized contracts as versioned JSON Schemas — authored resources (Config,
RulesetBinding, MergePolicy with `prove`/`onFailure`), EvaluationInput, DecisionRecord,
ReplayBundle, PresentationModel, PublicationReceipt, provider request/response envelope, and
the adopter test-fixture format (ADR-0014) — remembering ADR-0017 §7: the schemas ARE the
public API, Go interfaces stay internal, no v1 SDK. Constraints: EntryRef subjects + matcher
domains (§5), obligations not vouch (§2), `require-review` distinct from `challenge` (§3),
typed fact states with `maxAge` (§4/§6), merge-result pins in preconditions (§1), apiVersion
group settled (OQ-1 residual). **Exit gate (D-016)**: the strict end-to-end contract fixture
of §8 — pinned target/merge result, renamed entry with stable identity, two required
obligations, an expired typed fact, a missing required approval, expected DecisionRecord +
redacted PresentationModel + publication preconditions — validates against the same schemas
as every example; no engine code before it exists.
D-017 additions: a **typed ApprovalEvidence contract** (resolves OQ-23; principal identity,
approval source/rule, self-approval policy, eligibility evidence, source + merge-result
pins, observation time + expiry, verifying forge capability — a discussion resolver is not
approval evidence; missing capability → never auto-merge, no silent downgrade to
challenge); **one sanitized named-consumer compatibility fixture** in the mandatory set
(phase, profile, comparison delta, approval evidence, marker fields, budget reservation
are structured fields, never inferred from messages/labels/rule names); EvaluationInput
stays backend-neutral so the unlocked E11 contract is not precluded. Story seeds:
schema-per-contract slices; fixture authoring; schema-validation CI job; predicate-scope
table freeze (ADR-0013); `assent lint` hard-error list spec (ADR-0010 amendment);
ApprovalEvidence slice; compatibility fixture; OQ-8/9/10/13/22 resolutions (OQ-21 moved to
P3-E4, OQ-23 resolved here).
**Fixture: committed; D-016 gate CLOSED** (P3-E1-S07, 2026-07-24) — the strict end-to-end
fixture `examples/contracts/d016-strict-fixture/` and the sanitized named-consumer
compatibility fixture `examples/contracts/named-consumer-compat/` are on `main` (@ `93ea446`)
and validate in CI: the schemas job (`TestExampleContractsFixturesValidate`) is green on `main`.
The gate is closed and **Phase 5 (E1–E9) engine code is unblocked**. See decisions **D-024**
(staged) and **D-025** (closure confirmed).

### P3-E2 — Versioning & compatibility spec — Planned
Specify ADR-0017 §9 as testable rules: safety-bearing resources reject unknown
fields/duplicate IDs/unknown enums; reports are additive-tolerant; provider majors
negotiated; named collections are lists with mandatory unique IDs; hashes over canonical
JSON with schema-version domain separation; do-not-generalize list restated as lint errors.
Produces the public `API_STABILITY.md` (oss-playbook #8) stating per-version guarantees for
policy schema / decision contract / test format and graduation criteria. **Exit gate**:
compat rules expressed as executable schema/fixture tests (valid + adversarial invalid
fixtures per rule). Story seeds: strict-decode fixture suite; canonical-JSON hash vectors;
version-negotiation matrix; API_STABILITY.md.

### P3-E3 — Example & pack migration to the frozen contracts — Planned
Migrate `examples/policies/**`, `examples/archetypes/**` (P1-E2 DRAFT fixtures) and the
starter-pack sketches to the frozen schemas: `prove:`/`onFailure:` replacing `effect: vouch`,
matcher domains replacing `path` overloading, DRAFT markers removed (ADR-0017 consequences,
P2-5). **Exit gate**: every committed example validates in CI against the Phase-3 schemas;
the archetype fixtures become the seed corpus for `assent test` goldens. Story seeds:
per-shape starter pack (topic-registry / service-catalog / infra-vars); rego example
quarantined behind a `# locked: D-012` marker; docs updated.

### P3-E4 — Policy lifecycle contracts: rollout phase, profiles, comparison — Planned
D-017 (B2–B4): resolve OQ-21 **in favor of** an explicit rollout phase on rules/packs —
`off` (parsed and linted, not evaluated), `observe` (evaluated and recorded, cannot alter
the enforcing decision or forge state), `enforce` (contributes obligations, blocks,
reviews, challenges, score); editing effects to simulate rollout loses policy identity and
breaks before/after comparison. DecisionRecord carries observed and enforcing findings
separately; phase transitions are visible in policy diffs. **Named policy profiles** with a
single-writer rule: exactly one profile may authorize forge writes, counterfactual profiles
are recorder-only; profiles must not become a second routing system — one precedence table
defines profile × environment/class-binding interaction. A **semantic comparison record**
with a closed delta taxonomy (stricter-intervention-added, destructive-or-authorization-
intervention-missed, subject-or-obligation-uncovered, newly-auto-mergeable,
score-threshold-change, explanation-only) and a versioned **PolicyComparisonSuite**: an
immutable corpus of ReplayBundles with stable case IDs plus machine-enforceable promotion
gates (zero missed destructive interventions, zero missed authorization/ownership
interventions, no unexpected obligation removal, bounded auto-merge widening, explicitly
accepted deltas). Rendered-message changes never enter semantic gates (ADR-0014 amendment
split). Authored as **ADR-0018**, accepted at the Phase-3 freeze review. **Exit gate**:
schemas represent phase, profile, comparison record, and suite; the named-consumer
compatibility fixture exercises observe-vs-enforce and one refused auto-merge-widening
delta. Story seeds: phase field + lint; profile precedence table; delta taxonomy;
comparison-suite format; `assent compare` CLI spec (implementation Phase 5+, E6).

### P3-E5 — Publication reconciliation protocol (database-free) — Planned
D-017 (B6): freeze the marker + reconciliation protocol in the publication contract as
**ADR-0019**, preserving D-007 (no database — the forge is the durable reconciliation
surface). Hidden-HTML marker with four distinct concepts: `slot` (stable identity of the
logical artifact from canonical fields — project/MR, rule ID, obligation, EntryRef, effect,
anchor), `occurrence` (hash of the safety-relevant occurrence so changed content cannot
inherit a previously resolved challenge), `decision` (DecisionRecord hash that requested
the state), `artifact` kind + schema version. Markers are correlation metadata only —
never decision input or authorization evidence; only bot-authored comments are parsed
(contributor marker spoofing ignored); markers carry no secrets, fact values,
user-controlled Markdown, or raw policy expressions. Reconciliation semantics frozen as a
numbered contract: recompute DesiredReviewState from trusted inputs, list paginated
bot-authored artifacts, update the one summary slot in place, leave the same unresolved
occurrence untouched, preserve resolution of the same occurrence across reruns, supersede
stale occurrences with fresh challenges, resolve no-longer-desired findings,
deterministically repair pre-existing duplicates (lowest forge ID canonical, repair
recorded in PublicationReceipt), rescan after publication before reporting success. Strict
duplicate prevention requires **one publisher per MR**: per-MR `resource_group` in the CI
template (serve mode: keyed per-MR lock); multi-replica HA is explicitly unsupported —
duplicates then only converge on the next reconciliation — and `doctor` reports which
guarantee the deployment provides. **Exit gate**: marker grammar and reconciliation state
table exist as schema + fixtures; P4-E1's exit gate consumes them (rerun idempotence).
Story seeds: marker grammar schema; reconciliation state table + fixtures; serialization
requirement folded into the setup walkthrough and doctor checklist; duplicate-repair
fixture.

## Phase 4 — Walking skeleton

### P4-E1 — Walking skeleton with trust boundaries from day one — Done (autonomous slices + live adoption, 2026-07-27)
Thinnest real slice, TDD: CLI in a GitLab CI job on the Spike-B-chosen e2e profile → parses
a one-field YAML change on a generated sample repo → evaluates one `assert` rule proving one
obligation → posts one resolvable thread or approves + SHA-pinned merges → emits the
DecisionRecord report. Trust-boundary behaviours are in the skeleton, not deferred:
policy loaded from target ref only, `.assent/**` MR → `assent-policy` meta-class BLOCK
golden, SHA-guard rejection on target/source movement (merge-result precondition, ADR-0017
§1), protected-pipeline `doctor` precondition check refusing to arm auto-merge, provider-less
run (no token near evaluation). **Exit gate**: the L3 skeleton e2e green + replayable;
determinism double-run gate active in CI; **rerun idempotence (D-017/P3-E5): a rerun and a
crash-then-rerun produce zero duplicate comments/threads under the serialized topology, and
seeded pre-existing duplicates are repaired deterministically**; **adoption gate (D-012): SATISFIED** — a real gitlab.com repository (`konrad.heimel/assent-lab`, D-037) ran assent on live MRs (REVIEW→thread + APPROVE→SHA-pinned merge; D-042); a synthetic fixture does not count. **Exit gate CLOSED; Phase 5 unblocked.** Story seeds: CLI+CI env
adapter; minimal differ (modify only); minimal obligations aggregation; minimal Reconcile
(thread, approve, pinned merge); report artifact; doctor preconditions; e2e harness reuse
from Spike B.

## Phase 5 — Implementation epics (re-derived from ADR-0013/0015/0016/0017 state)

### E1 — Change model & EntryRef classifier — Planned
JSON/YAML/HCL-tfvars adapters → value tree → structural diff with first-class positions,
deletes, opt-in rename fold (never laxer than delete), input resource limits with
opaque→REVIEW fail-safe (ADR-0003 + amendments); `entries:` declarations → stable `EntryRef`
subjects for document/map/keyed-list collections, unkeyed lists rejected at lint; matcher
domains `files`/`values.pointers`/`fileEvents`/`valueChanges` (ADR-0017 §5); environment +
class routing incl. the built-in `assent-policy` meta-class (ADR-0008/0015 §1). **Exit
gate**: golden + property tests over the archetype corpus incl. adversarial near-threshold
rename pairs; billion-laughs/limits corpus fails closed. Seeds: one adapter per format;
differ; rename fold; EntryRef derivation; classifier; limits.

### E2 — Obligations decision engine — Planned
ADR-0007 as reshaped by ADR-0017 §2–4: per-binding `require:` lists; rules `prove` exactly
one named obligation with `onFailure: {effect, code}`; AND-only composition; union of
denies; `require-review` outcome satisfied only by forge-proven eligible approval;
tri-state predicates fail safe; per-firing points against per-binding thresholds (points are
outcome contributions, `score` is not an effect); one-shot arming restrictions for expiring
facts (§4); deterministic order-independent aggregation into DecisionRecord. **Exit gate**:
golden decision tests for every archetype expected-decision pair; determinism double-run;
"broad rule cannot satisfy an obligation it does not name" adversarial golden; **observe
cannot alter enforce** golden (a rule flipped to `observe` changes recorded findings but
never the enforcing outcome or forge state, per P3-E4). Seeds: obligation registry;
aggregation; points; arming logic; phase-aware aggregation (observed vs enforcing
findings); DecisionRecord emission.

### E3 — Policy surface: envelope loader, CEL assert backend, lint — Planned
MergePolicy/RulesetBinding/Config loading with strict decode (P3-E2 rules), target-ref-only
loading (ADR-0015 §1); `assert` all/any/not trees with CEL leaves on cel-go per Spike A
decisions (coercion strategy, cost budget, purity env, per-leaf trace); one activation model
shared with `{{ }}` message interpolation, load-time compile errors (ADR-0016 §2);
`assent lint` hard errors (obligation coverage, reserved classes, fail-open restrictions,
tests-per-rule, unkeyed lists, undeclared projections). **Exit gate**: lint catches every
hard-error fixture; all archetype packs load and evaluate to their expected decisions.
D-017 (B10) adds a **generated rule catalogue** from loaded packs (stable rule/obligation
IDs, docs links, rollout phase, required facts/capabilities, classes + matcher domains,
possible finding codes/effects, deprecation metadata) — an additive-tolerant report, the
single source for generated docs and lint; no second handwritten registry. Seeds: loader;
CEL backend; tree walker + trace; interpolation; lint; catalogue generation.

### E4 — GitLab forge adapter: Snapshot/Resolve/Reconcile — Planned → **decomposed** ([p5-e4-gitlab-forge/spec.md](p5-e4-gitlab-forge/spec.md))
The `Snapshot → Resolve → Reconcile(DesiredReviewState, Preconditions) → PublicationReceipt`
port (ADR-0017 §7) on GitLab: resolvable threads with stable finding keys (idempotent
upsert), approve/merge with source+target SHA and merge-result preconditions failing closed
(§1, ADR-0015 §2), `require-review` evidence via the P1-E3-S02 approval-eligibility chain,
capability flags per tier (gap → never auto-merge), `doctor` typed capability/precondition
report (ADR-0017 §9). **Exit gate**: L2 cassette contract tests + the L3 conformance cases
"target advanced after evaluation → rejected/re-evaluated" and "MR edits own policy →
BLOCK", plus the P3-E5 reconciliation conformance cases (rerun idempotence,
occurrence-supersession, deterministic duplicate repair, spoofed contributor marker
ignored). Seeds: snapshot; marker-keyed thread reconcile (ADR-0019); approval evidence
(P3-E1 ApprovalEvidence contract); pinned merge; doctor. **Autonomous slice:** S01–S10
(httptest/fakes); **infra-gated:** S11 live L3 re-run.

### E5 — Provider host + builtins — Planned → **decomposed** ([p5-e5-provider-host/spec.md](p5-e5-provider-host/spec.md))
Typed provider protocol from Spike C frozen in P3-E1: first-wave builtins = **forge-groups +
`repo-file` + resource→owner** (closes REF-GAP-1/2); HTTP/exec tier; ownership-file optional;
**OIDC/Keycloak/LDAP deferred** until a named consumer (S09). Projection-minimized requests;
`resolved|unavailable|invalid|expired` with `observedAt`/`expiresAt`/`maxAge` as arming
preconditions; write-token isolation + argv hygiene (ADR-0015 §7) and digest-pinned exec;
sensitive tier 15m (INBOX F1/F2); controlling facts never fail open. **Exit gate**: isolation
harness in CI; state-machine goldens; lint rejects fail-open on controlling facts; hermetic
resolved-facts path; seed REF-EX C5–C7. Seeds: host S01–S05; builtins S06–S08; exit S10.

### E6 — Adopter test harness `assent test` + examples — Planned
The frozen ADR-0014 fixture format executed by the production pipeline with providers/forge
stubbed: directory cases + inline `cases.yaml`, `--update` golden flow, `--coverage` with
per-rule both-polarity requirement, obligation/finding-code assertions with `exact` safety
default, determinism double-run per case; dogfooded on `examples/` (packs gate themselves in
CI). **Exit gate**: every shipped example pack green under `assent test`; a deliberately
broken pack fails with the expected/actual diff UX. D-017 (B3/B4) adds the
**PolicyComparisonSuite runner**: `assent compare` evaluates baseline vs candidate profiles
over an immutable ReplayBundle corpus, classifies deltas per the P3-E4 taxonomy, and
enforces the promotion gates (missed destructive/authorization intervention → hard fail;
unaccepted widening → fail; explanation-only → pass); comparison runs are side-effect-free.
Seeds: runner; expect matcher; update flow; coverage lint; examples CI job; comparison
runner + promotion-gate evaluation.

### E7 — E2E & conformance infra — **AUTONOMOUS SLICE CLOSED** (S01–S05 + S08, D-087)
Full INVEST stories in [p5-e7-e2e-conformance/spec.md](p5-e7-e2e-conformance/spec.md).
E4 already delivered L2 hermetic conformance (`internal/forge/conformance`, D-079); E7
**productionizes** Spike-B profiles, adds the sample-repo generator for all three
`examples/repos/` shapes, publishes a forge-neutral conformance **catalog** (ADR-0005) indexing
E4 cases plus remaining ADR-0015/0017 adversarial cases provable hermetically, wires the
**determinism gate** + **sanitization check** into verify (gitleaks + `go vet -tags e2e`
already present), and scaffolds L3 live re-run + optional kind lab (**infra-gated**). **Exit
gate CLOSED (D-087):** S01–S05 + S08 green — verify wires determinism + sanitization +
e2e-vet; catalog complete; L1 suite green. **S06/S07 optional** (kind lab + live L3, absorbs
E4-S11). Seeds: profile task wiring; repo generator; catalog; determinism job; security jobs;
kind lab; L3 harness.

### E8 — Renderer & presentation — **AUTONOMOUS COMPLETE (D-098)**
Full INVEST stories in [p5-e8-renderer/spec.md](p5-e8-renderer/spec.md). ADR-0016 tier 0 only
(D-012/D-015): renderer-owned envelope (markers, escaping, redaction, clamping outside
customizable regions), config knobs (verbosity per env, emoji, collapse threshold, locale), CEL
message rendering from PresentationModel + activation, `assent render` against fixtures,
default-theme golden markdown snapshots, presentation lint, `en` locale catalog, summary-comment
slot (P3-E5 step 3 — closes D-073). Tiers 1–2 (slots/full templates) stay designed seams.
**Exit gate CLOSED (D-098):** S01–S14 green — render goldens + `-count=2` determinism wired;
summary slot hermetic; wording changes provably do not break safety tests (ADR-0014 amendment split).
**14 stories (14 autonomous)** — E9 release may proceed.
Seeds: `internal/render/**`; `examples/render/**`; `assent render --finding`; config
`presentation:` (D-088); forge `UpsertComment` (S12); judgment calls D-088–D-097.

### E9 — Distribution & release — **AUTONOMOUS COMPLETE (D-111 infra pending)**
Full INVEST stories in [p5-e9-distribution/spec.md](p5-e9-distribution/spec.md). oss-playbook
execution: goreleaser (binaries, brew, curl+checksum, `go install`), cosign keyless + SLSA +
SBOM, git-cliff notes without SHAs, CI hardening **audit** (extend D-045 CodeQL/Scorecard/
govulncheck — do not duplicate), mkdocs-material product-only nav (planning/openspec excluded
from published nav), README maturity table, VHS demo tapes, OQ-2 GitLab mirror disposition
(**D-105 defer**). **Autonomous exit gate CLOSED (S13 half):** `hack/release/exitgate_test.sh`
runs snapshot + checksum verify + strict docs build; S01–S04 + S07a + S08→S09 + S10 + S11 + S12
green; **D-099–D-110 cited.** **Infra half pending (D-111):** tag `v0.1.0` (D-108) publishes
signed assets (S05–S06, D-109), Homebrew tap (S07b, D-107), live curl/go install + docs site proof.
**14 stories (10 autonomous closed, 4 infra-gated)** — **Next after E9:** PolicyComparisonSuite
full runner (D-057, separate epic). Seeds: `.goreleaser.yaml`, `release.yaml`, `hack/release/**`,
install script, product nav trim.

### PCS — PolicyComparisonSuite full runner — **SPEC READY** (claim after E9 autonomous close)
Full INVEST stories in [p5-pcs-policy-comparison/spec.md](p5-pcs-policy-comparison/spec.md).
Completes D-057 deferred scope beyond the E6-S09 seed: profile→pack activation, remaining
four delta classifiers, all five promotion gates, per-delta `acceptedDeltas` allowlist,
`ComparisonRecord` emission, multi-case immutable corpus, and `assent compare --suite` with
ADR-0018 exit codes. **Extends** `internal/compare/**` + `cmd/assent/compare.go`; **`git diff
schemas/` == 0** (frozen comparison + replay-bundle contracts). Side-effect-free — never
Reconcile. **9 stories (9 autonomous)**. **Do first: PCS-S01** (profile→pack activation).
Seeds: `examples/comparison/**`, `hack/compare/exitgate_test.sh`.

### E10 — GitHub adapter + Actions entrypoint — **Locked (D-012)**
Unlocks with a named consumer. Seam kept honest by the P1-E3-S03 dossier (REQUEST_CHANGES +
conversation-resolution parity, merge queue as merge-result pin, base-ref workflow trust)
and by the conformance suite being forge-neutral (E7). No frozen contract until unlock.

### E11 — Complex-rule backend (Rego) — **Unlocked (D-017), implementation after Phase 4**
The named consumer's multi-pass / cross-manifest / set-difference / graph-relationship
checks are the consumer D-012 required. Contract committed in Phase 3 (EvaluationInput
stays backend-neutral, P3-E1); implementation only in the named-consumer expansion, and
evidence-based per rule: each ported rule tries CEL first, the backend is built when a
concrete rule demonstrably exceeds the tier-1 ceiling. Shape unchanged: tier-2 escape hatch
inside the same envelope (ADR-0002 v2), violations-shaped modules, explicit
obligation-proof polarity (no implicit "no violation = proof"), OPA capability sandbox
(D-013), same typed EvaluationInput, structured proof/finding output, declared data, no
I/O, no control over aggregation. Hard boundary: no domain-aware joins and no Go rule
plugins in `internal/core`.

### E12 — Service tier: `serve`, sweep, merge budgets, post-merge audit — **Unlocked (D-017), implementation after Phase 4**
Rescoped from "`serve` webhook mode" to the operational tier the named consumer needs;
seams designed now, implemented only after the D-012 adoption gate and a representative
rule port. Contents: (a) **serve** — v1.x wrapper around the identical pipeline; security
requirements already fixed (ADR-0015 §6: HMAC verification, event dedup key, idempotent
publishing, per-repo tokens, target-ref policy loading) plus the ADR-0017 §4 reconciler
that revokes armed merges when expiring facts control them; keyed per-MR lock (P3-E5).
(b) **Merge budgets** (OQ resolved as execution policy, not score): per-run caps with
deterministic candidate ordering need no store; exact cross-run/window budgets are
supported only under one serialized sweep; forge-artifact ledgers may be adopted only with
proven atomicity; fail closed when the coordination guarantee is unavailable; audit event
per reservation and merge; `doctor` reports the provided guarantee. (c) **Batch sweep**
(OQ-20): evaluate many candidates, but every write goes through the same per-MR
preconditions, reconciliation, budgets, and audit records — no bulk bypass; first
implementation is one serialized sweep process, horizontal workers unsupported until a
lease mechanism exists. (d) **Post-merge audit** (OQ-19): correlate merged commits to
their DecisionRecord + PublicationReceipt, detect revert/rollback/operator-marked bad
outcomes, emit a durable safety event, optionally open a revert MR via a separately
authorized capability (never a direct revert push), and feed adjudicated outcomes into
policy comparison without treating every human revert as proof the decision was wrong.

### E13 — Remote policy packs — **Locked (D-012)**
`git::` pack sources pinned by commit SHA (tags are mutable), checksum/signature verified,
same target-ref/no-self-modification trust rules as local policy (ADR-0010 amendment,
ADR-0015 §1). Unlocks with a named consumer running a central policy repo (OQ-5).

### E14 — Kubernetes CRD/CR validation adapter — **Planned, gated on Spike D + ADR-0020**
Optional adapter **outside the generic decision core** (D-017 B11), committed only if the
[P2-E6 Spike D](p2-e6-spike-crd/spec.md) feasibility verdict supports it. Scope per the
spike-informed ADR-0020: multi-document YAML with preserved boundaries/positions; GVK
classification; stable subjects from GVK + namespace + name; CRD schemas loaded from the
trusted target ref, a checked-in pinned bundle, or a typed provider result with content
digest; CR instances validated against the matching CRD version's structural OpenAPI v3
schema with Kubernetes semantics (not a generic JSON Schema validator), including
`x-kubernetes-validations` CEL with old/new transition rules where the prior resource
exists; fail closed to REVIEW on missing schema, unsupported conversion/defaulting,
invalid CRD structure, or capability gaps; structured findings with GVK, EntryRef, schema
path, instance path, source span; schema + validator digests in
DecisionRecord/ReplayBundle. Trust rule is non-negotiable: gating validation uses the
**target-ref** schema — a branch-modified CRD may be compiled for observe-mode comparison
but cannot relax the schema judging its own MR; new CRD + first instances with no trusted
schema → human review; CRD definition changes are `require-review` by default. Explicitly
out: admission webhooks, mutating admission, live cluster state (optional trusted provider
territory, e.g. a pinned server-side dry-run fact); Helm/Kustomize source is consumed only
as rendered manifests or via a separately declared pinned render provider. **Exit gate**:
conformance fixtures pass (per Spike D corpus), not merely accepting CRD-shaped YAML.
