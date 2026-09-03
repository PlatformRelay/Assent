# ADR-0013 appendix — `assert` syntax gallery (paper spike, OQ-11/OQ-12)

Six rule archetypes written concretely in the three candidate syntaxes, inside the ADR-0010
envelope. Verdicts feed the matrix in [ADR-0013](0013-assert-syntax-and-backend.md). All
snippets are design fiction against the draft PolicyInput (ADR-0011).

## Predicate scope (variables visible to `assert`)

| Variable | Meaning | Scope |
| --- | --- | --- |
| `old`, `new`, `path`, `kind`, `file` | promoted fields of the matched `Change` | change |
| `entry` | head-state value tree of the containing entry | change |
| `oldEntry` | base-state value tree of the containing entry (absent for adds) | change |
| `changes` | all changes in this class slice | change |
| `facts` | provider results (ADR-0004) | both |
| `mr` | author, branches, labels | both |
| `env` | environment label from the classifier (ADR-0008) | both |

Naming precedent: K8s ValidatingAdmissionPolicy's `object` / `oldObject`.

Candidate syntaxes:

- **T** — kyverno-json assertion trees (`assert.all[].check`, JMESPath in parenthesized
  keys, backtick JSON literals). Syntax verified against kyverno-json `main` docs 2026-07-21.
- **C** — bare CEL expression string (the current draft-sample style).
- **H** — hybrid: `all`/`any`/`not` combinators, CEL leaves, per-leaf `message`; a plain
  string is shorthand for one leaf.

---

## 1. Bounded numeric change with a fact (quota)

```yaml
# T — tree
- name: partition-increase-within-quota
  match: { changes: [{ path: "**/partitions", kind: modify }] }
  assert:
    all:
      - message: "partition change out of bounds"
        check:
          (new >= old): true
          (new <= facts.quota.max_partitions): true
  effect: vouch
```

```yaml
# C — CEL string
  assert: "new >= old && new <= facts.quota.max_partitions"
```

```yaml
# H — hybrid full form
  assert:
    all:
      - cel: new >= old
        message: "partitions may not decrease ({{ old }} -> {{ new }})"
      - cel: new <= facts.quota.max_partitions
        message: "partitions {{ new }} exceeds quota {{ facts.quota.max_partitions }}"
```

**Verdict:** T's tree adds no structure — the payload is flat, so both checks are already
expressions-in-keys. C is shortest but fails as an undifferentiated `false`. H names the
failing bound. H wins on ADR-0012 grounds.

## 2. Ownership (set membership)

```yaml
# T
  assert:
    all:
      - check:
          (contains(facts.author.groups, entry.owner)): true
```

```yaml
# C and H shorthand — identical
  assert: "entry.owner in facts.author.groups"
```

**Verdict:** CEL's infix `in` reads like English; JMESPath needs the `contains()` function
call. C/H tie; T readable but noisier.

## 3. Allow-listed-fields-only-changed

Per-change form (idiomatic — vouch coverage is per change, ADR-0007):

```yaml
# T
- name: only-safe-fields
  match: { classes: [kafka-topic] }
  assert:
    all:
      - check:
          (contains(`["/partitions", "/retentionMs", "/config/cleanup.policy"]`, path)): true
```

```yaml
# C / H shorthand
  assert: 'path in ["/partitions", "/retentionMs", "/config/cleanup.policy"]'
```

Whole-slice variant (when one rule must judge the set):

```yaml
# C / H shorthand — CEL macro
  assert: 'changes.all(c, c.path in ["/partitions", "/retentionMs"])'
```

**Verdict:** first real T casualty — the JMESPath list literal needs a backtick-quoted JSON
array inside a parenthesized YAML key: three nested quoting regimes. CEL's native list
literal wins outright. Macros (`all(c, …)`) are the most programmer-y corner of CEL;
per-change evaluation keeps most users away from them.

## 4. Deletion detection

Owned by the envelope in all three candidates — no `assert` needed (ties):

```yaml
- name: no-topic-deletion
  match: { changes: [{ path: "topics/**", kind: delete }] }
  effect: block
  message: "Topic deletion is never auto-mergeable."
```

Conditional deletion (allow only unused dev topics) needs a predicate:

```yaml
# C / H shorthand
  assert: 'env == "dev" && facts.usage.consumer_groups == 0'
# H full form gives each condition its own message (same shape as §1)
```

**Verdict:** tie on the base case — evidence that `match` already owns the structural part,
which is precisely why an additional structural tree layer in `assert` is redundant.

## 5. Environment-conditional strictness

First choice is **no predicate at all**: route env strictness via `bindings.yaml`
(ADR-0008/0010) — separate packs/thresholds per environment. Inside a single rule:

```yaml
# T — JMESPath has no conditional operator; emulate with boolean algebra
  assert:
    all:
      - check:
          (env == 'prod' && new <= `12` || env != 'prod' && new <= `48`): true
```

```yaml
# C — CEL ternary
  assert: 'env == "prod" ? new <= 12 : new <= 48'
```

```yaml
# H — combinators read as a rate table
  assert:
    any:
      - all: [ { cel: env == "prod" }, { cel: new <= 12 } ]
      - all: [ { cel: env != "prod" }, { cel: new <= 48 } ]
```

