# ADR-0013: `assert` authored syntax and backend: CEL-leaf condition trees on cel-go

| | |
| --- | --- |
| **Status** | Accepted (hybrid CEL-leaf trees on cel-go — Spike A; P2-E5) |
| **Date** | 2026-07-21 |
| **Deciders** | Konrad Heimel |
| **Context links** | [ADR-0002 policy surface](0002-policy-frontends-rego-declarative.md) · [ADR-0010 config files](0010-config-files-repo-layout.md) · [ADR-0011 ports](0011-core-ports-and-contracts.md) · [ADR-0012 presentation](0012-presentation-templates-debug.md) · OQ-11 · OQ-12 · D-006 · [appendix: syntax gallery](0013-appendix-syntax-gallery.md) |

## Context

ADR-0002 settled the envelope (one YAML frontend; `assert` tier 1, `rego` tier 2) and left two
coupled questions: the **authored syntax** of `assert` (OQ-12) and its **backend** (OQ-11).
Method: a paper spike — six rule archetypes written concretely in every candidate syntax
(full gallery in the [appendix](0013-appendix-syntax-gallery.md)), plus dependency
fact-checking (2026-07-21, GitHub API):

- **kyverno-json**: latest release v0.0.3; last push 2025-01-07 (~18 months dormant);
  93 stars; 7 non-bot contributors, one of whom holds ~90% of commits; go.mod pulls
  k8s.io/apimachinery, k8s.io/client-go, gin — heavy for a single static binary (ADR-0001).
- **cel-go** (moved to the `cel-expr` org): v0.29.2 released 2026-07-08; pushed 2026-07-20;
  3 036 stars; adopted by Kubernetes (ValidatingAdmissionPolicy, CRD validation rules) and by
  Kyverno itself for its next-generation ValidatingPolicy types.

The syntax finding that decides everything: assertion trees assert the shape of **one**
document; assent's tier-1 predicate compares **two** states (`old`/`new`) plus facts. In every
archetype the tree collapses into JMESPath expressions inside parenthesized YAML keys — the
structure adds nothing, the JMESPath dialect (backtick JSON literals, weak errors) costs a lot.

## Options

| Option | Pros | Cons |
| --- | --- | --- |
| Kyverno-json assertion trees, embedded kyverno-json | genuine Kyverno syntax reuse; engine for free | tree vestigial for diff payloads; JMESPath literals/errors; dormant pre-1.0 dep, bus factor ≈1; heavy transitive deps; authored syntax not portable off the engine |
| Bare CEL strings, cel-go (current draft samples) | reads like a spreadsheet formula; healthy dep; terminating & cost-budgeted | `a && b` fails as bare `false` — no per-conjunct attribution (ADR-0012); long predicates become string blobs |
| **Condition tree (`all`/`any`/`not`) with CEL leaves + per-leaf `message`; plain string = single-leaf shorthand; cel-go** | CEL readability **and** per-leaf failure attribution/messages; mirrors K8s ValidatingAdmissionPolicy `validations` (the modern "Kyverno-style"); draft samples stay valid as shorthand | small bespoke combinator walker to maintain; leaves are still code-in-strings |
| Do nothing (defer to implementation) | none | syntax is a frozen public contract (Phase 3); deferral means accidental design |

## Decision

**We choose the hybrid — `assert` as an `all`/`any`/`not` condition tree with CEL-expression
leaves (plain-string shorthand for one expression), implemented on cel-go behind the
`PredicateBackend` wrapper — because it combines CEL's infix readability with per-leaf failure
attribution and rides the healthiest dependency, accepting a small combinator walker of our
own and that leaves remain expressions, over kyverno-json trees (structurally wrong for
old/new payloads; dormant upstream) and bare CEL strings (cannot explain which conjunct
failed).**

Shorthand and full form are the same document:

```yaml
# shorthand — exactly the existing draft samples (ADR-0010, examples/)
assert: "new >= old && new <= facts.quota.max_partitions"

# full form — one message per leaf; the failing leaf names itself in the finding
assert:
  all:
    - cel: new >= old
      message: "partitions may not decrease ({{ old }} -> {{ new }})"
    - cel: new <= facts.quota.max_partitions
      message: "partitions {{ new }} exceeds quota {{ facts.quota.max_partitions }}"
```

The ceiling case (cross-field old/new consistency) stays inside tier 1 — see the
[appendix](0013-appendix-syntax-gallery.md) for all six archetypes and where each candidate
syntax breaks.

### Trade-off matrix (weights sum to 100; scores 1–5)

Readability and error quality carry 45% because the target persona and ADR-0012 are the
product thesis; dependency health 15% because the facts above are stark; reversibility only 5%
because the wrapper (ADR-0011) already bounds the blast radius. Readability scores are persona
judgment (subjective); dependency scores are measured.

