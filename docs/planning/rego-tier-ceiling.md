# The tier-1 (CEL) ceiling — where `assert` runs out, with concrete rules

**Owner:** E11-S01 (`openspec/specs/p5-e11-rego-backend/spec.md`). **Authority:** D-141
(implementation unlock), D-156 (this record). **Satisfies:** REQ-E11-S01-01 (a concrete rule
per named shape, its attempted CEL leaf, and the specific reason it fails) and
REQ-E11-S01-02 (any shape found CEL-expressible is **struck from E11's scope**).

This document is a **scope-reduction instrument**, not a feature argument. D-017 required
"each ported rule tries CEL first, the backend is built when a concrete rule demonstrably
exceeds the tier-1 ceiling". D-141 lifted that as a *gate*; it did not lift it as a *design
need*. Two of the four shapes `openspec/specs/later-phases.md:284` names turn out not to
justify a second backend, and they are struck here.

Every claim below is checked against **the CEL surface this repository actually binds** — not
CEL in general. The ceiling is set by what assent *binds*, not by what `cel-go` can parse.

---

## 1. The tier-1 surface, as actually bound

`internal/core/aggregate/evaluate.go` `newEvalEnv` builds the environment with **eleven
variables and nothing else**:

```go
cel.NewEnv(
  cel.Variable("old", cel.DynType),      cel.Variable("new", cel.DynType),
  cel.Variable("entry", cel.DynType),    cel.Variable("oldEntry", cel.DynType),
  cel.Variable("path", cel.StringType),  cel.Variable("kind", cel.StringType),
  cel.Variable("file", cel.StringType),  cel.Variable("env", cel.StringType),
  cel.Variable("changes", cel.ListType(cel.DynType)),
  cel.Variable("facts", cel.DynType),    cel.Variable("mr", cel.DynType),
)
```

That matches the frozen predicate-scope table (`docs/planning/predicate-scope.md`) exactly.
**Zero extension libraries and zero custom functions are registered** — no `ext.Bindings`, no
`ext.Math`, no `ext.Lists`, no `ext.Strings`, no optional types. Five further properties bound
the surface, and each is load-bearing below:

| Property | Where it is fixed |
| --- | --- |
| Combinators are **boolean**, not dataflow: `all`/`any`/`not` combine leaf *truth values*; no value crosses a leaf boundary | `internal/core/aggregate/asserttree.go` (`walkAssertTreeDepth`, depth ceiling 32) |
| One leaf is compiled and evaluated **once, standalone**, under a fixed cost budget of `1_000_000` | `evaluate.go` `celCostBudget`, `evalLeaf` |
| The evaluation unit is **one file** | `change.ChangeSet` is documented "the canonical, order-stable set of changes for **one file**" (`internal/change/diff.go:129`); `assent run` takes exactly one `--subject file:<path>` (`cmd/assent/run.go:266`) and diffs that file alone (`:293`) |
| `facts` may be addressed **only** as a static `facts.<provider>.<name>.value…` dot chain | `internal/lint/facts_ref.go` — `facts['x']` and bare `facts` are `facts-reference-syntax` hard errors; a third segment other than `value` or an envelope escape is `facts-reference-shape` (D-051 Option B) |
| Ordering raw text is refused in **every** spelling (`a < b`, `string(a) < string(b)`, `bytes(a) < bytes(b)`) | `evaluate.go` `textOrderGuard` (D-131, ADR-0013 Amendment 1) |

### 1.1 Function census — measured, not recalled

A throwaway probe reconstructed `newEvalEnv` byte-faithfully against the pinned
`github.com/google/cel-go v0.31.0` and compiled each candidate. Results:

| Compiles | Rejected (`undeclared reference`) |
| --- | --- |
| `size(...)`, `filter`, `map`, `all`, `exists`, `exists_one`, `in`, `has` | `sum(...)` and `<list>.sum()` |
| chained comprehensions — `changes.filter(c, …).all(c, …)` | `math.*` (the `ext.Math` library is not registered) |
| nested comprehensions — `changes.all(c, changes.exists(d, …))` | `reduce(...)` — no fold of any kind |
| `oldEntry.acls.filter(a, !(a in entry.acls))` | `lists.*` (`ext.Lists` is not registered) |
| `facts.registry.topics.value[string(new)].retentionMs` | `cel.bind(...)` (`ext.Bindings` is not registered) |
| `string(new) in facts.registry.topics.value` | `.?field` / `orValue` (optional syntax unsupported) |
| `matches`, `startsWith`, `endsWith`, `timestamp`, `duration`, `int`, `double` | `now`, `rand` — non-determinism is not reachable (rule 7 holds) |

**Reproduction** (deliberately not committed — it would add a dependency edge E11-S03 has not
been authorised to add): copy `newEvalEnv` verbatim into a nested throwaway module pinned to
`cel-go v0.31.0`, call `env.Compile` on each expression above, and print the issue set. The
root `go.mod`/`go.sum` must stay byte-unchanged and `go list ./...` must not enumerate the
probe — the same containment E11-S00 uses.

### 1.2 Two ceilings that are *not* expressiveness — and that Rego does not lift

Three of the verdicts below turn on this distinction, so it is stated once, up front.

- **Input availability.** A predicate can only reason over what is in `EvaluationInput`.
  REQ-E11-S05-01 pins that a Rego module receives **the identical `EvaluationInput`**, and
  `later-phases.md` fences the tier at "declared data, **no I/O**". So a rule that fails
  because the data it needs is not in the input **fails identically under Rego**. That is not
  a tier-1 ceiling; it is an input contract, and adding a second expression language cannot
  move it.
- **Declaration vocabulary.** `schemas/provider/v1alpha1/response.schema.json` freezes
  `declaration.type` to `boolean | string | integer | principal` and `cardinality` to
  `single | set`. There is **no declarable object/map type**. A fact whose value is a mapping
  is undeclarable, whichever backend reads it. (See residual OQ-36.)

---

## 2. Shape A — multi-pass

### A1 · Fold/aggregate over a collection — **EXCEEDS TIER 1**

**Rule (generic).** *Bulk-change budget:* across one manifest, the **total** increase in
`partitions` contributed by a single merge request may not exceed 64; any single change is
fine, the sum is what is governed.

**Attempted CEL leaf.**

```cel
changes
  .filter(c, c.kind == "modify" && string(c.path).endsWith("/partitions"))
  .map(c, int(c.new) - int(c.old))
  .sum() <= 64
```

**Why it fails.** `sum` is an `undeclared reference`. So is `reduce`, `math.sum`, and every
`lists.*` helper — none of `ext.Math`, `ext.Lists` or `ext.TwoVarComprehensions` is registered
by `newEvalEnv`, and registering one would widen the frozen predicate scope, which E11's
non-goals fence explicitly. CEL's four collection macros (`all`, `exists`, `exists_one`,
`filter`/`map`) are **boolean or shape-preserving**; none of them folds a list into a scalar.
`size()` is the *only* aggregate in the surface, so **counting is expressible and summing is
not**: `size(changes.filter(c, c.kind == "add")) <= 3` compiles and is the idiom the corpus
already uses. Every other aggregate a budget rule needs — sum, min, max, average, product —
has no spelling.

