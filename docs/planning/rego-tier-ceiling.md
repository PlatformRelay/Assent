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
| the **value binder** `[expr].all(v, …)` — CEL's standard poor-man's `let`: computes `expr` once and binds it to `v` | `reduce`, `transformList`, `transformMap`, two-var `all(i, x, …)` — every construct that could make traversal *depth* depend on the data |
| nested comprehensions — `changes.all(c, changes.exists(d, …))` | `reduce(...)` — no fold of any kind |
| `oldEntry.acls.filter(a, !(a in entry.acls))` | `lists.*` (`ext.Lists` is not registered) |
| `facts.registry.topics.value[string(new)].retentionMs` | `cel.bind(...)` (`ext.Bindings` is not registered) |
| `string(new) in facts.registry.topics.value` | `.?field` / `orValue` (optional syntax unsupported) |
| `matches`, `startsWith`, `endsWith`, `contains`, `timestamp`, `duration`, `int`, `double`, `+` on strings | `split`, `substring`, `indexOf` — every string **decomposition** function (`ext.Strings` is not registered). Read §1.1's note: this does **not** block working with encoded values |
| | `now`, `rand` — non-determinism is not reachable (rule 7 holds) |

The string row is worth reading carefully, and **also worth not over-reading** — an earlier
draft of §5 did exactly that. The surface can **test** a string (`matches`, `startsWith`,
`endsWith`, `contains`) but cannot **take it apart**: no `split`, no `substring`, no `indexOf`,
no character indexing. That is a real absence. It is **not**, however, a barrier to working with
encoded values, because `+` on strings and `in` are both stdlib: over a finite in-input
candidate set you can always **rebuild** the string you were going to decode and test membership
instead. §5 shows that construction recovering an edge's far end and detecting a 3-cycle without
any decomposition function. What the absence does cost is spelling *generic* string surgery,
which no shape in this record needs.

**Reproduction** (deliberately not committed — it would add a dependency edge E11-S03 has not
been authorised to add): copy `newEvalEnv` verbatim into a nested throwaway module pinned to
`cel-go v0.31.0`, call `env.Compile` on each expression above, and print the issue set. The
root `go.mod`/`go.sum` must stay byte-unchanged and `go list ./...` must not enumerate the
probe — the same containment E11-S00 uses. The expression list is the leaves quoted in §2–§5,
plus these five, whose verdicts carry §5:

```text
REJECTED  facts.graph.edges.value.exists(s, s.split("|")[0] == string(new))         undeclared 'split'
REJECTED  facts.graph.edges.value.exists(s, split(s, "|")[0] == string(new))        undeclared 'split'
REJECTED  facts.graph.edges.value.exists(s, s.substring(0, s.indexOf("|")) == ...)  undeclared 'substring'
REJECTED  facts.graph.edges.value.exists(s, s.indexOf("|") > 0)                     undeclared 'indexOf'
REJECTED  !transitiveClosure(entry.dependsOn).exists(d, d == entry.name)            undeclared 'transitiveClosure'
COMPILES  <the §5 encode-and-compare 3-cycle leaf>                                  see below
COMPILES  facts.graph.nodes.value.filter(m, (string(new) + "|" + m) in facts.graph.edges.value)
```

The last two were **evaluated**, not merely compiled, under `cel.CostLimit(1_000_000)` against
`edges: ["orders|billing","billing|ledger","ledger|orders"]` and
`nodes: [orders, billing, ledger, payments]` — yielding `true/true/true/false` and `[billing]`
respectively. Evaluation matters here: a compile-only check would have left §5's claim about
what CEL can *do* with those primitives untested.

> **This list is NOT complete, and saying so is the point of writing it down** (2026-09-03,
> D-166). Until this note the sentence above read "the expression list is **exactly** the leaves
> quoted in §2–§5, plus these five" — which a reader would use to conclude that everything the
> probe ran was written down. It was not. The probe also ran the **BFS-frontier expressions**
> behind §5's cost envelope — §5's own line "without the frontier binder, naive nesting exceeds
> the budget at `k=4`" is a result that only a frontier form could have produced — and **not one
> of those expressions was recorded anywhere**, which is exactly why §5's figures cannot be
> reproduced. The reproduction recipe above therefore reconstructs §1.1's census and §2–§5's
> quoted leaves; it does **not** reconstruct §5's cost table. See §5's note for the full
> disclosure and for the one fixture that would close both gaps.

