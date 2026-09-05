# P5-REDMAIN-N — changelog classifier integrity: literal-emoji commit subjects

**Epic ID / REQ prefix:** `REDMAIN-N` / `REQ-REDMAIN-N<n>-<nn>`.

**Origin:** the session INBOX entries `REDMAIN-N1` and `REDMAIN-N2`, both filed 2026-09-03 by
the `REDMAIN-F01` reviewer and both left unclaimed. The INBOX states the coupling explicitly —
*"fixing the detector without also rejecting literal emoji at commit time leaves the door open,
and vice versa. One lane should own both."* This spec is that lane.

**Decision:** [D-168](../../../docs/decisions/decisions.md).

---

## Problem

One defect with two halves. Both were verified in-tree at `20c80cb` before anything was
written; neither is inferred from the INBOX prose.

### Half A — the `### Other` detector is fail-open for the shape that causes the bug

`hack/release/changelog_gate_test.sh` §8 exists to forbid an entry that declares a **fileable**
conventional type from rendering in the catch-all `### Other` group — the REL-14 / D-137 defect.
Its detector was:

```
grep -nE "^- :[a-z0-9_]+: ($FILEABLE_TYPES)[(:]"
```

The leading `:[a-z0-9_]+: ` is **mandatory** in that pattern, so the detector can only see an
entry whose subject carries an ASCII gitmoji shortcode. Rendering the changelog at `20c80cb`
puts three entries under `### Other`:

```
- 👷 ci(docs): stop uploading the Pages artifact on pull requests
- :rewind: revert(kind): defer durable lab; authorize via D-038
- :test(release): add CI audit gate for single CodeQL workflow
```

The first declares the fileable type `ci` and is exactly what §8 forbids, but it begins with a
**literal emoji**, so the pattern cannot match it. `### Other` grew 2 → 3 entries while §8 kept
printing its hardcoded prose *"revert + one malformed subject"*. A guard that is blind to the
one subject shape that produces the defect is not a guard.

### Half B — nothing rejects a literal-emoji subject before it is published

`GUIDELINES.md` § Repository discipline mandates the **ASCII shortcode**
(`:construction_worker:`, not `👷`). `cliff.toml`'s parsers key on that shortcode, so a
literal-emoji subject matches none of them and falls through the `.*` catch-all into `### Other`
on the published GitHub Release page — the REL-14 defect through a different door. There is no
commit-message gate anywhere in `hack/**` or `.github/workflows/**`: the rule was enforced by
human attention only, and `dfdae69` is the commit where that failed.

`dfdae69` is published history and hard rule 2 forbids rewriting it, so the remedy must tolerate
it explicitly rather than pretend it is not there.

## Scope / non-goals

- **In scope:** a gate that rejects a commit subject whose first character is non-ASCII, and a
  §8 detector that sees a fileable type behind an ASCII shortcode, behind a literal emoji, or
  behind no prefix at all.
- **Not in scope:** re-filing `dfdae69`'s rendered entry out of `### Other`. That needs a
  `cliff.toml` parser entry and `cliff.toml` is owned by another lane in this wave; the entry is
  exempted here by **commit SHA**, and the exemption is built to red the moment it stops being
  needed (REQ-REDMAIN-N2-02).
- **Not in scope:** enforcing the *whole* of the commit convention (a conventional type, a
  scope, a shortcode present at all). `build(deps): bump …` from Dependabot and `Merge pull
  request …` from GitHub both violate the full rule and both are legitimate; the narrow rule
  "the subject must not START with a non-ASCII character" is the one with zero false positives
  over all 893 commits of this repo's history and is the one that breaks the classifier.
- **Not in scope:** a new `task check` stage. The stage list is pinned by `CHECK_STAGES` in
  `hack/audit/exitgate_test.sh`, which asserts the Taskfile's `check:` list is *equal* to it — a
  22nd stage is a change to that pin, and it is out of this lane's fence. Both halves therefore
  reach `task check` through the existing `release-changelog-gate-test` stage, which is where
  §8, the sibling half of the same defect, already lives.

## Requirements

- **REQ-REDMAIN-N1-01** *(commit subject gate · both polarities)* — a commit whose subject's
  first character is not ASCII is rejected by a gate, not by a human. **Given** a repository
  whose history contains a commit with a literal-emoji subject, **when** the gate runs, **then**
  it exits non-zero and names the offending SHA and subject; **given** a history whose subjects
  are all ASCII-leading, **then** it exits 0. The gate self-validates its own detector on every
  run against a fabricated known-bad and known-good subject, so a mistyped pattern fails loudly
  instead of matching nothing. Test: `hack/release/commit_subject_gate.sh` (new) with its
  polarity proof in `hack/release/changelog_gate_test.sh` §9; Verify: `bash
  hack/release/changelog_gate_test.sh`; Level: L1
- **REQ-REDMAIN-N1-02** *(reachability)* — the gate runs where a new commit is actually
  visible: as a step of `verify.yaml`'s `verify:` job (which, unlike the changelog drift gate,
  is **not** guarded off `pull_request`, so a lane's literal-emoji commit reds its own PR), and
  locally inside `task check` via the `release-changelog-gate-test` stage. Deleting either
  wiring reds the gate. Test: `hack/release/changelog_gate_test.sh` §9 wiring assertions +
  mutation; Verify: `bash hack/release/changelog_gate_test.sh`; Level: L1
- **REQ-REDMAIN-N1-03** *(published history is tolerated by SHA, never by shape)* — the
  exemption for pre-existing history is a list of **commit SHAs**, so no future commit can
  inherit it. Each exempt SHA must resolve in this repository, be an ancestor of `HEAD`, and
  **itself be a real detection** — an exempt SHA that is not a violation reds the gate as a
  stale exemption. Test: `hack/release/commit_subject_gate.sh` self-checks + §9 mutation (strip
  the exemption ⇒ the gate reds naming `dfdae69`); Verify: `bash
  hack/release/changelog_gate_test.sh`; Level: L1
- **REQ-REDMAIN-N2-01** *(the `### Other` detector sees every prefix shape)* — an entry
  rendered under `### Other` that declares a fileable conventional type is reported regardless
  of whether the type is preceded by an ASCII gitmoji shortcode, by a literal emoji, or by
  nothing. **Given** a rendered `### Other` block containing `- 👷 ci(docs): …`, **when** §8's
  detector runs, **then** that line is reported; the pre-fix detector is shown, in the same
  section, not to report it. Test: `hack/release/changelog_gate_test.sh` §8 + §8b; Verify: `bash
  hack/release/changelog_gate_test.sh`; Level: L1
- **REQ-REDMAIN-N2-02** *(the legacy exemption is self-retiring)* — §8's exemption for
  `dfdae69`'s rendered entry is derived from the SAME SHA list as REQ-REDMAIN-N1-03 (one
  authority, not two), and an exempt subject that no longer appears under `### Other` reds the
  gate with "remove the exemption". So when `cliff.toml` gains the parser entry that files this
  line under Chores, the scaffolding is forced out rather than left behind. Test:
  `hack/release/changelog_gate_test.sh` §8; Verify: `bash
  hack/release/changelog_gate_test.sh`; Level: L1

## Verification

```
mise exec -- task check          # 21 stages; stage 11 (release-changelog-gate-test) carries §8/§8b/§9
bash hack/release/commit_subject_gate.sh
```