**Why Rego lifts it.** `sum`, `max`, `min`, `count` and `product` are OPA builtins over data
already in the input; no I/O and no extra data are required. This shape is therefore a genuine
tier-2 justification and it is the **strongest** one in this record.

**Scale note, not an expressiveness note.** Where a fold *can* be hand-simulated (see A2), the
`1_000_000` cost budget bounds how far. A cost overrun is an `evalLeaf` error → `predicate.error`
→ REVIEW, so it degrades safely; it is a scale ceiling, and it is not offered here as an
expressiveness argument.

### A2 · A named intermediate reused across checks — **STRUCK**

**Rule (generic).** *Compute the set of newly added ACL entries once, then assert three things
about it* (all are prod-scoped; none names a wildcard principal; none exceeds the per-MR count).

**Attempted CEL leaf.**

```cel
cel.bind(added, changes.filter(c, c.kind == "add"),
         size(added) <= 5 && added.all(c, !c.new.contains("*")))
```

**Verdict: STRUCK — CEL expresses this, by inlining.** `cel.bind` is an `undeclared
reference` (`ext.Bindings` is not registered) and the assert tree cannot carry the value
either: `all`/`any`/`not` combine leaf *booleans*, so no derived collection crosses a leaf
boundary. But both are **rewrites, not walls** — `changes.filter(c, c.kind == "add")` is a
pure, deterministic sub-expression, and re-deriving it inside each leaf is semantically
identical:

