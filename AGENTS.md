# AGENTS.md — working agreement for AI-assisted development

Read [`docs/vision.md`](docs/vision.md) and [`docs/planning/meta-plan.md`](docs/planning/meta-plan.md)
first each session; check [`docs/planning/open-questions.md`](docs/planning/open-questions.md)
before deciding anything an OQ already covers.

## Hard rules

1. **Open source hygiene**: never commit employer names, internal system names, internal
   policy content, or verbatim material from private codebases (D-002). Sample repos and
   rules are generic, generated equivalents.
2. **Public repo is live**: `github.com/PlatformRelay/Assent` exists (D-001/D-009 satisfied).
   Push to `origin main` only under explicit per-session operator authorization; never
   force-push, rewrite published history, or delete released tags.
3. **Git identity**: `Konrad Heimel <konrad.heimel@gmail.com>` (already set in local config).
   Never add AI co-author trailers. Never change git config beyond this repo.
4. **Spec/test-driven**: specs (`openspec/`) before code; failing test before implementation;
   `task check` green before every commit.
5. **Commits**: `:gitmoji: type(scope): summary` — ASCII gitmoji shortcode, conventional type,
   one logical change per commit.
6. **Decisions are written down**: architecture → ADR (`docs/adr/`, template provided);
   project/process → decision log (`docs/decisions/decisions.md`, D-nnn); unresolved →
   open question (OQ-nnn). Don't decide silently.
7. **Determinism**: nothing probabilistic in the decision path — no LLM calls, no wall-clock
   or randomness dependence in `internal/core` / `internal/change`.

## Current phase

Meta-plan **Phase 5 — Epic execution**. Phases 0–4 are complete, and **E1–E9 have all shipped**
(E9 as `v0.1.0`). The epic table in [`docs/planning/meta-plan.md`](docs/planning/meta-plan.md)
is the authority on per-epic status — this file deliberately does not restate it, because a
duplicated status line here is what went stale for a month (DOC-03 / D-176). Follow-on epics
cut during Phase 5 (**EFE**, **PCS**, **AUD**) and the deferred tier live there too: **E10**
GitHub adapter (unlocked D-140) and **E11** Rego backend (unlocked D-141) are decomposed and
executable; **E12** `serve` and **E13** remote packs are not decomposed. Decompose spec-first
(`openspec/` before code) before implementing anything that is not already decomposed.
