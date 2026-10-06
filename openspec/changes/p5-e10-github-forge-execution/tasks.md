# Tasks — P5-E10 GitHub forge adapter execution

Requirement IDs reference the authoritative epic spec
(`openspec/specs/p5-e10-github-forge/spec.md`); each task lists the REQs whose
Given/When/Then and `Verify:` lines are its acceptance bar. Test-first: every task starts
from its failing test. One logical change per commit; `task check` green before every commit.

Execution decisions binding all tasks: the two recorded in [proposal.md](proposal.md)
(composite `(project, mr)` content-accessor handles; `forge.ErrUnauthorized` lifted to the
port), plus the S00 model's call-site inventory (two migrate, six stay) and case-ID table.

## 1. Seam wave

- [ ] **T0 (S00 obligation 1 / S01 DoD residue)** — mint the **twelve** S00 case IDs as
      `catalog.yaml` rows with adapter dispositions, and implement the ones their wave owns:
      `fork-head-unchanged-file-no-lifecycle` + `fork-head-genuine-delete-detected` (T1, fake
      + GitLab factories; GitHub factory at T6); `forbidden-never-renders-as-absent`,
      `absent-file-still-renders-as-absent`, `ratelimit-403-is-transport-error`,
      `metadata-only-token-is-not-absence` (T1 for the fake + GitLab factory via
      `ErrUnauthorized`/transport mapping; T6 adds the GitHub factory; T13 proves parity or
      cited deferral); `threads-resolvable-graphql`, `capability-report-exhaustive`,
      `capability-unknown-never-arms`, `capability-supported-does-arm`,
      `capability-gap-string-is-merge-result-only`, `capability-gap-positive-control` (T3,
      conformance cases + catalog rows, not merely package-local table tests). A case not yet
      executable on a factory carries its row with a cited deferral until its story lands —
      never an unowned case.
- [ ] **T1 (E10-S02)** — `forge.RunPort` composite port + neutral factory + depguard:
      `forge.Forge + forge.Snapshotter + forge.Resolver + GetMR + FileAtRef +
      FileAtBase/FileAtHead`; `forge.ErrUnauthorized` lifted to the port (REQ-E10-S02-01's
      sentinel half, with the Q4 forbidden≠absent case green at this story on the fake +
      GitLab factories — `forge.ErrUnauthorized` is the port concept both adapters map into);
      port identity exposure (REQ-E10-S02-06); fake implements `RunPort` directly (REQ-03);
      migrate exactly the two governed-subject call sites, keep all six policy loads on
      `FileAtRef` (REQ-05, both source-level guards); retire `cmd/assent`'s `forgePort` AND
      `refFilePort` (REQ-01); depguard denies both adapters from `cmd/assent` with the
      anti-vacuity positive control rebuilt (REQ-02, REQ-04); invariant guard on
      forge-read/write interfaces in `cmd/assent` (REQ-07); fork-MR conformance cases +
      policy-load trust-boundary case (REQ-05). Mechanical proofs of REQ-E10X-01-01: a
      compile-time assertion that no adapter constructor takes a project argument, and the
      conformance factory serving several `Config.Project` values from one backend.
      Also, in this commit: correct the epic spec's stale anchors the S00 drift table names
      (`Describe(project, mr)` → `GetMR(project, mr)` at spec.md:282;
      `provider_host.go:275` → `:292` at spec.md:340-341), mirroring S00's precedent.
- [ ] **T2 (E10-S03)** — collapse `gitlab.SyntheticDigest` call-sites onto
      `Snapshot.Heads.MergeResultDigest`; goldens byte-identical (REQ-01, REQ-02).
- [ ] **T3 (E10-S04)** — `forge.Capability` closed eleven-flag enum; `CapabilityReport`
      supported/absent/unknown + reason; gap computed at the port; `unknown` == `absent`
      for arming with the paired positive control (REQ-01/02); probe transport failure is a
      hard error, never `unknown` (REQ-05); GitLab adapter returns a `CapabilityReport`,
      unprobed = `unknown` (REQ-03); retire the `@` heuristic ⇒ the prescribed
      user-visible GitLab arming decision row + changelog entry (REQ-04, judgment call (e)).
      Conformance cases of S00's Q2/Q3 tables land here: `capability-report-exhaustive`,
      `capability-unknown-never-arms`, `capability-supported-does-arm`,
      `capability-gap-string-is-merge-result-only`, `capability-gap-positive-control`
      (+ `threads-resolvable-graphql`'s row dispositioned at T5's GitHub round-trip).
- [ ] **T4 (E10-S05)** — port-level transport requirements as conformance cases: bounded
      reads + pagination caps with fail-closed exhaustion (REQ-01); idempotent-GET-only
      retry, writes never retried (REQ-02); context deadlines as a port requirement with a
      deadline-bounded conformance case (ADR-0021 item 4).

## 2. Adapter wave