```cel
size(changes.filter(c, c.kind == "add")) <= 5
changes.filter(c, c.kind == "add").all(c, !string(c.new).contains("*"))
```

Chained and nested comprehensions both compile (§1.1). The residual cost is **legibility and
evaluation cost, not expressiveness**, and neither is a licence to add a second backend. A
hostile reviewer will point out that a chained comprehension *is* a second pass; that reviewer
is right, and this record concedes it — which is exactly why only A1 survives from this shape.

---

## 3. Shape B — cross-manifest reference — **STRUCK, all three sub-shapes**

This is the shape `DEM-S05` (judgment call (f), D-142) was nominated to probe. It splits into
three sub-shapes with different reasons and the **same verdict**: none of them justifies E11.

### B1 · Membership in a registry — **STRUCK (CEL-expressible, already in the corpus)**

**Rule (generic).** An ACL entry may only reference a topic that appears in the topic registry.

**CEL leaf that does the job.**

```cel
string(new) in facts.registry.topics.value
```

A `cardinality: set`, `type: string` output is exactly a list of names; `factsToCEL` binds a
resolved fact's `value` through `toCEL`, so it arrives as a CEL list and `in` is a standard
operator. The corpus already ships this idiom —
`examples/packs/service-catalog/.assent/packs/catalog/rules/ownership.yaml:21` uses
`entry.owner in facts.author.groups.value`, and it is green in the dogfood packs.

### B2 · Keyed attribute lookup — **STRUCK (CEL-expressible, two ways)**

**Rule (generic).** The team requesting an ACL must own the referenced topic.

Two spellings work today:

```cel
# (i) a purpose-built provider returns the joined attribute directly
entry.team == facts.resource_owner.owner.value

# (ii) index into a registry fact by a key taken from the change
facts.registry.topics.value[string(new)].retentionMs > 0
```

(i) is shipped: `internal/provider/builtin/resource_owner.go` keys the lookup by the governed
entry identity and returns one `type: string, cardinality: single` fact —
`examples/archetypes/referenced-resource-ownership/` exercises it in both polarities.
(ii) compiles, and it is **lint-clean**: `internal/lint/facts_ref.go`'s D-051 shape check
permits `facts.<p>.<n>.value` plus arbitrary deeper navigation, and `selectChainFields` stops
the chain at the index, so a *dynamic* index past `.value` is accepted. Only an index on
`facts` itself (`facts[x]`) is a hard error.

**Two residuals, recorded honestly, neither of which Rego fixes.** (a) Spelling (ii) relies on
a fact whose value is a **mapping**, and the frozen provider declaration has no object type
(§1.2) — the value is undeclarable, so this is a contract gap, raised as **OQ-36**, not a
backend argument. (b) The generic `builtin/repo-file` cannot supply an arbitrary registry: it
resolves a **basename** by walking up from the change anchor and maps each output to a
**top-level key** of that one file (`repo_file.go` `findMostSpecific` / `answerRepoFile`) — no
glob, no directory enumeration. So a join over "every manifest under `topics/**`" needs a
provider written for it, which is a **domain-aware join** — declined permanently by D-017 and
fenced out of E11's non-goals. Rego is on the wrong side of that fence too.

### B3 · Same-changeset cross-file reasoning — **STRUCK from E11's justification**

**Rule (generic).** An ACL referencing a topic that the *same* merge request deletes must not
evaluate as "topic present".

**Attempted CEL leaf.** There is nothing to write. `changes` binds
`in.ChangeSet.Changes` (`bindLeafActivation`), and that ChangeSet is one file's:
`change.ChangeSet` is documented "for one file"; `assent run` strips one `--subject
file:<path>` and calls `changeSetForGoverned(governed, base, head)` on it alone; the
changed-file fold propagates only `classify.ClassAssentPolicy` and opacity, never a second
file's changes; `adoptertest.Case` is singular (`File string; Base, Head []byte`). No single
evaluation can contain both files, and there is no cross-subject aggregation at the run seam.
`REQ-DEM-S05-04` already records this finding independently.

**Why this is struck rather than escalated.** The blocker is **input availability**, not
expressiveness (§1.2). REQ-E11-S05-01 pins the Rego module to the *identical*
`EvaluationInput`, and the tier is fenced to "declared data, no I/O" — so a Rego module
evaluating this rule sees exactly the same single-file changeset and fails in exactly the same
way. **Adding the Rego tier does not make this rule writable.** The only mechanism that
resolves it today is a provider fact read from the checkout tree
(`cmd/assent/provider_host.go` `checkoutFS`), which is available to both tiers equally and is
gated on `--checkout` (DEM-S14, infra-gated).