| Criterion | Wt | Trees/kyverno-json | CEL string/cel-go | Hybrid/cel-go |
| --- | --- | --- | --- | --- |
| Readability (non-programmer platform engineer) | 25 | 2.5 | 4.0 | 4.5 |
| Error messages + testability (ADR-0012) | 20 | 2.0 | 3.0 | 5.0 |
| Expressiveness ceiling before Rego | 15 | 2.0 | 4.0 | 4.0 |
| Dependency health / bus factor | 15 | 1.0 | 5.0 | 5.0 |
| Implementability + maintenance in Go | 10 | 3.0 | 4.0 | 4.0 |
| Determinism guarantees | 10 | 4.0 | 5.0 | 5.0 |
| Reversibility behind wrapper | 5 | 2.0 | 4.0 | 4.0 |
| **Weighted total** | | **2.3** | **4.05** | **4.6** |

## Consequences

- **Predicate scope becomes contract**: `old`, `new`, `path`, `kind`, `file` (matched change),
  `entry` / `oldEntry` (containing entry head/base state — K8s `object`/`oldObject`
  precedent), `changes`, `facts`, `mr`, `env`. The existing `entry.owner` ownership sample
  stays valid. Freezes with fixtures in Phase 3.
- All existing draft samples remain valid as shorthand — no rework of ADR-0010 or examples.
- Determinism for free: CEL is non-Turing-complete, side-effect-free, and cost-budgeted; the
  purity invariant of `Predicate.Eval` (ADR-0011) needs no sandboxing effort.
- kyverno-json is **dropped from Spike A**; Chainsaw-style fixture UX for the test harness
  (ADR-0002) is unaffected — that borrowing never depended on the engine.
- Spike A narrows to residual code risk: (1) numeric type coercion YAML/HCL→CEL
  (`CrossTypeNumericComparisons` vs adapter-side normalization) — highest risk; (2) error UX
  for missing facts/unknown fields; (3) cost limit + purity of the standard env; (4) per-leaf
  trace wiring into `Finding`/`Trace` for `explain`; (5) one activation model serving CEL and
  message templates.
- Foreclosed: syntax-level compatibility with kyverno-json policies. Accepted — that ecosystem
  is K8s-admission-shaped, not diff-shaped.
- Reversibility: swapping cel-go for another CEL implementation is wrapper-internal; CEL is a
  spec with multiple implementations. Swapping *away from CEL* would break authored policies —
  that part of the decision is effectively one-way once packs exist.

## Amendment 1 (2026-08-08, D-131): ordering operators refuse text operands

Residual code risk (1) above — "numeric type coercion YAML/HCL→CEL … highest risk" — turned out
to have a fail-open in it. CEL defines `<`, `<=`, `>`, `>=` over **strings** as a lexical
compare, so an ordering leaf silently answers a boolean when its operands bind as text rather
than numbers, and lexically `"6" >= "12"` is **true**: a quoted `partitions: "12"` → `"6"`
shrink evaluated `new >= old` to true, proved `non-destructive`, and reached **APPROVE**. The
converse is equally wrong — a legitimate grow `"6"` → `"12"` evaluated false and **BLOCKed**.

**Amendment: in tier-1 `assert`, an ordering operator whose operand actually evaluates to text —
a CEL `string` or `bytes` value — is an evaluation ERROR, not an answer** (fail-safe direction,
GUIDELINES §2: the error routes to `predicate.error` → REVIEW). The check is on the value at
evaluation time, not on the leaf's syntax — whether `new >= old` is sound depends on the
adopter's data, so no static check of the policy can decide it.

Consequences for authors:

- Ordering **quoted** numerics means coercing first: `int(new) >= int(old)` (already the idiom
  in this repo's tests), or `double(...)`. Comparing ISO-8601 dates means `timestamp(a) < timestamp(b)`.
- **Ordering raw text is no longer expressible in tier-1** and graduates to the Rego escape
  hatch (ADR-0002) — consistent with this ADR's design taste: don't grow a programming language
  in YAML. There is no exempt spelling: `string(a) < string(b)` and the byte-wise
  `bytes(a) < bytes(b)` are the same lexical sort as `a < b` and are refused with it. No policy
  in the corpus, the comparison suite, or either dogfood pack ordered text.
- The refusal set is exactly CEL's two text-shaped types, `string` and `bytes`. Every other
  `<`-comparable type in the frozen predicate scope — `int`, `uint`, `double`, `bool`,
  `duration`, `timestamp` — is a genuine ordering and is untouched; `list`, `map`, `type` and
  mixed numeric pairs have no `<` overload to begin with. That census is what makes the claim
  above true rather than merely intended, and a test pins it against env drift.
- Equality (`==`/`!=`), membership (`in`), and the string member functions (`startsWith`,
  `contains`, `matches`, `size`) are exact rather than ordering and are untouched.

## Counterpoints considered

- *"This isn't real Kyverno syntax, so D-006 is betrayed."* — Strongest objection. Answer:
  Kyverno's own current direction (ValidatingPolicy) and K8s ValidatingAdmissionPolicy both
  use exactly this shape — CEL expressions with per-expression messages in a YAML envelope.
  CEL-in-YAML *is* contemporary Kyverno style; kyverno-json's JMESPath trees are the legacy
  branch, and a dormant one (facts above).
- *"CEL is still code; the persona wanted no code."* — True at the leaves. But the gallery
  shows the tree alternative doesn't remove code, it hides the same expressions in
  parenthesized YAML keys with backtick literals — strictly harder to read. Genuinely
  code-free authoring would need a form builder, which is a tooling layer, not a syntax.
