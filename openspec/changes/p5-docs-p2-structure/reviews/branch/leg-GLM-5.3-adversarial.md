The mechanical verbatim check of the moved essays shows exactly the two allowed in-text transformations plus the `###→##` relevel. Now reading the full files at HEAD for context.
All checks done. Final persona sweep and report.

## Verdict: CONCERNS
No CRITICAL. Two WARNINGs, both cheap to fix, neither invalidates the story's substance. Verified mechanically: the moved essays diff against their pre-move `cli.md` text in exactly the sanctioned transformations (`###`→`##`, `#assent-doctor` retarget, the "CI snippet above" deixis → `cli.md#assent-run` link); `docs/index.md` equals the branch README plus the spec's three P0 hunks modulo link targets (one mechanical diff, one diff artifact); corpus arithmetic in REQ-DOC2-S03-04 holds (`exitgate_test.sh:262-275`: 11 entries, 10 present, MIN 8, 4-removal mutant → 6 < 8); the anchor sweep finds only the three retargeted references the spec inventories (ADR-0009's pointer confirmed left untouched); all REQ-DOC2-S00/S01/S02/S03 statuses: holds. Saboteur: tried broken links/anchors, un-gated content, corpus escape — one landed (below). Security Auditor: no secrets or denylist material on the new surfaces; placeholder tokens only; sanitization scans tracked files repo-wide so the new pages are covered.

## Findings
- [WARNING] The README↔index.md quick-start copy has no mechanical pin — divergence is invisible to every gate — `docs/index.md:50-87`
  Failure: an edit to README's quick start without the index copy (the same accident E10 already shipped once) leaves every gate green: `readme_smoke_test.sh` executes README only, `truthlag` DOC-05/DOC-11 pin README+install.md only, DOC-02 finds no site URLs in index.md, and S01-04's equality is a one-time diff review. The deliberate `@v0.4.0`-vs-`@v0.1.0` split (spec-settled, PR 190 pending) makes future accidental drift indistinguishable from it; DOC-13 is recorded as future duty but tracked nowhere but spec prose.
  Fix: mirror the existing DOC-06 precedent (truthlag's API-stability mirror pin) — extract README's quick start with the three P0 hunks applied and diff against index.md's copied span modulo link targets.
  Confidence: 85
- [WARNING] The committed TestNoStaleProductClaims evidence is in the vacuous shape the repo's own doctrine forbids — `openspec/changes/p5-docs-p2-structure/evidence/t005-nostale.log:1`
  Failure: the log is a bare `ok ... 0.851s`; `go test -run RE` exits 0 when RE matches nothing, and `exitgate_test.sh:62-63` states the repo standard is parsing `-v` for `--- PASS: <name>` by name. A renamed/absent test would have produced this identical record; the spec's own Verify clause (S03-04) prescribed the weak command.
  Fix: re-record with `go test -v -run TestNoStaleProductClaims` and keep the named `--- PASS:` line; amend the spec delta's Verify to require it. (The test itself does pass — I read it and grepped the walked roots.)
  Confidence: 80
- [NOTE] New gitignore exclusion `**/reviews/**/*.err` widens an exclusion register, with a side effect on the sanitization scan — `.gitignore:19-21`
  Failure: ignored files drop out of `check-sanitization.sh`'s `git ls-files --others --exclude-standard` surface, so a force-added `.err` would skip sanitization; nothing in `hack/` reads these files, and ignoring ANSI/absolute-path transcripts is protective against exactly the D-002 class, so the trade is sound — but it is a register entry the fitness lens says to surface.
  Fix: none needed; if wanted, note the exclusion in the decision log as the sanitization-scope caveat it is.
  Confidence: 90

## Could not check
- No gate was executed by this review (read-only): docs-build, docs-gates, TestNoStaleProductClaims, exitgate, sanitization results are taken from the committed `t005-*.log` records at tip 85eb7c8 (b7cf7cb adds only the evidence files); I verified internal consistency (smoke log's 3-ran/4-skipped shape matches the deduped README; strict build ran with one pre-existing INFO anchor note on `planning/spikes/spike-secure-setup.md`, not touched by this branch).
- The full (no-argument) exitgate toolchain layer — cassettes, `task check`, determinism, schema freeze vs v0.1.0 — was not run; the log's own footer says `--text-only` "certifies NOTHING about the tree's actual state", and the layer cannot currently be green at any tip due to the pre-existing baseline-coverage red (90.5% < 91%, E10), declared out of scope by the orchestrator.
- PR 190's actual README hunks (open GitHub PR) were not fetched; the copy check used the spec's own enumeration of the three hunks applied by hand.
- Mermaid rendering fidelity of the new `docs/index.md` flowchart (config present in `mkdocs.yml:42-46`; visual output unseen).
