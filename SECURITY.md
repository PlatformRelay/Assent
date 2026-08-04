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

**Release artifact verification** (E9-S06/S12, D-109/D-110):

Tagged releases (`v*.*.*`) publish **SHA256 checksums**, **cosign keyless Sigstore
bundles** (`.sigstore.json`), **SPDX SBOMs** (syft via goreleaser), and **SLSA provenance**
(`actions/attest` + `release-provenance.intoto.jsonl`). Local snapshot builds
(`task release-snapshot`) skip signing/SBOM — no OIDC outside GitHub Actions.

| Gate | When enforced | Verify |
| --- | --- | --- |
| SHA256 checksums | always | `sha256sum -c checksums.txt` (or `shasum -a 256`) |
| Cosign (keyless) | tagged release | `cosign verify-blob` with bundle (see below) |
| SBOM (SPDX JSON) | tagged release | inspect `*.spdx.json`; optional syft/grype scan |
| SLSA provenance | tagged release | `gh attestation verify` on subjects in `checksums.txt` (see below) |

Maintainers and CI verify goreleaser output with `task release-verify` (or
`hack/release/verify-artifacts.sh`) after `task release-snapshot` or a tagged release download.
**Cosign** runs when `.sigstore.json` bundles are present beside archives; the autonomous snapshot
path skips cosign when bundles are absent. Post-S06 releases use `--require-signature` to fail
closed without bundles.

### Verify a tagged release (copy-paste)

Download assets for tag `vX.Y.Z` from
[GitHub Releases](https://github.com/PlatformRelay/assent/releases), then:

```bash
TAG=vX.Y.Z
REPO=PlatformRelay/assent
gh release download "$TAG" --repo "$REPO" --dir dist-verify

# 1) Checksums (required — fail closed on mismatch)
cd dist-verify
sha256sum -c checksums.txt

# 2) Cosign — per-archive bundles (keyless, GitHub Actions OIDC issuer)
ARCHIVE=assent_X.Y.Z_linux_amd64.tar.gz   # adjust OS/arch
cosign verify-blob \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp '^https://github.com/PlatformRelay/assent/' \
  --bundle "${ARCHIVE}.sigstore.json" \
  "${ARCHIVE}"

# Cosign — checksum manifest (covers archives + SBOMs listed inside)
cosign verify-blob \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp '^https://github.com/PlatformRelay/assent/' \
  --bundle checksums.txt.sigstore.json \
  checksums.txt

# 3) SBOM — SPDX JSON per archive (syft / goreleaser sboms pipe)
ls -1 *.spdx.json
# Optional: syft sbom validate assent_X.Y.Z_linux_amd64.tar.gz.spdx.json

# 4) SLSA provenance — `actions/attest` with `subject-checksums` attests the files
#    *listed inside* checksums.txt (archives + SBOMs), not checksums.txt itself.
#    `release-provenance.intoto.jsonl` is the exported bundle for offline verification.
gh attestation verify "${ARCHIVE}" \
  --owner PlatformRelay \
  --bundle release-provenance.intoto.jsonl

# Verify every attested subject from the checksum manifest (archives + *.spdx.json):
while read -r _ name; do
  name="${name#\*}"
  [[ -n "$name" ]] || continue
  gh attestation verify "$name" --owner PlatformRelay --bundle release-provenance.intoto.jsonl
done < checksums.txt
```

Install helper: `hack/install.sh --archive … --checksums …` verifies SHA256 first; add
`--require-signature` after downloading a signed release.

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