### 1.2 The one property the whole record rests on, measured rather than recalled

§5's verdict — the only shape-level claim in this document with nothing behind it but itself —
reduces to a single property: **the number of iteration *levels* a tier-1 CEL expression performs
is *syntactic* — the nesting depth of its comprehensions is fixed by the expression text when the
rule is authored, and cannot be made to depend on the data.** So an expression's maximum path
length through a graph is fixed at authoring time, which is what forecloses unbounded
reachability.

**Stated that precisely on purpose, because the shorter version is false.** An earlier wording of
this headline read "the number of iterations a tier-1 CEL expression performs cannot be made to
depend on the data" — read alone that is wrong, and quotable against this record: a comprehension
over an input collection performs `|N|` iterations, and `|N|` is data. What is data-independent is
the *number of nested levels*, not the count within a level. The argument the record needs, and
uses, is the levels one; only the headline was mis-stated (**corrected 2026-09-03, D-166**).

Earlier drafts supported the property with "CEL is non-Turing-complete by design", which is a
recalled argument in a section headed *measured, not recalled*. Compiled against `newEvalEnv`:

```text
REJECTED  !transitiveClosure(entry.dependsOn).exists(d, d == entry.name)   undeclared 'transitiveClosure'
REJECTED  changes.reduce(a, x, a + 1, 0) > 0                               undeclared 'reduce'
REJECTED  [1,2,3].reduce(a, x, a + x, 0) == 6                              undeclared 'reduce'
REJECTED  changes.transformList(x, x).size() > 0                           undeclared 'transformList'
REJECTED  changes.transformMap(k, v, v).size() > 0                         undeclared 'transformMap'
REJECTED  changes.all(i, x, i >= 0)                                        undeclared 'all'  (no two-var form)
REJECTED  cel.bind(x, changes, x.size() > 0)                               undeclared 'cel'
REJECTED  range(3).size() == 3   /   lists.range(3).size() == 3            undeclared 'range' / 'lists'
REJECTED  for (x, changes) { x }                                           reserved identifier: for
```

There is no fold, no user-defined function, no self-reference, no loop form, and no generator;
the four comprehension macros iterate exactly one level over one collection and cannot call
themselves. Depth must therefore be written out, and cel-go hard-caps that at a parser recursion
limit of **250** (`expression recursion limit exceeded: 250` at nesting depth 260; depth 200
still compiles). **That is the ceiling — one property, measured nine ways.** A bounded `k`-hop
check really is an approximation when the data outruns the authored `k`: on a 4-cycle, the `k=3`
form evaluates `false` and the `k=4` form `true`.

### 1.3 Two ceilings that are *not* expressiveness — and that Rego does not lift

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

### 1.4 On the `assent run` path, almost every binding is a **scalar**

This is measured, and it decides two of the five verdicts below. `assent run` diffs the
governed file in **document mode** (`changeSetForGoverned` → `change.Diff`), and document-mode
`walkNode` emits a `Change` **only where two scalars differ**: a sequence on either side, or a
map-vs-non-map type flip, returns a reason → the whole ChangeSet is **opaque** → fail-safe
REVIEW (`internal/change/diff.go`, `walkNode` and the `vSequence` leaf marker). Collection mode
(`DiffEntries`, which projects sequence elements) is reached **only** from
`internal/adoptertest`; `cmd/` references it nowhere.

Consequently, in production:

| Binding | Shape on the `assent run` path |
| --- | --- |
| `old`, `new` | always a **scalar** (`DecodeCanonical` of a scalar render) |
| `entry`, `oldEntry` | the same scalars — `EvalChange.Entry` is `json:"-"`, in-memory only, and `internal/adoptertest/entrytree.go` `populateEntries` is its **sole writer** (OQ-35) |
| `changes[i].old`, `changes[i].new` | always scalars, for one file |
| `facts.<p>.<n>.value` | a scalar, or a **flat list of scalars** under `cardinality: set`. A nested/mapping value is undeclarable (§1.3, OQ-36) |
| `mr.labels` | a flat list of strings |

