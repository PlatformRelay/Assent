# GitHub addressing & representation model (E10-S00)

> **Status**: written, awaiting maintainer LGTM (E10-S00 is `[autonomous · design · LGTM]`).
> **Governing ADR**: [ADR-0021](../adr/0021-multi-adapter-forge-seam.md) items 5–8 — this
> document *executes* them. Nothing here contradicts items 5–8, so ADR-0021 is not amended.
> **Inputs**: [forge-dossier-github.md](forge-dossier-github.md) (the spec input, not
> guesswork), [forge-dossier-gitlab.md](forge-dossier-gitlab.md), ADR-0015 §1/§2/§4/§8,
> ADR-0017 §1/§3, ADR-0020, GUIDELINES "Safety invariants".
> **Raises**: [OQ-33](open-questions.md) — `protected-pipeline-source` has no decidable
> predicate on either forge today; **OQ-34** — whether an adapter-computed CODEOWNERS eligible
> set can ever be `full` under ADR-0017 §3's "typed eligible principals".
>
> **Revision (post-review)**: the first version graded GitHub `eligible-approval-evidence` as
> `supported{full}`. That contradicted this document's own named spec input (dossier §2 grades
> the property `partial`) and was the exact "papered over with a heuristic" outcome
> `REQ-E10-S00-02` forbids. It is now **`unknown`** → OQ-34. Q2's row-9 section keeps the wrong
> answer visible rather than editing it away, because *how* it was wrong is the reusable part.

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

Exactly **two** call sites migrate; **six** must not.

**Anchor drift, attributed to the document that actually carries it** — this matters because
S02 reads the epic spec as its own DoD, so a correction filed against the wrong document
leaves the stale anchor exactly where an implementer will trip on it:

