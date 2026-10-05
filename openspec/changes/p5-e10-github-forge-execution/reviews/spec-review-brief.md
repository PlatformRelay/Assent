# Review brief — E10 GitHub forge execution change (spec-set review)

You are one of three independent reviewers on DIFFERENT models. You have NOT seen the
author's context; judge only these artifacts. Do NOT edit any file — read-only review.

## Subject

The spec set that will drive implementing GitHub forge support in this repo
(`assent`, a deterministic auto-merge gate; today GitLab-only; adding GitHub):

- `openspec/changes/p5-e10-github-forge-execution/proposal.md`
- `openspec/changes/p5-e10-github-forge-execution/tasks.md`
- `openspec/changes/p5-e10-github-forge-execution/specs/p5-e10-github-forge-execution/spec.md`
- The authoritative epic spec they execute:
  `openspec/specs/p5-e10-github-forge/spec.md` (stories S02–S17 are in scope; S00/S01 already
  landed)
- Governing decisions: `docs/adr/0021-multi-adapter-forge-seam.md`,
  `docs/planning/github-addressing-model.md` (S00 answers; S02 obligation 2b)

## Target question

Would executing these tasks as written deliver the epic spec? Specifically:

1. Missing, misordered, or unverifiable tasks; any REQ of the epic spec S02–S17 not covered
   by a task.
2. Contradictions between proposal/tasks/delta and the epic spec, ADR-0021, or the S00
   addressing model (e.g. the two execution decisions: composite `(project, mr)` handles on
   FileAtBase/FileAtHead vs ADR-0021's two-argument sketch; `forge.ErrUnauthorized` lift).
3. Fail-closed gaps: anywhere the plan could let arming happen on an unproven capability, or
   a trust-boundary load (policy from target ref) migrate onto an MR-relative accessor.
4. Scope honesty: is anything claimed out-of-scope actually load-bearing for the in-scope
   stories? Is S16/S18 exclusion safe?
5. The S00-specified conformance case IDs that must exist (twelve minted in the addressing
   model's tables) — does the plan ensure they are implemented and catalogued?

## Output format (return exactly this)

For each finding: `SEVERITY: CRITICAL|MAJOR|MINOR | file:anchor | finding | recommended fix`.
Then a one-line verdict: `VERDICT: PROCEED` or `VERDICT: FIX-FIRST`. Cite file paths and
anchor text you actually read. If you verify something is fine, do not list it.
