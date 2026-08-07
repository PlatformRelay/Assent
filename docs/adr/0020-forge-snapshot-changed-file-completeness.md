# ADR-0020: Forge snapshot changed-file completeness contract

| | |
| --- | --- |
| **Status** | Accepted (D-119 — implemented in P5-AUD-S01) |
| **Date** | 2026-08-06 |
| **Deciders** | Konrad Heimel |
| **Context links** | [ADR-0008](0008-change-classification-routing-scope.md) §4 · [ADR-0015](0015-trust-boundaries-merge-integrity.md) §1 · [ADR-0017](0017-contract-model-obligations.md) §1 (honest capability gaps) · D-042 · D-076 · D-077 · REL-07 (PROJECT-AUDIT-2026-08-06) |

## Context

In `--checkout`-less runs, the forge Snapshot's changed-file list is the **sole**
`.assent/**` detector beyond the governed subject (`cmd/assent/run.go` snapshot fold →
`foldSnapshotPaths`). The GitLab adapter's `mrChangedFiles` issues a single unpaginated
`GET /merge_requests/:iid/changes`, decodes neither `overflow` nor `changes_count`, and
maps 404 to an empty list. GitLab truncates the `changes` array at instance diff limits
(and has deprecated `/changes` in favour of the paginated `/diffs` endpoint). An MR padded
past the limit — contributor-controlled — can therefore carry a `.assent/**` policy edit
invisible to the fold, starving GUARD 1 (D-042: an MR cannot vouch for itself) and
permitting approve+CAS-merge of a self-modifying MR. This is the fail-open class the
reserved-class guard exists to prevent. Checkout mode is unaffected (D-077: the local
tree is the sole authority there).

The decision needed now: the completeness contract for snapshot changed-file
enumeration, and the exact failure semantics when completeness cannot be proven.

## Options

| Option | Pros | Cons |
| --- | --- | --- |
| (a) Keep `/changes`; decode `overflow` + `changes_count`; truncation and 404 → hard error | Smallest diff; strongest-looking closure | Deprecated endpoint; hard error emits no DecisionRecord and no reviewer-visible thread; contributor-inducible truncation becomes a CI-red DoS lever |
| (b) Migrate to paginated `/diffs` with a page ceiling; ceiling → hard error | Supported endpoint; bounded requests | No independent count cross-check; same silent-to-reviewers hard-error semantics |
| (c) **Hybrid**: paginated `/diffs` + `changes_count` cross-check; unprovable completeness → typed gap → opaque changeset → REVIEW; 404/non-200 → hard error | Supported endpoint; two independent completeness signals; failure lands on the existing `changeset.undecidable` axis with an auditable record and a resolvable thread; merge remains impossible | Larger diff; N/100 requests for large MRs |
| (d) Do nothing | Zero cost | Leaves the P1 fail-open open |
| (e) Truncation → forced `ClassAssentPolicy` (BLOCK, GUARD-1 zero writes) | Maximally conservative label | Asserts a self-edit that may not exist; GUARD 1 suppresses even the explanatory thread — least informative outcome for an *uncertain* state |

## Decision

We chose (c): **snapshot changed-file enumeration must either prove completeness or
declare an honest gap that degrades the run to fail-safe REVIEW**, because an
unenumerable change set is epistemically identical to an opaque diff and must land on the
same frozen axis (`changeset.undecidable` → REVIEW, record emitted, thread posted, never
approve/merge) — accepting the cost of paginated requests and a larger adapter diff, over
(a)/(b) whose hard errors are silent to MR reviewers and lose the audit record, and over
(e) which asserts facts not in evidence while suppressing all reviewer-facing output.

Contract, normatively:

1. **Neutral type** — `forge.Snapshot` gains
   `ChangedFilesComplete bool` and `ChangedFilesGap string`
   (`ChangedFilesGap` non-empty iff `ChangedFilesComplete == false`; mirrors the
   ADR-0017 §1 `mergeResultDigest`/`capabilityGap` honesty pattern). Every adapter and
   fake must set them explicitly.