> **Accuracy note for a downstream lane.** `REQ-DEM-S05-04` describes that tree as the
> "merged-result checkout". The code reads `<root>/head` when present, and `checkout.go:44`
> documents `head/` as "the MERGE-REQUEST HEAD: content under judgment". Head and merge result
> are not the same tree. Fixing that sentence is DEM's, not E11-S01's — flagged, not edited.

**Consequence for E11's documentation (binds S12).** E11 must not be described anywhere as
delivering cross-manifest reasoning. It does not, and REQ-E11-S12-01 forbids claiming a
capability this record struck.

---

## 4. Shape C — set difference — **STRUCK as an expressiveness claim; blocked on a binding-parity defect**

**Rule (generic).** No ACL entry may be removed from a prod entry without an explicit
`allow-removal` label on the merge request.

**CEL leaf that expresses it.**

```cel
oldEntry.acls.filter(a, !(a in entry.acls)).size() == 0 || "allow-removal" in mr.labels
```

It compiles (§1.1) and the semantics are right: `filter` + `in` + `size` is set difference, and
symmetric difference, subset and disjointness follow the same way (`oldEntry.acls.all(a, a in
entry.acls)` for subset, `entry.deps.all(d, oldEntry.deps.exists(o, o == d))` for containment).
**CEL expresses set difference over a bounded collection, so this shape is struck.**

**The residual, which is a defect and not a ceiling.** The leaf above needs `entry`/`oldEntry`
to be **whole-entry value trees**. They are — but only under `assent test`:

- `bindLeafActivation` binds `toCEL(entryOr(ch.Entry, ch.New))`, falling back to the scalar
  `new`/`old` when `ch.Entry` is nil;
- the **only** writer of `ch.Entry` is `internal/adoptertest/entrytree.go` `populateEntries`,
  called from `adoptertest.go:288`;
- `internal/evaldecode.BuildEvaluationInput` — the sole production builder, used by
  `cmd/assent/evaldecode.go` — never sets it, and `cmd/` contains **no** reference to
  `EntryConfig`, `DiffEntries` or `change.Entries` at all.

So on the `assent run` path `entry` binds a **scalar**, `entry.acls` is a no-such-attribute
error → `predicate.error` → REVIEW. The same rule passes in `assent test` and degrades to
REVIEW in production. That asymmetry is fail-safe, so it is not urgent, but it is real and it
is **not E11-S01's to decide** — raised as **OQ-35**.

**Why this shape must not be struck quietly.** Striking it on the strength of the *test-harness*
binding would remove backend justification for a capability production does not have, and it
propagates straight into S05's input binding and S07's violation shape. The verdict here is
therefore: **struck as expressiveness, conditional on OQ-35** — if the operator resolves OQ-35
by removing the entry binding rather than extending it to `assent run`, this shape returns to
the ceiling and E11's scope reopens by one shape.

---

## 5. Shape D — graph relationship — **EXCEEDS TIER 1 (with a data caveat)**

**Rule (generic).** A service manifest may not declare a dependency that is **transitively**
reachable back to itself — no dependency cycles at any depth.

**Attempted CEL leaf.**

```cel
!transitiveClosure(entry.dependsOn).exists(d, d == entry.name)
```

**Why it fails.** `transitiveClosure` is an `undeclared reference`, and nothing in the surface
replaces it. CEL is deliberately non-Turing-complete: there is **no recursion, no fixpoint, no
user-defined function, no `while`, and no fold** (§1.1) — the four comprehension macros iterate
exactly one level over one collection. A cycle check to a *fixed* depth `k` can be hand-unrolled
into `k` nested comprehensions (`entry.deps.all(d, entry.deps.all(e, e != d))` compiles), but
that is a different rule: it answers "no cycle of length ≤ k", it must be rewritten whenever the
graph deepens, and each unrolling multiplies against the `1_000_000` cost budget. **Unbounded
reachability has no spelling at tier 1.** The same holds for every other graph question —
shortest path, ancestor-of, strongly-connected component, topological order.

