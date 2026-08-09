# P5-AUD — Audit remediation 2026-08-06 (fail-closed enumeration, release-gate truth, post-release truth lag)

**Epic ID / REQ prefix:** `AUD` / `REQ-AUD-Snn-nn`. (Not `E10`–`E13` — those are meta-plan-reserved
and E10/E13 are Locked per D-012; the EFE/PCS precedent is a named, collision-free prefix for a
cross-cutting epic. `AUD` = the remediation epic for `agent-context/PROJECT-AUDIT-2026-08-06.md`.)

**Problem**: The 2026-08-06 six-lens audit at `e668d0e` (v0.1.0+7) verdicted **READY WITH
CONDITIONS** and found the series' first P1: **REL-07** — `mrChangedFiles`
(`internal/forge/gitlab/snapshot.go:99-137`) issues one unpaginated `GET
/merge_requests/:iid/changes`, never decodes `overflow`/`changes_count`, and maps 404 → `nil, nil`.
In `--checkout`-less runs that list is the SOLE `.assent/**` detector beyond the governed subject
(`cmd/assent/run.go:300-305` → `foldSnapshotPaths`), so a truncated enumeration can starve the
D-042 "MR cannot vouch for itself" guard — the exact fail-open class GUARD 1 exists to prevent.
Beyond it: the CHANGELOG drift gate is wired nowhere and regressed post-tag (RELSE-01); v0.1.0 was
published while verify on the tag SHA was red (RELSE-05 — structural hole, benign this time); the
DecisionRecord `toolDigest` is `sha256(version string)`, not the promised build digest (ARCH-03);
and the released front-of-house surfaces (CLI help "pre-alpha: no commands implemented yet",
README quick-start that exits 2, walkthrough "design fiction" banner, API_STABILITY "fileEvents not
yet implemented") describe the pre-release state (DOC-05/06/07/08/09/10/11). P3s: forge transport
unbounded reads / no retry / marker brittleness (REL-03/04/06), emit-after-merge ordering (REL-08),
coverage floor at exactly 90.0% with three untested fail-closed families (TEST-02/03/05/06), and
supply-chain pin gaps (SEC-01/03/04). Architecture debts: manual boundary enforcement (ARCH-01),
`gitlab.MRInfo`/`ErrNotFound` leaking through the read port (ARCH-02), dormant `assent-jcs-v1`
canonical hash vs the ad-hoc sha256 that carries the released contract (ARCH-04), stale C4 (ARCH-05).

**Key ground truth**: the fail-closed spine itself re-verified sound — this epic closes the gaps
AROUND it, it does not redesign it. **Release conditions for the next tag are S01+S02+S03 only**;
everything else is engineering health. The architect lane's decisions land in the SAME PR as this
spec: **ADR-0020** (Proposed — the REL-07 completeness contract) and decision rows
**D-119..D-123** — stories S01/S04/S16 bind to them directly, no pending references remain. (The
dormant P2-E6 Spike-D earmark that previously reserved the number 0020 for the Kubernetes-adapter
ADR has been repointed to a dedicated ADR whose number is assigned when Spike D closes.)

**Scope**: **Now** (release conditions): S01 REL-07 fail-closed enumeration; S02 RELSE-01 changelog
regenerate + gate wired; S03 RELSE-05 verify-green tag gate. **Next**: S04 ARCH-03 toolDigest
truth; S05 DOC-08 CLI help + reference; S06 docs truth-lag sweep (DOC-05/06/07/09/10/11); S07
ARCH-01 automated boundaries; S08 REL-08 emit ordering; S09 SEC-04 task pin. **Later**: S10
REL-03/SEC-08 bounded reads + page caps; S11 REL-04 retry/backoff/context; S12 REL-06 marker
resilience;
S13 TEST-02/05/06 depth bundle + coverage ≥91%; S14 SEC-01/SEC-03 supply-chain pins; S15 ARCH-02
port lift (pre-E10); S16 ARCH-04 replay-bundle hash wiring; S17 ARCH-05 C4 sync; S18 exit gate.

**Non-goals** (fenced): **SEC-05** `HOMEBREW_TAP_GITHUB_TOKEN` rotation to a fine-grained PAT,
**SEC-06** tag ruleset, **RELSE-07** `enforce_admins` — all three are OPERATOR-ONLY (live GitHub
settings / secrets), tracked in the backlog residual table, NOT stories here. **SEC-07** (in-place
asset replacement) is accepted; the patch-tags-not-replacement rule lands as one runbook line in
S06, no process machinery. **TEST-04** (`cmd/assent` outside the D-010 gate) stays compensated by
the binary dogfood gates — widening the gate denominator is a separate operator decision. **No
semantic change to any FROZEN schema** — S04 touches ONLY the `toolDigest` `description` string
(D-120 annotation-only edit; exact replacement text in the Appendix; `git diff schemas/` otherwise
0). **No GitHub forge adapter** (E10) — S15 only lifts the port so E10 becomes possible.
Worktree/branch pruning is coordinator hygiene, not a story.

**ADRs / reuse**: **ADR-0020 + D-119** (the S01 completeness contract), ADR-0015 §2/§8 + D-042
(the guard S01 protects), ADR-0017 §1 (pins/capability honesty — S04/S16; §9 domain separation —
S16), **D-120** (toolDigest — S04), **D-121** (canonical-hash split — S16), **D-122** (emit
ordering — S08), **D-123** (boundary enforcement — S07), ADR-0019 (reconcile protocol — S12 must
not weaken author-identity filtering), ADR-0005 (forge abstraction — S15), ADR-0011 (ports — S07
lands Amendment 3, text in the Appendix, in the same change as its enforcement), ADR-0016/0012
(presentation — untouched), D-010 (coverage floor — S13), D-016/E9-S03 (changelog gate — S02),
D-077 (checkout authority — S01 leaves checkout mode alone), D-113/D-114 (corpus immutability /
the digest claim S16 makes true). **Reuse, do not re-implement**: `internal/forge/gitlab` httptest
cassette pattern (E4-S02), `hack/release/*.sh` gate-script pattern, `TestCorePurity` scanner,
`internal/core/hash` (frozen `assent-jcs-v1` + vectors), `cliff.toml`/
`hack/release/verify-changelog.sh`.

**Executability**: **every story `[autonomous]`** — httptest fixtures, workflow/Taskfile edits
gated by actionlint + script tests, docs gated by `mkdocs build --strict` + link check, tests-only
lanes gated by the coverage number. No live GitLab/GitHub needed anywhere (S03 is asserted via a
dry workflow-logic test + actionlint; its live proof is the next tag itself).

**Judgment calls (decide-and-log unless marked 🔴)**:
(a) **S01 degrade shape** — DECIDED by ADR-0020 / D-119: unprovable completeness → typed gap →
opaque changeset → fail-safe REVIEW (`changeset.undecidable`, record + thread); diff-endpoint
404/non-200 → hard error, zero writes. The story invariant stands regardless of refactors:
incomplete or unavailable enumeration NEVER reaches APPROVE and NEVER performs forge writes.
(b) **S02 gate placement** — wire `changelog-verify` into BOTH `task check` and verify CI. If
per-commit regeneration churn proves noisy, relaxing to CI-only is a decide-and-log walk-back;
never to "wired nowhere".
(c) **S04 / S16 disposition** — DECIDED: D-120 binds S04 to the build-info digest (branch (i))
plus the description-only schema edit; D-121 binds S16 to the byte-vs-document split with exactly
`compare.ReplayBundleDigest` switching to the domain-separated digest. The acceptance criterion
survives the decision: the released contract statement and the implementation AGREE.
(d) **S12 marker skip semantics** — a malformed marker on a BOT-authored note is skipped WITH a
warning (reconcile proceeds; worst case duplicate slot post, repaired by the existing
duplicate-repair path) — availability fix only; author-identity filtering (ADR-0019) is untouched,
so no spoof surface opens. Contributor-note markers stay invisible regardless of well-formedness.
This entry IS the log (pre-logged via this spec — no separate D-row needed unless the implementer
disagrees at build time; in that case open a D-row before landing).
(e) **S13 coverage target** — operator asked to "bump coverage a bit": acceptance = aggregate
`./internal/...` ≥ **91.0%** measured. The D-010 GATE floor stays 90% unless the operator
separately raises it (log if raised).

**Lane plan (implementation lanes own DISJOINT paths; sequence WITHIN a lane, not across)**:
- **Lane A — forge/transport**: S01 → S10 → S11 → S12 → S15(+cmd swap, last). Owns
  `internal/forge/**`, `internal/provider/transport.go`. Disjointness caveat (explicit):
  S01's fold wiring is a PRODUCTION edit inside Lane C's tree — the checkout-less snapshot
  fold in `cmd/assent` (`run.go` fold + `checkout.go`) — plus the
  `cmd/assent/run_changedset_test.go` test file. Safe by wave ordering (S01 is wave-1;
  Lane C's S04/S08/S16 are wave-2+), but Lane C must rebase on S01 before starting, and
  neither lane touches those files concurrently.
- **Lane B — CI/release plumbing**: S02 → S03 → S09 → S14. Owns `Taskfile.yml`, `CHANGELOG.md`,
  `.github/workflows/{verify,release,schemas}.yaml`, `hack/release/**`.
- **Lane C — cmd run-path/record**: S04 → S08 → S16. Owns `cmd/assent/run.go` (+ the S04
  `schemas/decision/**` description-only edit per D-120). S15's `run.go` port swap lands AFTER
  Lane C drains.
- **Lane D — docs/CLI truth**: S05 → S06 → S17. Owns `cmd/assent/main.go`, `README.md`,
  `API_STABILITY.md`, `docs/**`, `mkdocs.yml`, comment-only lines in `internal/core/policy/policy.go`.
- **Lane E — guardrails/tests**: S07 → S13. Owns `.golangci.yml`, `internal/core/purity_test.go`,
  new `*_test.go` files only (coordinate S13's forge test files after Lane A lands S10–S12).
- **S18 exit gate** — after all lanes drain.

**Dependency order**: **S01** (the P1 — release condition, engine-grade; anchor) ∥ S02 ∥ S03 (three
independent lanes, all release conditions) → next wave S04, S05, S06, S07, S08, S09 (all ∥ across
lanes) → later wave S10 → S11 → S12 (Lane A serial), S13 (after Lane A), S14, S15 (last code
story), S16, S17 → **S18** exit gate. **Do first: S01** — the only P1, a small bounded diff, and
the next tag is conditioned on it. **Release conditions: S01, S02, S03. Engine-grade review: S01,
S08, S12, S15, S16. Semver/release-visible: S02, S03, S04, S05, S16.**

---

## AUD-S01 — ⚠️ REL-07: fail-closed truncated/404 changed-file enumeration (the P1) [autonomous · engine-grade review]

> **⚠️ Decision-path adjacent: `snapshot.ChangedFiles` is the sole `.assent/**` detector in
> checkout-less runs (`run.go:300-305`); an incomplete list can starve GUARD 1/D-042 and approve+
> CAS-merge a self-rewriting MR. Fail-safety review required. Mechanism per ADR-0020 / D-119
> (landed in the same PR as this spec). Touches the trust boundary — maintainer LGTM required.
> The invariant under any mechanism detail: incomplete or unavailable enumeration NEVER reaches
> APPROVE and NEVER performs forge writes.**

**As a** repo operator running assent without `--checkout` **I want** an incomplete or unavailable
MR changed-file enumeration to degrade the run fail-closed **so that** a contributor can never pad
an MR past the instance diff limit (or exploit a 404) to hide a `.assent/**` policy edit from the
self-vouch guard.

**Goal**: implement the ADR-0020 contract (normative there, points 1–6):
(1) **Neutral type** — `forge.Snapshot` gains `ChangedFilesComplete bool` + `ChangedFilesGap
string` (gap non-empty iff incomplete; value-typed so the zero value `false` fails SAFE — an
adapter that forgets to set it degrades to REVIEW, never fail-open); every adapter and fake sets
them explicitly. (2) **GitLab mechanics** — `mrChangedFiles` migrates off the deprecated
unpaginated `/changes` to paginated `GET /merge_requests/:iid/diffs?per_page=100&page=N` with a
hard ceiling of 100 pages (10,000 files; named constant). Completeness requires ALL of: the
enumeration terminated below the ceiling; the MR's `changes_count` (a string on the MR GET) parses
as a plain integer with no `+` suffix; and that integer equals the number of enumerated diff
entries. Any violation (ceiling hit, `+` suffix, count mismatch in either direction, or a decoded
per-entry `overflow`-class marker) → `ChangedFilesComplete=false` with a specific
`ChangedFilesGap` reason — never a silently short list. (3) **Hard errors** — HTTP 404 or any
non-200 on the diffs endpoint is a hard error (the 404→empty-list mapping is removed): the MR
provably exists by then, so a missing diff resource is forge anomaly, never an empty change set.
(4) **Run semantics** (`--checkout` unset) — incomplete → the snapshot fold sets
`changeSet.Opaque=true` with `OpaqueReason = "forge changed-file enumeration incomplete: " +
ChangedFilesGap`; the existing `decide` short-circuit yields REVIEW with finding code
`changeset.undecidable` (no new finding code). The `.assent/**` dominance check over whatever
paths WERE returned still applies — a visible `.assent/**` path dominates to BLOCK. (5)
`--checkout` mode untouched (D-077 local-tree authority). (6) The fake forge gains truncation and
diff-endpoint-404/5xx knobs; the conformance suite gains the three required cases below.

**Operator input**: no (ADR-0020 / D-119 decide the mechanism; cite them in the implementation
commit).

**Dependencies**: none (ADR-0020 + D-119 land in the same PR as this spec). Lane coordination:
besides the adapter work, points (1)/(4) are production edits OUTSIDE Lane A's owned paths — the
`forge.Snapshot` widening and the checkout-less fold wiring in `cmd/assent` (`run.go` fold +
`checkout.go`) sit in Lane C's tree. Wave ordering protects this (S01 is wave-1, Lane C starts
wave-2 rebased on S01); state it in the lane handoff, same as the `run_changedset_test.go` note
on REQ-AUD-S01-04.

**Acceptance criteria (G-W-T)**:
- Given a fake/httptest forge returning a paginated diff list that terminates below the ceiling
  with `changes_count` equal to the enumerated count, when `assent run` executes checkout-less,
  then behavior is byte-identical to today (regression guard on the happy path).
- Given `changes_count = "1000+"` (or count ≠ enumerated in either direction, or the page ceiling
  hit), when the run executes checkout-less, then the decision is REVIEW with finding code
  `changeset.undecidable`, `OpaqueReason` prefixed `forge changed-file enumeration incomplete:`, a
  schema-valid record is emitted, exactly one thread and ZERO approve/merge operations are
  written, and the exit code is 0.
- Given a truncated list that still contains a `.assent/**` path, when the run executes, then the
  decision is BLOCK with zero forge writes (GUARD-1 dominance over the gap-degrade).
- Given the diffs endpoint returns 404 (or 5xx), when Snapshot builds, then a hard error
  propagates (no empty-set fallback), the exit code is non-zero, zero forge writes occur, and no
  record file is created.
- Given `--checkout` is set, when the run evaluates, then snapshot completeness fields are ignored
  for classification (assert with a truncated snapshot + clean checkout → NOT REVIEW-degraded;
  D-077 unchanged).
- Fake forge exposes truncation + 404 knobs; the conformance suite contains the three cases above
  (truncated → REVIEW; truncated-with-visible-`.assent/**` → BLOCK; 404 → hard error) as REQUIRED
  cases, not optional; the adapter has httptest coverage for pagination, ceiling, `changes_count`
  mismatch, `+`-suffix, and 404. Both polarities per repo test discipline.

**Definition of done**: contract fields + adapter migration + fold wiring live; httptest cassettes
for all polarities above; the run-level never-APPROVE/never-write invariant pinned; conformance
cases required; `task check` + determinism gate green; maintainer LGTM (trust boundary).

**Not in scope**: retry/backoff (S11); read bounds beyond the diff pagination ceiling (S10); the
port lift (S15).

Requirements:
- **REQ-AUD-S01-01** *(fail-closed · truncation)* — pagination ceiling hit or a decoded per-entry
  overflow-class marker → `ChangedFilesComplete=false` + specific `ChangedFilesGap` (ADR-0020
  point 2); never a silently short list. Test: `internal/forge/gitlab/snapshot_test.go`; Verify:
  `go test ./internal/forge/gitlab/... -run TestChangedFilesOverflowFailsClosed`; Level: L0
- **REQ-AUD-S01-02** *(fail-closed · count cross-check)* — `changes_count` mismatch (either
  direction) or a `+`-suffixed count → the same incomplete degrade. Test:
  `internal/forge/gitlab/snapshot_test.go`; Verify:
  `go test ./internal/forge/gitlab/... -run TestChangedFilesCountMismatchFailsClosed`; Level: L0
- **REQ-AUD-S01-03** *(hard error · 404/non-200)* — diffs-endpoint 404 or any non-200 → hard error
  (the 404→empty mapping is removed); the run does not proceed to a decision. Test:
  `internal/forge/gitlab/snapshot_test.go`; Verify:
  `go test ./internal/forge/gitlab/... -run TestChangedFiles404IsError`; Level: L0
- **REQ-AUD-S01-04** *(run invariant)* — a checkout-less run over an incomplete enumeration hiding
  a `.assent/**` edit yields REVIEW `changeset.undecidable` (schema-valid record, exactly one
  thread, zero approve/merge, exit 0) — never APPROVE; a partial list with a visible `.assent/**`
  path BLOCKs with zero writes; a complete cassette reproduces today's decision byte-identically;
  `--checkout` is unaffected. Test: `cmd/assent/run_changedset_test.go` (Lane A owns this file for
  this REQ; coordinate with Lane C) + `internal/forge/conformance` required cases; Verify:
  `go test ./cmd/assent/... -run TestRunEnumerationIncompleteNeverApproves`; Level: L1

## AUD-S02 — RELSE-01: regenerate CHANGELOG + wire `changelog-verify` into `task check` and verify CI [autonomous · release-sensitive]

**As a** release consumer **I want** `CHANGELOG.md` to carry a real `[0.1.0]` section and the drift
gate to fire on every commit and CI run **so that** the changelog can never again silently regress
after a tag (it regressed once already — fixed at `49ba1ad`, drifted at HEAD).

**Goal**: (1) `task changelog-write` + commit — `## [0.1.0] - 2026-08-05` section present, the 7+
post-tag commits under `## Unreleased`; (2) add `changelog-verify` to the `check` task list in
`Taskfile.yml` (after `compare-exitgate-test`); (3) add a `changelog-verify` step to the `verify`
job in `.github/workflows/verify.yaml` (uses the pinned `tools:git-cliff`, `GIT_CLIFF_VERSION`
v2.13.1 — no new mutable install). Judgment call (b): both placements; CI-only walk-back is
decide-and-log.

**Operator input**: no.

**Dependencies**: none. **Semver visibility**: release surface — the CHANGELOG is a published
contract artifact; flag in the commit body.

**Acceptance criteria (G-W-T)**:
- Given HEAD after this story, when `task changelog-verify` runs fresh, then it exits 0 and
  `CHANGELOG.md` contains a `## [0.1.0] - 2026-08-05` section.
- Given a subsequent commit that does NOT regenerate the changelog, when `task check` (or the
  verify workflow) runs, then it FAILS with the drift diff (gate proven in the failing direction —
  demonstrate once in a scratch worktree, pin via the gate-script test).
- Given `cliff.toml` output identical to `CHANGELOG.md`, when the gate runs, then it passes with no
  network access beyond the pinned git-cliff download.

**Definition of done**: `[0.1.0]` section committed; `check` includes `changelog-verify`;
verify.yaml step added; both-polarity proof recorded.

**Not in scope**: release.yaml (S03); raising the coverage floor; changelog content policy changes.

Requirements:
- **REQ-AUD-S02-01** — `CHANGELOG.md` regenerated: `[0.1.0]` section present, post-tag commits under Unreleased; `task changelog-verify` green. Test: `hack/release/verify-changelog.sh` (existing); Verify: `task changelog-verify`; Level: L1
- **REQ-AUD-S02-02** *(gate wiring · both polarities)* — `task check` and the verify workflow each run `changelog-verify`; an injected stale changelog fails them. Test: `hack/release/changelog_gate_test.sh` (new — asserts wiring presence + failing polarity in a temp copy); Verify: `bash hack/release/changelog_gate_test.sh`; Level: L1

## AUD-S03 — RELSE-05: release job asserts verify-green on the tag SHA before building [autonomous · release-sensitive]

**As a** maintainer **I want** the tag-triggered release job to refuse to build unless the `verify`
check suite concluded SUCCESS on the exact tag SHA **so that** a future tag on a broken commit can
never ship signed, attested artifacts (v0.1.0 in fact published while `release-exitgate` on
`49ba1ad` was red).

**Goal**: first step of the `release` job in `.github/workflows/release.yaml` (before setup-go /
goreleaser): resolve the tag's commit SHA (both `push` and `workflow_dispatch` paths), query the
GitHub checks API (`gh api` with the job's `GITHUB_TOKEN`, read scope already present) for the
`verify` workflow's conclusion on that SHA, and `exit 1` unless it is `success`. `pending`/missing
verify → fail with a "re-run when verify is green" message (no wait-loop — re-dispatch is cheap and
a wait hides red).

**Operator input**: no.

**Dependencies**: none (S02's verify step naturally becomes part of what this gate asserts).
**Semver visibility**: release pipeline — flag in commit body.

**Acceptance criteria (G-W-T)**:
- Given a tag whose SHA has verify concluded `success`, when the release job runs, then the gate
  step passes and the build proceeds.
- Given a tag whose SHA has verify `failure`, missing, or still `pending`/`in_progress`, when the
  release job runs, then the job fails AT the gate step — before goreleaser, cosign, or any publish
  step executes.
- Given `workflow_dispatch` with an existing tag input, when the job runs, then the SAME gate
  applies to the dispatched tag's SHA (the rebuild path is not a bypass).

**Definition of done**: gate step present in both event paths; actionlint green; gate logic
extracted to `hack/release/verify-tag-gate.sh` so the polarity table is testable without a live
run.

**Not in scope**: making `release-exitgate` a required PR check (RELSE-08 — operator residual,
backlog row AUD-RELSE-08); branch/tag protection (operator, non-goals).

Requirements:
- **REQ-AUD-S03-01** — the release job's first step fails unless verify concluded success on the tag SHA; covers push + dispatch paths. Test: `hack/release/verify_tag_gate_test.sh` (new — table over success/failure/pending/missing via a stubbed `gh`); Verify: `bash hack/release/verify_tag_gate_test.sh`; Level: L1
- **REQ-AUD-S03-02** — actionlint + workflow ordering: the gate step precedes every build/sign/publish step in the job DAG. Test: `hack/release/verify_tag_gate_test.sh` (step-order assertion over the YAML); Verify: `bash hack/release/verify_tag_gate_test.sh`; Level: L1

## AUD-S04 — ARCH-03: `toolDigest` derives from Go build info (D-120) [autonomous · release-sensitive]

**As a** DecisionRecord consumer replaying a decision (OQ-9) **I want** `pins.toolDigest` to be
what the schema says it is **so that** two different-content builds can never share a digest that
claims to identify the evaluating build.

**Goal**: today `run.go:345` pins `sha256(version string)` (dev default `"0.0.0-dev"`) while
`schemas/decision/v1alpha1/decision-record.schema.json:71-75` promises a "content digest of the
evaluating tool build". Per **D-120**: `pins.toolDigest` becomes `sha256:` over the canonical
`debug.ReadBuildInfo` text (module path/version/sum, dependency sums, `vcs.revision`,
`vcs.modified`) — a deterministic build-content proxy: same-version different-content builds now
differ; constant within one binary (determinism gate safe); works for `go install`, source, and
goreleaser builds including `0.0.0-dev`. Fallback when build info is unavailable:
`sha256("buildinfo-unavailable\n" + version)` — honestly labeled, never a fabricated content
claim. Known honest residual (per D-120): dirty-tree edit content is not captured (flagged via the
hashed `vcs.modified=true`). Companion: the schema `description` for
`$defs.pins.properties.toolDigest` is replaced with the D-120 wording — annotation-only, the SOLE
permitted `schemas/` diff in this epic; the exact replacement text lives in this spec's Appendix
and lands as its own reviewed commit citing D-120 (it trips the `git diff schemas/` exit-gate
guard by design).

**Operator input**: no (D-120 binds; cite it in the commits).

**Dependencies**: D-120 (landed in the same PR as this spec). Lane C (owns `cmd/assent/run.go`).
**Semver visibility**: DecisionRecord pins contract — flag in commit body; note the digest-value
change for record consumers in the CHANGELOG (pre-D-120 records carry sha256(toolVersion) —
identifiable by matching that digest).

**Acceptance criteria (G-W-T)**:
- Given two binaries built from different source claiming the same version string, when each emits
  a record, then their `toolDigest` values differ; given the same binary run twice, then the
  digests are identical (determinism gate at `-count=2` stays green byte-identically).
- Given any emitted digest, when checked, then it matches `^sha256:[0-9a-f]{64}$`; given a build
  with no build info (test binary), then the fallback is exercised by a unit test and equals
  `sha256("buildinfo-unavailable\n" + version)` (schema `minLength: 1` never violated).
- Given the schema edit, when `git diff` runs on the schema file, then it touches NO validation
  keywords (description string only) and existing v0.1.0-era record fixtures still validate.
- Given the D-016 golden replay / byte-exact fixtures that embed `toolDigest`, when the suite
  runs, then they are regenerated in the same commit and the determinism suite passes.

**Definition of done**: build-info digest live per D-120; fallback unit-tested; records
schema-valid; D-016 reproduction green with regenerated goldens; schema description commit
separate + cited.

**Not in scope**: `policySha` mechanics (S16); any other schema field; any validation-keyword
change.

Requirements:
- **REQ-AUD-S04-01** — `toolDigest` = sha256 over canonical build-info text per D-120: different-content same-version builds differ; format `^sha256:[0-9a-f]{64}$`; constant within one binary. Test: `cmd/assent/tooldigest_test.go` (new); Verify: `go test ./cmd/assent/... -run TestToolDigestContract`; Level: L0
- **REQ-AUD-S04-02** *(edge)* — missing/partial build info → the labeled fallback digest (formula pinned by test); record still schema-valid; D-016 replay + regenerated goldens green; `git diff schemas/` shows only the D-120 description string. Test: `cmd/assent/tooldigest_test.go`; Verify: `go test ./cmd/assent/... -run 'TestToolDigestFallback|TestD016Reproduction'`; Level: L1

## AUD-S05 — DOC-08: real CLI help + `docs/usage/cli.md` reference [autonomous · CLI-surface visible]

**As a** new user who just `brew install`ed v0.1.0 **I want** `assent` / `assent --help` to list
the real subcommands instead of "pre-alpha: no commands implemented yet" **so that** the shipped
binary's front door matches the product.

**Goal**: replace the `cmd/assent/main.go:111` stub with a generated usage listing of every
dispatched subcommand (run, doctor, lint, test, compare, render, catalogue, version, …) + one-line
synopses; `--help`/`help` exits 0, bare invocation prints usage and keeps exit 2, unknown
subcommand prints usage + exit 2. Add `docs/usage/cli.md` (short command reference, one section per
subcommand, flags tables for run/test/compare) and wire it into `mkdocs.yml` nav.

**Operator input**: no. **Semver visibility**: CLI surface (help text + help exit code) — flag in
commit body.

**Dependencies**: none. Lane D (owns `cmd/assent/main.go` — disjoint from Lane C's `run.go`).

**Acceptance criteria (G-W-T)**:
- Given the built binary, when `assent --help` runs, then every dispatched subcommand appears with
  a synopsis and the exit code is 0; the string "no commands implemented yet" appears nowhere in
  the binary or repo (grep-pinned).
- Given a bare `assent` or an unknown subcommand, when invoked, then usage prints to stderr and the
  exit code is 2 (scripts relying on the current bare-invocation contract don't break).
- Given `docs/usage/cli.md`, when `task docs-build` (strict) runs, then it is green and the page is
  in nav; every subcommand named in help has a section (drift-pinned by test).

**Definition of done**: help output test-pinned against the dispatch table (a subcommand added
without help coverage fails the test); docs page live in nav.

**Not in scope**: a flag-parsing framework migration; README/walkthrough (S06).

Requirements:
- **REQ-AUD-S05-01** — help lists exactly the dispatched subcommand set (derived from one shared table — no dual maintenance); `--help` → 0, bare/unknown → 2. Test: `cmd/assent/main_help_test.go` (new); Verify: `go test ./cmd/assent/... -run TestHelpListsAllSubcommands`; Level: L0
- **REQ-AUD-S05-02** — `docs/usage/cli.md` exists, is in `mkdocs.yml` nav, covers every subcommand in the help table; strict docs build green. Test: `cmd/assent/main_help_test.go` (doc-drift assertion) + `task docs-build`; Verify: `go test ./cmd/assent/... -run TestCLIDocCoversSubcommands && task docs-build`; Level: L1

## AUD-S06 — Docs truth-lag sweep: DOC-05/06/07/09/10/11 (+ stale code comments) [autonomous]

**As a** prospective adopter reading the published docs **I want** every front-of-house surface to
describe the SHIPPED product **so that** the first-touch experience (quick-start, walkthrough,
stability contract) doesn't fail or lie in either direction.

**Goal**: one docs lane, six fixes: (1) **DOC-07** `README.md:81-82` — `assent lint .assent/` →
`assent lint .` / `assent test .` (the CLI joins `.assent` itself, `lint.go:70-77`); commands
smoke-verified against the built binary. (2) **DOC-06** `API_STABILITY.md:56-57` (+ published
mirror) — replace "fileEvents … not yet implemented" with the shipped EFE add/delete status
(kinds ⊆ {add,delete}, modify/rename load-rejected); fix the two stale comments at
`internal/core/policy/policy.go:100-102,121` (comment-only — coordinate the file with Lane A/E:
no code lines). (3) **DOC-09** `docs/usage/walkthrough.md:3-4` — replace the "design fiction"
banner with per-section **Shipped / Planned** banners (run/test/doctor/lint/compare = shipped;
init/scan/stats/explain = planned/nonexistent). (4) **DOC-05** `README.md:48` — repoint the
nonexistent `docs/adr/0014-policy-test-harness.md` link to
`docs/adr/0014-adopter-test-format.md`. (5) **DOC-10** `docs/planning/meta-plan.md` — epic table
renumbered to the current E-numbering (match the README maturity table). (6) **DOC-11**
`docs/usage/install.md` + README — `go install` yields `0.0.0-dev` (no ldflags): add the caveat,
point version-stamped users at brew/release archives. Plus one runbook line (SEC-07): prefer patch
tags over in-place asset replacement.

**Operator input**: no.

**Dependencies**: S05 (help text referenced by walkthrough banners); EFE spec (for the DOC-06
wording). Lane D.

**Acceptance criteria (G-W-T)**:
- Given a fresh clone with the built binary, when every README quick-start command block is
  executed verbatim, then each exits 0 (pinned by a smoke test, not by eyeball).
- Given API_STABILITY.md, when grepped for "not yet implemented" in the fileEvents note, then no
  stale claim remains and the stated kinds match `loader.go` behavior.
- Given the walkthrough, when built, then no "design fiction"/"Nothing below is implemented"
  banner remains and every section carries a Shipped or Planned banner that matches the S05
  dispatch table.
- Given `mkdocs build --strict` + the README link, when checked, then zero broken links (DOC-05
  fixed) and the meta-plan epic table matches the README maturity table's numbering.

**Definition of done**: all six findings closed; smoke test green; strict docs build green.

**Not in scope**: C4 diagrams (S17); CLI reference (S05); local gitignored AGENTS.md (DOC-03,
operator-local).

Requirements:
- **REQ-AUD-S06-01** *(DOC-07 · executable truth)* — every README quick-start command runs green against the built binary. Test: `hack/docs/readme_smoke_test.sh` (new); Verify: `bash hack/docs/readme_smoke_test.sh`; Level: L1
- **REQ-AUD-S06-02** *(DOC-05/06/09/10/11)* — stale banners/claims/links/numbering gone; strict docs build + link integrity green; policy.go comment fixes are comment-only (no code diff). Test: `task docs-build` + `hack/docs/truthlag_pins_test.sh` (grep pins for the retired phrases); Verify: `task docs-build && bash hack/docs/truthlag_pins_test.sh`; Level: L1

## AUD-S07 — ARCH-01: automated boundary enforcement per D-123 (depguard + purity-walk extension) [autonomous]

**As a** maintainer **I want** the architecture boundaries enforced by CI, not by discipline **so
that** a violating import fails a machine gate the moment it appears (3rd audit finding this;
boundaries are factually clean today — the CONTROL is what's missing).

**Goal**: per **D-123**, two layers: (1) `.golangci.yml` `depguard` deny-rules — the guarded tree
(`internal/core/**`, `internal/change/**`, `internal/glob`, `internal/lint`,
`internal/catalogue`, `internal/evaldecode`, `internal/compare`, `schemas/**`) may import none of
`internal/forge/**`, `internal/render/**`, `cmd/**`, `net/**` (package-level; clock/env/rand
axes stay CALL-level in the walk — depguard cannot distinguish `time.Now` from `time.Time`).
(2) Extend the `TestCorePurity` walk (`internal/core/purity_test.go:157`) to `../evaldecode`,
`../compare`, AND `../../schemas` (call-level: `time.Now`/`os.Getenv`/`os.Environ`/rand/net;
adversarial self-test retained). Scope note (recorded in D-123): this deliberately EXTENDS the
AGENTS.md rule-7 pure tree — `internal/evaldecode` (engine input decode) and `internal/compare`
(D-116/D-117 gate determinism) join the determinism guard; `schemas` is embedded compile-time
authority. (3) **ADR-0011 Amendment 3** (verbatim text in this spec's Appendix) lands in the SAME
change as the enforcement — no window where the "arch-lint enforced" claim is false again; the
commit cites D-123.

**Operator input**: no.

**Dependencies**: D-123 (landed in the same PR as this spec). Lane E (owns `.golangci.yml`,
`purity_test.go`).

**Acceptance criteria (G-W-T)**:
- Given a synthetic violating import (e.g. a guarded-tree package importing `internal/forge` or
  `net/http`), when `golangci-lint run` executes, then it FAILS naming the depguard rule
  (adversarial polarity proven the same way `TestCorePurityCatchesImpurity` proves the walker).
- Given the clean tree at HEAD, when lint + the extended purity walk run, then both are green (no
  false positives to burn trust; boundaries were verified factually clean at audit).
- Given `internal/evaldecode`, `internal/compare`, or `schemas` growing a `time.Now` /
  `os.Getenv`, when tests run, then the purity walk fails with a located violation.
- Given the change that lands the enforcement, when reviewed, then ADR-0011 Amendment 3 is IN the
  same change (no separate follow-up), and `task check` runs both layers (verify already runs
  golangci-lint — no new CI step).

**Definition of done**: depguard rules live in the existing lint gate; walk extended to all three
directories; Amendment 3 landed with the enforcement; ADR-0011 claim true; D-123 cited.

**Not in scope**: importas cosmetics; widening the D-010 denominator; the pre-E10 tripwire rule
denying `internal/forge/gitlab` from a neutral orchestration package (deferred to E10 per the
architect's ARCH-02 note — do not add before the lift starts).

Requirements:
- **REQ-AUD-S07-01** *(both polarities)* — D-123 depguard deny-rules active over the full guarded tree; clean tree green; synthetic violation fails lint (proven via a golangci config test fixture or a temp-file adversarial run documented in the test). Test: `hack/lint/depguard_test.sh` (new); Verify: `bash hack/lint/depguard_test.sh`; Level: L1
- **REQ-AUD-S07-02** — purity walk covers `../evaldecode`, `../compare`, and `../../schemas`; adversarial subcase still catches injected impurity in the extended directories. Test: `internal/core/purity_test.go`; Verify: `go test ./internal/core/... -run 'TestCorePurity|TestCorePurityCatchesImpurity'`; Level: L0

## AUD-S08 — ⚠️ REL-08: emit the DecisionRecord before forge reconcile (D-122) [autonomous · engine-grade review]

> **⚠️ Ordering change in the run tail (`run.go:386` reconcile vs `:393` emit) — audit-trail
> integrity: today an emit failure AFTER a merge loses the record of a merge that already
> happened. D-122 decides the mechanism.**

**As a** compliance operator **I want** the DecisionRecord persisted before any forge write **so
that** every merge that occurs has its record on disk even if emission would have failed.

**Goal**: per **D-122** — new invariant: NO forge write without a schema-valid, durably-emitted
DecisionRecord. Order in `orchestrate`: build → marshal → schema-validate (stays where it is at
`run.go:362`) → **EMIT** (`--emit`: atomic write-then-rename — write `<path>.tmp` in the SAME
directory, then `os.Rename`; stdout emission unchanged) → `forge.Reconcile` → summary. An emit
failure aborts the run with a hard error and ZERO forge writes (fail-closed: no record ⇒ no
action). Pure reordering: `recordJSON` is fully determined pre-Reconcile (the receipt lives in the
summary line, not the record) — byte-identical records; marker digests and the determinism gate
are unaffected. Rejected by D-122: post-reconcile record stamping (breaks record byte-stability vs
the marker `decision` digest).

**Operator input**: no.

**Dependencies**: D-122 (landed in the same PR as this spec). Lane C, after S04 (both own
`run.go`).

**Acceptance criteria (G-W-T)**:
- Given `--emit` pointing at an unwritable path, when the decision is APPROVE with arming
  satisfied, then the run exits non-zero and the fake forge records ZERO operations (the new
  invariant, tested at the approve polarity where it bites — the merge cannot outrun its own
  record).
- Given emit succeeds and Reconcile then hard-fails, when the run ends, then the record file
  exists and is schema-valid (durability polarity).
- Given a healthy run reaching APPROVE+merge, when it executes, then the emitted bytes are
  byte-identical to pre-change goldens, stdout ordering (record, then summary) is unchanged, and
  the summary still reports the receipt.
- Given a fail-closed reconcile refusal (arming unmet, SHA moved), when the run executes, then the
  record is already emitted and exit remains 0 (existing contract preserved).
- Given any exit path, when the run ends, then `<path>.tmp` does not survive; the rename-based
  write is same-directory (assert temp path parent == target parent).

**Definition of done**: ordering swapped per D-122 with the atomic write; all polarities pinned;
determinism + D-016 replay green; D-122 cited in the commit.

**Not in scope**: record content; summary format; retry of failed emits.

Requirements:
- **REQ-AUD-S08-01** *(fail-closed)* — emit failure → zero forge writes, non-zero exit (approve polarity); `<path>.tmp` never survives; rename is same-directory. Test: `cmd/assent/run_emit_order_test.go` (new, fake forge write-counter); Verify: `go test ./cmd/assent/... -run TestEmitFailureBlocksForgeWrites`; Level: L1
- **REQ-AUD-S08-02** — happy path byte-identical record + receipt summary; reconcile-hard-fail leaves a schema-valid record on disk; fail-closed-refusal contract unchanged. Test: `cmd/assent/run_emit_order_test.go`; Verify: `go test ./cmd/assent/... -run TestEmitBeforeReconcileByteStable`; Level: L1

## AUD-S09 — SEC-04: pin the Task version in verify.yaml [autonomous]

**As a** maintainer **I want** the two `go install …/task@latest` steps (`verify.yaml:67,89`)
pinned to an exact version **so that** a mutable upstream release can't change gate behavior (or
supply-chain posture) under CI silently.

**Goal**: pin `task@vX.Y.Z` (current stable at authoring) in both jobs; single source the version
via a workflow-level `env` so the two jobs can't skew.

**Operator input**: no. **Dependencies**: none. Lane B (after S02 — same file).

**Acceptance criteria (G-W-T)**:
- Given verify.yaml, when grepped for `task/v3/cmd/task@`, then every hit carries the pinned
  version and zero hits carry `@latest`.
- Given the pinned version, when both CI jobs run, then all existing gates pass unchanged.

**Definition of done**: no `@latest` in any workflow; actionlint green.

Requirements:
- **REQ-AUD-S09-01** — zero `@latest` installs across `.github/workflows/**`; task pinned + deduplicated via env. Test: `hack/lint/workflow_pins_test.sh` (new grep gate — also serves S14); Verify: `bash hack/lint/workflow_pins_test.sh`; Level: L1

## AUD-S10 — REL-03/SEC-08: bounded response reads + pagination caps [autonomous]

**As a** repo operator **I want** every forge/provider HTTP response read bounded and every
pagination loop capped **so that** a hostile or broken endpoint can't OOM the run or spin it
unbounded (availability hardening; decision impact already capped by differ ceilings). Findings
closed: **REL-03** and its security alias **SEC-08** (the audit lists the unbounded reads under
both IDs — the S18 disposition gate maps both to this story).

**Goal**: (1) `io.LimitReader` (with an over-limit = error, not silent truncation — truncated
bytes must never be parsed as complete) at `internal/forge/gitlab/gitlab.go:108` (`c.do`) and
`internal/provider/transport.go:107`; limits sized generously above legitimate payloads (MB-order,
constant, documented). (2) Cap the `listDiscussions`/`listNotes` page loops
(`gitlab.go:222-269, 288-327`) at a documented max-pages constant; hitting the cap is an ERROR
(fail-closed — an incomplete thread list must not drive reconcile), never a silent partial. (The
S01 diff-pagination ceiling is separate and ADR-0020-owned; this story must not alter its REVIEW
semantics.)

**Operator input**: no. **Dependencies**: S01 (same file, Lane A serial).

**Acceptance criteria (G-W-T)**:
- Given a response body exceeding the limit, when read, then an error naming the limit propagates —
  no truncated parse, no OOM (httptest with an oversized body).
- Given a paginator that never shortens its pages, when listing, then the loop errors at the cap
  instead of spinning; reconcile does not proceed on the partial list.
- Given all existing cassettes, when the suite runs, then green (limits invisible to legitimate
  traffic).

**Definition of done**: both read sites bounded; both loops capped; existing conformance suite
green.

**Not in scope**: retry/backoff (S11); marker semantics (S12); the S01 diff-enumeration ceiling
(ADR-0020-owned).

Requirements:
- **REQ-AUD-S10-01** *(fail-closed)* — over-limit body → error at both read sites; legitimate payloads unaffected. Test: `internal/forge/gitlab/gitlab_limits_test.go` + `internal/provider/transport_test.go`; Verify: `go test ./internal/forge/gitlab/... ./internal/provider/... -run 'TestBoundedRead'`; Level: L0
- **REQ-AUD-S10-02** *(fail-closed)* — page-cap hit → error, reconcile refuses; normal pagination unchanged. Test: `internal/forge/gitlab/gitlab_limits_test.go`; Verify: `go test ./internal/forge/gitlab/... -run TestPaginationCapFailsClosed`; Level: L0

## AUD-S11 — REL-04: retry/backoff + context deadlines on the GitLab client [autonomous]

**As a** repo operator on a flaky network **I want** idempotent GitLab reads retried with bounded
backoff under a context deadline **so that** one transient 5xx/timeout doesn't fail a run that
would have decided correctly — WITHOUT retrying non-idempotent writes into duplication.

**Goal**: in `internal/forge/gitlab` (`c.do` seam): (1) context plumbed through requests with a
per-request timeout; (2) bounded retry (e.g. 3 attempts, jittered exponential backoff, retry only
on transport errors/429/5xx) for **idempotent GET/HEAD only**; (3) writes (POST/PUT — approve,
merge, notes) are NEVER auto-retried (CAS/idempotence discipline stays the caller's, ADR-0019);
(4) deterministic in tests (injected sleeper — no wall-clock in assertions).

**Operator input**: no. **Dependencies**: S10 (same file). Lane A.

**Acceptance criteria (G-W-T)**:
- Given a GET that 503s twice then 200s, when called, then the call succeeds after backoff and the
  attempt count is pinned.
- Given a GET failing beyond the retry budget or exceeding the context deadline, when called, then
  a hard error propagates (run fails closed as today — retries change availability, never
  decisions).
- Given any POST/PUT failing transiently, when called, then exactly ONE attempt was made (write
  non-retry pinned both by test and by a table over the method map).

**Definition of done**: retries live behind the single `do` seam; fake-sleeper determinism;
conformance suite green.

**Not in scope**: provider transport retries (providers own their freshness contract);
circuit-breaking.

Requirements:
- **REQ-AUD-S11-01** — idempotent-GET retry with bounded jittered backoff succeeds after transient failure; budget/deadline exhaustion → hard error. Test: `internal/forge/gitlab/retry_test.go` (new); Verify: `go test ./internal/forge/gitlab/... -run TestIdempotentRetry`; Level: L0
- **REQ-AUD-S11-02** *(fail-safe · writes)* — POST/PUT never auto-retried (single attempt pinned per write endpoint). Test: `internal/forge/gitlab/retry_test.go`; Verify: `go test ./internal/forge/gitlab/... -run TestWritesNeverRetried`; Level: L0

## AUD-S12 — ⚠️ REL-06: malformed BOT-marker skip-with-warning [autonomous · engine-grade review]

> **⚠️ Reconcile-protocol behavior change (ADR-0019). Today a malformed marker JSON on a
> bot-authored note errors reconcile until hand-deleted (fails closed but bricks the MR). The fix
> must not weaken author-identity filtering — contributor notes stay invisible regardless of
> marker well-formedness (spoof surface unchanged).**

**As a** repo operator whose bot note got corrupted **I want** reconcile to skip a malformed
bot-marker with a warning instead of hard-failing forever **so that** one bad note doesn't require
manual surgery — while a wrongly-parsed marker can never approve anything.

**Goal**: at the `parseMarker` sites (`internal/forge/gitlab/gitlab.go:250,310`): a bot-authored
note whose marker JSON is malformed is SKIPPED (treated as not-a-slot-note) with a warning carried
on the receipt/summary; reconcile proceeds. Worst case is a duplicate slot post — repaired by the
existing duplicate-repair path (conformance `TestConformanceDuplicateRepair`). Contributor-note
handling unchanged. Judgment call (d) governs this story and is PRE-LOGGED via this spec's
judgment-call section — no separate D-row needed; if the implementer disagrees at build time, open
a D-row before landing.

**Operator input**: no (judgment call (d), pre-logged above). **Dependencies**: S10/S11 (same
file). Lane A.

**Acceptance criteria (G-W-T)**:
- Given a bot note with malformed marker JSON among healthy slots, when Reconcile runs, then it
  completes, the malformed note is skipped, a warning is surfaced, and the healthy slots reconcile
  exactly as before.
- Given the skip produces a duplicate slot post, when the next reconcile runs, then the existing
  duplicate-repair converges (idempotence preserved, double-run byte-identical).
- Given a CONTRIBUTOR note with a well-formed or malformed marker, when Reconcile runs, then it is
  invisible either way (author-identity filter untouched — pinned).

**Definition of done**: skip-with-warning live; duplicate-repair convergence + spoof-invisibility
pinned; determinism gate green.

**Not in scope**: marker format changes; deleting the malformed note automatically (write
minimization).

Requirements:
- **REQ-AUD-S12-01** — malformed bot-marker → skip + warning; reconcile completes; healthy slots unaffected. Test: `internal/forge/gitlab/marker_resilience_test.go` (new); Verify: `go test ./internal/forge/gitlab/... -run TestMalformedBotMarkerSkipsWithWarning`; Level: L0
- **REQ-AUD-S12-02** *(fail-safe · spoof + convergence)* — contributor markers invisible regardless of well-formedness; post-skip duplicate repaired on next reconcile; double-run stable. Test: `internal/forge/conformance/reconciliation_test.go`; Verify: `go test ./internal/forge/conformance/... -run 'TestConformanceDuplicateRepair|TestSpoofedMarkerStillIgnored'`; Level: L1

## AUD-S13 — Test-depth bundle: TEST-02/05/06 + coverage headroom ≥91% [autonomous · tests-only]

**As a** maintainer **I want** the three untested fail-closed families covered by behavior tests
**so that** the D-010 gate stops sitting at exactly 90.0% (steering, not measuring) and the
operator's "bump coverage a bit" lands as verified behavior, not filler.

**Goal**: (1) **TEST-02** — `toCEL` overflow semantics: a `json.Number` that fits neither int64
nor float64 is **refused** — it binds a CEL error value; pin what a predicate over it does
(errors → fail-safe, never a silent coercion, and never the lexical string form). *Amended after
**D-131** / ADR-0013 Amendment 1 landed on `main`: as specified, this arm pinned the old
**string-fallback** contract, which the D-131 lane replaced precisely because the fallback was a
silent demotion to text. TEST-02 was written to red if that branch ever changed, and it did; the
requirement now pins the refusal with the same rigour (the branch really is reached, the refusal
names the number, and non-relational operators — `type()`, `int()`, `string()`, arithmetic —
cannot launder it).* (2) **TEST-05** —
`reconcileClearSlot` (`internal/forge/forge.go:568-…`, 56.5%): table over its fail-closed branches
(resolve failure, partial clear, already-clear idempotence) — each error branch both-polarity.
(3) **TEST-06** — `repo_file` builtin (`internal/provider/builtin/repo_file.go`): path-containment
table (`cleanRel`/`underAnyRoot` — traversal `../`, absolute, root-escape symlink-shaped inputs →
rejected) and expiry table (`expiresAt` — missing/zero/negative maxAge → error; boundary asOf).
(4) Aggregate `./internal/...` coverage ≥ **91.0%** measured by the existing D-010 recipe (gate
floor stays 90% per judgment call (e) unless the operator raises it).

**Operator input**: no (log if the gate floor is raised past 90).

**Dependencies**: sequence AFTER Lane A lands S10–S12 (shared `internal/forge` test dirs). Lane E.

**Acceptance criteria (G-W-T)**:
- Given an over-range `json.Number`, when bound through `toCEL` into any predicate — relational,
  equality, explicit `int()`/`string()` coercion, or arithmetic — then the evaluation errors →
  fail-safe (never APPROVE, and never the lexical string form); the refusing branch's hit count is
  non-zero in the profile.
- Given each `reconcileClearSlot` error branch, when driven by a fake forge, then the fail-closed
  outcome is asserted (and the happy idempotent clear too).
- Given traversal/absolute/escape paths and expired/undeclared maxAge, when `repo_file` answers,
  then containment rejects and expiry synthesizes the non-resolved state — never a fact from
  outside the roots or past expiry.
- Given `go test -coverprofile ./internal/...`, when totaled, then ≥ 91.0%.

**Definition of done**: three test families landed; coverage ≥91.0% printed by the gate; zero
production-code changes.

**Not in scope**: TEST-04 (`cmd/assent` gate widening — fenced); raising the D-010 floor.

Requirements:
- **REQ-AUD-S13-01** *(TEST-02)* — toCEL overflow refusal (D-131) covered incl. predicate fail-safe polarity, the non-exponent literal, and the non-relational operators. Test: `internal/core/aggregate/evaluate_tocel_test.go` (new); Verify: `go test ./internal/core/aggregate/... -run TestToCELOverflowFailsSafe`; Level: L0
- **REQ-AUD-S13-02** *(TEST-05)* — reconcileClearSlot branch table, error branches both-polarity. Test: `internal/forge/clearslot_test.go` (new); Verify: `go test ./internal/forge/... -run TestReconcileClearSlotBranches`; Level: L0
- **REQ-AUD-S13-03** *(TEST-06)* — repo_file containment + expiry tables (security-adjacent). Test: `internal/provider/builtin/repo_file_test.go` (extend); Verify: `go test ./internal/provider/builtin/... -run 'TestRepoFileContainment|TestRepoFileExpiry'`; Level: L0
- **REQ-AUD-S13-04** *(headroom)* — aggregate `./internal/...` coverage ≥ 91.0% via the D-010 recipe. Test: the coverage gate itself; Verify: `task coverage` (printed pct ≥ 91.0); Level: L1

## AUD-S14 — SEC-01 + SEC-03: ajv lockfile + `persist-credentials: false` everywhere [autonomous]

**As a** maintainer **I want** the schemas-CI npm install lockfile-pinned and every checkout
credential-scrubbed **so that** the two remaining supply-chain pin gaps close.

**Goal**: (1) **SEC-01** — replace `npm install -g --ignore-scripts ajv-cli@5.0.0 ajv-formats@3.0.1`
(`schemas.yml:59-60`) with a committed `package.json` + `package-lock.json` (e.g.
`hack/schemas-validator/`) and `npm ci --ignore-scripts` (transitive tree pinned, not just the
roots). (2) **SEC-03** — add `persist-credentials: false` to every checkout missing it (verify.yaml
both jobs, schemas.yml) — matching the 7 workflows that already set it.

**Operator input**: no. **Dependencies**: Lane B, after S09 (shared `workflow_pins_test.sh`).

**Acceptance criteria (G-W-T)**:
- Given schemas CI, when the validator installs, then `npm ci` resolves solely from the committed
  lockfile and the stock-validator gate stays green; a lockfile-absent or drifted state fails.
- Given every workflow checkout, when grepped, then `persist-credentials: false` is present on all
  of them (pinned by the S09 grep gate, extended).

**Definition of done**: lockfile committed; `npm ci` in CI; zero unpinned checkouts.

Requirements:
- **REQ-AUD-S14-01** *(SEC-01)* — lockfile-pinned ajv install via `npm ci --ignore-scripts`; stock Draft 2020-12 gate green. Test: `hack/validate-schemas-stock.sh` run + `hack/lint/workflow_pins_test.sh`; Verify: `bash hack/lint/workflow_pins_test.sh`; Level: L1
- **REQ-AUD-S14-02** *(SEC-03)* — every `actions/checkout` across `.github/workflows/**` sets `persist-credentials: false`. Test: `hack/lint/workflow_pins_test.sh`; Verify: `bash hack/lint/workflow_pins_test.sh`; Level: L1

## AUD-S15 — ⚠️ ARCH-02: lift `MRInfo`/`ErrNotFound` into the forge port (pre-E10) [autonomous · engine-grade review]

> **⚠️ Port-boundary refactor across `internal/forge` + `cmd/assent/run.go` (~6 sites:
> `run.go:61,420,473,496,533,624`, `run_render.go:20`). Behavior-preserving by contract — the
> refactor gate is byte-identical goldens. The full target shape (incl. `forge.RunPort` and the
> `SyntheticDigest` collapse) is seeded in
> [docs/planning/design-notes/e10-forge-port-lift.md](../../../docs/planning/design-notes/e10-forge-port-lift.md);
> this story ships only the mechanical lift below.**

**As a** future GitHub-adapter author **I want** the orchestration read port to speak
forge-neutral types (`forge.MRInfo`, `forge.ErrNotFound`) instead of `gitlab.*` **so that** a
second adapter slots in without rewriting `cmd/assent`.

**Goal**: define `MRInfo` + `ErrNotFound` (sentinel or `errors.Is`-able) in `internal/forge`;
`internal/forge/gitlab` returns/wraps them; `cmd/assent` imports `gitlab` ONLY at construction
(`gitlab.New`). Pure mechanical lift, zero behavior change. Binding constraint carried from the
ARCH-02 note: no new `gitlab.*` type may be added to `forgePort` in the meantime.

**Operator input**: no. **Dependencies**: LAST code story — after Lanes A (S01,S10–S12) and C
(S04,S08,S16) drain, since it touches both trees.

**Acceptance criteria (G-W-T)**:
- Given the lift, when grepping `cmd/assent` for `gitlab.`, then only the constructor site(s)
  remain; the compile proves the port is self-contained.
- Given the full suite (D-016 replay, conformance, dogfood, determinism), when run, then
  byte-identical outputs — the refactor changed no behavior.
- Given a 404-shaped forge error, when checked via `errors.Is(err, forge.ErrNotFound)`, then it
  matches through the wrap chain (the `fileAtRefOrAbsent` presence signal keeps working).

**Definition of done**: port neutral; adapter-only gitlab imports; suite byte-green.

**Not in scope**: any GitHub adapter code (E10, Locked per D-012); the `forge.RunPort` composite
port + `SyntheticDigest` collapse (E10, per the design note).

Requirements:
- **REQ-AUD-S15-01** — `forge.MRInfo`/`forge.ErrNotFound` own the port; `cmd/assent` imports `gitlab` only for construction (grep-pinned). Test: `internal/forge/port_test.go` (new) + `hack/lint/depguard_test.sh` (S07 rule extended); Verify: `go build ./... && bash hack/lint/depguard_test.sh`; Level: L1
- **REQ-AUD-S15-02** *(refactor gate)* — D-016 replay + conformance + determinism byte-identical pre/post. Test: existing suites; Verify: `task check && task determinism`; Level: L1

## AUD-S16 — ARCH-04: wire `assent-jcs-v1` into the replay-bundle digest (D-121) [autonomous · release-sensitive]

**As a** record consumer **I want** ONE stated canonical-hash story **so that** the frozen,
vector-locked `assent-jcs-v1` (`internal/core/hash`, dormant) and the ad-hoc `sha256(bytes)` that
actually carries the released `policySha` contract stop coexisting ambiguously.

**Goal**: per **D-121** (DECIDED — corrects the D-114 drift: `internal/compare/suite.go:43-56`
computes undomained `sha256(json.Marshal(decoded))`, not the `assent.canonical-json.v1` digest
D-114 claims): codify the byte-vs-document split — digests over BYTE artifacts (`policySha` over
policy bytes, marker occurrence/decision digests over judged/emitted bytes, `toolDigest`) stay raw
`sha256:<hex>` (byte identity is the point); digests over SCHEMA-OWNED JSON DOCUMENTS that
consumers re-parse and re-verify use `internal/core/hash.Digest` (ADR-0017 §9 domain separation)
with the schema `$id` as domain. Exactly ONE digest switches: `compare.ReplayBundleDigest` →
`hash.Digest("https://assent.dev/schemas/decision/v1alpha1/replay-bundle.schema.json", raw)`.
Migration (pre-v1, same implementation commit): regenerate every `replayBundleDigest` in
`examples/comparison/*/suite.yaml`; caseIds and bundle bytes unchanged (D-113 immutability
preserved — the algorithm is versioned by the D-121 row). ADR-0019 marker grammar untouched.

**Operator input**: no (D-121 binds; cite it in the commit). **Dependencies**: D-121 (landed in
the same PR as this spec); Lane C after S08. **Semver visibility**: changes released
`replayBundleDigest` values — flag + CHANGELOG note for corpus consumers.

**Acceptance criteria (G-W-T)**:
- Given the vector-locked test vectors, when `compare.ReplayBundleDigest` runs, then its output
  equals `hash.Digest(<replay-bundle schema $id>, raw)`; given an old-format (undomained) digest
  in a suite, then it is rejected with `ErrDigestMismatch` (fail-closed proven both polarities).
- Given the corpus, when the implementation commit lands, then every
  `examples/comparison/*/suite.yaml` digest is regenerated in the SAME commit, the
  `assent compare --suite` exit gate is green, and no `caseId` or bundle bytes changed (D-113
  held).
- Given `pins.policySha`, when the suite runs, then it remains `sha256(raw policy bytes)` with the
  golden unchanged — a test pins the byte-vs-document split against over-eager canonicalization.
- Given the marker grammar tests (ADR-0019), when run, then unchanged; `task check` green; D-121
  cited in the commit.

**Definition of done**: `ReplayBundleDigest` domain-separated; corpus regenerated same-commit;
raw-byte digests guarded by test; contract text and implementation agree; D-121 cited.

**Not in scope**: `toolDigest` (S04); marker digests (explicitly raw per D-121 — no ADR-0019
churn); new hash algorithms.

Requirements:
- **REQ-AUD-S16-01** — `compare.ReplayBundleDigest` = `hash.Digest(<replay-bundle schema $id>, raw)` per D-121; old-format digests fail closed with `ErrDigestMismatch` (both polarities); corpus regenerated in the same commit with the compare exit gate green and caseIds/bundles byte-unchanged. Test: `internal/compare` digest tests + `hack/compare/exitgate_test.sh`; Verify: `go test ./internal/compare/... -run TestReplayBundleDigest && bash hack/compare/exitgate_test.sh`; Level: L0
- **REQ-AUD-S16-02** *(guard · byte artifacts stay raw)* — `pins.policySha` remains `sha256(raw policy bytes)` (golden unchanged); marker digests untouched. Test: `cmd/assent/policysha_test.go` (new); Verify: `go test ./cmd/assent/... -run TestPolicyShaStaysRawBytes`; Level: L1

## AUD-S17 — ARCH-05: C4 diagrams synced to the shipped architecture [autonomous]

**As a** new contributor **I want** `docs/architecture/c4-container.md` (+ context) to show what
exists **so that** rego/gRPC/WASM/GitHub appear as PLANNED (marked), not unmarked-existing, and the
package sketch matches `go list ./internal/... ./cmd/...`.

**Goal**: redraw the container diagram from the real package graph (22 internal packages per the
rewritten `internal/README.md`); mark speculative elements with an explicit "planned" style/legend;
drop or fence the stale package sketch.

**Operator input**: no. **Dependencies**: none. Lane D last.

**Acceptance criteria (G-W-T)**:
- Given the diagram, when compared against `go list`, then every EXISTING element names a real
  package/binary and every non-existing element is visibly marked planned (legend present).
- Given `task docs-build` strict, when run, then green.

**Definition of done**: diagrams truthful; strict build green.

Requirements:
- **REQ-AUD-S17-01** — C4 container/context match reality with a planned/shipped legend; strict docs build green. Test: `task docs-build` + review checklist in the PR body; Verify: `task docs-build`; Level: L1

## AUD-S18 — Exit gate: audit conditions closed + gates green at the new bar [autonomous]

**As a** maintainer **I want** one gate that proves the audit's conditions and this epic's bar
hold together **so that** the next tag ships with the P1 closed, the changelog gate live, the
verify-gated release path, and the raised coverage measured — and none of it can rot silently.

**Goal**: a `hack/audit/exitgate_test.sh` (mirroring the E9/PCS exit-gate pattern) asserting: (1)
the S01 fail-closed cassettes pass; (2) `task check` green INCLUDING `changelog-verify` (S02) at
≥91.0% coverage (S13); (3) the release workflow contains the S03 gate step (structural pin); (4)
the S06/S05 truth pins pass (no retired phrase resurfaces); (5) determinism double-run green; (6)
`git diff schemas/` empty except the S04/D-120 `toolDigest` description line; (7) the
finding→story disposition table in this spec is fully checked (every audit ID: closed here,
fenced-to-operator, or explicitly accepted).

**Operator input**: no.

**Dependencies**: AUD-S01..S17.

**Acceptance criteria (G-W-T)**:
- Given HEAD after all stories, when the exit-gate script runs fresh, then every check above passes
  in one invocation and double-runs byte-identical.
- Given any single regression (stale changelog, dropped gate step, coverage dip below 91, retired
  phrase reintroduced), when the gate runs, then it FAILS naming the finding ID.

**Definition of done**: exit gate green locally and wired into the `release-exitgate` CI job;
backlog rows flipped to Done; the three operator-only residuals (SEC-05/SEC-06/RELSE-07) handed
over explicitly.

Requirements:
- **REQ-AUD-S18-01** — one-invocation exit gate covering (1)–(7), wired into `release-exitgate`; failure names the finding ID. Test: `hack/audit/exitgate_test.sh` (new); Verify: `bash hack/audit/exitgate_test.sh`; Level: L1
- **REQ-AUD-S18-02** — disposition completeness: every 2026-08-06 audit finding ID maps to a story REQ, an operator residual, or a logged acceptance — asserted by the gate's table check. Test: `hack/audit/exitgate_test.sh`; Verify: `bash hack/audit/exitgate_test.sh`; Level: L1

---

## Appendix: deferred ready-to-commit artifacts

Two artifacts were authored in the architect lane but are deliberately NOT committed with this
spec — each lands inside the story whose change makes it true. Their text is fixed here verbatim
so the implementing lane copies, never re-drafts.

### A.1 — ADR-0011 Amendment 3 (lands in the SAME change as AUD-S07's enforcement)

Append to `docs/adr/0011-core-ports-and-contracts.md`. **Rule: this text lands in the same change
as S07's depguard + purity-walk enforcement** — never before (the claim would be false) and never
after (a window where it is false again).

```markdown
## Amendment 3 (2026-08-06, D-123 — boundary enforcement mechanism made true)

The first invariant above claimed "arch-lint enforced" while enforcement was in fact
manual review plus a purity walk covering only part of the pure tree (audit finding
ARCH-01, open across three audits). As of D-123 the invariant reads, and is enforced as:

- `internal/core/**`, `internal/change/**`, `internal/glob`, `internal/lint`,
  `internal/catalogue`, `internal/evaldecode`, `internal/compare`, and `schemas/**`
  import no port implementations (`internal/forge/**`, `internal/render/**`, `cmd/**`)
  and no `net/**` — enforced by golangci-lint `depguard` deny-rules in `.golangci.yml`
  (package-level, fails `task check`/CI verify);
- the same tree contains no `time.Now`, `os.Getenv`/`os.Environ`, or `math/rand`
  call-sites — enforced by the `TestCorePurity` AST walk in
  `internal/core/purity_test.go`, extended beyond its original directories to
  `../evaldecode`, `../compare`, and `../../schemas` (call-level; keeps its adversarial
  self-test proving the guard would fire).

`internal/evaldecode` and `internal/compare` are added to the guarded tree
deliberately: both sit on decision paths (engine input decode; D-116/D-117 compare
gates) and inherit the hard rule that nothing probabilistic, wall-clock- or
randomness-dependent may live there. "arch-lint enforced" elsewhere in this ADR should
be read as "depguard + purity-walk enforced" per this amendment.
```

### A.2 — `toolDigest` schema description replacement (lands as its own reviewed commit in AUD-S04, citing D-120)

In `schemas/decision/v1alpha1/decision-record.schema.json`, at
`$defs.pins.properties.toolDigest.description`. **Rule: annotation-only; lands as its own
reviewed commit in S04 citing D-120 — it trips the `git diff schemas/` exit-gate guard by
design** (the guard exists to force exactly this kind of reviewed, cited schema commit).

Replace:

```json
"description": "Content digest of the evaluating tool build (OQ-9 replayability)."
```

with:

```json
"description": "Deterministic build-content proxy for the evaluating tool: sha256 over the binary's canonical Go build info (module path/version/sum, dependency checksums, VCS revision + dirty flag) per D-120 (OQ-9 replayability). Falls back to sha256(\"buildinfo-unavailable\\n\"+toolVersion) when build info is absent. Records emitted by pre-D-120 builds carry sha256 of toolVersion only."
```

---

## Appendix B — 2026-08-06 audit finding → disposition table (AUD-S18)

**This table is the authority REQ-AUD-S18-02 checks.** `hack/audit/exitgate_test.sh` carries the
canonical list of finding IDs and asserts this table has exactly one row per ID, that every `Done`
row names a story heading and REQ IDs that exist in this spec, that every `Operator` row names a
row that exists in `openspec/specs/backlog.md`, and that every `Accepted` row names a decision row
that exists in `docs/decisions/decisions.md` and mentions that finding.

**Derivation of the 37 IDs** (the audit file `agent-context/PROJECT-AUDIT-2026-08-06.md` is
session-local and gitignored, so the gate's embedded list is the in-tree authority — this paragraph
is how a future reader re-derives it): every ID named in that audit's *Findings — P1*, *Findings —
P2 (deduplicated)* and *Findings — P3 (grouped)* sections, PLUS every ID its *Prior findings
disposition* table re-marked "still open" or "accepted" at `e668d0e` (SEC-02, DOC-04, A-01). Prior
IDs the same table marked **fixed** (REL-02, REL-05, DOC-01, DOC-02, RELSE-02, RELSE-03, RELSE-04)
are deliberately NOT rows here: they map to none of the three dispositions, having been closed
before this epic opened. The coordinator-hygiene note (stale `lane-*` worktrees) carries no finding
ID and is `/handover` work, not a story.

**Disposition vocabulary** — exactly three tokens:
- `Done` — closed by a story in this epic. Owner = the `AUD-Snn` story; Evidence = its REQ IDs.
- `Operator` — fenced out of the epic because it is a live GitHub setting or secret. Owner = the
  backlog residual row that tracks it.
- `Accepted` — deliberately not fixed. Owner = the decision row that logs the acceptance.

**Scope statement — what this table and its gate do NOT certify.** This is the exit gate for the
**2026-08-06** audit only. It says nothing about defects found after that audit. Two decision-path
fail-opens found during this epic's own execution are tracked in
[open-questions.md](../../../docs/planning/open-questions.md) and **block the release tag
independently of this table**; see *Post-audit release blockers* below. A green run of
`hack/audit/exitgate_test.sh` means "the audit's conditions are closed and the epic's bar holds" —
it does **not** mean "all known fail-opens are closed" and it is **not** a release clearance.

| Finding | Sev | Disposition | Owner | Evidence |
| --- | --- | --- | --- | --- |
| REL-07 | P1 | Done | AUD-S01 | REQ-AUD-S01-01 REQ-AUD-S01-02 REQ-AUD-S01-03 REQ-AUD-S01-04 |
| RELSE-01 | P2 | Done | AUD-S02 | REQ-AUD-S02-01 REQ-AUD-S02-02 |
| RELSE-05 | P2 | Done | AUD-S03 | REQ-AUD-S03-01 REQ-AUD-S03-02 |
| ARCH-03 | P2 | Done | AUD-S04 | REQ-AUD-S04-01 REQ-AUD-S04-02 |
| DOC-08 | P2 | Done | AUD-S05 | REQ-AUD-S05-01 REQ-AUD-S05-02 |
| DOC-05 | P3 | Done | AUD-S06 | REQ-AUD-S06-02 |
| DOC-06 | P2 | Done | AUD-S06 | REQ-AUD-S06-02 |
| DOC-07 | P2 | Done | AUD-S06 | REQ-AUD-S06-01 |
| DOC-09 | P2 | Done | AUD-S06 | REQ-AUD-S06-02 |
| DOC-10 | P3 | Done | AUD-S06 | REQ-AUD-S06-02 |
| DOC-11 | P3 | Done | AUD-S06 | REQ-AUD-S06-02 |
| ARCH-01 | P2 | Done | AUD-S07 | REQ-AUD-S07-01 REQ-AUD-S07-02 |
| REL-08 | P3 | Done | AUD-S08 | REQ-AUD-S08-01 REQ-AUD-S08-02 |
| SEC-04 | P3 | Done | AUD-S09 | REQ-AUD-S09-01 |
| REL-03 | P3 | Done | AUD-S10 | REQ-AUD-S10-01 REQ-AUD-S10-02 |
| SEC-08 | P3 | Done | AUD-S10 | REQ-AUD-S10-01 |
| REL-04 | P3 | Done | AUD-S11 | REQ-AUD-S11-01 REQ-AUD-S11-02 |
| REL-06 | P3 | Done | AUD-S12 | REQ-AUD-S12-01 REQ-AUD-S12-02 |
| TEST-02 | P3 | Done | AUD-S13 | REQ-AUD-S13-01 |
| TEST-05 | P3 | Done | AUD-S13 | REQ-AUD-S13-02 |
| TEST-06 | P3 | Done | AUD-S13 | REQ-AUD-S13-03 |
| TEST-03 | P3 | Done | AUD-S13 | REQ-AUD-S13-04 |
| SEC-01 | P3 | Done | AUD-S14 | REQ-AUD-S14-01 |
| SEC-03 | P3 | Done | AUD-S14 | REQ-AUD-S14-02 |
| ARCH-02 | P3 | Done | AUD-S15 | REQ-AUD-S15-01 REQ-AUD-S15-02 |
| ARCH-04 | P3 | Done | AUD-S16 | REQ-AUD-S16-01 REQ-AUD-S16-02 |
| ARCH-05 | P3 | Done | AUD-S17 | REQ-AUD-S17-01 |
| SEC-05 | P2 | Operator | AUD-OPS | rotate `HOMEBREW_TAP_GITHUB_TOKEN` to a fine-grained PAT (live repo secret) |
| SEC-06 | P3 | Operator | AUD-OPS | tag ruleset on `v*.*.*` (live GitHub ruleset) |
| RELSE-07 | P3 | Operator | AUD-OPS | branch protection `enforce_admins` on `main` (live GitHub setting) |
| RELSE-08 | P3 | Operator | AUD-RELSE-08 | make `release-exitgate` a required PR check (live branch protection) |
| SEC-02 | P3 | Accepted | D-132 | check-gap compensated by the four required CI contexts |
| SEC-07 | P3 | Accepted | D-132 | in-place asset replacement; patch-tag runbook line landed in AUD-S06 |
| TEST-04 | P3 | Accepted | D-132 | `cmd/assent` outside the D-010 denominator; compensated by binary dogfood gates |
| DOC-03 | P3 | Accepted | D-132 | globally-gitignored operator-local `AGENTS.md`; not in the tree |
| DOC-04 | P3 | Accepted | D-132 | planning docs out of the public nav by design |
| A-01 | P3 | Accepted | D-132 | glob recompile; no hot-path evidence (E3) |

### Post-audit release blockers (NOT 2026-08-06 findings — outside this table's disposition)

Found during this epic's execution, after the audit anchor. They are **not** audit findings, so they
are not rows above; they **are** tag blockers, so the exit gate names them and refuses to be read as
release clearance. The gate asserts this section exists, that each row cites an `OQ-<n>` that
resolves in `docs/planning/open-questions.md`, and that each carries a status token.

| Defect | Question | Status |
| --- | --- | --- |
| A relational CEL leaf over string-bound operands returns a silently wrong boolean instead of erroring (verified BLOCK→APPROVE flip on quoted YAML scalars) | OQ-27 | OPEN — dedicated decision-path lane |
| `builtin/repo-file` enforces path containment but not filesystem containment: a symlink under a declared root yields facts from outside the roots | OQ-28 | OPEN — dedicated provider lane |
