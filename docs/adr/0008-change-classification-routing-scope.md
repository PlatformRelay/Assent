# ADR-0008: Change classification, ruleset routing, and rule scope

| | |
| --- | --- |
| **Status** | Accepted (P2-E5) |
| **Date** | 2026-07-21 |
| **Deciders** | Konrad Heimel |
| **Context links** | [ADR-0003 change model](0003-canonical-change-model.md) · [ADR-0007 effects](0007-rule-effects-decision-aggregation.md) |

## Context

One repo holds many kinds of entries; one MR may touch several. "This part of the diff is a
Kafka-topic change" must route those changes into the topic ruleset; a tfvars edit under
`prod/` must route into a stricter binding than the same edit under `dev/`. Separately, some
rules must see more than the diff: a naming-convention rule should be able to comment on a
touched entry whose *pre-existing* state violates convention, which requires the full branch
state, not just the changed fields.

## Decision (proposed)

### 1. Classification stage

After the differ (ADR-0003), a **classifier** assigns each ChangeSet entry one or more
**change classes** via declarative matchers (path globs + content predicates, e.g.
`file: topics/**` or `has(new.partitions)`), and detects the **environment** (path convention
or repo config). Classes and environments are just labels — repos define their own.
Unclassified changes get the implicit class `unclassified` (which no vouch rule should match
→ fail-safe REVIEW per ADR-0007).

### 2. Ruleset routing

A **`RulesetBinding`** document maps `(change class, environment)` → policy packs + risk
threshold. One MR touching topics *and* tfvars evaluates both packs, each over its slice of
the ChangeSet; aggregation (ADR-0007) runs over the union of findings with the **strictest
matching threshold**.

### 3. Rule scope

Each rule declares `scope`:

- `change` (default) — predicate sees the matched ChangeSet entries (old/new values).
- `branch` — predicate additionally sees the **full repo state at the head SHA** (parsed
  value trees of the checked-out branch). For: conventions on touched-but-not-changed fields,
  cross-entry uniqueness, referential checks inside the repo.

Determinism holds: head SHA is pinned input; branch scope is still a pure function.

### 4. Local checkout is mandatory

Evaluation always runs against a **local checkout of the MR source branch** (merged-result
checkout where the forge supports it). CI has this for free; webhook mode (ADR-0009) clones
per event. No API-only file fetching — partial views breed nondeterminism and n+1 API calls.

## Consequences

- The classifier is the routing seam: packs stay small and per-domain (topic pack, catalog
  pack, tfvars pack) and can be shared/versioned independently — the marketplace unit.
- `branch`-scoped rules are costlier (parse the tree); the engine parses lazily per class/glob.
- Cross-*repo* state stays out of scope: that is what fact providers are for (ADR-0004).

## Counterpoints considered

- *"Put match globs on every rule instead of a classifier stage."* — Works at small scale,
  but environment × class × pack routing then lives half-duplicated inside every rule;
  bindings centralize it and make the risk-threshold table explicit and auditable.

## Amendment (2026-07-21, second review P2-10): fail-safe by construction

Convention becomes lint hard-errors: (a) a `vouch` rule must be scoped to at least one
explicit class or non-catch-all path — `match: {changes: [{path: "**"}]}` with effect
`vouch` fails `assent lint`; (b) the `unclassified` and `assent-policy` classes are
engine-reserved — a vouch rule matching them is rejected at load, not merely discouraged;
(c) environment matchers declare explicit `priority` (or must be provably non-overlapping) —
silent order-dependence of "last match wins" is removed, and reordering the list cannot
silently re-route prod to dev thresholds.

## Amendment 2 (2026-08-09, D-133): the checkout is content under judgment

§4 mandates a local checkout but says nothing about who authored it. Two fail-open defects
in the checkout reader were argued away on the assumption that a `--checkout` tree is trusted
operator input. It is not. With `--checkout` the local tree is the **sole** authority (D-077),
and `head/` is the **merge-request head**: content the contributor wrote. Git stores a symlink
as a mode-120000 blob, and `git clone` / `git worktree` / `git checkout` materialise it as a
real, possibly dangling, POSIX symlink — so a contributor can ship one.

The boundary is therefore:

1. **Contained reads.** Every read of a checkout side goes through `os.OpenRoot` +
   `(*os.Root).FS()` — the same containment idiom the provider builtins use (D-129), so the
   codebase has one, not two.
2. **Symlinks are refused, never followed.** A root FS blocks escapes at the syscall level but
   still follows a *relative* link that resolves back inside the root, so the refusal is
   explicit and covers every component of the path, not just the last. Containment is anchored
   at `base/` and `head/`: those are operator-provisioned and may themselves be symlinks;
   nothing beneath them may be.
3. **Refusal is an error, and a partial enumeration is an error.** Neither may be answered as
   "absent" or quietly skipped: absence is the EFE-S03 presence signal (`nil` = absent,
   non-nil zero-length = present-but-empty), and a dropped path is a path the changed-file set
   never sees — which is how a `.assent/**` edit escapes the D-042 self-vouch guard.

Consequence, stated plainly: a repository that legitimately contains a symlink cannot be
judged via `--checkout` today; the run fails closed with a named refusal and writes nothing to
the forge. Loosening this means folding the refusal opaque (fail-safe REVIEW), never
following the link.
