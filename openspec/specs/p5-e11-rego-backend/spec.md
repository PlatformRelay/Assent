# P5-E11 — Complex-rule backend: Rego predicate tier

**Epic ID / REQ prefix:** `E11` / `REQ-E11-S0n-nn`.

**Unlock:** D-141 (2026-08-10). E11's *contract* was already unlocked by D-017; what was gated
was **implementation**, twice over: "after Phase 4" (satisfied — the Phase-4 adoption gate
closed with D-042) and, per D-017, **evidence-based per rule** — "each ported rule tries CEL
first, the backend is built when a concrete rule demonstrably exceeds the tier-1 ceiling."
D-141 records the operator lifting that per-rule evidence gate and what it does **not** waive.

**Problem**: ADR-0002 v2 promises "one Kyverno-style YAML envelope, **pluggable expression
backends**" — and the ADR index has carried the status line "**pluggable half unbuilt: Rego is
E11**" ever since. There is exactly one backend (CEL, ADR-0013), and the CEL leaf is
restricted to the frozen predicate-scope table: single-pass, single-subject, no set
operations across manifests, no graph relationships. The escape hatch was designed and
committed — `examples/policies/rego/bounded_change.rego` exists and is *quarantined* behind a
`# locked: D-012` marker with a CI guard (P3-E3-S03/S04) forbidding any declarative example
from referencing a `rego:` leaf. E11 is the epic that removes that quarantine and makes the
second tier real.

**Two hard constraints that shape every story:**

