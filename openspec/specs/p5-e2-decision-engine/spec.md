# E2 — Decision engine + CEL predicate backend

**Problem**: P4-E1 shipped, and E1 hardened, the input half of the pipeline — a
format-agnostic, positioned, add/delete/rename `ChangeSet` plus a reserved-class classifier and
the four matcher-domain evaluation primitives (`internal/core/classify/matcher.go`). But the
*decision* half is still the walking-skeleton stub: `internal/core/aggregate/aggregate.go`
compiles and evaluates exactly **one** CEL string `when` per rule (no `all`/`any`/`not` tree, no
per-leaf message), binds only `old`/`new`/`changes` into the CEL env (not the frozen
predicate-scope set), consumes a **toy YAML** parsed by `cmd/assent/policy.go` (not the P3-frozen
`MergePolicy`/`RulesetBinding`/`Config` contracts), hardcodes every `Finding.Points` to `0`, has
no risk threshold, can never satisfy a `require-review` obligation (no ApprovalEvidence path), and
has no `off`/`observe`/`enforce` phase split — `internal/core/decision/record.go:209` hardcodes
`Observed: []`. E2 is where the decision engine described in ADR-0007 (effects + aggregation +
risk points), ADR-0013 (CEL `assert`/`when` backend), ADR-0017 (obligations, `require-review`,
one-shot arming, tri-state fail-safe), and ADR-0018 (phase/profile lifecycle) actually lands —
re-seated on the frozen P3 schemas — so the engine reproduces the strict D-016 §8 exit-gate
DecisionRecord (`schemas/d016_strict_fixture_test.go` +
`examples/contracts/d016-strict-fixture/`) from its own `EvaluationInput` + `MergePolicy` +
`RulesetBinding`, not just validates that fixture's shape.

**Scope**: retire the toy `cmd/assent/policy.go` YAML and load the frozen
`MergePolicy`/`RulesetBinding`/`Config`/`Pack` contracts under strict decode (unknown
field/enum/dup-key rejected) into engine types (E2-S01); re-seat the evaluator to run over the
frozen `EvaluationInput` (`changeSet`/`facts`/`mr`/`require`) with the **complete** frozen
predicate-scope activation model (`old`, `new`, `path`, `kind`, `file`, `entry`, `oldEntry`,
`changes`, `facts`, `mr`, `env` — `docs/planning/predicate-scope.md`), single-leaf `when` still
(E2-S02); add the `all`/`any`/`not` combinator walker over CEL leaves with per-leaf `message`
attribution into the finding (E2-S03); enforce full ADR-0017 §2 multi-obligation **AND** coverage
— every `require[]` obligation proven for every governed subject, uncovered/errored → fail-safe,
no `anyOf` (E2-S04); enforce the ADR-0007 F6 / ADR-0017 §6 fact tri-state fail-safe — a
controlling fact in state `unavailable`/`invalid`/`expired` can never yield APPROVE (E2-S05);
accrue author-declared `rule.points` per matched firing and apply the per-binding risk threshold
(`sum(points) ≤ risk.threshold ⇒ APPROVE`, over ⇒ REVIEW) (E2-S06); satisfy `require-review` only
from a **separately injected**, forge-proven `ApprovalEvidence` whose `pins.sourceSha` matches the
evaluated `sourceSha` (stale ⇒ unsatisfied; `verifyingCapability: none` ⇒ capability gap, never
auto-merge; author/bot excluded) (E2-S07); split evaluation into `off`/`observe`/`enforce` phases
with the pack-phase **ceiling**, routing observe findings to `findings.observed` (structurally
excluded from aggregation) and enforce findings to `findings.enforcing`, threading real `Observed`
into `record.go:209` (E2-S08); resolve the single covering `PolicyProfile` per `(environment,
class)` binding (coverage → specificity → config-order) and surface its `writes: true|false`
write-authority, recorder-only profiles never arming (E2-S09); and run the whole engine end-to-end
over the frozen D-016 strict fixture, reproducing its DecisionRecord byte-for-byte after canonical
serialization (E2-S10).

**Non-goals** (fenced to their owning epic — do not re-scope here):
- The declarative YAML *policy authoring/lint* frontend — the envelope loader's **authoring
  ergonomics**, `assent lint`'s hard-error enforcement of reserved-class/catch-all-vouch/unkeyed-
  list/undeclared-predicate-scope rules, and the `.assent/**` authoring surface — **E3**. E2 loads
  and *evaluates* the frozen schemas and rejects malformed packs at load (strict decode); it does
  not ship the human-facing `lint` pass, its diagnostics UX, or authoring conveniences. Where a
  rule below says "rejected at load," that is the engine's strict-decode/​structural refusal, not
  E3's lint diagnostics.
- **Comparison / `assent compare` / `PolicyComparisonSuite` / the five promotion gates** — **E6**.
  ADR-0018 §3 explicitly tags comparison "impl Phase 5+ / E6". E2 *produces* the DecisionRecords
  E6 later diffs (the `observed`/`enforcing` split E2-S08 lands is the raw material comparison
  consumes); it does not implement the delta taxonomy, the suite, or the `compare` command.
