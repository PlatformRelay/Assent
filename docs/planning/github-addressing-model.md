# GitHub addressing & representation model (E10-S00)

> **Status**: written, awaiting maintainer LGTM (E10-S00 is `[autonomous · design · LGTM]`).
> **Governing ADR**: [ADR-0021](../adr/0021-multi-adapter-forge-seam.md) items 5–8 — this
> document *executes* them. Nothing here contradicts items 5–8, so ADR-0021 is not amended.
> **Inputs**: [forge-dossier-github.md](forge-dossier-github.md) (the spec input, not
> guesswork), [forge-dossier-gitlab.md](forge-dossier-gitlab.md), ADR-0015 §1/§2/§4/§8,
> ADR-0017 §1/§3, ADR-0020, GUIDELINES "Safety invariants".
> **Raises**: [OQ-33](open-questions.md) — `protected-pipeline-source` has no decidable
> predicate on either forge today.

This is E10 story zero: four questions answered on paper before the port signature freezes.
Each answer names the conformance case that will prove it. Case IDs are *minted here and
implemented in S01/S07/S14* — `internal/` is not this story's to touch, so no row is added to
`internal/forge/conformance/catalog.yaml` by this change.

**Evidence classes** used throughout, so a reader can tell a fact from a plan:

| Tag | Meaning |
| --- | --- |
| `code` | read from this tree at the cited line, this change's HEAD |
| `dossier` | cited endpoint/field in `forge-dossier-github.md` (docs snapshot 2026-07-21) |
| `unverified` | plausible but not confirmed against a live forge — resolves to `unknown` |

---

## Q1 — How does the port name the head content of a fork PR?

**Decision: confirm ADR-0021 item 5.** The governed subject is addressed **relative to the
merge request**, never by `(project, branch-name)`. `forge.RunPort` gains

```
FileAtBase(mr, path string) ([]byte, error)
FileAtHead(mr, path string) ([]byte, error)
```

and `FileAtRef(project, path, ref string)` **survives unchanged** for ref-addressed decision
inputs. The two accessors are not alternatives: item 5 decides which is legal where, and the
split *is* the trust boundary of GUIDELINES Safety 3 / ADR-0015 §1.

### The concrete failure this prevents

`code`: `cmd/assent/run.go:270` reads the governed base at `info.TargetBranch` and `:274`
reads the governed head at `info.SourceBranch`, both through `fileAtRefOrAbsent`
(`run.go:465`, call at `:466`), which maps `forge.ErrNotFound` to `nil` bytes. `forge.MRInfo`
(`internal/forge/port.go:40`) carries `SourceBranch` but **no source-repository identifier**.
On GitHub a fork PR's head branch does not exist in the base repository, so that read 404s →
`nil` → `change.OneSidedLifecycle(base, nil)` → `(KindDelete, true)` → **a fabricated
whole-file DELETE the contributor never made**: a spurious BLOCK, or an APPROVE reached on
invented change semantics.

### Verified call-site inventory (this is the load-bearing part)

ADR-0021 warns "verify against the tree before relying on either list", and the check was
worth running — two of its line anchors have drifted. As of this change's HEAD:

| Call site | Reads | Ref | Disposition |
| --- | --- | --- | --- |
| `run.go:270` | governed subject, base side | `info.TargetBranch` | **migrates → `FileAtBase(mr, path)`** |
| `run.go:274` | governed subject, head side | `info.SourceBranch` | **migrates → `FileAtHead(mr, path)`** |
| `run.go:203` | MergePolicy | `info.TargetBranch` | stays `FileAtRef` |
| `run.go:211` | RulesetBinding | `info.TargetBranch` | stays `FileAtRef` |
| `run.go:230` | `.assent/config.yaml` (provider-host declarations) | `info.TargetBranch` | stays `FileAtRef` |
| `run.go:249` | policy pack | `info.TargetBranch` | stays `FileAtRef` |
| `provider_host.go:82` | provider host declaration | `targetRef` | stays `FileAtRef` |
| `provider_host.go:292` | **resource-owner registry** | `targetRef` | stays `FileAtRef` — **D-130** |