1. **The authoring schema names the backend; the decision schema does not.** P3-E1-S02's
   backend-neutrality guarantee (REQ-P3-E1-S02-01: "no field naming a predicate backend …
   anywhere in the schema") applies to `EvaluationInput` — and it holds, so **no decision
   contract changes**. But `schemas/policy/v1alpha1/merge-policy.schema.json` defines the
   predicate leaf as `{"additionalProperties": false, "required": ["cel"], "properties":
   {"cel": …, "message": …}}`. A `rego:` leaf therefore **requires a policy-schema change**.
   `API_STABILITY.md:19` permits exactly this within `v1alpha1`: "Additive field additions
   within a major require an openspec change + version bump before they become required" and
   "within a major, changes are additive-only for reports and **announced-only for authored
   policy**." This spec *is* that openspec change. **E11 is therefore the first epic whose DoD
   is `git diff schemas/` != 0** — every prior epic required it to be zero, and a reviewer
   applying the old habit will flag the correct change as a violation.

2. **Rule 7 (determinism) is the sharpest constraint, and the obvious implementation breaks
   it.** AGENTS.md rule 7 forbids anything probabilistic or wall-clock-dependent in the
   decision path. Rego's standard builtins include `time.now_ns()`, `rand.intn()`, and
   `http.send()` — each of which would make the same policy over the same ChangeSet produce
   different decisions on different runs. Worse, the *reflexive* safety measure — "bound Rego
   evaluation with a timeout" — is itself a rule-7 violation: a wall-clock deadline makes the
   decision depend on machine speed and load, so a policy that passes on a fast runner blocks
   on a slow one. E11 must bound evaluation **deterministically** (S06) and deny the
   non-deterministic builtins **structurally** (S04), not by convention.

**Key ground truth (de-risks the epic):**
- **The contract shape is already decided and reviewable.** ADR-0002 v2: the envelope owns
  `match` / `effect` / `points`; a Rego module **only computes violations** over the policy
  input. `examples/policies/rego/bounded_change.rego` is the committed, reviewed illustration
  of that shape. E11 implements the decided contract; it does not redesign it.
- **`later-phases.md` fixes the boundaries**: violations-shaped modules, **explicit
  obligation-proof polarity (no implicit "no violation = proof")**, OPA capability sandbox
  (D-013), the same typed `EvaluationInput`, structured proof/finding output, declared data,
  **no I/O**, and **no control over aggregation**. Hard boundary: no domain-aware joins, no Go
  rule plugins in `internal/core`.
- **`docs/planning/rego-escape-hatch.md` names E11 as the only lane permitted to remove the
  quarantine marker** — and the guard that enforces it lives in P3-E3-S04
  (`hack/check-migration-invariants.sh`). Unquarantining is a story here (S11), not a
  side-effect of any other story.
- **`internal/core` stays I/O-free** (`TestCorePurity`). Module *loading* is I/O and lives at
  the loader tier alongside CEL compilation; module *evaluation* is pure computation and may
  live in core only once S04's sandbox makes that true by construction.

**Justification, post-S01 (D-156) — read this before adding anything to the epic.** S01 has
landed and **narrowed E11**: `docs/planning/rego-tier-ceiling.md` strikes **cross-manifest**
and **set-difference** from the four named shapes and strikes the named-intermediate half of
**multi-pass**. What remains, and the only thing E11 may be justified or documented as
delivering, is: **folds/aggregates over the in-input collections** (CEL has `size()` and no
other aggregate) and **recursive/graph reasoning over an adjacency the input already carries**.
**Both are unconditional on today's shipped input contract** — neither waits on any open
question. The strongest single piece of evidence is the graph shape: a
`{type: string, cardinality: set}` fact carrying encoded edges is fully in contract and
deliverable over the `http` transport on the plain `assent run` path, and over it tier-1 CEL can
express only a check to some **fixed depth `k` written into the rule text** — because the
iteration count of a CEL expression cannot be made data-dependent: `reduce`, `transformList`,
`transformMap`, two-var `all`, `range` and `cel.bind` are all `undeclared reference`, `for` is a
reserved identifier, and depth must therefore be spelled out against cel-go's parser recursion
cap of 250. **Unbounded reachability has no spelling at all.** Rego answers the actual question
at any depth with `graph.reachable`. The claim is about **expressiveness only** — on a small
graph a large `k` is both affordable and complete (measured in the record's §5), so this is not
an argument that CEL is too slow.
Cross-manifest reasoning is an *input-availability* limit, not a tier-1 expressiveness limit —
REQ-E11-S05-01 pins the identical input, so the Rego tier fails on it identically. See the
E11-S01 section for the verdict table and the binding consequences for S05/S07/S11/S12.

**Scope**: (S00) the deterministic-budget feasibility spike, in a nested throwaway module so it
adds no dependency; (S01) the tier-1 ceiling, recorded with concrete exceeding rules; (S02) additive
`rego:` leaf in the policy schema; (S03) module loading + compile-time errors; (S04) OPA
capability sandbox (D-013); (S05) input binding to the identical `EvaluationInput`; (S06)
deterministic evaluation budget; (S07) violations → findings with explicit obligation-proof
polarity; (S08) aggregation boundary — the module never controls effect/points; (S09)
`assent lint` hard errors for Rego rules; (S10) `assent test` + goldens; (S11) remove the
quarantine + update the P3-E3-S04 guard; (S12) docs/maturity truth; (S13) exit gate.

**Non-goals** (fenced): **GitHub adapter** (E10 — separate spec, unlocked by D-140);
**`serve`** (E12); **remote packs** (E13, still Locked per D-012); **domain-aware joins** and
**in-process Go rule plugins** (D-017 declined both, permanently — not deferred);
**giving Rego control over aggregation, effects, or points** (ADR-0002 v2 boundary);
**WASM or gRPC predicate backends** (still Locked per D-012 — D-141 unlocks Rego only);
**widening the frozen predicate-scope table for CEL**; **any `EvaluationInput` change**
(the whole point of P3-E1-S02's neutrality is that none is needed — if a story finds one
necessary, that is a design failure to surface, not a schema bump to make).

**ADRs**: 0002 v2 (**the governing ADR** — pluggable backends; its "pluggable half unbuilt"
status line is retired by S12), 0007 (effects and aggregation — the boundary Rego must not
cross), 0013 (CEL as tier 1; the ceiling E11 sits above), 0017 §2/§3/§9 (obligations, proof
polarity, do-not-generalize list), 0018 (phase/profile — a Rego rule is phase-gated like any
other). **Related decisions**: D-012 (escape-hatch quarantine), D-013 (OPA capability
sandbox), D-017 (contract unlock + declined items), D-141 (implementation unlock).
**Reuse**: the E2 decision engine's leaf-evaluation seam, `internal/core/policy`'s strict
loader, E3's lint hard-error framework, E6's `assent test` harness, the committed example.
**New**: `rego:` leaf, OPA integration + capability file, deterministic budget, violation
mapping.

**Executability**: S00–S13 all **`[autonomous]`** — hermetic, no infrastructure. S02, S04,
S06, S07 are **engine-grade** (frozen-schema change, sandbox, determinism, decision polarity)
and additionally **`[maintainer LGTM]`**: S02 changes a published contract and S04/S06/S07 are
the decision path itself.

**Dependency order**: {S00, S01} → S02 → S03 → S04 → S05 → S06 → S07 → S08 → {S09, S10} → S11
→ S12 → S13 — **14 stories, S00–S13.** **Do first: S00 and S01**, which are independent of each
other and of judgment call (d), so both can run immediately and in parallel. S01 (the ceiling
document) determines whether the backend's shape is right; building it without one reproduces
the speculative-generality risk D-012 existed to prevent. S00 (the budget spike) determines
whether the backend can be bounded at all.

**What is blocked, and by what — stated precisely, because the obvious reading is wrong:**

- **S02 is blocked on S00**, not merely sequenced after it. S02 changes a **published
  contract**; if S00 returns "no deterministic budget exists", the `rego:` leaf that should be
  announced is the `phase: observe`-only shape, not the gating shape (REQ-E11-S00-03).
- **S03 and S04 are both blocked on judgment call (d)** — this corrects an earlier reading of
  this spec which said S01–S03 were unblocked. They are not. **S03 is the story that *effects*
  the narrowing (d) governs**: REQ-E11-S03-03 adds OPA to `go.mod`, and S03's own Test paths
  sit in `internal/core/policy/**` — inside the D-123 guarded tree. Landing S03 while (d) is
  open converts rule 7's decision-path guarantee from link-enforced to capability-enforced
  **silently and green**, because — as (d) documents — *neither* purity gate is transitive.
  Deciding a hard-rule narrowing by merging a story is exactly what rule 6 forbids. (d) also
  decides *which package* the evaluator lives in, so S03–S08's `internal/core/policy` paths are
  written against **(d1)** and must be re-pathed if the operator answers **(d2)**.
- The operator's 2026-08-10 answer — *accept and pin OPA* — disposes of (d)'s **supply-chain**
  half and rejects **(d3)**. It does **not** discriminate **(d1) from (d2)**: (d2) also accepts
  and pins OPA while keeping the guarded tree OPA-free. That half remains open and is the one
  that unblocks S03.

## Judgment calls (decide-and-log / operator)

(a) **DECIDED — the `rego:` leaf is an additive `oneOf` alternative, not a replacement.**
Every policy valid before E11 stays valid; a `rego:` leaf is rejected by an older assent
binary through strict-decode, which is the **correct** direction (an old binary must not
silently ignore a rule it cannot evaluate). This is backward-compatible and deliberately
forward-**in**compatible, and S02 records it in `API_STABILITY.md` as an announced additive
change — no `apiVersion` bump.

(b1) **DECIDED — the feasibility spike is story E11-S00 and S02 is blocked on it.** Adversarial
review, marked verify-not-verified: OPA's public `rego` package may bound evaluation only via
`context.Context` cancellation, with **no supported deterministic instruction/step budget**. If
so, judgment call (b) below cannot be satisfied as written — and an S05→S06 order would stall
the epic at story 7 of 14 with the **schema already changed (S02)** and the **OPA dependency
already added (S03)**. **Resolution:** mitigation (i) is adopted and given a story rather than
left as a preference — **E11-S00**, which runs first, carries REQ IDs and a DoD like any other
story, and is deliberately built in a **nested throwaway module** so it can answer the question
without adding OPA to `go.mod` (which judgment call (d) has not yet authorised). Fallback
(ii) — if no deterministic budget exists, Rego-backed rules are restricted to `phase: observe`,
reporting but never gating — is retained and is now REQ-E11-S00-03, which also re-scopes S02's
published schema change to match, because an observe-only leaf is a different contract from a
gating one. (iii) stands unchanged: relaxing (b) is **not** acceptable, and "timeout → BLOCK" is
not a resolution.

(b) **DECIDED — evaluation is bounded by a deterministic budget, never a wall-clock timeout.**
Per rule 7: the bound is an OPA evaluation-step/instruction budget that yields the identical
outcome on any machine. A wall-clock deadline is permitted **only** as an outer backstop that
can never change a decision — i.e. exceeding it is a hard process error, never a policy
outcome (not a BLOCK, not an APPROVE, not a skipped rule). If that separation cannot be
implemented cleanly, the story fails and the operator is asked; it is not resolved by
"timeout → BLOCK", which would make the decision machine-dependent while *looking* fail-closed.

(c) **DECIDED — zero violations is NOT proof of an obligation.** `later-phases.md` names this
explicitly and it is the single easiest thing to get wrong: a module that errors, is
misconfigured, matches nothing, or returns an empty set would otherwise silently *satisfy* a
required obligation. A Rego-backed obligation is proven only by an explicit, structured proof
value; absence of violations satisfies **non-obligation** rules only. S07 owns both polarities
and must test the failing one.

(d) **🔴 OPERATOR — BLOCKING. Adopting OPA narrows AGENTS.md rule 7's *mechanism*, and both
existing purity gates would miss it.** This is the sharpest finding of the design session and
it must be decided before S03, not discovered during it.

*The gap, verified:* `internal/core/purity_test.go` parses each guarded file and flags
**that file's own** imports (`math/rand`, `crypto/rand`, `net`, `net/*`) and selectors
(`os.Getenv`, `time.Now`); `.golangci.yml`'s `pure-tree` depguard is `list-mode: lax`,
deny-only, over **direct** imports. **Neither is transitive.** So a file in
`internal/core/**` importing `github.com/open-policy-agent/opa/rego` passes both gates
green — while transitively linking `net/http` (OPA ships the `http.send` builtin), plus its
own clock and randomness use. The `net` deny exists precisely to encode "no network stack on
the decision path" (D-123), and OPA would defeat it invisibly.

*Why S04's sandbox does not by itself resolve it:* the capability set makes `http.send`
uncallable **from policy**, which is the real threat. But the guarantee's *nature* changes
from "the network stack is not linked into the decision path" (structural, checkable by
grep) to "the network stack is linked but unreachable from policy" (behavioural, resting on
a capability file). That is a weaker guarantee, and it is a hard-rule change — it cannot be
made silently by a story.

*Options:*
- **(d1) Accept the narrowing, explicitly [RECOMMENDED].** Rule 7's decision-path guarantee
  becomes capability-enforced rather than link-enforced for the Rego tier. Requires an
  **ADR-0011/rule-7 amendment** and a decision row — not just this spec. Pair it with
  **extending the purity guard to a transitive check** (`go list -deps`) that asserts the
  guarded tree's transitive closure contains no `net` **except** through the single,
  explicitly allowlisted OPA path — so the exception is visible, pinned, and cannot widen to
  a second dependency unnoticed.
- **(d2) Keep the guarded tree OPA-free** — evaluation behind an interface, implementation in
  an unguarded package injected from `cmd/assent`. Honest about the boundary, but it moves
  part of the decision path *outside* the tree rule 7 guards, which is arguably worse: the
  guarantee is then neither link-enforced nor guard-covered.
- **(d3) Drop OPA.** A hand-rolled Rego evaluator would be far worse in every dimension.
  Rejected unless the operator rejects (d1) and (d2).

*Supply chain, separately:* OPA is a large dependency with a large transitive tree on a
project shipping cosign signing, SLSA-grade provenance, `govulncheck`, and Scorecard. It
materially changes binary size, vulnerability surface, and Dependabot load. S03 pins it and
records the size delta.

*Status (2026-08-10) — the two halves have diverged and must not be conflated:*
- ✅ **Supply-chain half: ANSWERED — accept and pin.** The operator accepted
  `github.com/open-policy-agent/opa` as a dependency. **(d3) is rejected** by that answer.
- 🔴 **Mechanism half: STILL OPEN — (d1) vs (d2), and this is the half that blocks S03.**
  Accepting the dependency does not say *where the evaluator lives* or *which gate enforces
  rule 7*: **(d2) also accepts and pins OPA** while keeping the guarded tree OPA-free. Reading
  "accept and pin" as settling (d) would silently choose (d1) — the exact silent hard-rule
  narrowing this judgment call exists to prevent. Per REQ-E11-S04-04 the answer needs an
  **ADR-0011/rule-7 amendment plus a `D-nnn` row**, whichever way it goes.

(e) **DECIDED — Rego modules are policy, and load from the target ref like all policy.**
ADR-0010/ADR-0015's trust rules apply unchanged: a module is loaded from the target ref,
never from the PR head, so a contributor cannot ship a rule change and have it govern their
own PR. S03 must not introduce a second, laxer load path.

(f) **DECIDED — E11 and E10 are independent and may run in either order or in parallel.**
They share no files: E10 is `internal/forge/**` + `cmd/assent` edge; E11 is
`internal/core/**` + `schemas/policy/**` + `examples/policies/rego/**`. The only coupling is
review bandwidth. Recommended sequencing is **E10 first** — it has a live-adoption story
(S18) whose infrastructure the operator must arrange, so starting it early parallelizes the
human dependency.

---

### E11-S00 — Spike: does OPA expose a deterministic evaluation budget? `[autonomous · spike]`

- **Goal**: answer, against a pinned OPA version and with a runnable reproduction, whether
  OPA's public API can bound evaluation by a **machine-independent instruction/step count**
  rather than by `context.Context` cancellation (wall clock).
- **Why first**: judgment call (b) requires exactly that budget, and judgment call (b1) records
  that the API may not exist. If it does not, the epic's shape changes — and every later story
  is downstream of that. Ordering this after S02/S03 would discover it with the **published
  schema already changed** and the **OPA dependency already in `go.mod`**: both irreversible in
  the annoying direction. This story exists so that discovery is free.
- **Dependencies**: none. It is deliberately **not** blocked on judgment call (d) — see
  REQ-E11-S00-01, which is what makes that true.
- **Definition of done**: `docs/planning/spikes/spike-e-rego-budget.md` records the verdict, the
  pinned OPA version it was established against, the exact API surface examined, and the
  reproduction command — and the repository's own `go.mod`/`go.sum` are byte-unchanged.

- **REQ-E11-S00-01** — Given judgment call (d) is **open** and is precisely the question of
  whether OPA may enter this module's dependency graph, when the spike is written, then it
  lives in its **own nested module** (`hack/spikes/rego/go.mod`) and the repository's root
  `go.mod`/`go.sum` gain **no OPA entry**. Go excludes a directory carrying its own `go.mod`
  from the parent module's `./...`, and this repository has **no `go.work`** (verified), so the
  spike is unbuildable by `task check` and cannot smuggle the adoption (d) has not yet
  authorised. Spiking a dependency is not adopting it.
  - Test: `hack/spikes/rego/go.mod`, root `go.mod`, root `go.sum`
  - Verify: `git diff --exit-code -- go.mod go.sum && ! go list ./... | grep -q 'spikes/rego' && task check`
  - Level: L0
- **REQ-E11-S00-02** — Given the question is empirical, when the spike runs, then it either
  (i) **names the public API** that bounds evaluation by a machine-independent count and
  demonstrates an **identical outcome and identical budget consumption** across N≥100 runs and
  across at least two `GOMAXPROCS` settings — the property S06 will later have to gate on — or
  (ii) records that **no such API exists** in the pinned version, with the surface examined
  enumerated so the finding is falsifiable rather than an absence-of-evidence claim.
  - Test: `hack/spikes/rego/`, `docs/planning/spikes/spike-e-rego-budget.md`
  - Verify: `cd hack/spikes/rego && go test ./...`
  - Level: L0
- **REQ-E11-S00-03** — Given the verdict routes the epic, when it is recorded, then it states
  the consequence explicitly and a `D-nnn` row captures it **before S02 changes the published
  schema**: outcome (i) → judgment call (b) stands unamended and S06 is buildable as written;
  outcome (ii) → the (b1)(ii) fallback is adopted, Rego-backed rules are restricted to
  `phase: observe`, and **S02's schema change is re-scoped accordingly** — a `rego:` leaf that
  can only ever observe is a *different published contract* from one that can gate, and
  shipping the gating shape first would announce a capability the epic cannot deliver. Under
  no outcome is this resolved by "timeout → BLOCK" (judgment call (b), (iii)).
  - Test: `docs/planning/spikes/spike-e-rego-budget.md`, `docs/decisions/decisions.md`
  - Verify: manual review
  - Level: L0

### E11-S01 — Record the tier-1 ceiling with concrete exceeding rules `[autonomous]` — ✅ **DONE (D-156)**

**Outcome — E11's scope is NARROWED. Read `docs/planning/rego-tier-ceiling.md` before S05,
S07 or S12.** Of the four shapes `later-phases.md` names, **two are struck outright, one is
struck in part, and one and a half survive**:

| Shape | Verdict | Reason (measured against `newEvalEnv`, not CEL in general) |
| --- | --- | --- |
| multi-pass — **fold/aggregate** over a collection | **EXCEEDS — justifies E11** | `sum`/`reduce`/`math.*`/`lists.*` are all `undeclared reference`; `size()` is the only aggregate in the surface, so counting is expressible and summing is not |
| multi-pass — **named intermediate** across checks | **STRUCK** | no `cel.bind` and the assert tree combines booleans, not values — but re-deriving the sub-expression inside each leaf is semantically identical and compiles |
| cross-manifest | **STRUCK (all three sub-shapes)** | membership is `x in facts.<p>.<n>.value`; keyed lookup is a purpose-built provider or `facts.<p>.<n>.value[key]` (compiles, lint-clean); **same-changeset cross-file is an input-availability limit, not an expressiveness one** — the evaluation unit is one file and REQ-E11-S05-01 pins the *identical* `EvaluationInput`, so a Rego module fails identically |
| set-difference | **STRUCK — unconditionally** | `oldEntry.x.filter(a, !(a in entry.x))` expresses it when the entry tree is bound; when it is not (`adoptertest` is the sole writer of `EvalChange.Entry`, so `assent run` binds a scalar) the failure is input availability and Rego fails identically. **Both resolutions of OQ-35 strike it**, so nothing downstream waits on that answer |
| graph-relationship | **EXCEEDS — justifies E11, unconditionally** | **The iteration count of a CEL expression cannot be made data-dependent** — no recursion, fold, user-defined function or loop form (`reduce`/`transformList`/`transformMap`/two-var `all`/`range`/`cel.bind` all undeclared, `for` reserved), so depth is a *syntactic* property capped by cel-go's 250 recursion limit ⇒ **unbounded reachability has no spelling**. A *bounded* `k`-hop check **is** expressible (encode-and-compare over a finite candidate set, verified by evaluation) and on a small graph a large `k` is affordable and even complete — so the ceiling is expressive, not performance. Rego answers it at any depth with `graph.reachable`. Adjacency is **in contract and available today** as a `{type: string, cardinality: set}` fact over the `http` transport — no `--checkout`, no OQ-35, no OQ-36, no schema change. **Caveat:** no provider in the corpus ships one yet — same epistemic standard as B1, less evidential weight (B1 has a shipped mechanism and infers only the set's source) |

**The headline, stated plainly so no later story over- or under-claims it: E11 has two
unconditional justifications on today's shipped input contract — the fold/aggregate shape and
the graph shape — and neither waits on an open question.** OQ-35 and OQ-36 are recorded
residuals of the measurement, not conditions on this scope decision.

**Binds E11-S04, whose denylist is not written yet:** `split` and `graph.reachable` are pure and
deterministic and **must not be denied** by the capability set. They are precisely what carries
the graph shape's justification, and a denylist drafted from "deny anything unfamiliar" would
strike out the epic's own strongest evidence. Recorded again in the S04 section. This says
nothing about judgment call (d) — *where* the evaluator lives is untouched.

**Binding consequences for later stories:**
- **S05** must **not** be widened to carry cross-manifest data. Widening the input is a
  different decision from adding a backend, and E11's non-goals fence it.
- **S07**'s violation shape must support a **fold result** (a computed scalar naming its
  contributing elements) and a **path/cycle witness** — not a cross-manifest reference.
