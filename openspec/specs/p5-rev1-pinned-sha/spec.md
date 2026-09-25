# P5-REV1 — Six-review remediation, lane B1: read the judged content at the pinned SHA

**Epic ID / REQ prefix:** `REV1` / `REQ-REV1-S01-nn`.

**Origin:** the six-leg Assent review round of 2026-09-25 (`data/assent-unify/report.md`
§4.1 U-04, §6 lane B1; `data/assent-diff-qfn/report.md` QFN-01) at HEAD `b8123cc`. This epic
is the register's **named first fix slice** — the one CRITICAL that is mechanical, is not
waiting on a captain answer, and is deadline-bearing because the E10 GitHub adapter inherits
the frozen `forgePort` shape and would clone the defect per-forge if it landed first.

**Vehicle:** one narrow, file-scoped fix lane. It closes exactly **item 1** of U-04
("read by the pin") plus its test lock. Items 2–4 of U-04 are a named follow-on (see
Non-goals) and are deliberately **not** claimed here.

---

## Problem

`assent run`'s decision path pins `info.SourceSHA` / `info.TargetSHA` once, from
`GetMR` (`cmd/assent/run.go:177`), and the approval and the merge compare-and-swap are
pinned to those commit SHAs. But **every content read on the live path resolves a mutable
branch name**, not the pinned commit:

| Site | Read | Ref passed today |
| --- | --- | --- |
| `cmd/assent/run.go:203` | MergePolicy bytes | `info.TargetBranch` |
| `cmd/assent/run.go:211` | RulesetBinding bytes | `info.TargetBranch` |
| `cmd/assent/run.go:230` | Config bytes (when `--config`) | `info.TargetBranch` |
| `cmd/assent/run.go:249` | Pack bytes (when `--pack`) | `info.TargetBranch` |
| `cmd/assent/run.go:270` | governed base bytes | `info.TargetBranch` |
| `cmd/assent/run.go:274` | governed head bytes | `info.SourceBranch` |

The invariant the merge leans on — *final head == pin ⇒ final head == what was judged* — is
therefore false. A contributor who controls push timing on the MR branch can push benign
bytes X2 (judged), then force-push back to the pinned X1 before `Reconcile`'s re-read;
the SHA-guard passes (head == pin) and GitLab merges X1 — bytes that were never evaluated.
The DecisionRecord carries `policySha/sourceSha/targetSha` but never a content digest, so the
record cannot distinguish the two states either. ADR-0015 §2's heading "Merge is SHA-guarded
(no TOCTOU)" describes the *branch-moved* guard, not a bytes-equal-to-pin guarantee.

The GitLab adapter already accepts a commit SHA as `?ref=` (`internal/forge/gitlab/gitlab.go:474`,
the same URL-escaped parameter), so the fix is one parameter at each of the six call sites.

---

## Non-goals

- **Do not implement U-04 items 2–4.** (2) the SHA-bound `GET /repository/compare` re-fold of
  the changed-file enumeration (shares its mechanism with the D-139 trio, lane B5); (3) the
  `CI_MERGE_REQUEST_SOURCE_BRANCH_SHA` belt; (4) the additive `headContentSha` record pin
  (lane B9). Each is a named follow-on in the same family and lands on its own evidence.
- **Do not change the `--checkout` tree↔SHA binding** (U-08 / QFN-04). That is the
  checkout-path sibling, owned by B5, and is a different mechanism.
- **Do not touch the E10 `forgePort` shape.** This lane is precisely what must land *before*
  E10 so the GitHub adapter does not inherit the defect.
- **No behaviour change beyond strictly stronger pinning.** The same bytes are read when the
  branch has not moved; the change is observable only when the branch name and the pin
  disagree, and then it fails toward judging the pinned (pinned-at-evaluation) content.
- **Do not amend the published `docs/usage/cli.md` remediation claim in this lane.** The
  checkout-less governed-content read becomes pinned here; the enumeration residual (item 2)
  is what the claim's second half still leans on, and it lands with B5.

---

## ADRs and decisions