Exactly **two** call sites migrate; **six** must not. Corrections to ADR-0021's anchors:
the resource-owner registry read is at `provider_host.go:292` (ADR-0021 cites `:275`, which
now falls inside that function's doc comment), and `refFilePort` is declared at
`provider_host.go:263` (ADR-0021 cites `:246`). `forgePort` at `run.go:64` is correct. A
third drift for S02 to absorb: ADR-0021 item 1 proposes `Describe(project, mr)`; the tree's
method is `GetMR(project, mr)` (`run.go:68`).

`run.go:230` and `provider_host.go:292` deserve the specific mention ADR-0021 gives them:
the first carries provider-host declarations, so migrating it would let a fork's head
redefine its own fact semantics; the second **decides who may approve**, and moving it onto
an MR-relative accessor re-opens D-130 verbatim. Migrating any of the six is a trust-boundary
regression, not a cleanup.

Out of scope for the migration, noted so S02 does not trip over it: when `--checkout` is set,
`run.go:282-287` overrides both sides from the local tree (EFE-S03 / ADR-0008 §4). That path
never touches the forge and is unaffected.

### How each adapter reaches the head (adapter-internal freedom)

Both forges expose an **MR-relative head ref inside the target project**, which is why this
shape is neutral rather than GitHub-shaped:

- **GitHub** — `GET /repos/{o}/{r}/contents/{path}?ref={pr.head.sha}`; the fork's head commit
  is reachable in the base repo as `refs/pull/{n}/head` (`dossier` C13). Addressing by the
  head **SHA** rather than the ref name is preferred: it is the same value pinned into
  `pins.sourceSha`, so the record and the read cannot disagree.
- **GitLab** — the source project id from the MR object, or the target project's
  `refs/merge-requests/{iid}/head`; either way the adapter already holds the MR object.

Two implementations are explicitly forbidden, because both re-mint the defect:
`FileAtBase`/`FileAtHead` delegating to `FileAtRef(project, path, info.SourceBranch)`, and
smuggling `refs/pull/N/head` into `MRInfo.SourceBranch` (it corrupts a documented field and
leaks into rendering — ADR-0021 item 5).

### Conformance cases

| Case id | Test name | Proves |
| --- | --- | --- |
| `fork-head-unchanged-file-no-lifecycle` | `TestConformanceForkHeadUnchangedFileNoLifecycle` | a fork MR whose governed file is **unchanged** yields *no* lifecycle event — the fixture's head branch name must not exist in the base repo, or the case cannot fail |
| `fork-head-genuine-delete-detected` | `TestConformanceForkHeadGenuineDeleteDetected` | **positive control**: a fork MR that really deletes the governed file still mints `KindDelete` |

The positive control is not optional. Without it, "never mint a DELETE from a fork" is
satisfiable by never minting a DELETE at all — the unfailable-assertion shape this repo's
review gate finds more often than any other defect, and the same pairing E10-S12 already
mandates for capability gaps.

---

## Q2 — What operationally decidable predicate makes each capability `supported`, per forge?

**Decision.** Every one of the dossier §4 flags gets a predicate of exactly one of two kinds.
Anything that is neither is `unknown`, and `unknown` is treated as `absent` for arming
(ADR-0021 §3).

- **`P` — probed.** A named read-only endpoint, a named response field, and a comparison,
  evaluated per run. A read that fails for permission reasons yields `unknown`, never
  `absent` (see Q4 — this is where Q4's mapping earns its keep).
- **`C` — contract-proven.** The adapter returns a constant, and that constant is licensed by
  a **named passing conformance case** for that adapter. No case, no constant: the flag is
  `unknown`. This is the rule that makes "we assume the API supports it" legal exactly once —
  when someone wrote the case — and it is what today's adapter is missing (below).

**Scope of `unknown ⇒ never arm`.** It binds *per capability, at its consultation point*, not
"all eleven must be `supported`". `code`: `internal/forge/precondition.go:47-82` consults
exactly three today — `protected-pipeline-source`, `threads-block-merge` and
`eligible-approval-evidence`. S04 must state the arming set explicitly; each capability
promoted into it that is currently assumed-true becomes an honest gap and owes a decision row
(epic judgment call (e)).

### The eleven flags

| # | Capability | GitLab predicate | GitHub predicate | Kind |
| --- | --- | --- | --- | --- |
| 1 | `resolvable-threads` | constant `supported`, licensed by the existing `p3e5-*` reconciliation cases | constant `supported`, licensed by a GraphQL case (`reviewThreads.isResolved` / `resolveReviewThread`, `dossier` C1/C2) | `C` |
| 2 | `threads-block-merge` | `GET /projects/{id}` → `only_allow_merge_if_all_discussions_are_resolved == true` (`code`: `snapshot.go:290`) | `GET /repos/{o}/{r}/branches/{base}/protection` → `required_conversation_resolution.enabled == true`, **or** the equivalent ruleset row from `GET /repos/{o}/{r}/rules/branches/{base}`; both must be consulted (endpoints `dossier` C3 + §2 step (a); **field name `unverified`**) | `P` |
| 3 | `blocking-review` | `absent` — GitLab has no `REQUEST_CHANGES` primitive; ADR-0017 §3 uses threads (`dossier` C4) | `required_pull_request_reviews.required_approving_review_count >= 1` on the base branch; without required reviews a `REQUEST_CHANGES` review does not block (`dossier` C4) | `P` |
| 4 | `review-dismissal-restrictions` | `absent` — no analogue | `required_pull_request_reviews.dismissal_restrictions` non-empty **and** `users[].id ∪ expand(teams[]) ∌ pr.user.id`; team expansion via `GET /orgs/{org}/teams/{slug}/members` (`dossier` C8′) | `P` |
| 5 | `sha-guarded-merge` | constant `supported`, licensed by `sha-guard-source-moved` / `sha-guard-target-advanced` (already in `catalog.yaml`) | constant `supported`, licensed by a GitHub-factory run of the same two cases — `PUT /pulls/{n}/merge` with `sha`, 409 on mismatch (`dossier` C10) | `C` |
| 6 | `deferred-merge-arming` | constant `supported` (MWPS is tier-independent, `dossier` C11) | `GET /repos/{o}/{r}` → `allow_auto_merge == true`; `enablePullRequestAutoMerge` fails without the repo setting (setting `dossier` C11; **field name `unverified`**) | `C` / `P` |
| 7 | `arming-revoked-on-push` | constant `supported` — any new commit cancels MWPS (`dossier` C11) | `supported` **only** when the substitute is configured — stale-approval dismissal **and** at least one required status check; a write-access push does **not** auto-disarm GitHub auto-merge (`dossier` §3 delta 2). Otherwise `absent`. (**Field names `unverified`** — C19 names the setting, not the API field) | `P` |
| 8 | `merge-result-pinning` | `GET /projects/{id}` → `merge_trains_enabled == true` (`code`: `snapshot.go:291`) | base-branch ruleset from `GET /repos/{o}/{r}/rules/branches/{base}` contains the merge-queue rule (endpoint `dossier` C14; **rule-type string `unverified`**) | `P` |
| 9 | `eligible-approval-evidence{full\|aggregate}` | `full` iff `GET /projects/{id}/merge_requests/{iid}/approval_rules` returns rules with `eligible_approvers[]` (`code`: `gitlab/snapshot.go` `hasApprovalRulesAPI`); else `absent` | `full` iff `required_pull_request_reviews.require_code_owner_reviews == true` **and** CODEOWNERS is readable **at the base ref** **and** every referenced team expands (`GET /orgs/{org}/teams/{slug}/members` → 200). Team expansion 403 → `unknown`. See below | `P` |
| 10 | `approval-reset-on-push{default\|opt-in}` | `GET /projects/{id}/approvals` → `reset_approvals_on_push == true` — **currently unprobed** (audit RELI-03; **`unverified`** — the field is named by RELI-03, not read from this tree) | stale-approval dismissal enabled on the base branch; the stronger variant is "require approval of the most recent reviewable push" (`dossier` C19/C8; **field names `unverified`**) | `P` |
| 11 | `protected-pipeline-source` | **no predicate today** — see below | **no predicate today** — see below | — |

**On the `unverified` tags.** The dossier is an *endpoint* study: for several rows it cites the
GitHub **UI setting** and the endpoint that carries it, but not the JSON field name. Those
field names are marked `unverified` rather than dressed up as `dossier`, and under this
document's own rule they resolve to **`unknown`** — hence non-arming — until S09 confirms each
against a live response. That is the fail-closed direction and it costs nothing: a cell that
turns out to be right is promoted at S09; a cell that turns out to be wrong never armed
anything. Rows 3, 4 and 9's GitHub field names are genuinely enumerated in dossier §2 and are
not tagged.

### Row 9 is the one that decides whether GitHub can ever gate — and it is decidable

The dossier's "no API returns the computed per-PR eligible code owners" reads like a dead end.
It is not, because assent never needed the *forge* to compute the set — it needs a
**forge-proven named principal set**, and `code` shows the engine already accepts two ways to
get one: `internal/core/aggregate/approval.go:126-133` admits
`VerifyingCapability ∈ {"approval-rules-api", "codeowners"}` and fails closed on anything
else, and `approvalSatisfies` counts an approver only if its id is in `ev.Eligibility`.

So:

- **`aggregate` is not a satisfying sub-value.** GraphQL `reviewDecision: APPROVED` proves the
  forge enforced *its* rule but names no principal; fed to the engine it is
  `VerifyingCapability: "none"` → a recorded capability gap that **never satisfies**
  (`approval.go:130`). Recording `aggregate` as `supported` would be exactly the
  paper-over REQ-E10-S00-02 forbids.
- **`full` is reachable on GitHub via `codeowners`**, which is already a first-class verifying
  capability: CODEOWNERS read **from the base ref** (fork-safe by GitHub's own definition,
  `dossier` §2 step (b)), matched client-side, teams expanded to ids, owners filtered to
  write-holders. The predicate above is what makes that honest rather than optimistic — and
  it is symmetric with GitLab Free, which is `absent` for the same reason (no named eligible
  set) and already refuses to arm (`precondition.go:70-78`).

This **refines**, and does not contradict, ADR-0021's Consequences prediction that
`eligible-approval-evidence` would plausibly be `unknown` on GitHub forever. The prediction was
about the *aggregate* route; the `codeowners` route was already in the engine. It also needs no
schema change: `code`,
`schemas/approval/v1alpha1/approval-evidence.schema.json:40-42` enumerates
`verifyingCapability` as exactly `["approval-rules-api", "codeowners", "none"]` — the value is
already frozen-legal.

**The asymmetry a reviewer will find, owned here rather than left to be discovered.** Two
things are true and both should be said. First, **no adapter produces `"codeowners"` today** —
`code`: the only writer is `internal/forge/gitlab/resolve.go:85`, which emits
`"approval-rules-api"`; `"codeowners"` appears in the tree solely as an accepted *input* value
(`approval.go:41`, `:127`) and in the frozen enum. GitHub would be its first producer. Second,
GitLab's `eligible_approvers[]` is **forge-computed** while GitHub's CODEOWNERS set is
**adapter-computed from forge-supplied data**, and ADR-0017 §3 says *forge-proven*. The
defence is that the frozen contract already ruled on this: it admits `codeowners` as a real
verifying capability, distinct from `none`, precisely because a CODEOWNERS file served by the
forge **at the base ref** is forge-supplied evidence — what the adapter contributes is
pattern-matching, not authority. The trust-boundary half is what makes it hold, and it is why
the predicate says *base ref*: a head-ref CODEOWNERS would let an author name themselves owner,
which is D-130's shape on a different file. If the maintainer reads ADR-0017 §3 more strictly
than the schema does, the consequence is stated and survivable — GitHub falls to `absent` for
require-review, exactly like GitLab Free, and gates nothing rather than gating wrongly.

### Row 11 is the honest failure, and it is worse than "GitHub lacks it"

`code`: `internal/forge/gitlab/snapshot.go:292` computes GitLab's arming prerequisite as

```go
caps.ProtectedPipelineExternal = strings.Contains(proj.CIConfigPath, "@")
```

A substring test for `@` is a *heuristic*, not a predicate: it shows the CI config names
another project, and proves neither that the referenced file sits on a protected branch nor
that the MR author cannot push to it. That is the SEC-04 shape ADR-0021 forbids by name — and
it is currently the single load-bearing arming prerequisite (`precondition.go:54-62`,
`RefusalInsecureTopology`). **Under this model's rules, `protected-pipeline-source` is
`unknown` on GitLab today**, not `supported`.

A real GitLab predicate exists but costs several reads on a second project (`ci_config_path`
resolves to project *X* → `GET /projects/X/protected_branches` covers the file's branch →
the MR author lacks push access there), and degrades to `unknown` whenever the token is not
scoped to *X*.

On GitHub there is no single readable analogue at all. The plausible predicate is a
**composite of environment and forge** (`unverified`): the run's trigger event is one whose
workflow definition GitHub loads from the base repository's default branch
(`pull_request_target` / `workflow_run` / `merge_group`), **and** that default branch requires
reviews, so the definition is not author-editable. Under `pull_request` from a fork the token
is read-only and secretless (`dossier` C17) — safe, and advisory-only per ADR-0015 §8, which
is a *non-arming* state, not a gap.

Consequences, stated rather than discovered:

1. Under `unknown ⇒ never arm`, **v1 GitHub comments but does not arm** unless OQ-33 is
   answered. ADR-0021 named this as a plausible shipped outcome; this document confirms it.
2. Retiring the `@` heuristic makes **GitLab** arming paths that pass today stop passing.
   That is the intended surfacing (judgment call (e)) and it owes its own decision row at
   S04/S09 — **not** here, because S00 changes no behaviour.
3. The composite predicate spans `cmd/assent`'s CI-env adapter and `internal/forge`, which
   ADR-0015 §4 did not anticipate. That is why it is a question, not a decision → **OQ-33**.

### What GitLab actually probes today (judgment call (e), made concrete)

`code`: `forge.CapabilityFlags` (`internal/forge/snapshot.go:83-92`) is five booleans plus a
tier. Mapped onto the eleven:

| Probed honestly (3) | Heuristic (1) | Hardcoded `true` (1) | Unprobed / implicitly assumed (7) |
| --- | --- | --- | --- |
| 2, 8, 9 | 11 | `MergeResultDigestRecordable` (the *record-only* axis of 8, `snapshot.go:268`) | 1, 3, 4, 5, 6, 7, 10 |

Seven flags become explicit `unknown` at S04 unless a conformance case licenses a `C`
constant. Rows 1 and 5 already have licensing cases; rows 3 and 4 are honestly `absent` on
GitLab; rows 6, 7 and 10 need either a case or a probe. This is the "surfacing is the point"
consequence ADR-0021 promised, with a count attached.

### Conformance cases

| Case id | Test name | Proves |
| --- | --- | --- |
| `capability-report-exhaustive` | `TestConformanceCapabilityReportExhaustive` | every enum member has an entry; a new member without one fails to compile or fails the case |
| `capability-unknown-never-arms` | `TestConformanceUnknownCapabilityNeverArms` | `unknown` at a consultation point refuses arming, `merges == 0` |
| `capability-supported-does-arm` | `TestConformanceSupportedCapabilityArms` | **positive control**, `merges == 1` — without it the previous case is vacuous (E10-S12's rule, applied one story early) |

---

## Q3 — Where does an eleven-capability report live in the record?

**Decision: confirm ADR-0021 item 8, option (ii)** — the multi-capability report is `doctor`
output and arming-refusal reasons only, **never the DecisionRecord**. `git diff schemas/ == 0`
holds; **no schema change is required or proposed by this story.**

### The schema, read rather than quoted

`code`: `schemas/decision/v1alpha1/decision-record.schema.json` `$defs.pins` (lines 65-101) is
`additionalProperties: false`; `capabilityGap` is `{"type": "string", "minLength": 1}` — no
enum, no pattern — and the `allOf` at 94-100 requires it iff `mergeResultDigest` is `null` and
**forbids it otherwise**. The top-level object is `additionalProperties: true`.

Three consequences follow directly, and the second is the one a reviewer will reach for:

1. `capabilityGap` models **one** capability, merge-result pinning. (A prior draft's claim that
   it "already models the absent-capability case" was **False** and stays retired.)
2. **Comma-joining eleven gaps into that string is not a loophole.** It passes validation, but
   it is a semantic lie against the field's own description, and it is *unavailable in the
   common case*: when `mergeResultDigest` **is** pinned, the field is forbidden outright. A
   channel that closes precisely when the merge succeeds is not an audit trail.
3. **There is therefore no gap-selection problem to solve.** The field is not a general gap
   channel that eleven candidates compete for; it is reserved for one capability, and the
   remaining ten have a different home. No priority order is needed, which is the desirable
   answer under AGENTS.md rule 7 — a total, deterministic selection over eleven contenders
   would have been one more thing to get wrong.

Recording the report in the open top-level object is **rejected**, per item 8: a safety-bearing
field that no schema validates and no consumer must read is a fail-closed guarantee in name
only.

### The audit-trail cost, stated plainly — and already visible

The cost is real: *a capability gap that blocks a merge leaves no trace in the DecisionRecord
beyond the existing single `capabilityGap` string.* It is also **pre-existing, not introduced
by E10**, and the tree shows exactly where:

- `code`: `internal/core/aggregate/aggregate.go:193` already computes
  `Result.CapabilityGaps map[string]string` (per governed subject,
  `capabilityGapNone = "approval-capability-none"`), and its own comment says S10 would thread
  it into `pins.capabilityGap`. It never did — `cmd/assent` reads `CapabilityGaps` in
  `doctor.go`/`doctor_forge.go` only, never in the record path.
- `code`: `cmd/assent/run.go:368` sets the record's `capabilityGap` to one **hardcoded
  merge-result string**, unconditionally.

So an approval-capability gap already fails closed and already leaves no record trace. E10
does not create that hole; it inherits it, bounds it, and names where it lives. Revisiting
option (i) — widening `pins` — is a `v1alpha2` conversation, unchanged.

### Where the report does live

`forge.CapabilityReport` (S04) → `doctor`'s typed report (ADR-0017 §9, the existing
`PreconditionProbe.CapabilityGaps` / `Refusals` shape in `internal/forge/precondition.go`) and
the arming-refusal reason surfaced to the operator. Both are unfrozen internal surfaces.

### Conformance case

| Case id | Test name | Proves |
| --- | --- | --- |
| `record-schema-unchanged-under-multi-gap` | `TestConformanceMultiGapRecordStillValidates` | a run with several `absent`/`unknown` capabilities still emits a record that validates against the frozen schema, and the doctor report carries all of them |

---

## Q4 — What is each adapter's HTTP-status → port-sentinel mapping?

**Decision: confirm ADR-0021 item 6, with the disambiguation made concrete.** `ErrNotFound`
means *absent*, never *forbidden*. `forge.ErrUnauthorized` is lifted to the port at S02
(`code`: it exists today only as `internal/forge/gitlab/gitlab.go:238`, adapter-private, while
`forge.ErrNotFound` is already neutral at `internal/forge/port.go:34`).

The precedent to stay consistent with is **AUD2-S02 / REL-03**: `code`:
`cmd/assent/provider_host.go:84` and `:276-283` — the fallback is gated on
`errors.Is(err, forge.ErrNotFound)` **alone**, so a 503, a throttle or a token scoped away
from the repo can no longer masquerade as an absent file. This story extends the same
discrimination one layer down, into the adapter that mints the sentinel.

### GitLab (verified against the tree)

| Status | Sentinel | Note |
| --- | --- | --- |
| 200 | content | `code`: `gitlab.go:486` region |
| 401 / 403 | `ErrUnauthorized` | `code`: `gitlab.go:238`, `:792` |
| 404 | `forge.ErrNotFound` (**absent**) | safe: GitLab distinguishes 403 from 404 for files |
| 429 / 5xx | transport error | retry/backoff per AUD-S11; never a sentinel |
| other | error | never absence |

### GitHub (the trap)

GitHub returns **404 for permission-denied resources** to avoid leaking existence, so *a bare
404 is not evidence of absence*. The adapter must disambiguate before it may return
`ErrNotFound`:

| Status | Context | Sentinel |
| --- | --- | --- |
| 404 on `/contents/{path}` | the **content-scope probe** below returned 200 for this token at this ref | `forge.ErrNotFound` — **absent** |
| 404 on `/contents/{path}` | the content-scope probe did **not** return 200 | `ErrUnauthorized`. **Never** `ErrNotFound` |
| 404 / 403 on `GET /repos/{o}/{r}` | — | `ErrUnauthorized`. No read below it may claim absence |
| 404 on `/branches/{b}/protection` or `/rules/**` | token lacks admin read (`GET /repos/{o}/{r}` → `permissions.admin != true`) | **not a sentinel** — the capability probe yields `unknown` (Q2). GitHub 404s an unprotected branch *and* an unauthorized read with the same status |
| 403 with `x-ratelimit-remaining: 0` or `Retry-After` | primary/secondary rate limit | transport error, retryable; never a sentinel (`unverified` on exact header set) |
| 403 otherwise | — | `ErrUnauthorized` |
| 401 | — | `ErrUnauthorized` |
| 409 on `PUT /pulls/{n}/merge` | `sha` mismatch | typed SHA-guard precondition failure (`dossier` C10), not absence |
| 405 on `PUT /pulls/{n}/merge` | not mergeable | typed precondition failure, not absence |
| 422 | validation | error, never absence |
| 429 / 5xx | — | transport error |

**The content-scope probe, and why it is not a repo-metadata probe.** The obvious
disambiguation — "`GET /repos/{o}/{r}` returned 200, so the token can read this repo, so a
content 404 is path absence" — is **wrong for a token shape E10-S06 explicitly supports**. A
fine-grained PAT with `metadata: read` and **without** `contents: read` gets 200 on the repo
object and 404 on every content read: under that rule, *forbidden* renders as *absent* and the
P0 this question exists to close re-opens through the front door.

The probe must therefore exercise the **same permission as the read it is licensing**: a
sibling content read in the same repo at the same ref — the root listing,
`GET /repos/{o}/{r}/contents?ref={sha}` — returning 200 proves `contents: read` is granted
*there*, and only then is a 404 on the specific path genuine absence. The probe is cacheable
per `(repo, ref)` for the run, so it costs one request, not one per governed read. It also
fails in the right direction on an unresolvable ref: a bad `ref` 404s the root listing too, and
"the ref does not exist" is correctly not path absence. (`unverified` — the exact Contents API
behaviour for the root-listing form is an E10-S06 live check; until confirmed, the residual
rule below applies and the adapter errors.)

**Residual rule (ADR-0021 item 6, restated as the fallback):** for any endpoint where the
adapter cannot distinguish absent from forbidden, it returns an **error**, not absence. The
content-scope probe is what converts "cannot distinguish" into "can" for governed-file reads,
and the `permissions.admin` probe does the same for the protection/ruleset reads; anything not
covered by a row in this table takes the residual rule.

### Conformance cases

| Case id | Test name | Proves |
| --- | --- | --- |
| `forbidden-never-renders-as-absent` | `TestConformanceForbiddenNotAbsent` | a governed-file read the forge refuses on permission grounds aborts the run with **zero forge writes** — never `nil` content, never a lifecycle event |
| `absent-file-still-renders-as-absent` | `TestConformanceAbsentFileIsAbsent` | **positive control**: a genuine 404 inside a readable repo still yields `forge.ErrNotFound`, so a real whole-file ADD/DELETE is still detected (EFE-S03 preserved). Without this, "always error" would pass the case above |
| `ratelimit-403-is-transport-error` | `TestConformanceRateLimit403NotAbsent` | a rate-limited 403 is retried/errored, never mapped to a sentinel |
| `metadata-only-token-is-not-absence` | `TestConformanceMetadataOnlyTokenNotAbsent` | the fixture answers `GET /repos/{o}/{r}` **200** and every `/contents/**` read **404** (the fine-grained-PAT shape). The run must abort, not mint a lifecycle event — this is the case that refuses the repo-metadata probe |

---

## Forward obligations this document creates

| # | Owed by | Obligation |
| --- | --- | --- |
| 1 | S01 | the ten case ids above appear in `catalog.yaml` with an adapter disposition (S01-03's strict-decode adapter list) |
| 2 | S02 | two call sites migrate, six do not; `forge.ErrUnauthorized` lifted to the port; ADR-0021's stale anchors (`provider_host.go:275→:292`, `:246→:263`, `Describe→GetMR`) absorbed |
| 2b | S02 | **resolve `FileAtBase(mr, path)`'s missing project binding before freezing the signature.** `FileAtRef` takes `project` as a parameter and today's client binds none, so ADR-0021's two-argument shape is under-specified: either the client binds the *target* project at construction (and the accessor is genuinely MR-relative) or `mr` becomes composite. Flagged here because a signature that freezes wrong is precisely what S00 exists to prevent; the choice is S02's, the ambiguity is not S02's to discover |
| 3 | S04 | the arming-relevant capability subset is stated explicitly; every `C` constant names its licensing case |
| 4 | S04 / S09 | retiring the `@` heuristic (and any other capability promoted into the arming set) is a **user-visible GitLab behaviour change** → its own `D-nnn` row + changelog entry, per epic judgment call (e). Not minted by S00, which changes no behaviour |
| 5 | operator | **OQ-33** — `protected-pipeline-source` predicate; until answered, v1 GitHub does not arm |

## Claims deliberately not resurrected

Both were found **False** by the 2026-08-10 adversarial review and are recorded here so a
later reader does not re-derive them:

- Extracting the conformance suite does **not** unblock `catalog.yaml`'s `github-deferred`
  rows — both are `level: L3, package: test/e2e`, gated on live GitHub infrastructure (S18).
- `capabilityGap` does **not** already model the absent-capability case generally; it models
  merge-result pinning, which is why it is singular and coupled to `mergeResultDigest`.