- **S12** must not describe Rego as enabling cross-entry or cross-manifest checks. ADR-0002's
  `rego` bullet currently calls it an "escape hatch for **cross-entry checks**"; that phrase is
  inaccurate for the shipped input contract and correcting it is S12's, under REQ-E11-S12-01.
- **S11**: the committed illustration `examples/policies/rego/bounded_change.rego` is
  **entirely tier-1 expressible** (both `violations` rules are per-change predicates, and
  `examples/policies/declarative/bounded-change.yaml` is the same rule in the envelope). When
  the quarantine lifts it must be labelled a *shape* illustration, never evidence of need.

- **Goal**: a written, reviewable statement of what CEL *cannot* express, grounded in real
  rules — the artifact D-017's evidence gate was protecting.
- **Why first**: D-141 lifts the per-rule evidence *gate*, not the design need. The four
  shapes `later-phases.md` names (multi-pass, cross-manifest, set-difference,
  graph-relationship) determine what the input binding (S05) and the violation shape (S07)
  must support. Building those without the ceiling document is guesswork.
- **Dependencies**: none.
- **Definition of done**: `docs/planning/rego-tier-ceiling.md` exists with ≥1 concrete,
  sanitized rule per named shape, each showing the CEL attempt and *why* it fails.

- **REQ-E11-S01-01** — Given the four shapes, when the ceiling document is authored, then each
  carries a concrete generic rule, the attempted CEL leaf, and the specific reason it cannot
  be expressed within the frozen predicate-scope table (`docs/planning/predicate-scope.md`) —
  and no employer or internal system name appears (D-002).
  - Test: `docs/planning/rego-tier-ceiling.md`
  - Verify: `bash hack/check-sanitization.sh && task check`
  - Level: L0
