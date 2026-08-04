# Policy lifecycle — promotion gates (P3-E4-S03)

Machine-enforceable **promotion gates** decide whether a candidate
`PolicyProfile` may be promoted relative to a baseline, given a
`PolicyComparisonSuite` run. Gates are **data** carried by the suite schema
(`schemas/comparison/v1alpha1/comparison-suite.schema.json`), not prose a human
interprets. `assent compare` (E6) consumes this table; exit codes map 1:1 to
gate outcomes (CLI contract authored in P3-E4-S04 / ADR-0018).

## Delta taxonomy (closed)

Every baseline↔candidate difference over a suite case classifies as exactly one
of:

| Kind | Meaning |
| --- | --- |
| `stricter-intervention-added` | Candidate adds a stricter intervention the baseline lacked |
| `destructive-or-authorization-intervention-missed` | Candidate misses a destructive or authorization/ownership intervention the baseline had |
| `subject-or-obligation-uncovered` | A subject or obligation covered by baseline is uncovered by candidate |
| `newly-auto-mergeable` | Candidate newly permits auto-merge where baseline did not |
| `score-threshold-change` | Score / threshold arithmetic changed the outcome |
| `explanation-only` | Non-semantic drift (including wording-only `message` template changes) |

No `"other"` / free-text kind. A difference matching none of the six is a hard
classification error (fail-closed).

## Promotion gate table

| Gate ID | Fails on delta kind(s) | Default | Acceptance |
| --- | --- | --- | --- |
| `zero-missed-destructive` | `destructive-or-authorization-intervention-missed` | fail | `per-delta-identity` via `acceptedDeltas` |
| `zero-missed-authorization-ownership` | `destructive-or-authorization-intervention-missed` | fail | `per-delta-identity` via `acceptedDeltas` |
| `no-unexpected-obligation-removal` | `subject-or-obligation-uncovered` | fail | `per-delta-identity` via `acceptedDeltas` |
| `bounded-auto-merge-widening` | `newly-auto-mergeable` | fail | `per-delta-identity` via `acceptedDeltas` |
| `explicitly-accepted-deltas` | `stricter-intervention-added`, `score-threshold-change` | fail | `per-delta-identity` via `acceptedDeltas` |

`explanation-only` is never listed in any gate's `failOnKinds` — rendered-message
and other presentation-only changes cannot trip a promotion gate.

The first two gates share a taxonomy member by design (destructive and
authorization/ownership misses are one closed kind); `gateId` distinguishes the
reporting axis for `assent compare` exit codes.

## `assent compare` exit codes (ADR-0018 / D-115)

Full suite mode and the single-dir seed both map process exit to promotion-gate
outcomes. The first failing gate sets the exit code; the report lists all gate
results on stdout.

| Exit code | Meaning |
| --- | --- |
| `0` | All promotion gates passed |
| `1` | Gate `zero-missed-destructive` failed |
| `2` | Gate `zero-missed-authorization-ownership` failed |
| `3` | Gate `no-unexpected-obligation-removal` failed |
| `4` | Gate `bounded-auto-merge-widening` failed |
| `5` | Gate `explicitly-accepted-deltas` failed |
| `6` | Load, schema, digest, or fail-closed classification error |

Suite invocation:

```text
assent compare --suite examples/comparison/<suite>/ \
  [--baseline-profile <name>] [--candidate-profile <name>] \
  [--record <dir>]
```

Single-case seed fixtures retain `assent compare <dir>` with the same exit table
(the seed applies only the `bounded-auto-merge-widening` gate, so a widening delta
exits `4`).

## `acceptedDeltas` allowlist

An allowlist entry **must** key by:

1. `caseId` (stable suite case), and
2. delta identity fields `rule` + `subject` (optional `obligation`), and
3. `kind`

Never by kind alone — that footgun would let an author accept every
`destructive-or-authorization-intervention-missed` delta in one stroke.
A `destructive-or-authorization-intervention-missed` delta **always fails** its
gate unless individually present in `acceptedDeltas`.

## Contracts

- Comparison deltas: `schemas/comparison/v1alpha1/comparison-record.schema.json`
- Suite + gates + `acceptedDeltas`: `schemas/comparison/v1alpha1/comparison-suite.schema.json`
- Immutable corpus: under a fixed `caseId`, `replayBundleDigest` must not change;
  revise by minting a new `caseId`