| Stale anchor | Carried by | Correct value |
| --- | --- | --- |
| `provider_host.go:275` (the D-130 resource-owner registry read) | **ADR-0021** — Decision items 1 and 5 | `provider_host.go:292`; `:275` now falls inside that function's doc comment |
| `Describe(project, mr string)` | **ADR-0021** — Decision item 1 | the tree's method is `GetMR(project, mr)` (`run.go:68`) |
| `provider_host.go:246 refFilePort` | **`openspec/specs/p5-e10-github-forge/spec.md:257`** (E10-S02's DoD) — the string `246` appears nowhere in ADR-0021 | `provider_host.go:263`; **fixed in this change**, since the epic spec is in this story's fence |

So ADR-0021's genuine drifts are **two**, not three, and the third belongs to the epic spec.
`run.go:64 forgePort` (spec.md:256) is **correct** and needs no change.

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
  is reachable in the base repo as `refs/pull/{n}/head` (source: **ADR-0021 item 5**, not the
  dossier — C13 names `refs/pull/N/`**`merge`**, a semantically different ref carrying *merged*
  content, not head content; `unverified` until S06 confirms the head ref live). Addressing by the
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
| 1 | `resolvable-threads` | constant `supported`, licensed by the existing `p3e5-*` reconciliation cases | constant `supported`, licensed by `threads-resolvable-graphql` / `TestConformanceThreadResolveRoundTrip` (`reviewThreads.isResolved` / `resolveReviewThread`, `dossier` C1/C2) | `C` |
| 2 | `threads-block-merge` | `GET /projects/{id}` → `only_allow_merge_if_all_discussions_are_resolved == true` (`code`: `snapshot.go:290`) | `GET /repos/{o}/{r}/branches/{base}/protection` → `required_conversation_resolution.enabled == true`, **or** the equivalent ruleset row from `GET /repos/{o}/{r}/rules/branches/{base}`; both must be consulted (endpoints `dossier` C3 + §2 step (a); **field name `unverified`**) | `P` |
| 3 | `blocking-review` | `absent` — GitLab has no `REQUEST_CHANGES` primitive; ADR-0017 §3 uses threads (`dossier` C4) | `required_pull_request_reviews.required_approving_review_count >= 1` on the base branch; without required reviews a `REQUEST_CHANGES` review does not block (`dossier` C4) | `P` |
| 4 | `review-dismissal-restrictions` | `absent` — no analogue | `required_pull_request_reviews.dismissal_restrictions` non-empty **and** `users[].id ∪ expand(teams[]) ∌ pr.user.id`; team expansion via `GET /orgs/{org}/teams/{slug}/members` (`dossier` C8′) | `P` |
| 5 | `sha-guarded-merge` | constant `supported`, licensed by `sha-guard-source-moved` / `sha-guard-target-advanced` (already in `catalog.yaml`) | constant `supported`, licensed by a GitHub-factory run of the same two cases — `PUT /pulls/{n}/merge` with `sha`, 409 on mismatch (`dossier` C10) | `C` |
| 6 | `deferred-merge-arming` | constant `supported` (MWPS is tier-independent, `dossier` C11) | `GET /repos/{o}/{r}` → `allow_auto_merge == true`; `enablePullRequestAutoMerge` fails without the repo setting (setting `dossier` C11; **field name `unverified`**) | `C` / `P` |
| 7 | `arming-revoked-on-push` | constant `supported` — any new commit cancels MWPS (`dossier` C11) | `supported` **only** when the substitute is configured — stale-approval dismissal **and** at least one required status check; a write-access push does **not** auto-disarm GitHub auto-merge (`dossier` §3 delta 2). Otherwise `absent`. (**Field names `unverified`** — C19 names the setting, not the API field) | `P` |
| 8 | `merge-result-pinning` | `GET /projects/{id}` → `merge_trains_enabled == true` (`code`: `snapshot.go:291`) | base-branch ruleset from `GET /repos/{o}/{r}/rules/branches/{base}` contains the merge-queue rule (endpoint `dossier` C14; **rule-type string `unverified`**) | `P` |
| 9 | `eligible-approval-evidence{full\|aggregate}` | `full` iff `GET /projects/{id}/merge_requests/{iid}/approval_rules` returns rules with `eligible_approvers[]` (`code`: `gitlab/snapshot.go` `hasApprovalRulesAPI`); else `absent` | **`unknown`** — see below. The CODEOWNERS route is legitimate (ADR-0017 §3 names it) and its preconditions are probeable (`require_pull_request_reviews.require_code_owner_reviews == true`, CODEOWNERS readable **at the base ref**, teams expandable via `GET /orgs/{org}/teams/{slug}/members`), but **none of them decides the property `full` denotes** — that the adapter-computed eligible set equals the forge's. No forge-readable predicate exists → `unknown`, promoted only by the fidelity case named below (**OQ-34**) | `P` → `unknown` |
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

### Row 9 decides whether GitHub can ever gate — and the honest answer is `unknown`

A first draft of this document declared GitHub row 9 `supported{full}` on three conjuncts:
`require_code_owner_reviews == true`, CODEOWNERS readable at the base ref, and every referenced
team expandable. **That was wrong, and the way it was wrong is worth recording**, because it is
the failure mode `REQ-E10-S00-02` names: each conjunct establishes that a CODEOWNERS file
exists and that GitHub *cares* about code owners. **None of them establishes what `full`
denotes** — that the adapter's computed eligible set **equals** the set GitHub would accept.
That property has no forge-readable predicate, which is exactly the situation row 11 handles
correctly. Two identical gaps had opposite treatments; the asymmetry, not the CODEOWNERS route,
was the defect.

**Decision: GitHub row 9 is `unknown` for v1**, promoted only by a named fidelity case (below).

### What is and is not in dispute

**CODEOWNERS is a legitimate route — this is not the objection.** ADR-0017 §3 names it
explicitly: `require-review` is "satisfied only by forge-proven eligible approval (**approval
rules / CODEOWNERS evidence**, typed eligible principals)". `code`: the frozen schema agrees —
`schemas/approval/v1alpha1/approval-evidence.schema.json:40-42` enumerates
`verifyingCapability` as exactly `["approval-rules-api", "codeowners", "none"]`, and
`internal/core/aggregate/approval.go:126-133` admits `"codeowners"` as a real capability,
distinct from `none`. So no schema change and no ADR amendment is implied by the route itself.

**The objection is §3's *other* clause — "typed eligible principals".** GitLab's
`eligible_approvers[]` is **forge-computed**; a GitHub CODEOWNERS set is **adapter-computed**
from forge-supplied bytes. What the adapter contributes is a matcher, and a matcher can be
wrong in a direction that is not fail-safe.