2. **GitLab mechanics** — `mrChangedFiles` migrates to
   `GET /merge_requests/:iid/diffs?per_page=100&page=N` following pagination to a hard
   ceiling of 100 pages (10,000 files). Completeness requires ALL of:
   the enumeration terminated below the ceiling; the MR's `changes_count` (from the MR
   GET, a string) parses as a plain integer with no `+` suffix; and that integer equals
   the number of enumerated diff entries. Any violation (ceiling hit, `+` suffix,
   count mismatch in either direction, or a decoded per-entry `overflow`-class marker)
   → `ChangedFilesComplete=false` with a specific `ChangedFilesGap` reason.
3. **Hard errors** — HTTP 404 or any non-200 on the diffs endpoint is a hard error
   (`Snapshot` returns an error; the run exits non-zero with zero forge writes). The MR
   provably exists at this point (`mrWithAuthor` succeeded), so a missing diff resource
   is forge anomaly, never evidence of an empty change set. The 404→empty-list mapping
   is removed.
4. **Run semantics (`--checkout` unset)** — when `ChangedFilesComplete == false`, the
   snapshot fold sets `changeSet.Opaque = true` with
   `OpaqueReason = "forge changed-file enumeration incomplete: " + ChangedFilesGap`
   (in addition to the existing `.assent/**` dominance check over whatever paths WERE
   returned — a visible `.assent/**` path still dominates to BLOCK). The existing
   `decide` short-circuit then yields REVIEW with finding code `changeset.undecidable`.
   No new finding code; the reason string carries the specifics.
5. **Checkout mode** — unchanged. D-077 local-tree authority stands; snapshot
   completeness is not consulted for classification there.
6. **Conformance obligations** — the fake forge must model truncation (a knob forcing
   `ChangedFilesComplete=false` with a gap reason, and a knob forcing diff-endpoint
   404/5xx). The conformance suite gains, as required cases: truncated snapshot →
   REVIEW record + thread, zero approve/merge writes; truncated snapshot whose partial
   list still contains `.assent/**` → BLOCK (GUARD 1, zero writes); diff-endpoint 404 →
   hard error, zero writes. The GitLab adapter gains httptest cases for pagination,
   ceiling, `changes_count` mismatch, `+`-suffix, and 404.

## Consequences

- The D-042 self-vouch guard can no longer be starved by MR padding in snapshot mode;
  the worst attacker-achievable outcome becomes REVIEW (human in the loop) instead of
  auto-merge.
- Large MRs cost up to 100 paginated requests; acceptable (bounded, read-only), and the
  ceiling is a named constant so instances with higher diff limits can be accommodated
  by a future flag without contract change.
- Signal interplay: GitLab caps `changes_count` with a `+` suffix (commonly at 1000
  files), so completeness becomes unprovable well below the 100-page/10,000-file
  ceiling — most of the ceiling headroom is unreachable in practice. This skews in the
  conservative direction only (the `+`-suffix violation degrades to REVIEW, never
  fail-open); implementers must not "fix" it by trusting the ceiling alone and dropping
  the `changes_count` cross-check.
- `forge.Snapshot` widens (additive Go struct change; not a serialized public schema).
  All fakes/tests must set the new fields explicitly. Go does not enforce struct-field
  initialization, so the real safety net is the zero value: `ChangedFilesComplete`
  defaults to `false`, which fails SAFE (an adapter that forgets to set it degrades to
  REVIEW rather than fail-open), and the required conformance cases fail loudly against
  an adapter that never reports completeness.
- `/changes` usage is eliminated ahead of GitLab's removal of the endpoint.
- Reversible: revert the adapter to `/changes` + `overflow` decode (option (a)) without
  touching the neutral contract — points 1/3/4/5/6 are endpoint-agnostic.

## Counterpoints considered

The strongest argument against is that REVIEW is "less closed" than a hard error: a run
that cannot enumerate should arguably refuse to produce any decision at all. It did not
win because both outcomes are equally merge-proof (REVIEW can never approve/merge), while
the hard error strictly loses information: no DecisionRecord for the audit trail, no
resolvable thread telling reviewers *why* assent stepped back, and a contributor-
triggerable red-CI lever. The repo's frozen taxonomy already encodes this judgment —
opaque diffs from the checkout fold degrade to REVIEW, not to exit 1 — and snapshot
truncation is the same epistemic state arrived at over the network.
