## Unified verdict: BLOCK  (legs ok: 4/5 — `Qwen3.8-2.4T-A95B-NVFP4-adversarial` exit=143, empty file)

| # | Sev | Claim (one line) | Where | Models | Legs | Conf |
|---|---|---|---|---|---|---|
| 1 | CRITICAL | Round-1 C2/C4 leak not closed: blobs still reachable at ancestor `b7cf7cb` and literals tracked in 4 review records; push + non-draft PR (`loop.md:78`) ships `<redacted: operator home path>` + `<redacted: internal model registry id>` into public history | ancestor `b7cf7cb` (`logs/T001,T004.log`); `loop.md:121,123`; `reviews/branch/register.md`; `reviews/branch/leg-{GLM-5.3,Qwen3.8-Flash-Next}-security.md` | 4 | DeepSeek-spec, GLM-adv, Qw2.4T-spec, QwFN-adv | 100 |
| 2 | CRITICAL | Sanitizer blind spot: `host_pat` misses hyphenated `internal-*` and no home-path pattern; CI runs it bare — no gate can redden these strings (proof: gates passed with violation in-tree) | `hack/check-sanitization.sh:24,77`; `.github/workflows/verify.yaml` | 4 | DeepSeek-spec, GLM-adv, Qw2.4T-spec, QwFN-adv | 100 |
| 3 | WARNING | Disposition cites phantom commit `e9a1ca6` (`git cat-file` fails); real fix is `49d573f` — unauditable record | `loop.md:121` | 2 | DeepSeek-spec, QwFN-adv | 100 |
| 4 | WARNING | Committed gate evidence trails tip: T005 at `85eb7c8`, T006 at `a74076e`, HEAD `d003e96`; post-evidence re-run claim lives only in loop prose | `evidence/T005.md:6`, `T006.md`; `loop.md:130-132` | 2 | Qw2.4T-spec, QwFN-adv | 85 |
| 5 | WARNING | Exitgate per-member coverage unpinned (disclosed, accepted deferral): deleting one non-required page or dropping one required entry reddens nothing | `hack/audit/exitgate_test.sh:275,830-831` | 2 | GLM-adv, QwFN-adv | 100 |
| 6 | WARNING | `.gitignore` widened by 4 patterns + scanner excludes ignored files → future secrets in evidence/logs leave the scan surface silently | `.gitignore:21,25,26,29`; `check-sanitization.sh:77` | 1 | DeepSeek-spec | 80 |
| 7 | WARNING | C1/sanitizer deferrals name a closer (hand-off) that is not yet written (`[ ] Hand-off` unchecked) — only record would be loop prose if merged now | `loop.md:78,120` | 1 | QwFN-adv | 80 |
| 8 | WARNING | Site home false deixis: index.md says the smoke test executes *index.md's* block; it executes only the README's — spec missed a third deixis case | `docs/index.md:82` | 1 | GLM-adv | 75 |

## Disagreements
- Verdict split 3×BLOCK vs 1×CONCERNS: Qw2.4T-spec rated the same leak WARNING(80) where the other three rated CRITICAL(70–95).
- C1/sanitizer deferral status: QwFN-adv says the named closer (hand-off file) doesn't exist; GLM-adv/Qw2.4T-spec say it's recorded and findable (`loop.md:106-108,120`).

## Nobody could check
- No gate re-run at HEAD `d003e96` (all read-only): docs-build, docs-gates, `exitgate --text-only`, `TestNoStaleProductClaims -v`, sanitization — taken from evidence that trails tip.
- Rendered `site/` HTML: anchor slugs never grepped from built pages (`validation.anchors` = info, rot deferred to DOCSNAV-R01); index.md mermaid rendering unseen.
- PR 190's actual README diff (open, external): three P0 hunks verified only as spec/evidence describe them; DOC-13 green-on-land unevaluable.
- Operator's local `ASSENT_SANITIZE_DENYLIST` contents (deliberately uncommitted); whether any other ref/worktree holds the leaked files.
- Origin report `data/assent-docs-audit/report.md` — outside the repo, unread.

Spec compliance itself was independently verified as holding (REQ-DOC2-S00..S03, all four legs): the block rests entirely on findings 1–2, not on the docs delivery.
