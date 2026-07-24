# `assent lint` hard-error list (ADR-0010 amendment)

This is the consolidated, authoritative list ADR-0010's amendment promised ("The lint
hard-error list … is consolidated in the Phase 3 spec for `assent lint`") and
`openspec/specs/p3-e1-schemas-fixture/spec.md` §P3-E1-S06 requires. Each row names the
triggering condition and the ADR/decision that mandates it, so a future `assent lint`
implementation (Phase 5) has one place to enumerate against — and so later epics that add
their own hard error (e.g. P3-E4's `no-implicit-enforce-phase`, `single-writer-profile`) extend this table instead of
inventing a second list.

A hard error fails `assent lint` (and therefore CI) unconditionally — it is never a warning,
and it is never contingent on what a predicate evaluates to at decision time. Several of
these exist specifically because ADR-0017's adversarial reviews found that letting the
*decision* layer catch a problem left a window where the *policy* itself was already unsafe
in a way lint could have caught before any MR triggered evaluation.

| Hard error | Triggering condition | Mandated by |
| --- | --- | --- |
| **Obligation coverage** | A `RulesetBinding`'s `require:` list names an obligation that no rule in the bound packs `prove`s (`prove.obligation`) — a required safety property with zero rules proving it. | ADR-0017 §2 (required obligations replace anonymous vouch coverage); `schemas/policy/v1alpha1/merge-policy.schema.json`'s `prove.obligation` description |
| **Reserved-class violation** | An MR that touches `.assent/**` carries a pack rule routing the built-in `assent-policy` meta-class to anything other than `block`/`challenge` — i.e. to `vouch` or to a `prove`/`onFailure` obligation-satisfying outcome — independent of what the rule's predicate evaluates to. | ADR-0015 §1 (policy MRs are `block`-by-default and may only relax to `challenge`, never `vouch`) |
| **Fail-open restriction** | A `config.yaml` `providers` entry backing a controlling or authorization fact (e.g. an `entries`-identity, ownership, or approval-eligibility provider) declares `failure: open`. | ADR-0004 amendment; ADR-0017 §6 ("Controlling facts never fail open") |
| **Tests-per-rule** | A rule has zero cases exercising it under `assent test --coverage` (directory-form `.assent/tests/**` or inline `cases.yaml`). | ADR-0010 ("`assent lint` fails packs without tests"); `schemas/testfixture/v1alpha1/test-expectation.schema.json` |
| **Unkeyed lists** | A class's `entries: {mode: list}` declaration has no `identity.pointer` — an unkeyed list collection, rejected at lint rather than guessed. | ADR-0017 §5; REQ-P3-E1-S01-03 (`merge-policy.schema.json`'s `entriesSpec.allOf` already enforces this at the schema level — lint surfaces the same rule with an actionable message before evaluation) |
| **Undeclared predicate-scope fields** | An `assert`/`when`/`cel` leaf references a top-level identifier outside the closed set frozen in [`docs/planning/predicate-scope.md`](predicate-scope.md) (`old`, `new`, `path`, `kind`, `file`, `entry`, `oldEntry`, `changes`, `facts`, `mr`, `env`). | ADR-0016 §2 (unknown fields are load-time errors, never `<no value>`); `docs/planning/predicate-scope.md` |
| **`no-implicit-enforce-phase`** | A rule or pack manifest (`pack.yaml`) omits an explicit `phase` field (`off`/`observe`/`enforce`) — rollout phase has no default, so an undecorated rule/pack is rejected rather than silently defaulting to one phase or the other. Named specifically so an author who edits `effect`/`onFailure` to approximate a rollout instead of using `phase` gets pointed at the sanctioned mechanism. | D-017 (B2); P3-E4-S01; `schemas/policy/v1alpha1/merge-policy.schema.json` (`rule.phase` required); `schemas/policy/v1alpha1/pack.schema.json` (`spec.phase` required ceiling); `docs/planning/policy-lifecycle-phase.md` |
| **`single-writer-profile`** | For any given `(environment, class)` binding, zero or more than one active `PolicyProfile` resolves `writes: true` (overlapping unconditional writers, or a binding with no writer when `Config.profiles` is present). Never last-one-wins — lint fails closed. | D-017 (B3); P3-E4-S02; `schemas/policy/v1alpha1/profile.schema.json` (`spec.writes` required); `schemas/policy/v1alpha1/config.schema.json` (`profiles[]` precedence table); `docs/planning/policy-lifecycle-profiles.md`; `docs/architecture/policy-profiles.md` |

## Notes

- This table is additive: a later epic that introduces a new safety-bearing construct adds its
  hard error here (with a citation) rather than starting a second list — P3-E4's
  `no-implicit-enforce-phase` and `single-writer-profile` rows above are examples of that
  pattern (schema-level fields; this row is the lint cross-reference).
- Hard errors are distinct from `assent lint` *advisories* (e.g. missing `docs.url` on a
  `challenge`/`block` rule) — advisories are out of scope for this table; only errors that fail
  the run belong here.
- Several rows are already enforced at the JSON Schema level (unkeyed lists, reserved-class
  routing's enum shape) — lint's job is to catch the same violation earlier, with a message
  that names the offending rule/binding, not to duplicate schema validation for its own sake.