- [ ] **T5 (E10-S06)** — `internal/forge/github`: REST + GraphQL transports behind
      adapter-internal interfaces, PAT + App-installation auth, httptest cassettes for both
      transports and both auth shapes; missing/expired credential fails closed; no token in
      any log or error; sanitization check green (REQ-01/02/03).
- [ ] **T6 (E10-S07)** — GitHub Snapshot: `MRInfo` (head/base SHAs, fork detection with the
      absent-head-repo trap closed), ADR-0020 completeness (truncation ⇒ opaque enumeration
      failure), merge-result pinning via `refs/pull/N/merge`; S00's Q1/Q4 cases green
      against the GitHub factory (`fork-head-unchanged-file-no-lifecycle`,
      `fork-head-genuine-delete-detected`, `forbidden-never-renders-as-absent`,
      `absent-file-still-renders-as-absent`, `metadata-only-token-is-not-absence`,
      `ratelimit-403-is-transport-error`).
- [ ] **T7 (E10-S08)** — Resolve → typed `ApprovalEvidence` (latest non-dismissed review per
      eligible reviewer; PR author + bots excluded; `REQUEST_CHANGES` as block signal, never
      approval; dismissed reviews never count); unprovable eligibility ⇒ capability gap,
      `require-review` unsatisfiable (REQ-01..03).
- [ ] **T8 (E10-S09)** — GitHub capability report: all eleven flags with state + reason;
      compile-time-exhaustive test; unverified-open-items report `unknown` (REQ-01/02).
- [ ] **T9 (E10-S10)** — Reconcile writes through the shared `internal/forge` engine
      (adapter supplies primitives; conformance replay cases green on the GitHub factory);
      marker filter matches the token identity's own user/app id (not "any bot"); malformed
      marker skipped with a warning (REQ-01/02).
- [ ] **T10 (E10-S11)** — SHA-guarded merge (`ErrSHAMoved` via the shared conformance cases
      on the GitHub factory), deferred arming via `enablePullRequestAutoMerge`,
      arming-revoked-on-push honoured (unknown/absent ⇒ arming refused), merge-queue pin
      proof or honest capability gap, queue/direct-merge exclusivity (REQ-01..04).
- [ ] **T11 (E10-S12)** — fail-closed deltas table: dismissal-restrictions, auto-merge
      revoke, merge queue simulated absent ⇒ `merges == 0` for all three, plus the
      mandatory positive control (`merges == 1` when all capabilities present) (REQ-01/02).
      REQ-E10-S12-03 closes autonomously as follows: the S00 Q2 predicate table is filled
      with the values the hermetic adapter actually reports (each `C` constant naming its
      licensing case; `unknown` cells citing their open dossier items), OQ-33/OQ-34 cited as
      open — **the live-repository confirmation of "supported against a real repository"
      remains S18's**, not a blocker inside this change; T14 carries the "S15 blocked until
      the table has real values" gate forward.

## 3. Integration wave

- [ ] **T12 (E10-S13)** — forge selection: `--forge {gitlab|github}` + unambiguous remote
      host autodetect; ambiguity or unknown host fails closed (never default-to-GitLab);
      selection goes through the neutral factory; depguard still denies adapters (REQ-01/02).
- [ ] **T13 (E10-S14)** — conformance parity: `RunSuite` against both factories; **the
      twelve S00-minted case IDs (T0) present and executed**, with every catalog row
      carrying an explicit adapter disposition (a bare row = a deferred row lacking a
      cited non-empty reason; the `github-deferred` L3 rows keep their deferral with cited
      reasons — live infra, S18); D-084 dispositioned; every non-deferred row dispositioned
      without inheriting any stale row count (REQ-01/02).
- [ ] **T14 (E10-S15)** — docs/maturity truth: README maturity row moves GitHub to its earned
      tier with a docs-truth test; C4 legend; `cli.md` documents `--forge`; dossier open
      items dispositioned; **blocked until T11 has filled S00's Q2 predicate table with
      real values** (REQ-E10-S12-03's gate); meta-plan/later-phases E10 row cites D-140
      (REQ-01/02).
- [ ] **T15 (E10-S16)** — Actions entrypoint: composite `action.yml` on a pinned,
      checksum-verified released binary; base-ref workflow trust documented; no new adapter
      behaviour (REQ-01/02). Last and independently droppable.
- [ ] **T16 (E10-S17)** — exit gate: `hack/forge/e10_exitgate_test.sh` proving all S17
      conditions in one invocation, citing D-140 + ADR-0021.

## Standing gates (every task)

- Specs before code; failing test before implementation; `task check` green before every
  commit; `:gitmoji: type(scope): summary` ASCII shortcode; no AI co-author trailers.
- `internal/core` stays I/O-free; `git diff schemas/` == 0; golden corpus byte-identical on
  GitLab paths (S03's byte-identical requirement).
- S02/S04 are `[maintainer LGTM]` core-contract work — the PR surfaces them rather than
  auto-merging; the LGTM flag is carried in the PR description.
- Any GitLab arming outcome change lands with its own D-nnn row + changelog entry
  (judgment call (e)).