- The **forge** halves of `require-review` and one-shot arming: **fetching** ApprovalEvidence from
  GitLab/GitHub (approval_rules → eligible_approvers → approval_state chain, OQ-23 /
  `forge-dossier-gitlab.md` §4) and the **arm/revoke** merge-precondition call — **E4/E8**. E2 owns
  only the decision-side satisfaction *logic* over an already-injected `ApprovalEvidence` and the
  decision-side arming *precondition* ("do not treat auto-mergeable when a controlling authorization
  fact is expired/expiring"); no network, no forge session, no arm/revoke write.
- **Provider fact resolution** (the built-in/HTTP/exec provider host, `max_age` resolution, the
  provider request/response protocol) — **E5**. E2 consumes already-resolved, typed `facts` from
  `EvaluationInput` (states `resolved`/`unavailable`/`invalid`/`expired`); it never calls a
  provider. The `factsResolvedAt` pins are passed through, not computed here.
- `assent test` adopter harness / `test-expectation` schema — **E6**.
- kind/e2e/testcontainer infra, any live-GitLab proof — **E7** (runs alongside per the meta-plan
  ordering constraint, but is a separate epic/spec; see the E7 decide-and-log note below).
- **`match.fileEvents`** authored matching. The frozen `merge-policy.schema.json` `match` $def
  exposes four domains — `files`, `values`, `fileEvents`, `valueChanges` — but E1 **deferred**
  whole-file `fileEvents` (git-detected whole-file add/delete/rename) to a fast-follow after
  E1-S08; E1 shipped `files`, `values.pointers` (`MatchValuePointers`), `valueChanges`
  (`MatchValueChanges`), and a distinctly-named `entryEvents`/`EntryRef` primitive instead. E2
  therefore wires only the domains E1 actually implemented — `files`, `values`, `valueChanges`. An
  authored `match.fileEvents` block is **rejected at load** with a reason naming the deferred
  domain (never silently matched or silently skipped). The D-016 fixture uses only `files` and
  `valueChanges`, so this fence blocks nothing this epic must reproduce.
- A `score` **effect**. The frozen effect enums are `comment`/`challenge`/`block` (rule `effect`)
  and `+require-review` (`onFailure.effect`) — there is **no `score` effect** (consistent with
  OQ-13: points + per-binding thresholds only in v1, no score→effect escalation). Author-declarable
  **`points: N` is in v1**, however — `merge-policy.schema.json:98` carries an author `points`
  integer on the rule `$def` (allowed alongside a rule's `prove`/`onFailure` or `effect`, orthogonal
  to the obligation/effect `oneOf`), accruing per firing (ADR-0007 Amendment 2). E2-S06 **honors
  that authored `rule.points`**; it does not invent an effect→points default table (see the S06
  judgment call for how the D-016 golden's `points` are reproduced from authored values).

ADRs: **0007** (rule effects — `comment`/`challenge`/`block`/`require-review`; aggregation order
block→BLOCK, else unresolved challenge→REVIEW, else uncovered obligation→REVIEW, else `sum(points)
≤ threshold`→APPROVE; F6 tri-state predicate error is fail-safe **by effect**; F7 score is
intra-MR/stateless; points accrue per firing, not per rule; `onFail`/`onFailure` false-branch, and
**predicate error never takes the onFail branch**). **0013** (`assert`/`when` = `all`/`any`/`not`
tree over CEL-string leaves, bare string = single-leaf shorthand; per-leaf `message`; predicate
scope is frozen contract — `old`/`new`/`path`/`kind`/`file`/`entry`/`oldEntry`/`changes`/`facts`/
`mr`/`env`, canonical example `cel: new >= old`; determinism-free because CEL is
non-Turing-complete, side-effect-free, cost-budgeted; residual code risks: numeric YAML/HCL→CEL
coercion, missing-fact/unknown-field error UX, cost-limit + std-env purity, per-leaf trace wiring,
one activation model for CEL + message templates). **0017** (§2 required obligations replace vouch,
AND-only no `anyOf`; §3 `require-review` = forge-proven eligible approval never a bare vouch; §4
one-shot arming = `facts.max_age` as arming precondition; §5 governed subjects / `EntryRef` /
matcher domains; §6 tri-state fail-safe; §8 the strict end-to-end fixture this epic reproduces; §9
do-not-generalize — no `anyOf` obligations). **0018** (§1 phase `off`/`observe`/`enforce` required,
no default; observe evaluated but structurally excluded from aggregation; pack phase is a
**ceiling** not additive; §2 named profiles + single-writer invariant, precedence
coverage→specificity→config-order; §3 comparison = E6). **0008** (classifier / reserved
`assent-policy`/`unclassified` dominance E2 must preserve; local-checkout evaluation; `env` label).
**0011** (core ports; `Predicate.Eval` purity invariant). **0016** (unknown-field references are
load-time errors, never `<no value>`). Consumes: `internal/core/aggregate/aggregate.go` +
`aggregate_test.go` (P4-E1 walking-skeleton evaluator — grown in place), `internal/core/decision/
record.go` + `record_test.go` (the DecisionRecord/PresentationModel serializer — `Observed`
threaded), `internal/core/classify/{classify,matcher}.go` (reserved-class dominance + the four
matcher primitives — wired to authored `match`, not weakened), `cmd/assent/policy.go` (the toy YAML
loader — retired), `cmd/assent/run.go` (the run adapter — re-seated on the loader), the frozen
`schemas/**` (implemented against, never broken), `examples/contracts/d016-strict-fixture/**` (the
S10 golden). D-016/D-017 (the obligation/lifecycle model this engine enforces), D-033 (pins stay
out-of-band, passed through), D-034 (protected-source verification stays an E4/E5 placeholder —
untouched).

## Executability classification (autonomous vs infra-gated)

**Every story in this epic is `[autonomous]`.** E2 is pure `internal/core` engine code (+ the
`cmd/assent` loader/run adapter re-seat) evaluated over fixtures already checked into
`schemas/`, `examples/contracts/`, and `examples/repos/`, plus in-memory injected inputs
(`EvaluationInput`, `ApprovalEvidence`). No story opens a network connection, a forge session, a
provider call, or reads a token: facts and approval evidence enter **pre-resolved** as injected
data (preserving `TestEvaluationIsProviderless`), and all clock/env/random/network stays in
`cmd/assent`, barred from `internal/core/**` by `internal/core/purity_test.go`'s `TestCorePurity`
static scan (which also covers `internal/change/**`). The forge-side fetch/arm/revoke halves of
`require-review` and one-shot arming are explicitly deferred to E4/E8; E2's slices of both are pure
decision logic over injected data, gate-able against fixtures with no live infra. If a future story
here is found to need live infra, it must be split out and re-tagged explicitly — none currently do.

## Judgment calls fixed by this spec (log to the operator, not `decisions.md` — agent-authored spec, not a D-nnn judgment)

- **The frozen D-016 fixture's `partitions-must-not-shrink.when` is corrected from `input.new >=
  input.old` to `new >= old` as a mandatory early lane, landed before E2-S10 (recommended
  before/with E2-S02), independently reviewed, logged `🔴 DECIDED`** (it edits a P3-frozen
  artifact). This is a confirmed fixture bug, not a second activation model: ADR-0013's canonical
  predicate example is literally `cel: new >= old` (ADR-0013 §Decision, line 53/58) and the frozen
  predicate-scope table (`docs/planning/predicate-scope.md`) is categorical — `old`/`new` are
  top-level and "a reference to anything else \[e.g. `input`] is an assent lint hard error." The
  same fixture's *other* rule correctly uses in-scope `facts.owner.team.state`, so the inconsistency
  is internal — the signature of a typo, not a design. **The engine (E2-S02) binds the frozen
  top-level scope and must NOT add an `input` alias to accommodate the buggy example** (that is the
  "hardcode a fixture-specific special-case" anti-pattern). `TestD016StrictFixture` validates
  document *shape*, not CEL variable names (confirmed by reading it), so the correction keeps every
  existing schema test green and leaves the expected DecisionRecord unchanged (`6 >= 12` is false ⇒
  obligation `non-destructive` unproven ⇒ `onFailure` block/`partition-count-shrunk`, exactly the
  frozen finding). This correction is a prerequisite for a clean S10 reproduction, because
  the out-of-scope `input` reference **fails at compile/load** (ADR-0016: an undeclared identifier
  is a load-time error, not a runtime `<no value>` — the engine-side guarantee is REQ-E2-S02-02), so
  the pre-fix fixture would fail to *load at all* rather than produce the golden's block finding — so
  `new >= old` is *required* for reproduction, not merely tidier.
- **Finding `points` come from the author-declared `rule.points`, not an engine effect-default —
  and the D-016 fixture's block rule is missing its `points: 10`, corrected on lane F.** ADR-0007
  makes points **explicit** (`points: N`, allowed alongside any rule, accruing per firing —
  Amendment 2); there is no inherent point weight for the `block` effect. The frozen
  `merge-policy.schema.json:98` carries that author `points` field (an earlier draft of this spec
  wrongly claimed no such field exists — a grep that silently aborted on an unrelated glob; the
  field is real and central to ADR-0007's bulk-change guard). The frozen D-016 DecisionRecord shows
  `points: 10` on the `block` finding and `points: 0` on the `require-review` findings, yet the
  fixture's `partitions-must-not-shrink` rule authors **no** `points` — so the `10` currently has no
  contract source. **E2-S06 honors authored `rule.points` per firing with no engine effect→points
  default table**; to reproduce the golden's `10` via the sanctioned mechanism, the fixture-fix lane
  (F) additionally adds `points: 10` to the `partitions-must-not-shrink` rule (a second fixture
  correction, same `🔴 DECIDED` lane, independently reviewed — points is allowed on an
  obligation-proving rule, orthogonal to the `prove`/`onFailure`↔`effect` `oneOf`). The
  `require-review` rule authors no points, so its findings are `0` naturally. Because a `block`
  finding forces BLOCK by aggregation order #1, the threshold arithmetic never decides the D-016
  block case; S06's threshold path (`sum(points) ≤ risk.threshold ⇒ APPROVE`) is exercised by its
  own non-block goldens. If an implementer finds a contract reason `block` should instead carry an
  engine-assigned default weight, that is a `🔴 DECIDED` reconciliation in S06 (define + log the
  default) — but the leading, contract-faithful resolution is authored `rule.points`, which invents
  nothing.
- **`require-review` satisfaction reads a separately injected `ApprovalEvidence`, not an
  `EvaluationInput` field.** `evaluation-input.schema.json` is closed (`additionalProperties:
  false`) and carries no approval evidence; `approval-evidence.schema.json` is a distinct document
  whose `pins` is a cross-file `$ref` to `DecisionRecord.pins`. E2-S07 takes `ApprovalEvidence` as
  a second injected input to the engine (nil ⇒ unsatisfied), never as a schema extension. Any story
  that appears to need a new field on a frozen P3 schema is a red flag — the schema is frozen; find
  the real injection point.
- **E2 sequenced before E7 (decide-and-log, not a silent drop).** The 2026-07-30 handover paired
  E2 with E7 (kind/e2e infra). E2's exit gate — reproducing the D-016 DecisionRecord from injected
  fixtures — is pure-engine and needs no live GitLab, so E2 proceeds first and does not block on E7.
  E7 remains the next epic to claim after E2 (every later epic's *live* exit gate depends on it);
  this spec does not close or de-scope E7, it defers it by one epic with the engine as the
  higher-leverage first move (realized value of E1's primitives waits on a working decision engine,
  not on infra).
- **The `all`/`any`/`not` combinator walker (E2-S03) and profile resolution (E2-S09) are OFF the
  S10 critical path.** The D-016 fixture's two `when`s are bare single-leaf strings (no combinator)
  and it declares no `profile`, so S10's reproduction depends on S01,S02,S04,S05,S06,S07,S08 — not
  S03 or S09. S03 and S09 are independently valuable and are validated by their own goldens; they
  are numbered before S10 for narrative order but are not its prerequisites (see the dependency
  diagram).

## Dependency order

```
E2-S01 frozen-contract loader ──► E2-S02 evaluator re-seated on EvaluationInput + full CEL scope
                                        │  (single-leaf when)
      ┌─────────────────────────────────┼───────────────────────────────┐
      ▼                                 ▼                                 ▼
E2-S03 all/any/not walker        E2-S04 multi-obligation AND         E2-S05 fact tri-state
   + per-leaf message               coverage across subjects            fail-safe
   (off S10 critical path)             │
                            ┌──────────┼───────────────┐
                            ▼          ▼               ▼
                   E2-S06 points   E2-S07 require-   E2-S08 phase off/observe/enforce
                   + risk threshold  review via         + pack ceiling (threads record.go:209)
                                     injected evidence      │
                                                            ▼
                                                   E2-S09 profile resolution + single-writer
                                                   (off S10 critical path)

Fixture-fix lane (F): two corrections to d016 partitions-must-not-shrink —
   (1) `when: input.new>=input.old` -> `new>=old` (out-of-scope `input`, ADR-0013/predicate-scope);
   (2) add missing `points: 10` (golden shows points 10 but the rule authors none — S06 has no
   engine default). 🔴 DECIDED, independently reviewed, landed before S10 (recommend before/with S02).

E2-S10 D-016 strict-fixture end-to-end reproduction ── depends on S01,S02,S04,S05,S06,S07,S08 (+ F)
```

**First slice: E2-S01 (frozen-contract loader).** It is the smallest independently-valuable slice
(a repo can load and structurally validate its real `MergePolicy`/`RulesetBinding`/`Config`
instead of the toy YAML), it retires the single largest source of drift between the engine and the
frozen contracts (`cmd/assent/policy.go`), and every later story consumes its output. The
**fixture-fix lane (F)** is independently startable from day one (it only edits one CEL string in a
fixture + a `🔴 DECIDED` note) and should land early so S10 is a clean reproduction rather than a
surprise.

---

## E2-S01 — Frozen-contract policy loader (retire the toy YAML) `[autonomous]`

**As a** repo owner **I want** `assent` to load my real `MergePolicy`, `RulesetBinding`, and
`Config` documents (the P3-frozen schemas) under strict decode **so that** the engine evaluates the
same contract my packs are authored and schema-validated against, not a private toy dialect that
can silently diverge.

**Goal**: add a loader in `internal/core` (a new package, e.g. `internal/core/policy`) that decodes
`MergePolicy`/`RulesetBinding`/`Config`/`Pack` YAML/JSON into engine types under **strict decode**
— unknown field, unknown enum value, and duplicate map key are all hard-rejected with a reason
naming the offending location (mirroring `schemas/compat_strictdecode_test.go`'s discipline), never
silently ignored. Strict-decode is enforced by reusing the **frozen JSON Schemas** (the `schemas`
package's compiled `MergePolicySchema`/`RulesetBindingSchema`/`ConfigSchema`/`PackSchema` — the
same authority `schemas/compat_strictdecode_test.go` validates against, so `additionalProperties:
false`/enum/`required`/`uniqueItems` are not re-implemented and cannot drift), then decoding into
engine types. An authored `match.fileEvents` domain — allowed by the schema but deferred by E1 — is
rejected by a **loader-level** semantic check naming the deferred domain (Non-goals fence). The
`assertTree` in `prove.when` is decoded **structurally** (bare-string | leaf | combinator one-of,
shape-validated) but **not** compiled to CEL — `cel.Compile` is deferred to E2-S02 — so S01 stays
off cel-go and pure. **Design constraint: `internal/core/policy` is self-contained and does NOT
import `internal/core/aggregate`** (S02 makes `aggregate` consume `policy`; a `policy → aggregate`
import now would cycle) — the `Effect`/`OnFailure`/rule types live in `policy`. Pure: no
clock/env/network/random. **The `cmd/assent/run.go` re-seat + `cmd/assent/policy.go` deletion are
NOT in this story** — see the Not-in-scope note (they depend on `EvaluationInput`, an S02
deliverable).

**Operator input**: no.

**Dependencies**: none (retires existing toy loader; every later story consumes it).

**Definition of done**: the frozen `examples/contracts/d016-strict-fixture/merge-policy.json`,
`ruleset-binding.json`, and a minimal `Config` load into engine types with every field surfaced
(`spec.entries[].{mode,root,identity.pointer}`, `spec.rules[].{name,phase,match,prove.{obligation,
when},onFailure.{effect,code},effect}`, `bindings[].{class,environment,packs,risk.threshold,
require}`); an unknown field / unknown enum / duplicate key each fail load with a located reason; an
authored `match.fileEvents` fails load naming the deferred domain; the `prove.when` assertTree is
decoded structurally (not CEL-compiled); `TestCorePurity` stays green over the new package; and the
package does not import `internal/core/aggregate`.

**Not in scope**: evaluating any rule / compiling any CEL (E2-S02); **the `cmd/assent/run.go`
re-seat + `cmd/assent/policy.go` deletion** — moved to E2-S02 (REQ-E2-S02-06), because the live run
path selects the governed file from `binding.Subject` (`run.go:159`) and the frozen `MergePolicy`
has **no** `subject` field: per-change governed subjects live in `EvaluationInput.changeSet.
changes[].subject`, an S02 deliverable, so re-seating run.go before `EvaluationInput` exists would
only preserve throwaway toy single-subject file-selection S02 immediately deletes
(implementation-discovered refinement, folded into S01's lane); `assent lint`'s human diagnostics UX
(E3 — this is strict *decode*, structural refusal, not lint); `ApprovalEvidence` loading (E2-S07);
profile/pack precedence resolution (E2-S08/S09 — this story decodes `Pack.spec.phase` and
`Config.profiles` into types but does not yet apply the ceiling/precedence).

Requirements:

- **REQ-E2-S01-01** — Given the frozen `d016-strict-fixture/merge-policy.json`, when the loader
  decodes it, then every field the engine will later consume is surfaced on the engine type —
  `spec.entries["topic-registry"].{mode:"document",identity.pointer:"/metadata/name"}`, both
  `spec.rules[]` with their `name`/`phase`/`match`/`prove.{obligation,when}`/`onFailure.{effect,
  code}` — with no lossy round-trip.
  - Test: `internal/core/policy/loader_test.go`
  - Verify: `go test ./internal/core/policy/... -run TestLoadMergePolicyFixture`
  - Level: L0
- **REQ-E2-S01-02** — Given a `MergePolicy`/`RulesetBinding`/`Config` document with (a) an unknown
  top-level or nested field, (b) an unknown enum value (e.g. `phase: audit`, `onFailure.effect:
  approve`), or (c) a duplicate mapping key, when the loader decodes it, then it returns a load
  error whose message names the offending field/enum/key location — never a silent drop or a
  zero-valued field (mirroring `compat_strictdecode_test.go`).
  - Test: `internal/core/policy/loader_test.go`
  - Verify: `go test ./internal/core/policy/... -run TestStrictDecodeRejectsUnknownAndDuplicate`
  - Level: L0
- **REQ-E2-S01-03** — Given a `MergePolicy` rule whose `match` uses the `fileEvents` domain, when
  the loader decodes it, then it is rejected at load with a reason naming `fileEvents` as a deferred
  (E1 fast-follow) domain — never silently loaded (which would later match nothing and read as "no
  such change") — while `files`, `values`, and `valueChanges` load normally.
  - Test: `internal/core/policy/loader_test.go`
  - Verify: `go test ./internal/core/policy/... -run TestFileEventsDomainRejectedAtLoad`
  - Level: L0
- **REQ-E2-S01-04** — Given the determinism rule, when the loader package lands, then
  `TestCorePurity` stays green over `internal/core/**` (the loader introduces no
  env/clock/network/random import), and decoding the same document twice yields structurally
  identical engine types (no map-iteration-order-dependent field surfaced).
  - Test: `internal/core/policy/loader_test.go`
  - Verify: `go test ./internal/core/policy/... -run TestLoaderDoubleRunStable`
  - Level: L0

## E2-S02 — Evaluator re-seated on `EvaluationInput` + full frozen predicate scope `[autonomous]`

**As a** rule author **I want** my CEL `when` to see the complete frozen predicate-scope set
(`old`, `new`, `path`, `kind`, `file`, `entry`, `oldEntry`, `changes`, `facts`, `mr`, `env`)
evaluated over the real `EvaluationInput` **so that** a predicate like `new >= old` or
`facts.owner.team.state == 'resolved'` resolves against the actual change and resolved facts, not
the walking-skeleton's `old`/`new`/`changes`-only env fed by a toy struct.

**Goal**: re-point `internal/core/aggregate`'s evaluator at the frozen `EvaluationInput`
(`changeSet.changes[]` with `subject`/`file`/`path`/`kind`/`old`/`new`, typed `facts`, `mr`,
`require[]`) loaded via E2-S01, and build the cel-go activation model that binds **exactly** the
frozen predicate-scope top-level fields (`docs/planning/predicate-scope.md`) — no more (a reference
to an undeclared identifier such as `input` surfaces as a load/compile error, never a silent
`<no value>`, per ADR-0016) and no less. Single-leaf `when` (a bare CEL string) still — the
`all`/`any`/`not` walker is E2-S03. Numeric YAML/HCL→CEL coercion (ADR-0013's highest residual
risk) is closed here: `old`/`new` int/double values compare injectively (a fail-safe REVIEW on a
coercion error, never a silent wrong answer). The cel-go env registers **zero** non-deterministic
functions/macros (no `time`/`now`/`rand`) and applies a cost budget.

**Operator input**: no.

**Dependencies**: E2-S01 (the loader that produces the rules this evaluator runs). The fixture-fix
lane (F) should land before this story so the D-016 `partitions-must-not-shrink.when` is in-scope
(`new >= old`).

**Definition of done**: a single-leaf `when: "new >= old"` over a `/partitions` modify (old=12,
new=6) evaluates to false; a `when: "facts.owner.team.state == 'resolved'"` over an expired owner
fact evaluates to false; a `when` referencing an undeclared identifier (`input.new`, `foo`) is
rejected at compile with a reason naming the undeclared reference — never `<no value>`; the cel-go
env exposes only the eleven frozen scope fields; the walking-skeleton `aggregate_test.go` fail-safe
suite (`TestPredicateErrorFailsSafe`, `TestOpaqueChangeSetFailsSafe`,
`TestAggregationOrderIndependent`, `TestDeterminismDoubleRun`) stays green; `TestCorePurity` green.

**Not in scope**: `all`/`any`/`not` trees + per-leaf message (E2-S03); multi-obligation coverage
beyond the single-obligation the skeleton already loops (E2-S04); points/threshold (E2-S06);
`require-review` evidence (E2-S07); phase split (E2-S08).

Requirements:

- **REQ-E2-S02-01** — Given a rule with single-leaf `when: "new >= old"` matched against a
  `/partitions` modify change (`old: 12`, `new: 6`) from an `EvaluationInput`, when the evaluator
  runs, then the predicate evaluates to **false** (obligation unproven) using the top-level `old`/
  `new` activation bindings — proving the frozen predicate scope, not a toy `input.`-wrapped model.
  - Test: `internal/core/aggregate/aggregate_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestSingleLeafNewOldOverEvaluationInput`
  - Level: L0
- **REQ-E2-S02-02** — Given a rule whose `when` references an identifier **not** in the frozen
  predicate-scope table (`input.new`, `object`, `foo`), when the evaluator compiles it, then
  compilation fails with a reason naming the undeclared reference (ADR-0016: unknown reference is a
  load-time error), never a runtime `<no value>` or a silent false. Adversarial case: `input.new >=
  input.old` (the pre-fix D-016 typo) is rejected here — this REQ is the engine-side guarantee that
  the fixture-fix lane (F) is *required*, not optional.
  - Test: `internal/core/aggregate/aggregate_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestUndeclaredPredicateReferenceRejected`
  - Level: L0
- **REQ-E2-S02-03** — Given the eleven frozen predicate-scope fields, when a `when` references each
  of `old`/`new`/`path`/`kind`/`file`/`entry`/`oldEntry`/`changes`/`facts`/`mr`/`env`, then each
  resolves to the corresponding `EvaluationInput`-derived value (e.g. `facts.owner.team.state`,
  `mr.author`, `kind == 'modify'`) and compiles clean; a twelfth invented field does not.
  - Test: `internal/core/aggregate/aggregate_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestFrozenPredicateScopeExactlyBound`
  - Level: L0
- **REQ-E2-S02-04** — Given two numeric values that would collapse or mis-order under a lossy
  coercion (a large integer vs. a near double, an int/double type mismatch), when a `when` such as
  `new >= old` compares them, then the comparison is injective/correct or fails safe to REVIEW with
  the coercion error surfaced — never a silent wrong boolean (ADR-0013 residual risk #1). Adversarial
  case: an `old`/`new` pair crafted to mis-compare under naive float coercion is proven correct or
  fail-safe.
  - Test: `internal/core/aggregate/aggregate_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestNumericCoercionInjectiveOrFailSafe`
  - Level: L0
- **REQ-E2-S02-05** — Given the determinism rule, when the evaluator re-seat lands, then the cel-go
  env registers no `time`/`now`/`rand`/non-deterministic function or macro, applies a cost budget,
  `TestCorePurity` stays green over `internal/core/**`, and evaluating the same `EvaluationInput` +
  policy twice yields a byte-identical finding set after canonical sort.
  - Test: `internal/core/aggregate/aggregate_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestEvaluatorDoubleRunStable`
  - Level: L0
- **REQ-E2-S02-06** — Given `EvaluationInput` now exists (this story), when `cmd/assent/run.go` is
  re-seated to load the frozen `MergePolicy`/`RulesetBinding` via the E2-S01 loader and build an
  `EvaluationInput` (facts-empty until E5 — an empty `facts` map is fail-safe, never APPROVE on a
  fact-referencing obligation) for the evaluator, then the toy `cmd/assent/policy.go` no longer
  exists and `cmd/assent/run_test.go` passes against real `MergePolicy`/`RulesetBinding` fixtures —
  retiring the toy loader (moved here from E2-S01 because governed subjects live per-change in
  `EvaluationInput`, absent before this story). The live path still reads content from the local
  checkout (ADR-0008 §4); this REQ changes policy *loading* + input *shape*, not file sourcing.
  - Test: `cmd/assent/run_test.go`
  - Verify: `go test ./cmd/assent/...`
  - Level: L1

## E2-S03 — `all`/`any`/`not` combinator walker + per-leaf message `[autonomous]`

**As a** rule author **I want** to compose CEL leaves with `all`/`any`/`not` and attach a
`message` to each leaf **so that** a multi-condition `assert` (e.g. "`new >= old` AND `new <=
facts.quota.max_partitions`") tells the contributor **which** conjunct failed, instead of one
opaque bare-string pass/fail.

**Goal**: implement a small bespoke combinator walker over cel-go leaves for the frozen
`assertTree` `$def` (`all`/`any`/`not` combinator over `leaf{cel,message}`, bare string =
single-leaf shorthand): `all` = AND (short-circuit to the first failing leaf for attribution),
`any` = OR, `not` = negation; each leaf's `message` (with `{{ old }}`/`{{ new }}`/`{{
facts.* }}` template expansion sharing the single activation model, ADR-0013 residual #5) names the
failing leaf in the resulting finding. Tri-state preserved: a leaf that errors propagates as error
(fail-safe by effect), it does not silently read as false.

**Operator input**: no.

**Dependencies**: E2-S02 (leaves are the single-leaf evaluator this story composes). **Off the S10
critical path** — D-016's `when`s are bare strings; this story is validated by its own goldens.

**Definition of done**: a two-leaf `all` where the second leaf fails produces a finding whose
message is the second leaf's; an `any` short-circuits to satisfied on the first true leaf; a `not`
inverts; a bare-string `when` still behaves exactly as E2-S02 (shorthand equivalence proven by a
golden); a leaf that errors inside an `all`/`any`/`not` propagates as error (fail-safe), never
silent-false; every golden double-runs.

**Not in scope**: obligation coverage across subjects (E2-S04); the `assent lint` check that a
leaf's message references only in-scope template fields (E3); nesting-depth limits on the tree
beyond a documented cost/depth ceiling.

Requirements:

- **REQ-E2-S03-01** — Given `when: {all: [{cel: "new >= old", message: "…"}, {cel: "new <=
  facts.quota.max_partitions", message: "over quota"}]}` where the first leaf holds and the second
  fails, when the walker evaluates it, then the overall result is false and the finding carries the
  **second** leaf's message ("over quota") — per-leaf failure attribution, not a whole-tree opaque
  fail.
  - Test: `internal/core/aggregate/assert_tree_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestAllShortCircuitsToFailingLeafMessage`
  - Level: L0
- **REQ-E2-S03-02** — Given an `any` tree with one true and one false leaf, when the walker
  evaluates it, then the result is true (satisfied); given a `not` over a true leaf, the result is
  false — proving OR/negation semantics.
  - Test: `internal/core/aggregate/assert_tree_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestAnyAndNotSemantics`
  - Level: L0
- **REQ-E2-S03-03** — Given a bare-string `when: "new >= old"` and the single-leaf-object form
  `when: {cel: "new >= old"}`, when both are evaluated over the same change, then they produce
  byte-identical findings — proving the bare string is exact shorthand for a one-leaf tree (no
  behavioural fork).
  - Test: `internal/core/aggregate/assert_tree_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestBareStringEqualsSingleLeaf`
  - Level: L0
- **REQ-E2-S03-04** — Given a leaf that errors (references an expired fact's absent `value`, or a
  type mismatch) nested inside an `all`/`any`/`not`, when the walker evaluates it, then the error
  propagates as a tri-state error (fail-safe by effect per ADR-0007 F6) — never silently collapsed
  to false (which could let an `any` spuriously satisfy or a `not` spuriously fire). Adversarial
  case: `any: [<erroring leaf>, <false leaf>]` does not resolve satisfied.
  - Test: `internal/core/aggregate/assert_tree_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestErroringLeafInTreeFailsSafe`
  - Level: L0
- **REQ-E2-S03-05** — Given the determinism rule, when the walker lands, then `TestCorePurity`
  stays green, tree evaluation is order-independent for `all`/`any` result (only the attributed
  message follows declared order), and every golden double-runs byte-identical.
  - Test: `internal/core/aggregate/assert_tree_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestAssertTreeDoubleRunStable`
  - Level: L0

## E2-S04 — Multi-obligation AND coverage across subjects `[autonomous]`

**As a** platform operator **I want** every obligation my binding lists in `require[]` to be
independently proven for **every** governed subject the MR touches **so that** an MR changing two
topics cannot auto-merge because only one topic's ownership was proven, and cannot slip an
unproven obligation past coverage.

**Goal**: implement full ADR-0017 §2 obligation coverage over the frozen model: a binding's
`require: [ownership, non-destructive]` is satisfied only when, for **each** required obligation
name and **each** governed `subject` present in the `changeSet`, a `prove.{obligation, when}` rule
matched that subject and its `when` held; an uncovered obligation (no proving rule fired for a
subject) or an errored proof is fail-safe — the obligation is **unproven**, its rule's `onFailure`
effect fires (per subject), and the run can never APPROVE. AND-only: no `anyOf`/alternative-proof
composition (ADR-0017 §9 do-not-generalize) — a `require` list is a conjunction. This grows the
walking-skeleton's single-obligation loop (`aggregate.go:233`, "this slice carries exactly one
required obligation") into multi-obligation × multi-subject coverage.

**Operator input**: no.

**Dependencies**: E2-S02 (the per-rule evaluator whose truth this coverage aggregates).

**Definition of done**: with `require: [ownership, non-destructive]` and two governed subjects
(`topic-registry:orders.events.v1`, `topic-registry:payments.settled.v2`), when `ownership` is
unproven for both (expired fact) and `non-destructive` is unproven for one (partition shrink), the
coverage produces one `require-review` finding per subject for the unproven `ownership` and one
`block` finding for the failed `non-destructive` — i.e. the exact three enforcing findings of the
D-016 golden (subject-scoped); an authored `anyOf`/alternative-obligation construct is rejected at
load (E2-S01 strict decode); a fully-covered MR (every obligation proven for every subject) yields
no obligation finding; every golden double-runs.

**Not in scope**: the risk-points threshold (E2-S06 — coverage decides *obligation* satisfaction,
points decide the residual risk check); `require-review`'s ApprovalEvidence satisfaction (E2-S07 —
this story treats `require-review` as unproven-until-evidence, which S07 then satisfies); phase
split (E2-S08).

Requirements:

- **REQ-E2-S04-01** — Given `require: [ownership, non-destructive]` and a `changeSet` touching two
  governed subjects, when only one subject's `non-destructive` fails and both subjects' `ownership`
  is unproven, then coverage emits a subject-scoped finding for **each** unproven (obligation,
  subject) pair — proving obligations are covered per subject, not once per MR.
  - Test: `internal/core/aggregate/coverage_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestObligationCoveragePerSubject`
  - Level: L0
- **REQ-E2-S04-02** — Given a `require[]` obligation for which **no** rule's `prove.obligation`
  matches a governed subject (uncovered), when coverage runs, then that obligation is **unproven**
  for that subject and the run cannot APPROVE — an uncovered obligation is fail-safe, never
  vacuously satisfied. Adversarial case: a `require: [ownership]` with a pack that proves only
  `non-destructive` yields REVIEW/BLOCK, never APPROVE.
  - Test: `internal/core/aggregate/coverage_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestUncoveredObligationFailsSafe`
  - Level: L0
- **REQ-E2-S04-03** — Given an authored attempt at alternative-proof composition (an `anyOf`-style
  obligation, or a `require` entry that no `prove` rule names), when the pack loads, then it is
  rejected (strict decode / coverage lint) — AND-only conjunction is the only obligation semantics
  (ADR-0017 §9). No silent OR.
  - Test: `internal/core/aggregate/coverage_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestNoAnyOfObligationComposition`
  - Level: L0
- **REQ-E2-S04-04** — Given a fully-covered MR (every `require[]` obligation proven true for every
  governed subject) with no block/challenge and score under threshold, when coverage runs, then it
  contributes **no** obligation finding and does not by itself prevent APPROVE — proving coverage is
  not a permanent REVIEW floor. (Full APPROVE still requires S06 threshold + S07 evidence where
  applicable; this REQ isolates that coverage alone, when satisfied, is silent.)
  - Test: `internal/core/aggregate/coverage_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestFullyCoveredEmitsNoObligationFinding`
  - Level: L0
- **REQ-E2-S04-05** — Given the determinism rule, when coverage runs, then it is order-independent
  over both the obligation list and the subject set (shuffling either yields a byte-identical
  finding set after canonical sort), `TestCorePurity` stays green, and every golden double-runs.
  - Test: `internal/core/aggregate/coverage_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestCoverageOrderIndependent`
  - Level: L0

## E2-S05 — Fact tri-state fail-safe (`unavailable`/`invalid`/`expired` never APPROVE) `[autonomous]`

**As a** platform operator **I want** a change whose controlling/authorization fact is
`unavailable`, `invalid`, or `expired` to never auto-merge **so that** a stale team-directory
lookup or a failed provider call fails closed to human review, not open to an approval it can no
longer justify (the decision-side of ADR-0017 §4 one-shot arming).

**Goal**: enforce the ADR-0007 F6 / ADR-0017 §6 tri-state discipline in the evaluator over the
frozen `facts` states (`resolved`/`unavailable`/`invalid`/`expired`): a `when` that references a
non-`resolved` fact's `value` (absent for `unavailable`/`invalid`/`expired` per
`evaluation-input.schema.json`) evaluates to a tri-state **error**, which is fail-safe by effect
(an obligation-proof error ⇒ obligation unproven ⇒ `onFailure` fires / never APPROVE); a `when`
that explicitly guards on `.state` (as the D-016 owner rule does: `facts.owner.team.state ==
'resolved'`) evaluates to false when the state is not `resolved`, which likewise leaves the
obligation unproven. This is the **decision-side arming precondition**: a controlling authorization
fact that is expired/expiring cannot make a run auto-mergeable. The forge arm/revoke write is E4.

**Operator input**: no.

**Dependencies**: E2-S02 (fact binding into the activation model).

**Definition of done**: an `expired` owner fact makes `facts.owner.team.state == 'resolved'` false
(⇒ ownership unproven ⇒ `require-review`, matching D-016); a `when` reading an `expired`/`invalid`
fact's absent `value` errors and fails safe (obligation unproven), never silently true/false in the
permissive direction; a `resolved` fact whose value satisfies the predicate proves normally; a
controlling provider configured `failure: open` for an authorization-class fact is rejected (a
controlling fact may not fail open — ADR-0017 §6); every golden double-runs.

**Not in scope**: provider fact *resolution* / `max_age` computation / the provider protocol (E5 —
facts arrive pre-resolved with their `state` already set); the forge arm/revoke merge-precondition
write (E4); `require-review` ApprovalEvidence (E2-S07).

Requirements:

- **REQ-E2-S05-01** — Given a `when: "facts.owner.team.state == 'resolved'"` and an owner fact in
  state `expired`, when the evaluator runs, then the predicate is **false** (obligation unproven),
  the rule's `onFailure` `require-review` fires, and the run cannot APPROVE — the D-016 owner-rule
  path.
  - Test: `internal/core/aggregate/facts_tristate_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestExpiredFactStateGuardFailsClosed`
  - Level: L0
- **REQ-E2-S05-02** — Given a `when` that references a non-`resolved` fact's **`value`** (absent for
  `unavailable`/`invalid`/`expired`), when the evaluator runs, then the leaf evaluates to a
  tri-state **error** that is fail-safe by effect (obligation unproven / effect fires), never a
  silent true or false in the permissive direction (ADR-0007 F6). Adversarial case: each of
  `unavailable`, `invalid`, `expired` is proven to fail closed, not just `expired`.
  - Test: `internal/core/aggregate/facts_tristate_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestAbsentFactValueErrorsFailSafe`
  - Level: L0
- **REQ-E2-S05-03** — Given a controlling/authorization-class fact whose provider is configured
  `failure: open`, when the policy loads/evaluates, then it is rejected — a controlling fact may not
  fail open (ADR-0017 §6); `failure: open` is permitted only for non-controlling advisory facts.
  - Test: `internal/core/aggregate/facts_tristate_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestControllingFactMayNotFailOpen`
  - Level: L0
- **REQ-E2-S05-04** — Given a `resolved` fact whose `value` satisfies the predicate (e.g.
  `facts.quota.max_partitions` present and the new value under it), when the evaluator runs, then
  the obligation proves normally — proving the fail-safe does not over-fire on healthy facts.
  - Test: `internal/core/aggregate/facts_tristate_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestResolvedFactProvesNormally`
  - Level: L0
- **REQ-E2-S05-05** — Given the determinism rule, when the tri-state logic lands, then
  `TestCorePurity` stays green (no clock introduced to compare `expiresAt` — expiry state is
  **pre-computed** by the provider tier and carried in `facts[].state`, never recomputed against
  `time.Now` in `internal/core`), and every golden double-runs.
  - Test: `internal/core/aggregate/facts_tristate_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestFactTristateDoubleRunStable`
  - Level: L0

## E2-S06 — Points per firing + per-binding risk threshold `[autonomous]`

**As a** platform operator **I want** each rule to be able to declare `points: N` that accrue per
firing and the binding's `risk.threshold` to gate APPROVE **so that** many small oddities can
accumulate to "human, please look" even when no single rule blocks, and the D-016 findings carry
their exact `points` (block=10, require-review=0).

**Goal**: implement the ADR-0007 aggregation tail over the **author-declared** `rule.points` field
(`merge-policy.schema.json:98`, an integer ≥ 0 allowed alongside a rule's `prove`/`onFailure` or
`effect`): a rule's authored `points` accrue **per matched firing** (ADR-0007 Amendment 2, not per
rule — the built-in bulk-change guard), summed across all findings, and compared to the active
binding's `risk.threshold`: `sum(points) ≤ threshold ⇒ APPROVE` (when obligations covered + no
block/unresolved-challenge), over threshold ⇒ REVIEW. A rule that authors no `points` contributes
0. **There is no engine effect→points default table** — a finding's `points` is exactly its rule's
authored value (ADR-0007 makes points explicit; `block` has no inherent weight). Retires the
walking-skeleton's hardcoded `Finding.Points = 0` (`aggregate.go:242,258,272,304`).

**Operator input**: no.

**Dependencies**: E2-S04 (points participate in aggregation only after obligation coverage /
block / challenge are decided — aggregation order #4, the last check). The fixture-fix lane (F)
adds the missing `points: 10` to the D-016 `partitions-must-not-shrink` rule so REQ-E2-S06-01
reproduces the golden from an authored value (see the epic judgment call).

**Definition of done**: a rule authoring `points: 10` produces a finding with `points: 10` and a
rule authoring none produces `points: 0` (reproducing the D-016 golden from authored values after
lane F); N firings of a `points: k` rule accrue `N×k` (per-firing, proven by a bulk-change golden:
e.g. ten `points: 1` firings against threshold 4 ⇒ REVIEW); an MR with covered obligations, no
block/challenge, and `sum(points) ≤ threshold` ⇒ APPROVE; the same MR with `sum(points) > threshold`
⇒ REVIEW; the block case's points never change the decision (block dominates by order #1) — the
threshold path is exercised by non-block goldens; every golden double-runs.

**Not in scope**: a `score` **effect** (no such enum — Non-goals; only `points: N` exists, and it
is in v1); cross-MR score accumulation (ADR-0007 F7 — intra-MR/stateless only); threshold
*downgrade* (force challenge→block in prod — OQ-13 resolved out of v1); any engine-assigned default
point weight per effect (explicitly rejected — points are author-declared).

Requirements:

- **REQ-E2-S06-01** — Given the D-016 rules after lane F (the `partitions-must-not-shrink` rule
  authoring `points: 10`, the `topic-owner-must-approve` rule authoring none), when aggregation
  runs, then the `block` finding carries `points: 10` and each `require-review` finding carries
  `points: 0` — reproducing the golden from **authored** `rule.points` values, with no engine
  effect→points default. Adversarial case: a rule authoring no `points` never acquires a nonzero
  weight from its effect alone.
  - Test: `internal/core/aggregate/points_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestAuthoredRulePointsReproduceGolden`
  - Level: L0
- **REQ-E2-S06-02** — Given a points-bearing rule that matches **N** changes, when aggregation
  runs, then points accrue **per firing** (N× the weight), not once per rule (ADR-0007 Amendment 2).
  Adversarial case: ten small firings against a low threshold sum over it ⇒ REVIEW — the bulk-change
  guard, proving salami-slicing within one MR does not defeat the threshold.
  - Test: `internal/core/aggregate/points_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestPointsAccruePerFiring`
  - Level: L0
- **REQ-E2-S06-03** — Given covered obligations, no block, no unresolved challenge, and `sum(points)
  ≤ binding.risk.threshold`, when aggregation runs, then the decision is **APPROVE**; with the same
  inputs but `sum(points) > threshold`, the decision is **REVIEW** — the aggregation-order #4 risk
  check.
  - Test: `internal/core/aggregate/points_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestRiskThresholdGatesApprove`
  - Level: L0
- **REQ-E2-S06-04** — Given a `block` finding present alongside any points total, when aggregation
  runs, then the decision is **BLOCK** regardless of the points sum (order #1 dominates #4) —
  proving points never rescue or worsen a block-bearing MR's decision.
  - Test: `internal/core/aggregate/points_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestBlockDominatesPointsTotal`
  - Level: L0
- **REQ-E2-S06-05** — Given the determinism rule, when points/threshold land, then the sum is
  order-independent over the firing set, `TestCorePurity` stays green, and every golden
  double-runs byte-identical.
  - Test: `internal/core/aggregate/points_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestPointsDoubleRunStable`
  - Level: L0

## E2-S07 — `require-review` via injected `ApprovalEvidence` `[autonomous]`

**As a** platform operator **I want** a `require-review` obligation satisfied only by forge-proven,
eligible, non-stale approval evidence **so that** an MR needing an owner's approval cannot
auto-merge on a bare vouch, a self-approval, or an approval given against a since-superseded commit
(ADR-0017 §3).

**Goal**: implement `require-review` satisfaction over a **separately injected** `ApprovalEvidence`
(nil ⇒ unsatisfied ⇒ the `require-review` finding stands, as in D-016): an obligation whose
`onFailure.effect` is `require-review` is satisfied **iff** an `ApprovalEvidence` is present whose
`pins.sourceSha` equals the evaluated `EvaluationInput`/DecisionRecord `sourceSha` (stale evidence
— sha mismatch — never satisfies), `verifyingCapability` is a real capability
(`approval-rules-api`/`codeowners`, not `none`), `approvalsRequired` is met by `approvedBy[]` after
excluding the MR author and bots (`eligibility`/`isAuthor: false`), and the evidence is not expired.
`verifyingCapability: none` ⇒ a **capability gap** recorded in `pins.capabilityGap`, which can never
auto-merge (fail-closed). No forge fetch — the evidence is injected pre-fetched (E4 fetches it).

**Operator input**: no.

**Dependencies**: E2-S04 (`require-review` is an obligation whose coverage this story satisfies).

**Definition of done**: with no `ApprovalEvidence` injected, a `require-review` obligation stays
unsatisfied — the finding stands (D-016 case (e)); with a valid, eligible, sha-matching evidence,
it is satisfied and no `require-review` finding is emitted for that subject; a **stale** evidence
(`pins.sourceSha` ≠ evaluated `sourceSha`) does **not** satisfy (the fail-open trap closed); a
self-approval (`approvedBy` == `mr.author`) or bot approval does not count toward `approvalsRequired`;
`verifyingCapability: none` records a capability gap and never auto-merges; every golden double-runs.

**Not in scope**: **fetching** the evidence from GitLab/GitHub (E4/E8 — the approval_rules →
eligible_approvers → approval_state chain, OQ-23); the forge arm/revoke write (E4); GitHub's
required-conversation-resolution parity mapping (E8).

Requirements:

- **REQ-E2-S07-01** — Given a `require-review` obligation and **no** injected `ApprovalEvidence`,
  when the engine runs, then the obligation is unsatisfied and a `require-review` finding is emitted
  for the subject (the D-016 case (e) path) — never satisfied by absence.
  - Test: `internal/core/aggregate/require_review_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestRequireReviewUnsatisfiedWithoutEvidence`
  - Level: L0
- **REQ-E2-S07-02** — Given an `ApprovalEvidence` whose `pins.sourceSha` **differs** from the
  evaluated `sourceSha` (stale — approval given against a superseded commit), when the engine
  evaluates `require-review`, then the obligation is **not** satisfied (the finding stands) — the
  staleness fail-open trap is closed. Adversarial case: an otherwise fully-eligible, capability-
  proven evidence with only a sha mismatch still fails to satisfy.
  - Test: `internal/core/aggregate/require_review_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestStaleApprovalEvidenceDoesNotSatisfy`
  - Level: L0
- **REQ-E2-S07-03** — Given an `ApprovalEvidence` whose `approvedBy[]` contains only the MR author
  (self-approval) or a bot, when the engine evaluates `require-review`, then those approvals do
  **not** count toward `approvalsRequired` (author/bot exclusion, `isAuthor: false`) and the
  obligation is unsatisfied.
  - Test: `internal/core/aggregate/require_review_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestSelfAndBotApprovalExcluded`
  - Level: L0
- **REQ-E2-S07-04** — Given an `ApprovalEvidence` with `verifyingCapability: none`, when the engine
  evaluates it, then a capability gap is recorded (`pins.capabilityGap`) and the run can never
  auto-merge (fail-closed) — a missing forge capability is not an approval, and a capability gap is
  distinct from a missing approval (locking the `d016_missing_approval_test.go` invariant).
  - Test: `internal/core/aggregate/require_review_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestNoneCapabilityIsGapNeverAutoMerge`
  - Level: L0
- **REQ-E2-S07-05** — Given a valid, eligible, sha-matching, non-expired `ApprovalEvidence` meeting
  `approvalsRequired` with eligible non-author approvers, when the engine evaluates `require-review`,
  then the obligation is satisfied and **no** `require-review` finding is emitted for that subject —
  proving the satisfaction path exists and is not permanently closed.
  - Test: `internal/core/aggregate/require_review_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestValidEligibleEvidenceSatisfies`
  - Level: L0

## E2-S08 — Phase `off`/`observe`/`enforce` + pack ceiling `[autonomous]`

**As a** platform operator rolling out a new rule **I want** to run it in `observe` (recorded, but
not affecting the decision) before `enforce` **so that** I can see what it *would* do on real MRs
without blocking anyone, and a pack-level phase caps every rule inside it.

**Goal**: implement ADR-0018 §1 phase semantics over the frozen required `phase` field (no
default): `off` ⇒ the rule is loaded/linted but **never evaluated**; `observe` ⇒ evaluated, its
findings routed to `findings.observed`, **structurally excluded** from aggregation (never changes
the decision, blocks, required-reviews, or score); `enforce` ⇒ evaluated, findings routed to
`findings.enforcing`, feeds the decision. The pack's `spec.phase` is a **ceiling** (never additive):
pack `off` ⇒ nothing in it evaluates; pack `observe` ⇒ every rule caps at observe (an `enforce` rule
inside an `observe` pack runs as observe); pack `enforce` ⇒ each rule's own phase stands. Threads the
real observed set into `record.go:209` (currently hardcoded `Observed: []`).

**Operator input**: no.

**Dependencies**: E2-S04 (the enforcing findings whose aggregation observe must be excluded from).

**Definition of done**: an `enforce` rule that would block routes its finding to `findings.enforcing`
and the decision is BLOCK; the identical rule at `observe` routes to `findings.observed` and the
decision is unchanged (that finding excluded from aggregation) — proven by a paired golden differing
only in `phase`; an `off` rule is never evaluated (no finding in either bucket); an `enforce` rule
inside an `observe`-ceiling pack runs as observe; a pack/rule with a missing `phase` is rejected at
load (no-implicit-enforce, E2-S01); `record.go`'s `Observed` reflects the real observe findings;
`schemas/phase_test.go` stays green; every golden double-runs.

**Not in scope**: profile resolution / single-writer (E2-S09); `assent compare`'s before/after diff
consuming the observed set (E6); per-rule rollout *scheduling* beyond the three-state field.

Requirements:

- **REQ-E2-S08-01** — Given two identical block-producing rules differing only in `phase`, when the
  engine runs, then the `enforce` one's finding lands in `findings.enforcing` and yields BLOCK,
  while the `observe` one's finding lands in `findings.observed` and the decision is **unchanged**
  (the observe finding structurally excluded from aggregation) — the core observe-vs-enforce split.
  - Test: `internal/core/aggregate/phase_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestObserveExcludedEnforceIncluded`
  - Level: L0
- **REQ-E2-S08-02** — Given a rule with `phase: off`, when the engine runs, then the rule is
  **never evaluated** — no finding in `observed` **or** `enforcing`, and (adversarial) a rule whose
  `when` would error is proven not to even compile-evaluate when `off` (off short-circuits before
  evaluation).
  - Test: `internal/core/aggregate/phase_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestOffRuleNeverEvaluated`
  - Level: L0
- **REQ-E2-S08-03** — Given a pack whose `spec.phase` is `observe` containing a rule whose own
  `phase` is `enforce`, when the engine runs, then the rule runs as **observe** (pack phase is a
  ceiling, never additive) — the rule's finding lands in `observed`, not `enforcing`. Adversarial
  case: a pack `off` makes an inner `enforce` rule evaluate not at all.
  - Test: `internal/core/aggregate/phase_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestPackPhaseCeiling`
  - Level: L0
- **REQ-E2-S08-04** — Given a `MergePolicy` rule or `Pack` with a **missing** `phase`, when it
  loads, then it is rejected (no-implicit-enforce, ADR-0018 §1 — the required field with no
  default) — reinforcing E2-S01's strict decode for the phase field specifically.
  - Test: `internal/core/aggregate/phase_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestMissingPhaseRejected`
  - Level: L0
- **REQ-E2-S08-05** — Given the determinism rule, when phase routing lands, then `record.go`'s
  `Observed` carries the real observe findings (no longer hardcoded `[]`), `schemas/phase_test.go`
  and `internal/core/decision/record_test.go` stay green, `TestCorePurity` stays green, and every
  golden double-runs byte-identical.
  - Test: `internal/core/aggregate/phase_test.go`
  - Verify: `go test ./internal/core/... -run 'TestObserve|TestReportDoubleRunStable'`
  - Level: L0

## E2-S09 — Profile resolution + single-writer authority `[autonomous]`

**As a** platform operator running multiple policy profiles **I want** exactly one covering profile
to hold write authority per `(environment, class)` binding **so that** two profiles can never both
try to arm/merge the same MR, and recorder-only profiles observe without ever mutating the forge.

**Goal**: implement ADR-0018 §2 profile resolution: given `Config.profiles` (an ordered precedence
table) and a set of `PolicyProfile` documents each with `spec.writes` (bool) and
`environments`/`classes` scope, resolve the **single** covering profile for a given `(environment,
class)` binding by coverage → specificity (narrower scope wins) → config-order tie-break; surface
its identity + `writes: true|false` into the decision so a downstream forge step knows whether this
run may write (arm/merge) or is recorder-only. The single-writer invariant — at most one covering
`writes: true` profile per binding — is enforced at load (two covering writers ⇒ rejected).

**Operator input**: no.

**Dependencies**: E2-S08 (phase and profile are the two lifecycle axes; profile builds on the
enforce/observe routing). **Off the S10 critical path** — D-016 declares no profile; validated by
its own goldens.

**Definition of done**: with two profiles both covering a `(prod, topic-registry)` binding, the
narrower-scoped one wins (specificity), then config order breaks a true tie; the resolved profile's
`writes` flag is surfaced into the decision; a recorder-only (`writes: false`) profile resolves and
observes but its resolution never sets write-authority true; two covering `writes: true` profiles
are rejected at load (single-writer invariant); every golden double-runs.

**Not in scope**: the forge write itself (arm/merge — E4); `assent compare`'s cross-profile
comparison (E6); profile *scheduling*/rollout automation.

Requirements:

- **REQ-E2-S09-01** — Given two profiles covering the same `(environment, class)` with different
  scope breadth, when resolution runs, then the **narrower** profile wins (specificity), and a true
  tie is broken by `Config.profiles` order — deterministic single-profile resolution.
  - Test: `internal/core/aggregate/profile_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestProfileSpecificityThenConfigOrder`
  - Level: L0
- **REQ-E2-S09-02** — Given two profiles that both cover a binding with `writes: true`, when the
  config loads, then it is **rejected** — the single-writer invariant (at most one covering writer
  per binding), never two profiles racing to arm the same MR.
  - Test: `internal/core/aggregate/profile_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestTwoWritersRejected`
  - Level: L0
- **REQ-E2-S09-03** — Given a resolved `writes: true` profile, when the decision is built, then the
  write-authority flag + resolved profile identity are surfaced into the decision; given a
  `writes: false` (recorder-only) resolution, the write-authority is false — a recorder-only profile
  never claims write authority.
  - Test: `internal/core/aggregate/profile_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestWriteAuthoritySurfacedRecorderOnlyNever`
  - Level: L0
- **REQ-E2-S09-04** — Given the determinism rule, when profile resolution lands, then resolution is
  order-independent given a fixed `Config.profiles` precedence (the precedence table, not map
  iteration, decides), `TestCorePurity` stays green, `schemas/profile_test.go` stays green, and
  every golden double-runs.
  - Test: `internal/core/aggregate/profile_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestProfileResolutionDoubleRunStable`
  - Level: L0

## E2-S10 — D-016 strict-fixture end-to-end reproduction `[autonomous]`

**As a** maintainer **I want** the whole engine, run over the frozen D-016 strict fixture's
`EvaluationInput` + `MergePolicy` + `RulesetBinding`, to **reproduce** its DecisionRecord **so
that** the §8 exit-gate fixture is proven not just *shape-valid* (as `schemas/
d016_strict_fixture_test.go` already checks) but *behaviourally reproduced* by the real decision
path — closing the contracts↔engine loop.

**Goal**: wire E2-S01..S08 into one end-to-end run: load the fixture's `MergePolicy`/
`RulesetBinding`, evaluate its `EvaluationInput` (three changes across two subjects, an expired
owner fact, no injected `ApprovalEvidence`), aggregate, serialize via `internal/core/decision`, and
assert the produced DecisionRecord equals the frozen `examples/contracts/d016-strict-fixture/
decision-record.json` after canonical serialization — `decision: BLOCK`, `findings.observed: []`,
the three enforcing findings (two `require-review` at `points: 0` for the two subjects' unproven
ownership, one `block` at `points: 10` for the partition shrink), and the pins block passed through.
Depends on the fixture-fix lane (F) having corrected `partitions-must-not-shrink.when` to
`new >= old`.

**Operator input**: no.

**Dependencies**: E2-S01, S02, S04, S05, S06, S07, S08, and the fixture-fix lane (F). **Not** S03
(the fixture's `when`s are single-leaf) or S09 (no profile declared) — those are validated by their
own goldens, off this critical path.

**Definition of done**: the engine, given only the fixture's `MergePolicy` + `RulesetBinding` +
`EvaluationInput` (and nil `ApprovalEvidence`), produces a DecisionRecord byte-identical to the
frozen `decision-record.json` after canonical serialization; the run double-runs byte-identical; the
produced record validates against `DecisionRecordSchema` (so S10 can never drift the record out of
schema); `schemas/d016_strict_fixture_test.go` and `schemas/d016_missing_approval_test.go` stay
green (this story reproduces the record they validate the shape of, never edits them); `TestCorePurity`
stays green.

**Not in scope**: reproducing the `PresentationModel`/`PublicationReceipt` (those are the forge/
presentation tiers — this story reproduces the **DecisionRecord**, the engine's own output;
presentation reproduction is E4/E-presentation); any live-forge proof (E7); comparison of two
records (E6).

Requirements:

- **REQ-E2-S10-01** — Given the frozen D-016 `MergePolicy` + `RulesetBinding` + `EvaluationInput`
  (with the F-lane-corrected `new >= old`), when the full engine runs with **no** injected
  `ApprovalEvidence`, then the produced DecisionRecord equals the frozen `decision-record.json`
  after canonical serialization — `decision: BLOCK`, `findings.observed: []`, and exactly the three
  enforcing findings with their `{rule, obligation, effect, subject, points, code}` values.
  - Test: `internal/core/aggregate/d016_reproduce_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestReproduceD016DecisionRecord`
  - Level: L1
- **REQ-E2-S10-02** — Given the produced D-016 DecisionRecord, when it is validated against
  `DecisionRecordSchema`, then it passes — the reproduced record can never drift out of the frozen
  schema (guards against an engine change that reproduces the golden values but violates the schema
  shape).
  - Test: `internal/core/aggregate/d016_reproduce_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestReproducedRecordValidatesSchema`
  - Level: L1
- **REQ-E2-S10-03** — Given the two `require-review` findings for the two subjects' unproven
  ownership, when the engine runs, then each is subject-scoped (`topic-registry:orders.events.v1`
  and `topic-registry:payments.settled.v2`) with `code: ownership-approval-missing` and `points:
  0` — proving the per-subject obligation coverage (E2-S04) and the injected-evidence-absent
  `require-review` path (E2-S07) compose correctly on the real fixture.
  - Test: `internal/core/aggregate/d016_reproduce_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestD016PerSubjectRequireReview`
  - Level: L1
- **REQ-E2-S10-04** — Given the determinism rule, when the end-to-end reproduction runs, then it
  double-runs byte-identical, references no clock/env/network/random anywhere in `internal/core`
  (`TestCorePurity` green), and the fixture-fix lane (F) is confirmed landed (the pre-fix
  `input.new` would fail REQ-E2-S02-02's undeclared-reference rejection, so a green S10 proves F
  landed).
  - Test: `internal/core/aggregate/d016_reproduce_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestD016ReproductionDoubleRunStable`
  - Level: L1
