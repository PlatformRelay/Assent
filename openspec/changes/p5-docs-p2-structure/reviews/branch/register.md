## Unified verdict: BLOCK  (legs ok: 9/9)

| # | Sev | Claim (one line) | Where | Models | Legs | Conf |
|---|---|---|---|---|---|---|
| 1 | CRITICAL | README↔index.md quick-start duplication is unguarded by any gate and the two copies already diverge (E-hunks, `@v0.1.0` vs `@v0.4.0`; index copy not smoke-executed; DOC-13 closer still open in PR 190) | README.md:22,76 vs docs/index.md:9,60; truthlag_pins_test.sh:170 | GLM, QwenFN, Qwen2.4T | GLM-adv, GLM-fit, QwenFN-spec, QwenFN-sec, Qwen2.4T-spec, Qwen2.4T-adv | 100 |
| 2 | CRITICAL | Operator home path `<redacted: operator home path>` committed ~149× plus temp paths in logs/evidence/task-prompt; sanitization scans denylist/hostnames only | logs/T002.log:524, T004.log:54, t005-build.log:6; task-prompt-T001.md:3 | GLM, QwenFN | GLM-sec, QwenFN-sec | 100 |
| 3 | CRITICAL | Phrase-corpus gate permits silent coverage loss of the new page: MIN=8 for 11 entries (headroom 2) and required-surface loop omits operating-safely.md | hack/audit/exitgate_test.sh:269-275,833 | DeepSeek, GLM | DeepSeek-adv, GLM-fit | 100 |
| 4 | CRITICAL | Employer/internal provider name `<redacted: internal model registry id>` committed in all four logs; CI sanitization runs bare without the local denylist and `host_pat` misses `internal-` prefixes | logs/T001.log:412 (+T002–T004); check-sanitization.sh:21 | QwenFN | QwenFN-sec | 95 |
| 5 | WARNING | Spec's "pointer inventory is three" is false (4th prose pointer in p4-e1-s11-adoption README:27); D-134's stale cli.md pointer also missing from hand-off residuals | spec.md:157-164; decisions.md:141 | GLM, DeepSeek, QwenFN, Qwen2.4T | GLM-sec, DeepSeek-adv, QwenFN-spec, Qwen2.4T-spec | 100 |
| 6 | WARNING | `.gitignore` widens exclusion register `**/reviews/**/*.err`; ignored files drop out of the sanitization scan surface (all legs judged the trade sound) | .gitignore:19-21 | GLM, DeepSeek, Qwen2.4T | GLM-adv, GLM-fit, DeepSeek-spec, Qwen2.4T-spec | 100 |
| 7 | WARNING | Cross-page doc anchors have no fitness function (`validation.anchors` off); a heading rename silently breaks ~6 links — pre-existing class deferred to DOCSNAV-R01 | cli.md:61,103; operating-safely.md:10; walkthrough.md:212 | GLM, Qwen2.4T | GLM-fit, Qwen2.4T-adv | 100 |
| 8 | WARNING | Committed TestNoStaleProductClaims evidence is vacuous (bare `ok`, not a named `--- PASS:` record; a renamed/absent test would look identical) | evidence/t005-nostale.log:1; exitgate_test.sh:62-63 | GLM | GLM-adv | 80 |

Dropped: DeepSeek-adv's operating-safely self-description NOTE (60, one leg); Qwen2.4T-adv's raw-transcript-size NOTE (60, one leg); DeepSeek-spec's lowercase repo link; QwenFN-spec's gate-matrix-one-commit-behind NOTE.

## Disagreements
- Verdict spread on identical evidence: 3 CLEAN (DeepSeek-spec, Qwen2.4T-spec, Qwen2.4T-adv), 4 CONCERNS, 1 BLOCK (QwenFN-sec) — the BLOCK turned only on logs that the other legs passed as "no secrets, placeholders only".
- Committed-logs severity: QwenFN-sec rates the employer name CRITICAL and home paths WARNING; GLM-sec found only the home paths (WARNING); the other three models found neither.
- Corpus floor MIN=8: DeepSeek-adv calls raising it to 10 a defect fix (WARNING); GLM-fit rates it NOTE, verifying the spec's own mutant arithmetic holds and preferring the required-surface fix; three spec legs call S03-04 holds as-specified.
- README↔index copy: the two spec legs hold S01-04 as spec-sanctioned; GLM legs escalate to WARNING because the sanctioned closer (DOC-13, PR 190) is not in this base and the divergence is gate-invisible.

## Nobody could check
- No leg executed any gate (all read-only): docs-build/docs-gates/exitgate/TestNoStaleProductClaims/sanitization accepted from committed t005-*.log evidence, recorded at 85eb7c8 — one commit behind tip b7cf7cb (delta openspec-only, judged immaterial, unverified at tip).
- Full no-argument exitgate layer (cassettes, `task check`, determinism, schema freeze) never run; baseline-coverage red (90.5% < 91%, E10) is pre-existing and declared out of scope.
- PR 190's actual README diff (open PR): every leg verified the three P0 hunks only as the spec describes them; whether DOC-13's scan goes green once it lands is unevaluable.
- Built `site/` HTML: anchor slugs derived from source headings + mkdocs naming rules, never grepped from rendered output; Mermaid rendering of the index.md flowchart unseen.
- Whether the operator's local `ASSENT_SANITIZE_DENYLIST` contains "<redacted>" (deliberately uncommitted; only provable that CI's layer is denylist-free).
