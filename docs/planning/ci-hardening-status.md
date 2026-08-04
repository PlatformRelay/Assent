# CI hardening status (oss-playbook #5)

Inventory of supply-chain and security CI gates for **assent**. Shipped by **D-045**; E9-S04
closes residual gaps only (**D-102** — audit-first, no duplicate CodeQL/Scorecard/govulncheck).

Last reviewed: 2026-08-04 (E9-S04).

| Gate | Status | Workflow / config path | Notes |
| --- | --- | --- | --- |
| **CodeQL** (SAST) | ✅ Exists | `.github/workflows/codeql.yaml` | Go + Actions matrix; weekly schedule. Exactly one workflow (REQ-E9-S04-03). |
| **OpenSSF Scorecard** | ✅ Exists | `.github/workflows/scorecard.yaml` | `publish_results: true`; SARIF upload. README badge links to scorecard viewer. |
| **govulncheck** (push) | ✅ Exists | `.github/workflows/verify.yaml` (`govulncheck` step) | Per push/PR; not duplicated elsewhere on push. |
| **govulncheck** (weekly) | ✅ Exists | `.github/workflows/vulncheck.yaml` | Schedule-only sweep; complements push gate. |
| **gitleaks** | ✅ Exists | `.github/workflows/verify.yaml` (`gitleaks` step) | HEAD-scoped history scan; CLI (no paid org action). |
| **SHA-pinned Actions** | ✅ Exists | All `.github/workflows/*.yaml` | Full commit SHAs + version comments (D-045). |
| **Dependabot** | ✅ Exists | `.github/dependabot.yml` | `gomod`, `github-actions`, `pip` (docs) weekly. |
| **actionlint** | ✅ Exists | `.github/workflows/actionlint.yaml` | Lint workflow YAML on workflow changes (E9-S04 residual). |

## Residual gaps (operator / later lanes)

| Item | Status | Owner |
| --- | --- | --- |
| Branch protection + required status checks on `main` | Gap | Operator (D-045 residual) |
| codecov / coverage badge | Gap | Deferred (not in D-045 scope) |
| renovate (vs Dependabot-only) | Gap | Optional; Dependabot covers actions + modules |

## Explicit non-actions (D-102)

- Do **not** add a second CodeQL workflow or matrix.
- Do **not** duplicate govulncheck on every push (weekly job in `vulncheck.yaml` is sufficient).
- Do **not** add a second Scorecard job.

Verify locally:

```bash
actionlint .github/workflows/*.yaml
hack/release/ci_audit_test.sh
```