So the only *navigable value tree* a production rule can reach is a fact value, and the frozen
declaration cannot describe a nested one. Under `assent test` the picture is different — which
is the whole of OQ-35.

**But "flat list of scalars" is not the same as "carries no structure", and the difference
decides Shape D (§5).** A `{type: string, cardinality: set}` output is fully in contract, and a
provider may put whatever it likes in each string — including an encoded edge, e.g.
`edges: ["orders|billing", "billing|ledger"]`. Every link is verified in-tree: the declaration
is legal (`schemas/provider/v1alpha1/response.schema.json`); `fact.value` carries **no**
JSON-Schema type constraint and `provider.ResolveFactsChecked` cross-checks the *declaration*,
never the value (`internal/provider/resolve.go`); outputs are operator-authored per provider
(`internal/provider/config.go`, `Outputs map[string]Declaration`); the `http` transport is live
on the plain `assent run` path with **no `--checkout`** (`cmd/assent/provider_host.go`,
`providerCallFor`); and a set fact binds as a CEL list, which ships green today. **So an
adjacency is available inside `EvaluationInput` right now — no `--checkout`, no OQ-35, no
OQ-36.**

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

Chained and nested comprehensions both compile (§1.1) — and the surface additionally has a value
binder, `[expr].all(v, …)`, which computes the sub-expression **once** and reuses it:

```cel
[changes.filter(c, c.kind == "add")].all(added,
  added.size() <= 5 && added.all(c, !string(c.new).contains("*")))
```

