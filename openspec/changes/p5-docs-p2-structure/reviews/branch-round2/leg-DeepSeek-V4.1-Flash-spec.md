## Verdict: BLOCK

The docs delivery itself holds — I read every REQ against the tree — but the branch as it stands must not be pushed: the round-1 transcript leak is *not* closed, it is only hidden from the tip.

## Findings

- [CRITICAL] Round-1 C2/C4 are **not closed**: untracking in `49d573f` removes the transcripts from the tip tree, but the leaked blobs remain reachable in ancestor `b7cf7cb` (`git cat-file -p b7cf7cb:openspec/changes/p5-docs-p2-structure/logs/T004.log` → 54× `<redacted: operator home path>`; `logs/T001.log` → `<redacted: internal model registry id>`). `loop.md:78` says "push branch, open NON-DRAFT PR", so the leak ships to public `github.com/PlatformRelay/Assent` (AGENTS.md hard rule 1 / D-002).
  Failure: `git push origin fm/assent-docs-p2-structure` → the home paths and employer alias become public in history, irreversibly, exactly the outcome C2/C4 claimed to fix.
  Fix: rewrite the branch before any push — squash/drop `b7cf7cb` (or `git filter-repo`) so no reachable commit carries the strings; then re-scan `git rev-list HEAD` blobs, not just the tip; add the home-path and `internal-<name>` patterns to `hack/check-sanitization.sh:24` as the durable fix.
  Confidence: 95

- [WARNING] The sanitizer blind spot the leak exposed is unaddressed: `hack/check-sanitization.sh:24` matches `*.corp|*.internal|…` but not `<redacted: internal model registry id>`, and has no home-path pattern; `.github/workflows/verify.yaml` runs it bare (no denylist). The fix relies on the file being absent, not on the scanner catching it.
  Failure: the same class of string committed to any *tracked* path (a `.md`, a script) still passes the gate.
  Fix: add `home_pat` + widen `host_pat` to `internal-[A-Za-z0-9-]+`; this is the loop.md:121 "hand-off proposal" — land it here, not later.
  Confidence: 85

- [WARNING] `.gitignore:21,25,26,29` widens the exclusion register by four patterns, and `hack/check-sanitization.sh:77` (`git ls-files --cached --others --exclude-standard`) excludes ignored files, so anything matching leaves the scan surface entirely. The exclusion register only shrinks.
  Failure: a future `openspec/changes/*/evidence/*.log` (or `reviews/**/*.err`) holding a real secret is never scanned and never committed — the gate is silent by construction.
  Fix: narrow the patterns to the exact leaked filenames, or have the pre-commit path scan ignored-but-present files under those dirs.
  Confidence: 80

- [NOTE] `loop.md:121` cites the untrack fix as commit `e9a1ca6`, which does not exist (`git cat-file -t e9a1ca6` → fatal); the real commit is `49d573f`. A disposition record pointing at a phantom SHA cannot be audited.
  Fix: correct the SHA.
  Confidence: 100

Spec lens (verified in code, not from the docs): REQ-DOC2-S00-01 holds (README:65–109 once each, in order); S01-01..04 hold (`docs/index.md:8–87`, three P0 hunks applied — E1–E9, `.tf` clause, `@v0.4.0`); S02-01 holds (`mkdocs.yml` Install→Walkthrough→CLI ref, Operating safely appended); S03-01..04 hold (moved text verbatim modulo the two named transforms, `cli.md` rows relinked, three pointer files byte-unchanged, page in corpus + required-surface). The S03-04 mutant arithmetic is **correct**: 11 listed, 10 present (quickstart.md dead), 4 removed → 6 < MIN 8 (`hack/audit/exitgate_test.sh:262–275,830,1928`). The five-pointer inventory is complete — my independent sweep found no sixth pointer. The branch delivers the spec; it is the unclosed leak, not the docs, that blocks.

## Could not check
- Ran no gate (read-only): accepted docs-build/docs-gates/exitgate --text-only/sanitization/TestNoStaleProductClaims green from `evidence/T005.md`/`T006.md`, which were recorded at `49d573f`/`a74076e`, one to three commits behind tip `d003e96`; the tip delta is openspec-only, judged immaterial but unverified at tip.
- Built `site/` HTML anchors (`#what-gates-approve-and-merge`, `#symlinks-in-the-checkout-tree`, `#checkout-less-runs-and-enumeration-completeness`) derived from headings + mkdocs slug rules, never grepped from rendered output; mermaid rendering of `docs/index.md:36` unseen.
- PR 190's actual README diff (open PR): the three P0 hunks verified only as the spec/evidence describe them; whether DOC-13's scan goes green once it lands is unevaluable.
- `data/assent-docs-audit/report.md` (the origin report) is outside this repo; not read.