- **REQ-E11-S01-02** — Given a shape might in fact be CEL-expressible, when the document is
  reviewed, then any shape found expressible in CEL is **struck from E11's scope** and
  recorded — the epic narrows rather than building an unjustified tier. **Satisfied by the
  verdict table at the head of this section (D-156):** cross-manifest and set-difference are
  struck, the named-intermediate half of multi-pass is struck, and only the fold/aggregate and
  in-input graph shapes remain as justification. A shape struck because the *data* is absent
  (cross-manifest sub-shape B3) is struck from E11 too, on the separate ground that Rego
  receives the identical input and therefore fails identically.
  - Test: `docs/planning/rego-tier-ceiling.md`, `openspec/specs/p5-e11-rego-backend/spec.md`
  - Verify: manual review
  - Level: L0

### E11-S02 — Additive `rego:` leaf in the policy schema `[autonomous · engine-grade · maintainer LGTM]`

- **Dependencies**: **S00** (blocking — its verdict decides whether the announced leaf is the
  gating shape or the `phase: observe`-only shape, per REQ-E11-S00-03; this is a published
  contract and announcing the wrong shape is not walk-back-able) and S01.
- **Definition of done**: `merge-policy.schema.json`'s `leaf` becomes a `oneOf` over the
  existing `cel` shape and a new `rego` shape; strict-decode still rejects unknown fields and
  a leaf carrying **both** backends; `API_STABILITY.md` records the announced additive change;
  every pre-E11 policy fixture still validates unchanged.