**Verdict:** T degrades into and/or algebra with backtick numerals. C's ternary is compact.
H is the most verbose but self-documents which regime failed. C/H tie; guidance stays
"prefer bindings".

## 6. Hard case — cross-field consistency, old vs new entry

"If `cleanup.policy` flips to `compact`, `retentionMs` must not change in the same MR."

```yaml
# T — the tree matches ONE payload; comparing two states forces the whole predicate
# into a single root-level JMESPath expression. The tree is now pure ceremony.
  assert:
    all:
      - message: "policy flip and retention change must not be combined"
        check:
          (entry.config."cleanup.policy" == oldEntry.config."cleanup.policy" || entry.retentionMs == oldEntry.retentionMs): true
```

```yaml
# C — one implication, written as ||
  assert: >-
    entry.config["cleanup.policy"] == oldEntry.config["cleanup.policy"]
    || entry.retentionMs == oldEntry.retentionMs
```

```yaml
# H — the any-decomposition reads as "one of these must hold", each explainable
  assert:
    any:
      - cel: entry.config["cleanup.policy"] == oldEntry.config["cleanup.policy"]
      - cel: entry.retentionMs == oldEntry.retentionMs
        message: >-
          cleanup.policy flip combined with a retention change
          ({{ oldEntry.retentionMs }} -> {{ entry.retentionMs }}) — split the MR.
```

**Verdict:** this is each syntax's ceiling probe. T fails structurally (single-payload
projection model); C works but buries the logic in one string; H stays legible and
attributable.

> **SUPERSEDED, same correction as the Ceiling summary below** (2026-09-03, D-166). This verdict
> used to close: "Anything harder (**cross-*entry* uniqueness, whole-branch conventions**)
> correctly graduates to `rego` per ADR-0002." Both named shapes were **struck** by E11-S01 /
> D-156, and neither graduates to `rego`: cross-entry uniqueness is tier-1 expressible where the
> data is present, and where it is absent the limit is **input availability**, which the Rego tier
> inherits unchanged (REQ-E11-S05-01 pins the identical `EvaluationInput`); "whole-branch
> conventions" is that same limit under another name — ADR-0002's own Amendment 1 now says so
> verbatim. The syntax comparison this verdict makes (T fails structurally, C buries, H stays
> attributable) is untouched. What *does* graduate to `rego` is folds/aggregates beyond `size()`
> and unbounded graph reachability. Full argument in the Ceiling summary note below.
>
> **Found only on the third sweep**, because it was written `cross-*entry*` with emphasis inside
> the word and `git grep cross-entry` cannot match that — recorded here because the miss, not the
> sentence, is the reusable lesson.

---

## Ceiling summary

| Archetype | Trees (T) | CEL string (C) | Hybrid (H) |
| --- | --- | --- | --- |
| Bounded change + fact | works, tree vestigial | works | works, best errors |
| Ownership membership | works, noisier | best | best |
| Allow-listed fields | quoting collapse | works | works |
| Deletion (envelope) | tie | tie | tie |
| Env-conditional | boolean-algebra hack | ternary | verbose but explicit |
| Cross-field old/new | **structural failure** | works, opaque blob | works, attributable |

T fails soonest (archetype 3 quoting, archetype 6 structurally). C never fails outright but
loses explainability as expressions grow. H tracks C's ceiling while keeping per-leaf
attribution — users are forced into Rego at the same point in both C and H.

> **SUPERSEDED where that sentence named the boundary** (2026-09-03, D-166). It used to end
> "…at the same point in both C and H: **cross-entry and whole-branch logic, exactly where
> ADR-0002 wants the tier boundary**". The *comparative* claim — that C and H share a ceiling,
> which is what this gallery exists to establish — is untouched and still stands. **Where that
> ceiling actually falls is not what this 2026-07-21 paper spike guessed.** E11-S01 measured it
> against the surface the repo actually binds and D-156 **struck both named shapes**: cross-entry
> logic is tier-1 expressible where the data is present, and where it is absent the limit is
> **input availability**, which the Rego tier inherits unchanged (REQ-E11-S05-01 pins the
> identical `EvaluationInput`) — so users are *not* forced into Rego there, and Rego would not
> help if they were; "whole-branch" is the same input limit under another name. The measured
> boundary is **folds/aggregates beyond `size()`** and **unbounded graph reachability**. See
> [ADR-0002](0002-policy-frontends-rego-declarative.md) Amendment 1 and
> [`docs/planning/rego-tier-ceiling.md`](../planning/rego-tier-ceiling.md) §6. This is design
> fiction from before the measurement, left standing as the spike's own record; the boundary it
> asserted is not.

## Backend facts (checked 2026-07-21 via GitHub API)

| | kyverno-json | cel-go |
| --- | --- | --- |
| Latest release | v0.0.3 | v0.29.2 (2026-07-08) |
| Last push | 2025-01-07 (~18 months) | 2026-07-20 |
| Stars | 93 | 3 036 (org: `cel-expr`) |
| Contributors | 7 non-bot; top holds ~90% of commits | broad; K8s ecosystem |
| Transitive deps | k8s.io/apimachinery, client-go, gin, JMESPath fork (+ cel-go v0.20.1 itself) | self-contained |
| Termination/cost model | JMESPath (pure; extension purity unverified) | non-Turing-complete, cost-budgeted |
