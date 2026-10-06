## Verdict: CLEAN

## Findings
- [NOTE] T002 tick absent from `tasks.md` while evidence claims it in scope — `openspec/changes/p5-docs-p2-structure/tasks.md:37` vs `evidence/T002.md:6`
  Failure: the commit `d91b2de` touches only `docs/index.md` + `evidence/T002.md`; T002's `- [x] — closed …` line is missing, unlike T001 whose tick landed in its own commit (`247187a`). A reader of `tasks.md` sees T002 still open after it is declared IMPLEMENTED.
  Fix: add the `- [x] — closed 2026-10-07, evidence: evidence/T002.md` line under `## T002`, or drop the tick from the evidence's "Files in scope".
  Confidence: 60 (may be intentionally deferred to close, but the evidence calls it in-scope now).
- [NOTE] Evidence's wording-check exclusion list is incomplete — `evidence/T002.md:45`
  Failure: row 4 documents only two exclusions (Read-the-docs paragraph; trailing blank lines) and claims the copied body is "byte-identical to the pinned sources modulo link targets". A third selection boundary exists and is unlisted: README's `Developers: gates live in the Taskfile` block (`README.md:105–109`, branch quick-start content) is deliberately not copied. The claim is true of what was copied, but a reviewer relying on the exclusion list cannot tell the Taskfile block was dropped on purpose rather than forgotten.
  Fix: name the Taskfile block as exclusion (C) in row 4, or state the copy region's end boundary explicitly.
  Confidence: 80 (block is verifiably absent from `docs/index.md`; S01-01's content list omits it, so the omission itself is spec-compliant).

## What I verified (not merely read)
- **Copy fidelity.** Reconstructed the pinned source (pr-190 `README.md` lines 21–30 + 35–92, then branch `README.md:93–103`) with all `](url)` targets stripped; diffed against `docs/index.md:8–87`. Only difference is one trailing blank line — matches the documented normalisation. The three P0 hunks are present: `E1–E9` (`index.md:9`), the `.tf` opaque clause (`index.md:29`), `@v0.4.0` (`index.md:60`); latest tag is `v0.4.0` (`git tag`).
- **Links.** Every target exists: `api-stability.md`, `adr/{0003,0014,0015,0021,…}.md`, `adr/README.md`, `architecture/c4-{context,container}.md`, `usage/{install,walkthrough}.md`, `planning/{meta-plan,open-questions}.md`, `decisions/decisions.md`, `vision.md`, both brand SVGs. `planning/**` is covered by `not_in_nav` (`mkdocs.yml:98`); `api-stability.md` is in nav (`:90`); mermaid fence is configured (`:42–46`) as on the existing C4 pages.
- **Requirements.** REQ-DOC2-S01-01/03 hold by file read (`index.md:8–100`); S01-02 targets all resolve; S01-04 wording equality holds modulo permitted link retargets. Hero+H1 kept, single H1, old two-sentence intro gone, meta-plan/open-questions moved to the Contributing tail, no leftover duplicate.
- **Sensors.** `bash hack/check-sanitization.sh` → exit 0 "sanitization check passed" (the four null-byte warnings are the script's own base64 scan, pre-existing). None of the 7 exitgate phrases (`hack/audit/exitgate_test.sh:850–857`) appears in `docs/index.md`; the scoped `not yet implemented` rule excludes index.md. Corpus canary token `assent` present.
- **Fitness.** No allow-list/exclusion/baseline widened: `PHRASE_CORPUS`, `not_in_nav`, pins untouched. The change adds deliberate README↔index.md duplication with no new ratchet; the drift cost is recorded (`evidence/T002.md:71,78`) and deferred to PR 190's DOC-13 scan — acceptable per spec, but see the ratchet note.

## Could not check
- `task docs-build` / `task docs-gates` / `readme_smoke_test.sh` / `truthlag_pins_test.sh` / `exitgate_test.sh` — not executed (read-only session; `docs-build` writes `site/`). Link existence and phrase absence checked by hand instead.
- The scratch wording-check script (`/var/folders/…/opencode/`) — not in-repo, so its "WORDING-CHECK: identical" result is not reproducible; I re-derived it independently.
- The T002 review register (`reviews/L-tasks/T002/`) — not read (other reviewers' output).