**ADRs:** 0005 (forge conformance suite), 0015 §2 (SHA-guarded merge, no TOCTOU), 0020
(changed-file enumeration completeness — untouched here).

**Decisions:** **D-182** (this lane: read the judged content at the pinned SHA). Related
history the lane must not re-derive: **D-033** (pinned SHAs live in `DecisionRecord.pins`),
**D-139** (the checkout-path sibling, SEC-01), **D-122** (emit-before-reconcile ordering —
unchanged).

**Reuse, explicitly:** `forge.FileAtRef` (`internal/forge/gitlab/gitlab.go:474`) already
accepts an arbitrary `ref`; `fileAtRefOrAbsent` (`cmd/assent/run.go:476`) already maps a 404
at that ref to the absent-file presence signal. No new adapter API, no new discriminator.

---

## Judgment calls (decide-and-log)

**(a) Target-side reads pinned to `TargetSHA` too — DECIDED: yes.** The exploit named in U-04
is source-side, and a moved target already trips the merge guard, so pinning the target-side
reads is not strictly required for the attack. It costs nothing, removes the maintainer-push
window ambiguity on the policy/config/pack loads, and makes one uniform rule ("every content
read is at a pin") rather than an asymmetry a later reader must re-justify. This is
diff-qfn's own item-1 wording.

**(b) The two named tests are *flipped*, not merely updated — DECIDED: the fakes now reject
the branch name.** Updating the assertions to expect the SHA would still pass if `orchestrate`
read the branch name *and* the fake accepted it. The polarity flip makes the test fail on a
regression: the run-path fake's file router serves the pinned SHAs and errors on any other
ref, and the adapter test asserts the `?ref=` it sends is the SHA. A test that pins the
vulnerable shape is worse than no test; flipping it is the fix's own test lock.

**(c) The move-and-restore case lives in two places, and the conformance-suite half asserts
the guard's *limit*, not its strength — DECIDED: state that plainly.** The forge conformance
suite (`internal/forge/conformance`) drives `forge.Reconcile` only; it cannot observe which
bytes an evaluation read. Its move-and-restore case therefore proves that the CAS **merges
once the head is back at the pin** — i.e. that the CAS alone cannot catch a move-and-restore,
which is exactly *why* the read must be pinned. The run-path case is the one that proves the
read itself is pinned (the branch points at benign bytes while the pin points at violating
bytes; the decision follows the pin). Both are required: one documents the guard's limit, the
other closes it.

---

## Story index

| ID | Story | Execution | Depends on | Gate contribution |
| --- | --- | --- | --- | --- |
| REV1-S01 | Read judged content at the pinned SHA (U-04 item 1) | **[autonomous · engine-grade]** | none | closes the last hole in ADR-0015 §2's headline invariant; pre-empts the E10 per-forge clone |

---

## REV1-S01 — Read the judged content at the pinned SHA [autonomous · engine-grade]

**As an** operator whose MR outcome is an auto-merge **I want** assent to judge the bytes at
the commit SHA it pinned and will merge **so that** a contributor who force-pushes the branch
after evaluation cannot make assent merge content it never evaluated.

**Goal:** in `cmd/assent/run.go`'s `orchestrate`, replace the branch-name ref at the six
content reads with the pinned commit: `info.TargetSHA` at `:203`, `:211`, `:230`, `:249`,
`:270`; `info.SourceSHA` at `:274`. Flip the two polarity tests (`gitlab_test.go:82`,
`run_test.go:388` — and the run-path fake's policy-load assertion at
`run_self_vouch_test.go:70`, which reads the same fake). Add the move-and-restore conformance
case in `internal/forge/conformance` (with its catalog row and the `Fixture.MoveSourceHead`
seam) and the run-path move-and-restore case proving the read follows the pin.

**Operator input:** none.

**Dependencies:** none. **This is a decision-path change** — engine-grade review required.

**Definition of done:** all six sites pass the pinned SHA; the run-path fake errors on any
non-SHA ref, so a regression to `info.TargetBranch`/`info.SourceBranch` reddens the existing
suite (non-vacuity); the run-path move-and-restore case reddens if `:274` is reverted to the
branch name; the conformance case is dispatched by `RunSuite`, passes against both the fake
and GitLab backends, and fails against the sabotaged backend; every existing decision outcome
on the shipped fixtures is byte-identical.

**Not in scope:** U-04 items 2–4; the `--checkout` tree binding; the enumeration re-fold;
the record's content digest; any adapter API change.

Requirements:

- **REQ-REV1-S01-01** — Given a live `assent run`, when the MergePolicy, RulesetBinding,
  Config, Pack, governed base and governed head are read, then each read resolves the pinned
  commit SHA (`info.TargetSHA`, or `info.SourceSHA` for the governed head) and never the
  mutable branch name.
  - Test: `cmd/assent/run_test.go` (fake file router serves the SHAs and errors on any other
    ref) + `cmd/assent/run_self_vouch_test.go` (policy-load ref == target SHA)
  - Verify: `go test ./cmd/assent/...`
  - Level: L1
- **REQ-REV1-S01-02** — Given the GitLab adapter's `FileAtRef`, when it is called with a
  commit SHA, then the request carries that SHA as `?ref=` (the adapter does not itself
  rewrite a SHA to a branch name).
  - Test: `internal/forge/gitlab/gitlab_test.go` (TestFileAtRef asserts the SHA ref)
  - Verify: `go test ./internal/forge/gitlab/...`
  - Level: L1
- **REQ-REV1-S01-03** — Given a branch whose tip has moved to bytes other than the pinned
  commit, when `orchestrate` judges the governed file, then the decision is produced from the
  pinned commit's bytes and no forge write is made on a decision that differs from the
  pinned content.
  - Test: `cmd/assent/run_test.go` (`TestRunJudgesPinnedSHAWhenBranchMoves`)
  - Verify: `go test ./cmd/assent/... -run PinnedSHA`
  - Level: L1
- **REQ-REV1-S01-04** — Given the MR source head moves away from the pin and is restored to it
  between evaluation and the merge CAS, when `forge.Reconcile` runs, then the CAS proceeds at
  the pin (the guard cannot distinguish a restore from a never-moved head) — the conformance
  case that documents why the read-pin is load-bearing — and the case is dispatched by
  `RunSuite` and indexed by `catalog.yaml`.
  - Test: `internal/forge/conformance/suite.go` (`sha-guard-source-moved-and-restored`) +
    `internal/forge/conformance/catalog.yaml`
  - Verify: `go test ./internal/forge/conformance/...`
  - Level: L1
- **REQ-REV1-S01-05** — Given the governed-head read is reverted from `info.SourceSHA` to
  `info.SourceBranch`, when the run-path move-and-restore test runs, then it fails
  (non-vacuity: the test measures the pin, not the fake).
  - Test: mutation performed and recorded in the story's review evidence
  - Verify: revert `run.go:285` to `info.SourceBranch`; `go test ./cmd/assent/... -run PinnedSHA` reddens
  - Level: L1
- **REV1-S01-06** — Given the shipped example fixtures, when the full gate runs, then every
  existing decision outcome is unchanged (the fix is strictly stronger pinning; it alters
  behaviour only when the branch name and the pin disagree).
  - Test: existing `cmd/assent` corpus + `task dogfood-examples`
  - Verify: `task check`
  - Level: L2

---

## Paths owned (file-disjoint guidance)

| Story | Owns (write) | Reads only |
| --- | --- | --- |
| REV1-S01 | `cmd/assent/run.go` (six ref arguments only), `cmd/assent/run_test.go`, `cmd/assent/run_self_vouch_test.go`, `internal/forge/gitlab/gitlab_test.go`, `internal/forge/conformance/suite.go`, `internal/forge/conformance/observe.go`, `internal/forge/conformance/backends_test.go`, `internal/forge/conformance/canfail_test.go`, `internal/forge/conformance/catalog.yaml` | `internal/forge/gitlab/gitlab.go`, `internal/forge/forge.go` |

**Integrator-owned, NOT implementer-owned:** `CHANGELOG.md`, `openspec/specs/backlog.md`'s
status column, and `docs/planning/meta-plan.md`'s epic status — shared across lanes.
