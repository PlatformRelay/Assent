# P5-E10 — GitHub forge adapter execution (S02–S17)

**Change ID:** `p5-e10-github-forge-execution`
**Executes:** `openspec/specs/p5-e10-github-forge/spec.md` (stories S02–S17; the authoritative
requirement set), governed by [ADR-0021](../../../docs/adr/0021-multi-adapter-forge-seam.md)
and [S00's addressing model](../../../docs/planning/github-addressing-model.md), under the
D-140 unlock. S00 (addressing model) and S01 (importable conformance suite) are already
landed upstream.

## Problem

assent has exactly one forge adapter. ADR-0021 (Option C) decides the seam before any GitHub
API call: a named composite `forge.RunPort`, an importable conformance suite (landed by
S01), a neutral capability model, port-level transport requirements, MR-relative governed
addressing, per-adapter status→sentinel mappings, port-level identity, and a doctor-scoped
capability report. S00–S01 of the epic are done; the seam remains half-built (`cmd/assent`
still declares an anonymous `forgePort` literal and `refFilePort`, still calls
`gitlab.SyntheticDigest`, capability vocabulary is still GitLab-private) and no GitHub
adapter exists.

**S01's DoD left one clause undischarged, inherited by this change:** the twelve
conformance case IDs S00 minted (`github-addressing-model.md`, "Conformance cases" tables)
were required in `catalog.yaml` by S01's DoD, but the landed S01 commit (99de1d7) added only
the `adapters:` field to pre-existing rows. Task T0 discharges that obligation here, so the
planned reviews' own gate (T13) can fail on the twelve rows' absence instead of going green
around it.

## Scope (stories in execution order)

- **Seam wave**: S02 (`forge.RunPort` + neutral factory + depguard + call-site migration),
  S03 (`SyntheticDigest` collapse), S04 (neutral capability model + the prescribed GitLab
  arming decision row), S05 (port-level transport requirements).
- **Adapter wave**: S06 (GitHub client: REST + GraphQL, PAT + App auth, cassettes), S07
  (Snapshot), S08 (Resolve), S09 (capability report), S10 (Reconcile writes), S11 (SHA-guard
  merge + deferred arming), S12 (fail-closed deltas).
- **Integration wave**: S13 (forge selection in `run`/`doctor`), S14 (conformance parity +
  catalog dispositions), S15 (docs/maturity truth), S16 (Actions entrypoint — last,
  independently droppable per D-140's closed sub-question), S17 (exit gate).

## Out of scope (fenced)

- **S18 live GitHub adoption proof** — `[infra-gated · operator]`; needs operator-provided
  infrastructure (a real GitHub repository, live PRs, a token). Not this change.
- **E11 Rego backend, E12 serve/webhooks, E13 remote packs, E14 CRD, third forges /
  plugin-forge protocol** — fenced by D-140 / ADR-0021 Option D.
- **Widening any frozen schema** (`git diff schemas/` == 0 is this change's DoD).
- **The audit's pre-existing GitLab findings** (SEC-01/SEC-04/SEC-05,
  RELI-01/02/03) — deferred by D-138/D-139, tracked there.
- **Fixing OQ-33/OQ-34 by decision in this change.** Both are open with leading answers.
  The epic's fail-closed default applies: GitHub's `eligible-approval-evidence` (row 9) and
  `protected-pipeline-source` (row 11) report `unknown` for v1 ⇒ **v1 GitHub comments and
  does not gate**. That is the documented product limitation the epic and ADR-0021 both
  name; this change ships it honestly (S12's fail-closed tests) rather than papering over
  either question. A conformance case that promotes row 9 (fidelity case) is minted as a
  candidate, not implemented as authority (OQ-34).

## Execution decisions (resolving S00 obligation 2b before the signature freezes)

1. **The MR handle on content accessors is composite**: `FileAtBase(project, mr, path)` /
   `FileAtHead(project, mr, path)` (not ADR-0021's two-argument sketch). S00 §Forward
   obligations 2b leaves the binding choice to S02: binding the client to one project at
   construction would hide a stateful binding inside an otherwise stateless port and change
   every factory signature, while the composite handle matches every other port method
   (`GetMR`, `Forge` writes, `ResolveRequest`) that already addresses an MR as
   `(project, mr)`. No adapter state, no signature asymmetry with `FileAtRef`.
2. **`forge.ErrUnauthorized` is lifted to the port** (S00 Q4): adapters wrap it; the
   conformance suite asserts forbidden ≠ absent per adapter.

## Counterpoints considered

- **"Skip the seam wave, write the GitHub adapter against today's implicit port."** Rejected:
  ADR-0021 Option A, rejected there for cause (fork PRs evaluate as fabricated DELETEs under
  today's `FileAtRef` addressing; the conformance suite is unusable by a second adapter).
- **"Implement only the adapter wave; defer the seam."** Rejected: the adapter wave's REQs are
  written against `forge.RunPort` (S07's fork cases, S11's SHA-guard cases run through the
  shared factory) — the seam is their substrate.
- **"Report `eligible-approval-evidence` as `supported` via CODEOWNERS so GitHub can arm."**
  Rejected: that exact over-claim is S00's recorded correction; `unknown` is the honest
  grading (OQ-34) and non-arming is the fail-closed direction.