- **REQ-E11-S02-01** — Given the leaf is `additionalProperties:false, required:["cel"]`, when
  the schema is extended, then a `{"rego": {...}}` leaf validates, a `{"cel": ..., "rego": ...}`
  leaf is **rejected** (exactly one backend per leaf), and an unknown key is still rejected.
  - Test: `schemas/policy/v1alpha1/merge-policy.schema.json`, `schemas/schema_test.go`
  - Verify: `go test ./schemas/... -run TestMergePolicySchema`
  - Level: L0
- **REQ-E11-S02-02** — Given backward compatibility, when the full pre-E11 example and
  fixture corpus is validated against the new schema, then **every document still validates**
  and no golden changes. **Both polarities are required**: a `oneOf` widening can also make a
  previously-**rejected** document validate, so the `do-not-generalize` guards
  (`schemas/testdata/compat/do-not-generalize/`, named as executable guards by
  `API_STABILITY.md`) must still reject everything they rejected before.
  - Test: `examples/**`, `test/**`, `schemas/testdata/compat/do-not-generalize/`
  - Verify: `task test && go test ./schemas/... -run TestDoNotGeneralize && git diff --exit-code -- examples/ test/`
  - Level: L1
- **REQ-E11-S02-03** — Given `API_STABILITY.md:19`'s "announced-only for authored policy",
  when the schema changes, then `API_STABILITY.md` and the changelog record it as an announced
  additive change within `v1alpha1` with **no `apiVersion` bump**, and state explicitly that an
  older binary rejects a `rego:` leaf by design.
  - Test: `API_STABILITY.md`, `CHANGELOG.md`
  - Verify: `task check`
  - Level: L0
- **REQ-E11-S02-04** — Given every prior epic's DoD was `git diff schemas/` == 0, when E11's
  gates run, then the schema-drift guard is **scoped**, not deleted: drift is permitted only
  in `merge-policy.schema.json` and only for this change; drift in any `schemas/decision/**`
  file still fails. **The guard is Go, not a shell script**:
  `internal/schemadrift/drift.go`'s `CheckGitFrozenOrD088PresentationOnly` compares against
  `origin/main` through a two-file fence list and is invoked from three exit-gate tests, so
  scoping it means adding a **third fence plus an `Allowed…Change` validator** — not editing
  `hack/`. The validator must accept *only* the additive `rego:` leaf; a fence that permits
  arbitrary drift in `merge-policy.schema.json` would retire the guarantee rather than scope it.
  - Test: `internal/schemadrift/drift.go`, `internal/schemadrift/drift_test.go`
  - Verify: `go test ./internal/schemadrift/... && task check`
  - Level: L1

### E11-S03 — Module loading and compile-time errors `[autonomous · engine-grade · maintainer LGTM]`

- **Dependencies**: S02 **and the operator's answer to judgment call (d)** — S03 cannot start
  without it. This story is where the narrowing (d) governs actually *happens*: REQ-E11-S03-03
  puts OPA in `go.mod`, and this story's Test paths are inside the D-123 guarded tree. Because
  neither purity gate is transitive, S03 would land **green** while converting rule 7's
  guarantee from link-enforced to capability-enforced — a hard-rule change made by merging a
  story, which rule 6 forbids. (d) additionally decides **which package** the evaluator lives
  in: the `internal/core/policy/**` paths below are written against **(d1)** and must be
  re-pathed to the injected, unguarded package if the operator answers **(d2)**.
