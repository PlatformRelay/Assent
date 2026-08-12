# Engineering guidelines

Standing rules for all assent development — human or agent. ADRs decide *what*; this file
holds the *how*. Conflicts: ADRs win; update this file via PR when they do.

## Safety invariants (never trade away)

1. **Default-deny**: an empty, broken, or non-matching policy set never auto-merges anything.
   Every change must be positively vouched (ADR-0007); `unclassified`/`opaque` never match a
   vouch rule.
2. **Fail-safe direction**: every error path (predicate error, missing fact, provider down,
   unparseable file, capability gap) degrades toward REVIEW/BLOCK, never toward APPROVE
   (ADR-0007 amendment, ADR-0004 amendment).
3. **Trust boundary**: decision inputs that *decide* (policies, config, bindings, templates)
   load from the target ref; the MR branch supplies only the material under judgment
   (ADR-0015). Never blur this line "for convenience".
4. **Write actions re-verify**: approve/merge are SHA-pinned; fail closed on drift
   (ADR-0015 §2).
5. **Determinism**: no clock, randomness, env, or network in `internal/core` /
   `internal/change`; facts pre-resolved; CEL cost-budgeted; every golden test double-runs
   and diffs (ADR-0006, ADR-0013).

## Testing (D-010)

- TDD: failing test first, always. One logical change per commit; `task check` green before
  every commit (includes the D-010 coverage gate on `internal/…`; the floor is the
  `COVERAGE_MIN` var in `Taskfile.yml`'s `coverage:` task — currently 91%, raised by D-128 —
  and CI runs that same task, so there is only ever one number to change).
- Test at the level that gives the proof: golden decision tests (L0) for engine semantics;
  the adopter harness (L1) for policy behavior; cassettes (L2) for adapters; real GitLab
  (L3, `//go:build e2e`) for forge semantics — never mocks for thread/approval/merge flows.
- Every security-relevant fix ships with an adversarial test (e.g. "MR edits its own
  policy → BLOCK", near-threshold rename pairs), not only the happy path.
- Run `go mod tidy` and `go vet -tags e2e ./...` before push — CI scans all build tags.

## Contracts

- The public contracts (PolicyInput, Decision/Findings, Provider, Forge conformance, test
  fixture format) change **only** via an openspec change proposal + version bump. No silent
  schema drift; serialized forms always carry `apiVersion`.
- Every exported error message that can reach an MR comment must be understandable by the
  contributor persona — no Go internals, no stack traces (ADR-0012).

## Dependencies

- Prefer stdlib; new deps need: active maintenance (commits within 6 months), >1 effective
  maintainer, compatible license (Apache-2.0/MIT/BSD), and no heavyweight transitive tail
  into the single static binary (ADR-0001, ADR-0013's dependency-health bar).
- SHA-pin CI actions; Dependabot keeps them current.

## Repository discipline

- Commits: `:gitmoji: type(scope): summary` (ASCII shortcode). No AI co-author trailers.
- Decisions get written down: architecture → ADR; project/process → decision log (D-nnn);
  unresolved → open question (OQ-nnn). Superseding, not editing, once Accepted.
- Open-source hygiene: no employer names, internal system names, or internal policy content;
  run the sanitization grep before committing. Generated output goes to `bin/` (gitignored) —
  never repo root. Don't reference demo assets that don't exist yet.
- Docs published on the future site = product docs under `docs/` only; `docs/planning/`,
  `openspec/`, and agent-context stay out of the mkdocs nav.

## Design taste

- Envelope owns structure (match/route/effect/points); expressions stay in leaves; anything
  beyond CEL's ceiling graduates to Rego — don't grow a programming language in YAML
  (ADR-0002/0013).
- Prefer bindings over per-rule environment conditionals; prefer `match` over predicates for
  structural conditions.
- Adapter code may be boring and forge-shaped; `internal/core`/`internal/change` stay pure
  and forge-free (arch-lint enforced).