**The concrete slip-through, spelled out because a vaguer statement would not have caught the
first draft.** An over-permissive matcher — first-match-wins instead of last-match-wins, an
unhandled section or negation syntax, case-folding, or a team-membership read returning a
superset of actual write-holders — puts a principal into `ev.Eligibility` that GitHub would not
accept. `approvalSatisfies` then finds the approver's id in the eligible set
(`approval.go:127`, `:150-158`) and records the obligation **satisfied**. *No arming is needed
for the harm*: the DecisionRecord — the product's whole thesis — asserts that a governance
obligation was met by a principal the forge does not recognise as eligible. GitHub's own
`require_code_owner_reviews` is not a backstop, because it enforces over the **PR's changed
files** while assent's eligible set is scoped to the **governed subject**; the two sets need
not coincide.

### Reconciling the spec input, which graded this `partial` and was right

`docs/planning/forge-dossier-github.md` §2 closes: *"Verdict for ADR-0017 §3: GitHub can prove
**that** eligible approval exists (aggregate `reviewDecision` + protection config) — `partial`
on proving **which** typed principal satisfied **which** rule."* The capability's sub-values
are `{full|aggregate}`, and `full` is precisely the "which typed principal" property the
dossier grades `partial`. **The first draft's `full` contradicted its own named spec input.**
`unknown` is the grading that agrees with it: not `absent` (the route exists and the evidence is
partly forge-supplied), not `full` (unproven), and non-arming under `unknown ⇒ never arm`.

This also **confirms rather than refines ADR-0021's Consequences prediction.** The ADR predicted
`eligible-approval-evidence` would plausibly report `unknown` on GitHub forever, and its stated
reason was dossier §2 step (b) — the CODEOWNERS step, not the aggregate route. That reason
stands.

### The `aggregate` sub-value satisfies nothing, on either forge

GraphQL `reviewDecision: APPROVED` proves the forge enforced *its* rule but names no principal.
Fed to the engine it is `VerifyingCapability: "none"` → a recorded capability gap that **never
satisfies** (`approval.go:129-130`). Recording `aggregate` as `supported` would be the
paper-over `REQ-E10-S00-02` forbids by name. This is symmetric with GitLab Free, which is
`absent` for the same reason — no named eligible set — and already refuses to arm
(`precondition.go:70-78`).

### What would promote row 9 to `full` — named, not hand-waved

`unknown` is a state with an exit, and S08 owns it. Two candidate routes, neither specifiable
as a *proof* today, which is why this is **OQ-34** rather than a decision:

1. **Fixture-corpus fidelity case** — `codeowners-eligible-set-matches-forge` /
   `TestConformanceCodeownersEligibleSetMatchesForge`, over a corpus covering last-match-wins,
   sections, negation, case sensitivity and team expansion, **paired with a positive control**
   proving the case reddens on a deliberately over-permissive matcher (this document's own
   E10-S12 pairing rule). *Limit, stated:* it proves the matcher against fixtures, not against
   GitHub's live computation — fidelity to the spec, not equality with the forge.
2. **Live cross-check** against GitHub's own code-owner determination for the PR
   (`unverified`). *Limit, stated:* any such signal is scoped to the PR's changed files and is
   mutable after PR open, so it is corroboration, not authority.

Until one is accepted and discharged, row 9 stays `unknown` and **GitHub does not satisfy
`require-review`** — the same standing as GitLab Free, which ships today. Consequence: with
rows 9 and 11 both `unknown`, **v1 GitHub comments and does not gate.** That is the fail-closed
outcome the project's thesis prefers over gating on an unproven set.

**One asymmetry owned rather than left to be found:** `"codeowners"` is accepted by the engine
and legal in the frozen enum, but **no adapter produces it today** — `code`: the only writer is
`internal/forge/gitlab/resolve.go:85`, which emits `"approval-rules-api"`. GitHub would be its
first producer, which is part of why the fidelity case is owed before, not after.

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