- **Definition of done**: modules resolve from the pack directory **on the target ref**
  (judgment call (e)); a module that fails to compile is a **load-time hard error** (E3 lint
  parity), never a runtime surprise; the OPA dependency is pinned.

- **REQ-E11-S03-01** — Given ADR-0010/0015 trust rules, when a Rego module is loaded, then it
  resolves through the **same target-ref policy load path** as YAML policy, and no second
  loader can read a module from the PR head — asserted by a test that places a hostile module
  on the head ref and proves it is not evaluated.
  **Placement, because the obvious location does not compile:** `internal/core/policy`'s loader
  tier is **bytes-in and pure** (`LoadMergePolicy(raw []byte)`); *all* ref-addressed reading
  lives in `cmd/assent` (`run.go:203`, `:211`, `:253`), and `.golangci.yml:39` denies
  `internal/forge` from `**/internal/core/**` — so a core-resident loader cannot fetch a ref at
  all. `internal/core/policy/rego_load.go` therefore takes an **injected reader** supplied by
  `cmd/assent`, and the hostile-module-on-the-head-ref test lives in **`cmd/assent`**, where the
  ref plumbing exists. If the operator answers **(d2)**, these paths move with the evaluator.
  - Test: `internal/core/policy/rego_load.go`, `rego_load_test.go`, `cmd/assent/run_rego_test.go`
  - Verify: `go test ./internal/core/... ./cmd/... -run TestRegoLoadsFromTargetRef`
  - Level: L1
- **REQ-E11-S03-02** — Given E3's hard-error framework, when a module fails to compile or
  references an undefined rule, then `assent lint` reports it as a **hard error** with the
  file and position, and `assent run` refuses to evaluate — fail closed, never skip the rule.
  - Test: `internal/core/policy/rego_load_test.go`
  - Verify: `go test ./internal/core/... -run TestRegoCompileErrorIsHardError`
  - Level: L1
- **REQ-E11-S03-03** — Given judgment call (d), when OPA is added to `go.mod`, then the
  version is pinned, `govulncheck` and Dependabot cover it, and the binary-size delta is
  recorded in the story's notes. **This REQ may not be started before (d) is answered** — it is
  the adoption itself, not a consequence of it.
  - Test: `go.mod`, `go.sum`
  - Verify: `task check && govulncheck ./...`
  - Level: L0
- **REQ-E11-S03-04** — Given both purity gates are **non-transitive** and (d1)'s entire premise
  is that the exception stays visible, when the operator answers **(d1)**, then this story also
  extends the purity guard to a **transitive** check (`go list -deps` over the guarded tree)
  asserting the transitive closure contains no `net`/`net/*` **except** through the single
  explicitly allowlisted OPA path — and a mutation control proves the guard goes red when a
  *second* dependency pulls `net` in, so the exception cannot widen unnoticed. Without this the
  narrowing is accepted but not enforced, which is strictly worse than the status quo: it reads
  as governed while checking nothing. If the operator answers **(d2)** this REQ is struck and
  replaced by the depguard rule denying the evaluator package from `internal/core/**`.
  - Test: `internal/core/purity_test.go`, `.golangci.yml`, `hack/lint/depguard_test.sh`
  - Verify: `task lint && task lint-depguard-test && go test ./internal/core/... -run TestCorePurity`
  - Level: L1

### E11-S04 — OPA capability sandbox `[autonomous · engine-grade · maintainer LGTM]`