That compiles, and it removes the re-derivation entirely. So the residual is **legibility, not
expressiveness and not evaluation cost** — the binder answers the cost half — and legibility is
not a licence to add a second backend. A hostile reviewer will point out that a chained
comprehension *is* a second pass; that reviewer is right, and this record concedes it. **This
shape is struck more firmly than the first draft struck it**, which is exactly why only A1
survives from multi-pass.

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
operator. **The *idiom* is shipped and green** —
`examples/packs/service-catalog/.assent/packs/catalog/rules/ownership.yaml:21` uses
`entry.owner in facts.author.groups.value` in the dogfood packs. Stated precisely, because the
distinction matters: that instance is an **identity** fact (the author's groups), not a
cross-manifest registry read. What the corpus proves is that a set-valued fact binds as a CEL
list and `in` decides membership over it; that the *source* of such a set can be another
manifest follows from the provider contract — any provider may return a set — not from that
example. The nearest shipped cross-manifest instance is the **keyed** form in B2.

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
(§1.3) — the value is undeclarable, so this is a contract gap, raised as **OQ-36**, not a
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
expressiveness (§1.3). REQ-E11-S05-01 pins the Rego module to the *identical*
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

**Why the strike is nonetheless unconditional.** Apply the §1.3 test to both resolutions of
OQ-35 and they converge:

- if OQ-35 is resolved by **extending** the entry binding to `assent run`, the leaf above works
  in production and the shape is struck because **CEL expresses it**;
- if OQ-35 is resolved by **narrowing** the documented contract to the scalar binding, the
  shape fails for **input availability** — there is no entry tree in `EvaluationInput` — and
  REQ-E11-S05-01 hands a Rego module that identical input, so **Rego fails identically**.

Either way the shape does not justify a second backend, so it is struck outright and nothing
downstream is left waiting on OQ-35. What OQ-35 *does* decide is whether the shape is writable
at all, which is a test/production-parity question, not a backend question.

---

## 5. Shape D — graph relationship — **EXCEEDS TIER 1, unconditionally**

**Rule (generic).** A service manifest may not declare a dependency that is **transitively**
reachable back to itself — no dependency cycles at any depth.

**The input is available today.** A provider declares
`edges: {type: string, cardinality: set}` and returns the adjacency as encoded pairs:

```yaml
edges: ["orders|billing", "billing|ledger", "ledger|orders"]
```

That declaration is in contract, the `http` transport that serves it runs on the plain
`assent run` path with no `--checkout`, and the value binds as a CEL list — every link verified
in §1.4. **No OQ-35, no OQ-36, no `--checkout`, no schema change.**

**Attempted CEL leaf.**

```cel
!transitiveClosure(entry.dependsOn).exists(d, d == entry.name)
```

**Why it fails.** `transitiveClosure` is an `undeclared reference` and nothing replaces it. CEL
is deliberately non-Turing-complete: **no recursion, no fixpoint, no user-defined function, no
`while`, no fold** (§1.2, measured nine ways) — the four comprehension macros iterate exactly one level over one
collection, and a comprehension cannot call itself. **Unbounded reachability therefore has no
spelling at tier 1**, and neither does any question that depends on it: shortest path,
ancestor-of, strongly-connected component, topological order.

**What *is* expressible, stated precisely, because an earlier draft of this record got it
wrong.** A cycle check to a **fixed** depth `k` is writable, and it does *not* need the string
decomposition CEL lacks. Over a finite in-input candidate set, **decode is replaceable by
encode-and-compare** — rebuild the edge string and test membership:

```cel
# a 3-cycle through the subject, using only `+`, `in` and nested exists()
facts.graph.nodes.value.exists(m, facts.graph.nodes.value.exists(n,
  (string(new) + "|" + m) in facts.graph.edges.value &&
  (m + "|" + n)           in facts.graph.edges.value &&
  (n + "|" + string(new)) in facts.graph.edges.value))
```

Compiled **and evaluated** under the real `1_000_000` cost limit against
`edges: ["orders|billing","billing|ledger","ledger|orders"]` and
`nodes: [orders, billing, ledger, payments]`, that returns `true / true / true / false` — correct
3-cycle detection with no `split`, no `substring`, no `indexOf`. The far end of an edge is
recoverable as a *value* the same way:
`nodes.filter(m, (string(new) + "|" + m) in edges)` → `[billing]`. Both primitives (`+` on
strings, `in`) were already in this record's own §1.1 census, so this was always derivable from
the table above.

**So the ceiling is `k` itself, not decoding — and `k` reaches further than an earlier draft of
this record claimed.** That draft asserted the cost grew as `O(|N|^(k-1)·|E|)` and "exhausts the
cost budget on any real graph". **Measured, that is the wrong complexity class and the wrong
conclusion.** Binding each BFS frontier once per level with `[expr].all(v, …)` (§1.1) makes the
cost *additive across levels* rather than multiplicative, and under the real
`cel.CostLimit(1_000_000)`:

```text
                              k=10            k=20        k=50
ring |N|=50  deg 5 |E|=250    ~10% of budget  ~25%        ~70%       (all correct)
ring |N|=200 deg 5 |E|=1000   ~45%            EXCEEDED    —
without the frontier binder   naive nesting exceeds the budget at k=4 on the |N|=50 graph
```

Cost grows roughly linearly in `k` at fixed `|N|`. At `|N|=50` a check to `k=50` fits in roughly
**70%** of budget — and since `k ≥ |N|`, CEL there is **not approximating at all: it decides
reachability exactly.** Many governed catalogs are well under 50 entries. The practical `k`
collapses around `|N|≈200`.

> ⚠️ **These figures are ILLUSTRATIVE, and this document cannot reproduce them — said plainly,
> because the rest of the record's method is *reproduce, don't reason*** (2026-09-03, D-166).
> Until this note they were published here to six significant figures — `89,551` / `235,297` /
> `686,317` at `|N|=50`, and `462,618` at `|N|=200` — a precision the record cannot back. They
> were published a second time, unmarked, in **D-156's own row** — this document's `Authority:`
> line and the more quotable copy — which is amended in the same pass rather than left standing.
> **The generating expression is quoted nowhere in this document**, and the probe that produced
> the numbers was deliberately not committed (§1.1, "Reproduction"), so no reader — including a
> later author of this record — can re-derive or falsify a single figure. What *is* written down
> is the setup: `newEvalEnv` verbatim, `cel-go v0.31.0`, the real `cel.CostLimit(1_000_000)`, a
> ring graph of the stated `|N|` and degree, and a BFS-frontier form built from the
> `[expr].all(v, …)` binder of §1.1. The exact expression text at each `k` is **not** recoverable
> from that, and this note will not invent one to close the gap. The figures are therefore
> rounded **to the nearest 5% of the `1_000_000` budget** (the rule is stated so the granularity
> is checkable and uniform: 8.96→10, 23.53→25, 68.63→70, 46.26→45) — and the argument uses exactly two things
> from them: that cost is **roughly linear in `k`** rather than exponential, and that a `k ≥ |N|`
> check **fits the budget at `|N|=50` and does not at `|N|=200`**. Neither needs a third digit.
> **What would close this:** one committed `assent test` fixture carrying a stubbed `edges` fact
> together with the frontier expression itself — the same fixture the deliverability caveat below
> already asks for, which would discharge both gaps at once. Until then the envelope is
> indicative, not a measurement a reader can check.

**This is recorded because it is the honest envelope, not because it is a second reason.** It is
not offered as an expressiveness argument — §2 refuses scale in that role and the same refusal
applies here. It changes nothing about the verdict, and a reader entitled to ask "would CEL
suffice for *our* graph?" is entitled to the measurement rather than an assertion that
forecloses the question. The one claim that carries the verdict is unchanged and narrow: **a
bounded `k`-hop check is expressible; unbounded reachability is not.**

**Why Rego lifts it.** Recursive rule definitions over `input` are the canonical Rego idiom and
`graph.reachable` answers the closure directly, at any depth, without unrolling; `split` turns
the encoded pairs back into an adjacency so the recursion has something to walk. All are pure,
deterministic, and need no I/O and no data beyond the input already bound.

**This is the cleanest tier-2 justification in the record, and it is unconditional.** An input
that is in contract and available today, over which tier-1 CEL can answer only a fixed-depth
approximation while Rego answers the actual question, is precisely the evidence D-017's per-rule
gate was asking for. It needs no answer from OQ-35 or OQ-36.

> **Caveat, stated because this record makes the same distinction for B1 (§3).** No provider in
> the corpus delivers an encoded adjacency today. That the shape is **declarable and
> deliverable** follows from the provider contract — any provider may return a
> `{type: string, cardinality: set}` output, and every link is verified in §1.4 — not from a
> shipped example. **The same epistemic standard as B1, but not the same evidential weight:**
> B1 has a shipped mechanism (`ownership.yaml:21`) and infers only the *source* of the set,
> whereas Shape D infers the whole delivery. That is why "unconditional" is qualified throughout
> as *on today's shipped input contract*, and why the cheapest thing that would close the gap is
> one `assent test` fixture carrying a stubbed `edges` fact.

**Two things this record got wrong before, recorded so the corrections are auditable.**

1. *Draft 1* claimed "a mapping-valued fact, which an adjacency needs, is undeclarable" and
   downgraded this shape to a contingency on OQ-35/OQ-36. An adjacency needs no mapping — a set
   of encoded strings carries one inside the frozen declaration. The error came from conflating
   a *provider-supplied catalog* (shapes B1/B2, struck here as working and shipped) with **B3,
   which is specifically same-changeset cross-file *diffs***.
2. *Draft 2* replaced that with a second "independent reason" — that CEL cannot decode an edge,
   and that this kills even a bounded two-hop check. **Refuted by execution** (above):
   encode-and-compare needs no decoding. Both errors are the same failure with the sign flipped
   — an **asymmetric evidentiary standard**, accepting awkward-but-working spellings when they
   *struck* a shape (A2's re-derivation, B2's dynamic index) and rejecting one when it would
   have *narrowed* a shape this record wanted to keep. The verdict was right both times; the
   argument was not. One standard now applies in both directions, and Shape D stands on
   reason 1 alone.

**Binds E11-S04 (the capability sandbox), which is not written yet.** `graph.reachable` is pure
and deterministic and **must not be denied** by the allowlist: it is what closes the graph at
any depth, and it alone carries this shape's justification. `split` should be allowed alongside
it — it is equally pure and it is the convenient way to rebuild the adjacency from the encoded
pairs — but it is a convenience, not the justification. A denylist drafted from "deny anything unfamiliar" would
strike out this shape's own justification. Recorded in the S04 section of the epic spec, not
only here.

---

## 6. Verdict summary and the resulting scope

| Shape | Verdict | Reason |
| --- | --- | --- |
| A1 fold/aggregate over a collection | **EXCEEDS — justifies E11** | no `sum`/`reduce`/`math.*`/`lists.*`; `size()` is the only aggregate |
| A2 named intermediate across checks | **STRUCK** | no `cel.bind` and no value flow across leaves, but inlining is semantically identical — and the surface's value binder `[expr].all(v, …)` computes the sub-expression **once**, so the residual is legibility, not expressiveness and not evaluation cost (§2) |
| B1 registry membership | **STRUCK** | `x in facts.<p>.<n>.value` over a `set` fact; already shipped in the corpus |
| B2 keyed attribute lookup | **STRUCK** | purpose-built provider, or `facts.<p>.<n>.value[key]` (compiles, lint-clean); residual OQ-36 |
| B3 same-changeset cross-file | **STRUCK from E11** | input availability, not expressiveness — the evaluation unit is one file and S05 pins the identical input, so Rego fails identically |
| C set difference | **STRUCK — unconditionally** | `oldEntry.x.filter(a, !(a in entry.x))` expresses it if the entry tree is bound; if it is not, the failure is input availability and Rego fails identically. Both resolutions of OQ-35 strike it |
| D graph relationship | **EXCEEDS — justifies E11, unconditionally** | **the iteration *levels* of a CEL expression are syntactic and cannot be made data-dependent** — no recursion, fold, user-defined function or loop form, so nesting depth is fixed by the expression text and hard-capped by cel-go's 250 parser recursion limit ⇒ **unbounded** reachability has no spelling (§1.2). A *bounded* `k`-hop check **is** expressible via encode-and-compare (verified by evaluation, §5) — the ceiling is `k`, not decoding — and with the `[expr].all(v, …)` frontier binder its cost is roughly linear in `k`, so on a small graph a large `k` is **affordable and even complete**: the ceiling is **expressive, not performance** (§5). The adjacency is in contract and deliverable today as a `{type: string, cardinality: set}` fact — no OQ-35, no OQ-36, no `--checkout`. **Caveat:** no provider in the corpus ships one yet (§5) |

**What E11 is now justified to build:** a tier-2 backend for **folds/aggregates over the
in-input collections** and **recursive/graph reasoning over an adjacency the input already
carries**. Both justifications are **unconditional** on today's shipped input contract; neither
waits on OQ-35 or OQ-36. Nothing else in this record supports the epic.

**The single strongest piece of evidence** is Shape D: an input that is declarable and
deliverable today, over which tier-1 CEL can express only a check to some **fixed, syntactically
written depth `k`** — because the *number of nested iteration levels* in a CEL expression is
syntactic and cannot be made data-dependent (§1.2) — while Rego answers the **actual**, unbounded question with
`graph.reachable`. That gap is exactly the per-rule evidence D-017's gate existed to demand.
Note the claim is about **expressiveness only**: on a small enough graph a large enough `k` is
both affordable and *complete* (§5 measures where), so this is not an argument that CEL is too
slow — it is an argument that CEL cannot write the rule that holds at any size.

**What E11 is no longer justified to claim:** cross-manifest reasoning of any kind, set
operations over entry trees, and "reuse a computed intermediate". Two of the four shapes
`later-phases.md` names are struck outright and a third is struck in part.

**Consequences for downstream stories** (recorded here; the epic spec carries the same text):

Each is marked with **how it is held** — because a consequence with no mechanism is a comment,
not a constraint, and saying so is cheaper than pretending otherwise.

- **S05** (input binding) need not carry cross-manifest data and must not be widened to fetch
  any — widening the input is a *different* decision from adding a backend, and E11's non-goals
  fence it. **Held by a gate:** `evaluation-input.schema.json` lives under
  `schemas/decision/v1alpha1/`, and REQ-E11-S05-03 pins
  `git diff --exit-code -- schemas/decision/`, so a widening reddens.
- **S04** (capability sandbox) **must not deny `graph.reachable`** — it is pure, deterministic,
  and the thing that closes the graph at any depth, which is Shape D's entire justification.
  `split` belongs in the same allowlist as an equally pure convenience for rebuilding the
  adjacency, but it is not what carries the shape. A denylist drafted from "deny anything unfamiliar" would
  strike out the epic's own strongest evidence. **Held by review — not by a gate, and the
  distinction is the point.** REQ-E11-S04-02's committed `allowed-builtins.golden` detects
  **drift**: it fires when the allowed set *changes*, so the sandbox cannot silently *widen*.
  It does **not** detect **omission** — an S04 author who simply never adds `graph.reachable`
  commits a golden without it and the test is green forever. S04 must therefore carry this floor
  as a reviewed acceptance criterion or write the pin itself. (This says nothing about judgment
  call (d); *where* the evaluator lives is untouched here.)
- **S07** (violation shape) must support a **fold result** (a computed scalar with the
  contributing elements named) and a **path/cycle witness**, not a cross-manifest reference.
  **Held by nothing — stated plainly.** There is no schema or gate over the violation shape at
  the time of writing; S07 must either carry this as a reviewed acceptance criterion or create
  the pin itself.
- **S12** (docs truth) must not describe Rego as enabling cross-entry or cross-manifest checks.
  ADR-0002 §"`rego`" called it an "escape hatch for **cross-entry checks**", which is inaccurate
  for the shipped input contract. **CORRECTED 2026-09-03 (D-166), ahead of S12** — the bullet now
  states the two measured shapes and ADR-0002 **Amendment 1** records the withdrawn sentence
  verbatim and why. Why it did not wait for S12, recorded because the mechanism is the lesson:
  REQ-E11-S12-01's normative text was broad enough to cover it, but its `Test:` list **omitted**
  `docs/adr/0002-*` — the actual wrong file — and its `Verify: task check` had no pin over the
  phrase, so **the gate would not have failed if the line survived**; and S12 is story 12 of 14,
  so if E11 never proceeded a published ADR would have stayed wrong indefinitely. The correction
  was carried instead by the standalone backlog residual **E11-R01**, independent of this epic —
  that residual, not REQ-E11-S12-01, is what actually kept it from being lost. **That omission is
  now closed:** `docs/adr/0002-policy-frontends-rego-declarative.md` **is** named in
  REQ-E11-S12-01's `Test:` list, so the ADR sits **inside** S12's sweep rather than outside it.
  It is a review pin and not a gate — nothing machine-consumes a REQ's `Test:` list — so a
  regression re-inserting the withdrawn sentence would still fail no check today.

**Corroborating observation.** The single committed illustration of the escape hatch,
`examples/policies/rego/bounded_change.rego`, is **entirely tier-1 expressible** — both of its
`violations` rules are per-change predicates over `input.changes` and `input.facts`, and
`examples/policies/declarative/bounded-change.yaml` is the same rule already authored in the
envelope. When S11 unquarantines it, it should be labelled as a *shape* illustration, not as
evidence that the tier is needed.

---

## 7. Residuals raised, not decided

**Neither gates any verdict in this record, and neither blocks E11.** Both are
test/production-parity and contract-hygiene questions surfaced while measuring the surface;
they are recorded so the measurement is reproducible, not as conditions on the scope decision.

| Ref | Question |
| --- | --- |
| **OQ-35** | `entry`/`oldEntry` bind whole-entry trees only under `assent test` (and only there is collection-mode diffing reached at all); `assent run` falls back to the scalar. Extend the binding to the run path, or narrow the documented contract? Shape C is struck either way (§4) and Shape D needs no entry tree (§5). What is at stake is a silent `assent test` / `assent run` divergence an adopter cannot see. |
| **OQ-36** | The frozen provider declaration has no object/map type, yet the authoring surface and `builtin/repo-file` together permit a mapping-valued fact and dynamic navigation into it. Is a mapping-shaped fact value in-contract? Touches B2's *second* spelling only — B2 is struck on its first spelling regardless, and Shape D needs only a flat `cardinality: set` fact. |

Neither is the escalated judgment call (d) (rule-7 mechanism, (d1) vs (d2)); that question is
untouched by this record, which writes no Go and adds no dependency. **(d) is no longer open:
[D-144](../decisions/decisions.md) (2026-08-16) resolved it to (d1) and unblocked E11-S03/S04 —
noted here because this sentence was written after D-144 and reads as though it were still
pending (D-164).**

**D-002 / rule 1.** Every rule in this document is a generated generic equivalent — topics,
ACLs, partitions, service dependencies. No employer, internal system, tenant, cluster or
hostname appears in any form. `bash hack/check-sanitization.sh` covers this file: it scans
`git ls-files --cached --others --exclude-standard`, so a new **untracked** file is in scope.
Note the `--exclude-standard` — a **gitignored** path is *not* scanned, which is why the
throwaway CEL probe of §1.1 is deliberately not committed and is **not** covered by that gate.
Its expressions are reproduced inline above, where the gate does see them.
