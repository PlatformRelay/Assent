## Verdict: BLOCK

## Findings

- [CRITICAL] An internal model-serving provider ID naming the employer is committed and would ship to the public repo — `openspec/changes/p5-docs-p2-structure/logs/T001.log:412` (`provider=<redacted: internal model registry id>`; the internal alias `galere` also at `T001.log:2,401` and `T002.log:2`, `T003.log:2`, `T004.log:2`)
  Failure: branch merged and pushed per D-001 → public `github.com/PlatformRelay/Assent` history permanently contains an employer/internal-system name, violating AGENTS.md hard rule 1 / D-002. The "sanitization green" claim is real but vacuous here: the employer-catching layer is the workspace-local `$ASSENT_SANITIZE_DENYLIST`, which CI never sets (`.github/workflows/verify.yaml:106` runs the script bare) and which is designed never to be committed; the built-in `host_pat` (hack/check-sanitization.sh:21) only matches `.internal`-suffixed hostnames, not hyphenated `<redacted: internal model registry id>`. The gate passed *with the violation in the tree* — that is the proof.
  Fix: redact the provider/alias lines from `logs/T00*.log` (or don't commit the transcripts, as `.err` files are now excluded), and add one built-in pattern to hack/check-sanitization.sh matching `internal-`-prefixed provider tokens so the ratchet catches the next one without the local-only denylist.
  Confidence: 95
- [WARNING] 156 committed absolute paths `<redacted: operator home path>/...` leak the operator's macOS username into the public repo — `openspec/changes/p5-docs-p2-structure/task-prompt-T001.md:3`, `logs/T004.log` (54 hits), `evidence/t005-build.log:1`
  Failure: push → public git history ties the OSS project to the maintainer's personal machine identity (the `.treehouse` workdir names also survive). The new `.gitignore:19-20` entry justifies excluding `.err` transcripts precisely because they contain "absolute paths" — the committed `.log`/task-prompt files have the identical defect, so the rationale is applied inconsistently.
  Fix: `sed 's|<redacted: operator home path>|~|g'` over the committed `logs/`, `evidence/`, `task-prompt-*.md` before push; same for any future evidence commit.
  Confidence: 90
- [NOTE] The de-dup leaves two executed-looking copies of the quick-start with divergent facts — `docs/index.md:60` says `@v0.4.0`, `README.md:76` says `@v0.1.0`, and index.md's inherited sentence "that is the fixture `hack/docs/readme_smoke_test.sh` executes this block against" is only true of the README copy: the script parses README.md alone (`hack/docs/readme_smoke_test.sh:4,61`).
  Failure: a reader copies the index.md block believing it gate-executed; the two copies drift exactly as the E10 merge duplication did — but no machine check runs the index.md copy.
  Fix: reword the index.md sentence to say the README block is the executed one (or extend the smoke test's section extraction to docs/index.md).
  Confidence: 80
- [NOTE] Trust-model claims that moved to `docs/usage/operating-safely.md` are faithful to the code, i.e. the move did not weaken a security statement: `-arm` "advisory only — it gates nothing" matches `cmd/assent/run.go:161`; symlink refusal (fail-closed, no forge writes) is real at `internal/provider/builtin/repo_file.go:132`; `binding-require-empty` exists under `internal/lint`. Phrase-corpus retarget at `hack/audit/exitgate_test.sh:270` keeps the fail-closed phrasing pinned in the new home.
  Confidence: 90

## Could not check
- Whether the operator's local `ASSENT_SANITIZE_DENYLIST` contains "<redacted>" — the file is deliberately non-committed and out of bounds; only provable that CI's sanitization layer is denylist-free and so could not catch it.
- Full manual read of all ~3,800 lines of committed logs/review transcripts; relied on pattern greps (token shapes, base64 runs, hostname/empid patterns, employer/system-name greps) — no secret values found, only the identifiers above.
- The CI run of the gate matrix at HEAD itself; took the claimed green results as given.