- **Dependencies**: S03 **and the operator's answer to judgment call (d)** — S04 cannot close
  without it, because (d1) and (d2) place the evaluator in different packages and gate it with
  different mechanisms. **S03 is blocked on the same answer** (see S03's dependencies); the only
  stories genuinely unaffected by (d) and free to proceed while it is pending are **S00, S01
  and S02**.
- **Definition of done**: D-013's sandbox is real — a capability set that **denies by
  default** and allows an explicit, enumerated builtin list; `http.send`, `net.*`,
  `opa.runtime`, `time.*`, `rand.*`, and any I/O builtin are unavailable; a module using one
  fails to **compile**, not at runtime; and the rule-7 boundary question is closed by an ADR
  amendment + decision row rather than by a green-but-non-transitive purity walk.

- ⚠️ **ALLOWLIST FLOOR, set by E11-S01 / D-156 — `graph.reachable` MUST be allowed**, and
  `split` alongside it. Both are pure and deterministic (no clock, randomness or I/O).
  **`graph.reachable` is the one that carries the epic's strongest justification**: it closes
  the graph at any depth, which tier-1 CEL cannot do at all — a CEL expression's iteration count
  cannot be data-dependent, so only a fixed, syntactically written depth is expressible
  (`docs/planning/rego-tier-ceiling.md` §1.2, §5). `split` is the convenient way to rebuild the
  adjacency from the encoded `"a|b"` pairs the input carries — equally pure, but a convenience,
  not the justification. A denylist drafted from "deny anything unfamiliar" would strike out the reason E11
  exists. **This floor is held by REVIEW, not by a gate:** REQ-E11-S04-02's golden detects a
  **change** to the allowed set, so the sandbox cannot silently *widen* — it cannot detect an
  **omission**, and a golden written without `graph.reachable` is green forever. S04 must carry
  this as a reviewed acceptance criterion or write the pin itself. **It says nothing about
  judgment call (d)** — *which* builtins are callable is orthogonal to *where* the evaluator
  lives and *which gate* enforces rule 7; (d1) vs (d2) is untouched and still blocking.

- **REQ-E11-S04-01** — Given rule 7 and D-013, when a module calls `http.send`, `net.lookup_ip_addr`,
  `time.now_ns`, `rand.intn`, or `opa.runtime`, then compilation **fails** with a message
  naming the denied builtin — one test case per denied builtin, each asserting failure.
  - Test: `internal/core/policy/rego_capabilities.go`, `rego_capabilities_test.go`
  - Verify: `go test ./internal/core/... -run TestDeniedBuiltins`
  - Level: L1
- **REQ-E11-S04-02** — Given allowlists rot silently, when the allowed builtin set changes,
  then a test comparing the effective set against a **committed golden list** fails — so
  adding a builtin is a deliberate, reviewed act, and an OPA upgrade that introduces new
  builtins cannot widen the sandbox unnoticed.
  - Test: `internal/core/policy/testdata/allowed-builtins.golden`
  - Verify: `go test ./internal/core/... -run TestAllowedBuiltinsGolden`
  - Level: L1
- **REQ-E11-S04-03** — Given judgment call (d) and the verified gap that **both purity gates
  are direct-import/direct-call only**, when Rego evaluation is placed, then the placement
  follows the operator's answer (d1/d2/d3) and the story **fails** if it lands a green
  `TestCorePurity` that is green only because the walk is non-transitive. Under **(d1)**: the
  purity guard is extended to a transitive `go list -deps` assertion over the guarded tree,
  with the OPA path as the single named, pinned exception, and a **mutation control** proving
  the new check goes red when a second `net`-reaching dependency is added. Under **(d2)**: a
  depguard rule denies the OPA package from every guarded directory, with a mutation control
  proving it fires.
  - Test: `internal/core/purity_test.go`, `.golangci.yml`, `hack/lint/depguard_test.sh`
  - Verify: `go test ./internal/core/... -run TestCorePurity && task lint`
  - Level: L1
- **REQ-E11-S04-04** — Given (d1) changes a hard rule, when that option is chosen, then an
  **ADR amendment** (ADR-0011 / AGENTS.md rule 7) and a `D-nnn` row land **before** S05, both
  stating plainly that the decision path's network guarantee became capability-enforced rather
  than link-enforced for the Rego tier. The story cannot close on spec text alone.
  - Test: `docs/adr/`, `docs/decisions/decisions.md`
  - Verify: manual — S05 is blocked until present
  - Level: L0

### E11-S05 — Input binding: the identical `EvaluationInput` `[autonomous · engine-grade]`

- **Dependencies**: S04.
- **Definition of done**: a Rego module sees the same typed input a CEL leaf sees, proving
  P3-E1-S02's neutrality claim empirically; **no `EvaluationInput` schema change**.

- **REQ-E11-S05-01** — Given REQ-P3-E1-S02-01, when a rule is evaluated by either backend,
  then both receive input derived from the **same** `EvaluationInput` instance — asserted by a
  test that evaluates an equivalent rule under both backends and compares the bound input.
  - Test: `internal/core/policy/rego_input_test.go`
  - Verify: `go test ./internal/core/... -run TestBackendsShareInput`
  - Level: L1
- **REQ-E11-S05-02** — Given typed facts (Spike C / OQ-17), when a fact is `unavailable`,
  `invalid`, or `expired`, then the module observes that **typed state** and cannot mistake it
  for a value — an unavailable fact must not read as absent-and-therefore-fine (the
  "absent-means-trusted" pattern the 2026-08-09 audit found three times).
  - Test: `internal/core/policy/rego_input_test.go`
  - Verify: `go test ./internal/core/... -run TestUnavailableFactVisibleToModule`
  - Level: L1
- **REQ-E11-S05-03** — Given `git diff schemas/decision/` must stay empty, when S05 lands,
  then no decision-contract file has changed.
  - Test: `schemas/decision/**`
  - Verify: `git diff --exit-code -- schemas/decision/`
  - Level: L0

### E11-S06 — Deterministic evaluation budget `[autonomous · engine-grade · maintainer LGTM]`

- **Dependencies**: S05.
- **Definition of done**: evaluation is bounded by a machine-independent budget; exceeding it
  can **never** produce a policy outcome (judgment call (b)); rule-7 determinism is proven by
  repeated evaluation.

- **REQ-E11-S06-01** — Given rule 7, when the same module evaluates the same input N times
  (N ≥ 100), then the output — violations, their order, and their messages — is **byte-identical
  every time**, including on a machine under load.
  - Test: `internal/core/policy/rego_determinism_test.go`
  - Verify: `go test ./internal/core/... -run TestRegoDeterminism -count=1`
  - Level: L1
- **REQ-E11-S06-02** — Given judgment call (b), when the evaluation budget is exceeded, then
  the run fails with a **process error**, and a test asserts the outcome is neither APPROVE nor
  BLOCK nor a silently-skipped rule — the failure mode is "assent could not decide", not a
  decision.
  - Test: `internal/core/policy/rego_budget_test.go`
  - Verify: `go test ./internal/core/... -run TestBudgetExceededIsNotADecision`
  - Level: L1
- **REQ-E11-S06-03** — Given Go map iteration order is random, when violations are emitted,
  then they are canonically sorted before entering the decision path — the same defect class
  the audit found in `internal/change/diff_hcl.go` and `entries.go`.
  - Test: `internal/core/policy/rego_eval.go`
  - Verify: `task determinism`
  - Level: L1

### E11-S07 — Violations → findings, with explicit obligation-proof polarity `[autonomous · engine-grade · maintainer LGTM]`

- **Dependencies**: S06.
- **Definition of done**: a module's violation set maps to findings with `EntryRef` subjects;
  **zero violations never proves an obligation** (judgment call (c)); both polarities tested.

- **REQ-E11-S07-01** — Given ADR-0002 v2's shape, when a module returns violations, then each
  maps to a finding carrying the rule, subject (`EntryRef`), and message, validating against
  the frozen `DecisionRecord` schema with no schema change.
  - Test: `internal/core/policy/rego_findings.go`, `rego_findings_test.go`
  - Verify: `go test ./internal/core/... -run TestRegoViolationsToFindings`
  - Level: L1
- **REQ-E11-S07-02** — Given `later-phases.md`'s explicit polarity rule, when a Rego-backed
  **required obligation** yields zero violations **without** an explicit structured proof,
  then the obligation is **NOT satisfied** and the decision is not APPROVE — asserted by a
  test whose module returns an empty violation set and whose expected outcome is
  *unsatisfied*.
  - Test: `internal/core/policy/rego_polarity_test.go`
  - Verify: `go test ./internal/core/... -run TestEmptyViolationsDoNotProveObligation`
  - Level: L1
- **REQ-E11-S07-03** — Given a module can be broken in ways that resemble success, when a
  module is undefined, returns a non-set value, or produces a malformed violation, then each
  case fails **closed** — three distinct test cases, each asserting `approve == false`.
  - Test: `internal/core/policy/rego_polarity_test.go`
  - Verify: `go test ./internal/core/... -run TestMalformedModuleFailsClosed`
  - Level: L1

### E11-S08 — Aggregation boundary `[autonomous · engine-grade]`

- **Dependencies**: S07.
- **Definition of done**: the envelope owns `match`, `effect`, and `points`; a module cannot
  influence any of them, and the boundary is enforced structurally rather than by convention.

- **REQ-E11-S08-01** — Given ADR-0002 v2 and ADR-0007, when a module emits a value that
  *looks* like an effect or a points value (e.g. `{"effect": "block", "points": 99}`), then it
  is **ignored**: the finding's effect and points come from the envelope, proven by a test
  whose module tries to escalate and whose expected outcome is the envelope's.
  - Test: `internal/core/policy/rego_boundary_test.go`
  - Verify: `go test ./internal/core/... -run TestModuleCannotSetEffectOrPoints`
  - Level: L1
- **REQ-E11-S08-02** — Given ADR-0018, when a Rego-backed rule carries a `phase`, then the
  same never-additive phase ceiling applies as for CEL rules (`CoverWithPhaseCeiling`) — a
  Rego rule is not a phase bypass.
  - Test: `internal/core/policy/rego_boundary_test.go`
  - Verify: `go test ./internal/core/... -run TestRegoRuleRespectsPhaseCeiling`
  - Level: L1

### E11-S09 — `assent lint` hard errors for Rego rules `[autonomous]`

- **Dependencies**: S08.
- **Definition of done**: E3's six hard-error checks have Rego equivalents where meaningful,
  plus Rego-specific ones (missing module, undefined entry rule, denied builtin, both-backends
  leaf); the rule catalogue (`assent catalogue`) reports Rego-backed rules faithfully.

- **REQ-E11-S09-01** — Given E3's framework, when a pack contains a broken Rego rule, then
  `assent lint` exits non-zero with a positioned, contributor-legible message per failure
  class — one test per class.
  - Test: `internal/lint/rego_test.go`
  - Verify: `go test ./internal/lint/`
  - Level: L1
- **REQ-E11-S09-02** — Given D-048's catalogue rules, when a Rego-backed rule is catalogued,
  then its entry is faithful (authored `phase`, `effectivePhase`, generated `docs.url`) and
  fabricates no lifecycle metadata.
  - Test: `internal/catalogue/`
  - Verify: `go test ./internal/catalogue/`
  - Level: L1

### E11-S10 — `assent test` support and goldens `[autonomous]`

- **Dependencies**: S08.
- **Definition of done**: an adopter can test a Rego-backed rule with the same
  `expect.yaml` / `cases.yaml` contract (ADR-0014, no schema change); `--coverage` counts Rego
  rules in **both** polarities.

- **REQ-E11-S10-01** — Given ADR-0014, when a Rego-backed rule is exercised by `assent test`,
  then the expectation format is unchanged and both a violating and a non-violating case are
  covered.
  - Test: `examples/` test fixtures
  - Verify: `assent test ./examples/... && task check`
  - Level: L1
- **REQ-E11-S10-02** — Given E6's both-polarity coverage rule, when `--coverage` runs, then a
  Rego rule counts as covered only when **both** polarities are exercised.
  - Test: `internal/adoptertest/`
  - Verify: `go test ./internal/core/...`
  - Level: L1

### E11-S11 — Remove the quarantine `[autonomous]`

- **Dependencies**: S10.
- **Definition of done**: per `docs/planning/rego-escape-hatch.md` ("Only **E11's own
  implementation lane** … may remove the marker"), the `# locked: D-012` marker is removed
  from `examples/policies/rego/**`, the P3-E3-S04 guard is **updated rather than deleted**,
  and the examples enter the schema-validation CI job and the golden corpus.

- **REQ-E11-S11-01** — Given the guard asserts the marker's presence, when the marker is
  removed, then `hack/check-migration-invariants.sh` is updated so it still forbids what
  remains forbidden (no `rego:` leaf in an archetype/starter pack that has not been migrated)
  and no longer asserts a marker that must not exist — the guard is never simply deleted.
  **`task check` does NOT run this guard** — `hack/check-migration-invariants.sh` is invoked
  only by `.github/workflows/schemas.yml`, so a green local gate proves nothing here. Invoke it
  directly, and add a mutation control proving the updated guard still goes red on an
  un-migrated pack carrying a `rego:` leaf.
  - Test: `hack/check-migration-invariants.sh`, `.github/workflows/schemas.yml`
  - Verify: `bash hack/check-migration-invariants.sh && task check`
  - Level: L1
- **REQ-E11-S11-02** — Given the example was excluded from CI, when the quarantine lifts, then
  `examples/policies/rego/bounded_change.rego` validates, compiles under S04's capabilities,
  and is exercised by `assent test`.
  - Test: `examples/policies/rego/bounded_change.rego`
  - Verify: `task check`
  - Level: L1

### E11-S12 — Docs and maturity truth `[autonomous]`

- **Dependencies**: S11.
- **Definition of done**: README feature-maturity moves Rego from **Locked** to its earned
  tier; **ADR-0002's index status line "pluggable half unbuilt: Rego is E11" is retired**;
  `docs/planning/rego-escape-hatch.md` records that the quarantine was lifted by E11-S11 and
  by what authority; predicate-scope docs state which backend each restriction applies to.

- **REQ-E11-S12-01** — Given the audit's docs-truth family, when E11 ships, then no document
  describes Rego as locked, quarantined, or unbuilt, and none claims a capability S01 struck
  from scope.
  - Test: `README.md`, `docs/adr/README.md`, `docs/planning/rego-escape-hatch.md`
  - Verify: `task check`
  - Level: L1

### E11-S13 — Exit gate `[autonomous]`

- **Dependencies**: S00–S12.
- **Definition of done**: `hack/policy/e11_exitgate_test.sh` proves in one invocation: denied
  builtins fail compilation (all cases); determinism over N ≥ 100 runs; the empty-violations
  polarity test present and failing-closed; the effect/points boundary held; the scoped schema
  drift confined to `merge-policy.schema.json`; `schemas/decision/**` unchanged; the example
  unquarantined and green; `task check` green.

- **REQ-E11-S13-01** — Given every prior story, when the exit gate runs, then it fails on any
  regression above and cites D-141 plus ADR-0002 v2.
  - Test: `hack/policy/e11_exitgate_test.sh`
  - Verify: `bash hack/policy/e11_exitgate_test.sh && task check`
  - Level: L1
