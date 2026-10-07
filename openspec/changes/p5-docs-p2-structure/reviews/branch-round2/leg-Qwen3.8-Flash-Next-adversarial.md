## Verdict: BLOCK

## Findings
- [CRITICAL] Round-1 C2/C4 fix is tree-only: the leak still ships on the branch's own delivery path (push + non-draft PR, `loop.md:78`) — `openspec/changes/p5-docs-p2-structure/logs/T001.log` (at `b7cf7cb`, 26 hits) and `docs/usage/operating-safely.md`-era records — `hack/audit/exitgate_test.sh` untouched
  Failure: `git show b7cf7cb:.../logs/T001.log` still returns 26× `<redacted: operator home path>` and `provider=<redacted: internal model registry id>`; `git ls-remote` confirms the branch is unpushed, so nothing is public **yet** — but opening the PR publishes those commits permanently as `refs/pull/N/head` even under squash-merge, violating AGENTS.md rule 1 / D-002. Worse, literal strings are tracked at HEAD itself: `loop.md:121,123`, `reviews/branch/leg-GLM-5.3-security.md:17`, `reviews/branch/leg-Qwen3.8-Flash-Next-security.md:5-11`, `reviews/branch/register.md:6,8` carry `<redacted: operator home path>` and `<redacted: internal model registry id>`; `check-sanitization.sh:24` `host_pat` matches `.internal` suffixes only, and CI runs it without the local denylist — so every gate is green *with the strings in the tree*, which is the proof the sensor cannot catch it.
  Fix: before push (allowed — history is unpublished, so rule 2's no-rewrite bar does not apply): recreate `b7cf7cb` without `logs/`, `task-prompt-*.md`, `evidence/t005-*.log`, and replace the literal strings in the four tracked records with placeholders (`/Users/<op>`, `internal-<provider>/…`) — the register stays intelligible without the verbatim identifiers.
  Confidence: 95
- [WARNING] C1's deferral points its closer at records that do not exist yet — `openspec/changes/p5-docs-p2-structure/loop.md:78,120`
  Failure: four dispositions ("recorded in the hand-off": README↔index sync pin, sanitizer hardening, ADR-0009/`p4-e1-s11`/D-134 residuals) reference a hand-off that is not written (`[ ] Hand-off` unchecked; no hand-off file in the change dir); if the branch merges without it, the only record of the gate-invisible README↔index prose divergence and the sanitizer gap is loop prose a future agent won't read. Partially mitigated and verified live: PR 190's DOC-13 file list *does* include `docs/index.md` (checked its diff) and the copied `@v0.4.0` equals the live latest tag, so the version-pin half has a real closer.
  Fix: write the hand-off file with those four residuals before opening the PR; it is one file.
  Confidence: 80
- [NOTE] Fix dispositions cite commit `e9a1ca6` which exists nowhere — `openspec/changes/p5-docs-p2-structure/loop.md:121`
  Failure: `git cat-file -t e9a1ca6` fails on all refs; an auditor tracing the C2 fix lands on a phantom (real fix is `49d573f`).
  Fix: point the row at `49d573f`.
  Confidence: 95
- [NOTE] The new required-surface entry's specificity is unpinned — `hack/audit/exitgate_test.sh:830-831`
  Failure: dropping `docs/usage/operating-safely.md` from the required list reddens nothing — the extended `missing-required` mutant still reds via `walkthrough.md`; disclosed and deferred in `evidence/T006.md:176-178`, so this is an accepted hole, not a hidden one.
  Fix: optional when `hack/audit/README.md` control counts are next touched: one per-entry mutant.
  Confidence: 85
- [NOTE] T005's matrix is recorded at `85eb7c8` but `exitgate_test.sh` changed twice after it (`a74076e`, `d003e96`); the re-run claim lives only in `loop.md:132`, not a matrix row — `openspec/changes/p5-docs-p2-structure/evidence/T005.md:6`. I verified the changed clause by reading: corpus 11 listed/10 present, missing-required removes 2 → required loop red, shrunk removes 4 → 6 < 8 — arithmetic sound.
  Confidence: 70

Closing status of round-1: C3, W5, W8 and the smoke-quote cell are **closed and verified in source** (required list + corpus + extended mutant; inventory now five in spec *and* proposal at `d003e96`; T005 row has the named `-v` record with matching Command cell and the literal smoke line — `readme_smoke_test.sh:126` emits exactly that form). The essay move is verbatim except the 8 spec-sanctioned transform lines (mechanically diffed); the three immutable pointer files are byte-unchanged; walkthrough `:212` and the cli.md anchors retarget correctly; S00/S01-01..04/S02-01/S03-01..04 read as holding or green-by-record. REQ-DOC2-S00..S03 would deliver as specced **except** the D-002 exposure, which the spec's own gates cannot see.

## Could not check
- Re-ran nothing: plan-mode read-only, so `docs-build`/`docs-gates`/`exitgate --text-only`/`TestNoStaleProductClaims`/`sanitization` greenness is taken from `evidence/T005.md`+`T006.md` + the brief's assertion, spot-verified only at source level (the mutation-clause logic above read by hand, not executed).
- Built `site/` HTML: anchor slugs and the index.md mermaid flowchart rendering never seen; `validation.anchors` is `info` (deferred to DOCSNAV-R01), so broken anchors would build green.
- PR 190's walkthrough hunks: confirmed disjoint at ±212 from its diff header contexts, but did not trial-merge either order.
- The operator's local `ASSENT_SANITIZE_DENYLIST` contents; whether any other ref/worktree contains the leaked files; sibling/workbench records (out of bounds).