**Why Rego lifts it.** Recursive rule definitions over `input` are the canonical Rego idiom, and
they need no I/O and no data beyond the input. This is the second genuine tier-2 justification.

**Caveat, stated so it cannot be over-claimed.** This shape only justifies E11 when the graph is
**already inside `EvaluationInput`** — i.e. the adjacency lives in the governed entry tree
(`entry.dependsOn`), in `changes`, or in a single fact value. A dependency graph assembled from
*other files* is Shape B3 wearing a different hat, and Rego does not lift that (§1.2). E11 must
be justified on the in-input form only.

---

## 6. Verdict summary and the resulting scope

| Shape | Verdict | Reason |
| --- | --- | --- |
| A1 fold/aggregate over a collection | **EXCEEDS — justifies E11** | no `sum`/`reduce`/`math.*`/`lists.*`; `size()` is the only aggregate |
| A2 named intermediate across checks | **STRUCK** | no `cel.bind` and no value flow across leaves, but inlining is semantically identical |
| B1 registry membership | **STRUCK** | `x in facts.<p>.<n>.value` over a `set` fact; already shipped in the corpus |
| B2 keyed attribute lookup | **STRUCK** | purpose-built provider, or `facts.<p>.<n>.value[key]` (compiles, lint-clean); residual OQ-36 |
| B3 same-changeset cross-file | **STRUCK from E11** | input availability, not expressiveness — the evaluation unit is one file and S05 pins the identical input, so Rego fails identically |
| C set difference | **STRUCK** (conditional on OQ-35) | `oldEntry.x.filter(a, !(a in entry.x))` expresses it; the residual is an `assent run` vs `assent test` binding-parity defect |
| D graph relationship | **EXCEEDS — justifies E11** | no recursion/fixpoint/fold; bounded unrolling is a different rule and costs against the budget. Only for graphs already in the input |

**What E11 is now justified to build:** a tier-2 backend for **folds/aggregates over the
in-input collections** and **recursive/graph reasoning over data already in
`EvaluationInput`**. Nothing else in this record supports it.

**What E11 is no longer justified to claim:** cross-manifest reasoning of any kind, set
operations over entry trees, and "reuse a computed intermediate". Two of the four shapes
`later-phases.md` names are struck outright and a third is struck in part.

**Consequences for downstream stories** (recorded here; the epic spec carries the same text):

- **S05** (input binding) need not carry cross-manifest data and must not be widened to fetch
  any — the whole point of B3's verdict is that widening the input is a *different* decision
  from adding a backend, and E11's non-goals fence it.
- **S07** (violation shape) must support a **fold result** (a computed scalar with the
  contributing elements named) and a **path/cycle witness**, not a cross-manifest reference.
- **S12** (docs truth) must not describe Rego as enabling cross-entry or cross-manifest checks.
  ADR-0002 §"`rego`" currently calls it an "escape hatch for **cross-entry checks**"; per this
  record that phrase is inaccurate for the shipped input contract and S12 owns correcting it.

**Corroborating observation.** The single committed illustration of the escape hatch,
`examples/policies/rego/bounded_change.rego`, is **entirely tier-1 expressible** — both of its
`violations` rules are per-change predicates over `input.changes` and `input.facts`, and
`examples/policies/declarative/bounded-change.yaml` is the same rule already authored in the
envelope. When S11 unquarantines it, it should be labelled as a *shape* illustration, not as
evidence that the tier is needed.

---

## 7. Residuals raised, not decided

| Ref | Question |
| --- | --- |
| **OQ-35** | `entry`/`oldEntry` bind whole-entry trees only under `assent test`; `assent run` falls back to the scalar. Extend the binding to the run path, or narrow the documented contract? Gates whether Shape C stays struck. |
| **OQ-36** | The frozen provider declaration has no object/map type, yet the authoring surface and `builtin/repo-file` together permit a mapping-valued fact and dynamic navigation into it. Is a mapping-shaped fact value in-contract? |

Neither is the escalated judgment call (d) (rule-7 mechanism, (d1) vs (d2)); that question is
untouched by this record, which writes no Go and adds no dependency.

**D-002 / rule 1.** Every rule in this document is a generated generic equivalent — topics,
ACLs, partitions, service dependencies. No employer, internal system, tenant, cluster or
hostname appears in any form. `bash hack/check-sanitization.sh` covers this file (it scans
`git ls-files --cached --others`, so an untracked new file is in scope).
