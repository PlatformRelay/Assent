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

Meta-plan **Phase 5 — Epic execution**. Phases 0–4 are complete: the Phase-4 walking-skeleton
exit gate (incl. the D-012 real-repo adoption gate) is CLOSED — engine code is expected now, not
deferred.

**Per-epic status is not restated here.** The epic table in
[`docs/planning/meta-plan.md`](docs/planning/meta-plan.md) is the single authority; read it
there. A duplicated status line in this file is what went stale for a month (DOC-03 / D-176),
and re-pinning that duplication with today's values would only reset its clock. **Not every
tier is available work** — some are still Locked under D-012's named-consumer requirement — so
check the meta-plan and [`docs/decisions/decisions.md`](docs/decisions/decisions.md) before
treating any epic as claimable.

Decompose spec-first (`openspec/` before code) before implementing anything not already
decomposed.
