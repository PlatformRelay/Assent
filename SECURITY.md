# Security Policy

assent is a **deterministic merge-decision engine** — it reads a change and a policy
and emits a fail-safe decision (`APPROVE` / `REVIEW` / `BLOCK`) plus an auditable
`DecisionRecord`. Because that decision can gate whether a change merges, correctness
and integrity of the decision path are first-class security concerns. The threat-relevant
design lives in [ADR-0015 (trust boundaries & merge integrity)](docs/adr/0015-trust-boundaries-merge-integrity.md).

## Reporting a vulnerability

Please report suspected vulnerabilities **privately** — do not open a public issue for
security problems.

- Use **GitHub Security Advisories** → *"Report a vulnerability"* on this repository,
  or email **konrad.heimel@gmail.com** privately.
- Include the affected version/commit, a description, reproduction steps, and impact.
- You will receive an acknowledgement; confirmed issues are prioritised and disclosed
  once a fix is available.

This is an open-source project without a formal SLA, but security reports are taken
seriously and handled promptly.

## Supported versions

The project is pre-1.0 (`v1alpha1`). Only the latest released version / the default
branch receives fixes. The API and policy contracts may change between alpha releases.

## Security posture

**Enforced in CI today** (see [`.github/workflows/`](.github/workflows/)):

- **Static analysis (SAST)**: `gosec` (via `golangci-lint`) on every push/PR, plus
  **CodeQL** (Go + GitHub Actions) on push/PR and a weekly schedule.
- **Dependency vulnerabilities**: `govulncheck` on every push/PR (`verify`) and on a
  weekly schedule (`vulncheck`); `go.sum` is committed; Dependabot proposes grouped
  version updates for Go modules, GitHub Actions, and the docs toolchain.
- **Secret scanning**: `gitleaks` runs in CI; GitHub push-protection + secret scanning
  are enabled at the repository level.
- **Supply-chain hardening**: all GitHub Actions are **pinned to a commit SHA**;
  workflows default to a **least-privilege**, read-only `GITHUB_TOKEN` and grant write
  scopes only per-job where required; npm lifecycle scripts are disabled during schema
  tooling installs (`--ignore-scripts`).
- **OpenSSF Scorecard**: runs weekly and publishes results; findings surface in the
  repository's code-scanning view.

**Design principles** (see ADRs under [`docs/adr/`](docs/adr/) and [GUIDELINES.md](GUIDELINES.md)):

- **Deterministic, fail-safe decisions**: the decision path in `internal/core` takes no
  wall-clock, randomness, network, or LLM dependency — determinism is a security
  property, so a decision cannot be nudged by nondeterministic input. Any evaluation
  error (undeclared reference, type/coercion error, unavailable/expired fact, cost
  overrun) fails **safe** to `REVIEW`/`BLOCK` — never a silent `APPROVE` (ADR-0007 F6).
- **Merge integrity**: an approved decision is pinned to the exact evaluated source via a
  content-addressed digest, so it cannot be applied to a source that changed after
  evaluation (ADR-0015).
- **Secret hygiene**: forge/provider tokens are redacted and never logged or written into
  a `DecisionRecord`; policies reference facts by query, never embed secrets inline.
- **Bounded, fail-closed inputs**: change parsing enforces size/depth/entry-count
  ceilings and rejects oversized or ambiguous inputs rather than proceeding.

## Community

- [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) — community behaviour standards
- [GUIDELINES.md](GUIDELINES.md) — engineering rules, including the security gates above