On GitHub there is no *single* readable field — but "no readable analogue at all" would
overstate it against this document's own cited input. **ADR-0015 §4 already frames the GitHub
condition, and in composite terms**: the assent job must come from a protected source —
*"GitHub: workflows from the target branch (`pull_request` runs the base-ref workflow for
forks; same-repo branches need branch protection on workflow paths)"* — and §4 states it is
"verified by `assent doctor`". Three candidate predicates follow, and OQ-33 asks which the
operator accepts:

1. **Same-repo route (§4's own second clause, largely forge-readable).** Branch protection or a
   path-restricted ruleset covering `.github/workflows/**` on the base branch, so a same-repo
   PR author cannot alter the definition that gates them. This is the closest thing to a
   GitLab-style forge-only predicate and §4 already names it; its readability limits
   (path-scoped push rulesets are org/Enterprise-shaped) are `unverified`.
2. **Fork route (§4's first clause).** Under `pull_request` from a fork GitHub runs the
   **base-ref** workflow with a read-only, secretless token (`dossier` C17). Safe, but
   advisory-only per ADR-0015 §8 — a *non-arming* state, not a gap, and therefore not a route
   to `supported`.
3. **Composite env+forge route** (`unverified`): the trigger event is one whose workflow
   definition GitHub loads from the base repository's default branch (`pull_request_target` /
   `workflow_run` / `merge_group`) **and** that branch requires reviews. This spans
   `cmd/assent`'s CI-env adapter and `internal/forge`, which is the part §4 did not anticipate
   being asked to *probe* rather than document.

Consequences, stated rather than discovered:

1. Under `unknown ⇒ never arm`, **v1 GitHub comments but does not arm** unless OQ-33 is
   answered. ADR-0021 named this as a plausible shipped outcome; this document confirms it.
2. Retiring the `@` heuristic makes **GitLab** arming paths that pass today stop passing.
   That is the intended surfacing (judgment call (e)) and it owes its own decision row at
   S04/S09 — **not** here, because S00 changes no behaviour.
3. Route 1 may be forge-readable enough to stand alone; routes 3 spans `cmd/assent`'s CI-env
   adapter and `internal/forge`. Which of these ADR-0015 §4 accepts as a *probe* — as opposed
   to documentation verified by hand — is not this story's to decide → **OQ-33**.

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
| `threads-resolvable-graphql` | `TestConformanceThreadResolveRoundTrip` | licenses row 1's GitHub `C` constant — post a thread, resolve it via GraphQL, read `isResolved`. No case, no constant: the flag reports `unknown` |
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
2. **Comma-joining eleven gaps into that string must be refused — but the schema does not
   refuse it today, and saying otherwise would be the same over-claim this document is trying
   to avoid.** Such a string passes validation; it is a semantic lie against the field's own
   description; and the `allOf` at `:94-100` forbids the field only *when `mergeResultDigest`
   is pinned*. `code`: `cmd/assent/run.go:368` calls `decision.MergeResultGap(...)`
   **unconditionally**, so `mergeResultDigest` is null and the channel is **open in 100% of
   runs today**. The schema clause is therefore a *latent* guard that goes live only once an
   adapter pins a real merge-result digest (E10-S03/S07 on a merge train or merge queue). What
   closes the loophole in the meantime is not the schema — it is the conformance case named
   below, which is why that case had to be rewritten.
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
| `capability-gap-string-is-merge-result-only` | `TestConformanceCapabilityGapCarriesOnlyMergeResult` | a run with **N** additional `absent`/`unknown` capabilities emits a `pins.capabilityGap` **byte-identical** to the same run with none of them, while the doctor report carries all N. This is the case that reddens on the comma-join adapter |
| `capability-gap-positive-control` | `TestConformanceCapabilityGapStillRecordsMergeResult` | **positive control**: the merge-result gap itself *is* still recorded, so the case above is not satisfiable by emitting an empty string |

The case this replaces — "the record still validates against the frozen schema" — was
**unfailable**, and the reason is worth keeping: `code`, `cmd/assent/run.go:394` already
validates every record against the schema before any write, so that clause is enforced
unconditionally by production code, and a comma-joining adapter would have *passed* it.

---

## Q4 — What is each adapter's HTTP-status → port-sentinel mapping?

**Decision: confirm ADR-0021 item 6, with the disambiguation made concrete.** `ErrNotFound`
means *absent*, never *forbidden*. `forge.ErrUnauthorized` is lifted to the port at S02
(`code`: it exists today only as `internal/forge/gitlab/gitlab.go:238`, adapter-private, while
`forge.ErrNotFound` is already neutral at `internal/forge/port.go:34`).

The precedent to stay consistent with is **AUD2-S02 / REL-03**: `code`:
`cmd/assent/provider_host.go:84` and `:299` (`:276-283` is that function's doc comment, which
explains the gate; `:299` is the gate) — the fallback is gated on
`errors.Is(err, forge.ErrNotFound)` **alone**, so a 503, a throttle or a token scoped away
from the repo can no longer masquerade as an absent file. This story extends the same
discrimination one layer down, into the adapter that mints the sentinel.

### GitLab (verified against the tree)

| Status | Sentinel | Note |
| --- | --- | --- |
| 200 | content | `code`: `gitlab.go:484` (the 404 → `ErrNotFound` branch is `:486`) |
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
"the ref does not exist" is correctly not path absence. **Diagnostic caveat for S06:** that
case is *safe* but its operator-facing message would be wrong — an unresolvable ref is not a
permission failure, so the adapter must word the error as "content unreadable at ref X:
absent and forbidden could not be distinguished", never as a bare authorization message. The
sentinel choice is fail-closed either way; only the wording is at stake. (`unverified` — the exact Contents API
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
| 1 | S01 | the **twelve** case ids above appear in `catalog.yaml` with an adapter disposition (S01-03's strict-decode adapter list). **Now enforced, not merely requested**: this obligation is written into E10-S01's DoD in `spec.md`, and S07/S14's DoDs name the cases they must run — the `Verify:` lines otherwise depend on an implementer voluntarily reading a planning doc. `codeowners-eligible-set-matches-forge` is a **candidate**, not minted: it is OQ-34's promotion route |
| 2 | S02 | two call sites migrate, six do not; `forge.ErrUnauthorized` lifted to the port; **ADR-0021's two** stale anchors absorbed (`provider_host.go:275→:292`, `Describe→GetMR`). The third, `provider_host.go:246→:263`, lived in E10-S02's own DoD (`spec.md:257`) and is **already fixed** by this change |
| 2b | S02 | **resolve `FileAtBase(mr, path)`'s missing project binding before freezing the signature.** `FileAtRef` takes `project` as a parameter and today's client binds none, so ADR-0021's two-argument shape is under-specified: either the client binds the *target* project at construction (and the accessor is genuinely MR-relative) or `mr` becomes composite. Flagged here because a signature that freezes wrong is precisely what S00 exists to prevent; the choice is S02's, the ambiguity is not S02's to discover |
| 3 | S04 | the arming-relevant capability subset is stated explicitly; every `C` constant names its licensing case |
| 4 | S04 / S09 | retiring the `@` heuristic (and any other capability promoted into the arming set) is a **user-visible GitLab behaviour change** → its own `D-nnn` row + changelog entry, per epic judgment call (e). Not minted by S00, which changes no behaviour |
| 5 | operator | **OQ-33** — `protected-pipeline-source` predicate (three candidate routes, one of them §4's own largely-forge-readable same-repo route); until answered, v1 GitHub does not arm |
| 6 | operator / S08 | **OQ-34** — whether an adapter-computed CODEOWNERS eligible set can be `full`. Until answered, GitHub row 9 is `unknown` and **GitHub does not satisfy `require-review`**, the same standing as GitLab Free. With rows 9 and 11 both `unknown`, **v1 GitHub comments and does not gate** |

## Claims deliberately not resurrected

Both were found **False** by the 2026-08-10 adversarial review and are recorded here so a
later reader does not re-derive them:

- Extracting the conformance suite does **not** unblock `catalog.yaml`'s `github-deferred`
  rows — both are `level: L3, package: test/e2e`, gated on live GitHub infrastructure (S18).
- `capabilityGap` does **not** already model the absent-capability case generally; it models
  merge-result pinning, which is why it is singular and coupled to `mergeResultDigest`.
