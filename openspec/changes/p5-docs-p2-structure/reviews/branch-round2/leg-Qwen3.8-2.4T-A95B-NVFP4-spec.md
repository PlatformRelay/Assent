## Verdict: CONCERNS

Spec coverage (REQ-DOC2-S00..S03, all verified against files, not claims):
- S00-01 HOLDS — `README.md:75-109`: diff is a pure 20-line deletion; caveat, lint/test, sample-repo, forge, GitHub-note, task-check each once, in order; no other README bytes changed.
- S01-01 HOLDS — `docs/index.md:8-100`: banner, Why(3 bullets), How-it-works(mermaid+stateless), Quick start, Start here, Contributing, top to bottom.
- S01-02 HOLDS (static) — all 13 link targets exist on disk; `planning/**` builds via `not_in_nav` (`mkdocs.yml:99-105`); strict build not re-run by me.
- S01-03 HOLDS — `docs/index.md:89-100`: product pages in Start here, meta-plan/open-questions in Contributing.
- S01-04 HOLDS — `docs/index.md:9` (E1–E9), `:29-30` (`.tf` opaque clause), `:60` (`@v0.4.0`), `:77-87` (E10-era forge-selection + GitHub comment-only carried as-is).
- S02-01 HOLDS — `mkdocs.yml` Usage: Install → Walkthrough → CLI reference (+ Operating safely after, per S03).
- S03-01 HOLDS — I ran the normalised diff myself: `3503cc0:cli.md:98-251` vs `operating-safely.md:10-163` differs only in 5 heading demotions, `#assent-doctor`→`cli.md#assent-doctor`, and the "CI snippet above" deixis → `[`assent run` invocation](cli.md#assent-run)`. Exactly the two sanctioned transformations.
- S03-02 HOLDS — `cli.md:88-91`: `-arm`/`-checkout`/`-pack` rows carry caveat+link, `--pack` targets `#how-to-keep-assent-advisory`; `--config` row untouched; pointer paragraph `cli.md:98-101`.
- S03-03 HOLDS — `cli.md:114` retargeted without "above"; `walkthrough.md:212` retargeted; `git diff 3503cc0..HEAD` over ADR-0009, p4-e1-s11 evidence README, decisions.md is EMPTY and all three pointer lines (incl. D-134's, `decisions.md:141`) verified present.
- S03-04 HOLDS — `exitgate_test.sh:270` corpus entry lands in the same commit as the page (`8fffed0`); arithmetic checks out: 11 listed, 10 present (`quickstart.md` dead), MIN 8; `shrunk` removes 4 → 6 < 8; required-surface `:831`, mutant `:1924`.
- Round-1 register fixes verified: C1 deferred+recorded (`loop.md:120`), C2/C4 untracked+ignored (no `logs/`/`task-prompt`/`evidence/*.log` in `git ls-files`), C3 fixed, inventory-of-five in spec+proposal, named nostale `-v` record (`evidence/T005.md:13`), emittable smoke quote.
- Nothing contradicted, absent, or harmful-stronger-than-spec; the required-surface addition exceeds S03-04's letter but is tightening-only.

## Findings
- [WARNING] C4's string still ships: employer/internal provider id and home path remain verbatim in tracked record rows — `loop.md:121,123`, `reviews/branch/register.md:6,8`, `reviews/branch/leg-Qwen3.8-Flash-Next-security.md:5-11`
  Failure: branch pushed per hand-off → public history contains `<redacted: internal model registry id>/...` and `<redacted: operator home path>`, violating AGENTS.md rule 1/D-002; `check-sanitization.sh` scans neither (hardening deferred to hand-off), so no gate reddens.
  Fix: redact both literals in the record rows (`internal-<redacted>`, `/Users/<operator>`) before push; findings stay identifiable.
  Confidence: 80 (`loop.md:123` argues quoted-pattern precedent, but that precedent quotes product phrases, not employer identifiers; not handled by any gate)
- [NOTE] The six new cross-page anchor links have no ratchet (`validation.anchors` = info); anchor presence rests on T004's one-time grep of built HTML (`evidence/T004.md:158`) — pre-existing class, deferred to DOCSNAV-R01 (`loop.md:126`)
  Failure: future heading rename in operating-safely.md → all six links rot silently, build stays green.
  Fix: none in this branch (fenced); keep the deferral visible at push.
  Confidence: 90
- [NOTE] Committed gate matrices trail HEAD: T005 records tip `85eb7c8`, T006 sensors ran at `a74076e`; HEAD `d003e96` adds the mutant rm line (`exitgate_test.sh:1924`) re-run only per `loop.md:130-132`
  Failure: if the extended mutant were mis-wired, no committed evidence row would show it.
  Fix: one fresh `--text-only` row at HEAD (orchestrator asserts green; delta is one `rm -f` arg).
  Confidence: 40
- Beyond any requirement: `.gitignore` exclusion classes (W6-accepted), required-surface entry, `operating-safely.md:7` walkthrough link, openspec process artifacts. No gate reads `openspec/**`.

## Could not check
- Gates not re-run at HEAD (read-only plan mode): docs-build, docs-gates, TestNoStaleProductClaims -v, exitgate --text-only, sanitization — accepted from T005/T006 evidence + orchestrator claim.
- Built `site/` HTML and index.md mermaid rendering (anchor record taken from `evidence/T004.md:158`).
- PR 190's actual README diff (open, external) — the three P0 hunks verified only as the spec describes and index.md carries them.
- Origin report `data/assent-docs-audit/report.md` and the sibling PR-190 tree; untracked `reviews/branch-round2/` (this round's output dir).
