<p align="center">
  <a href="https://platformrelay.github.io/assent/">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="docs/assets/brand/assent-logo-light.svg">
      <img src="docs/assets/brand/assent-logo.svg" alt="assent — deterministic, policy-driven auto-merge" width="420">
    </picture>
  </a>
</p>

<p align="center">
  <a href="https://github.com/PlatformRelay/assent/actions/workflows/verify.yaml"><img src="https://github.com/PlatformRelay/assent/actions/workflows/verify.yaml/badge.svg" alt="Verify"></a>
  <a href="https://github.com/PlatformRelay/assent/actions/workflows/schemas.yml"><img src="https://github.com/PlatformRelay/assent/actions/workflows/schemas.yml/badge.svg" alt="Schemas"></a>
  <a href="https://platformrelay.github.io/assent/"><img src="https://img.shields.io/badge/documentation-GitHub%20Pages-2ea44f?logo=materialformkdocs&logoColor=white" alt="Documentation"></a>
  <a href="https://github.com/PlatformRelay/assent/actions/workflows/docs.yaml"><img src="https://github.com/PlatformRelay/assent/actions/workflows/docs.yaml/badge.svg" alt="Docs"></a>
  <a href="https://securityscorecards.dev/viewer/?uri=github.com/PlatformRelay/assent"><img src="https://api.securityscorecards.dev/projects/github.com/PlatformRelay/assent/badge" alt="OpenSSF Scorecard"></a>
  <a href="https://github.com/PlatformRelay/assent/blob/main/LICENSE"><img src="https://img.shields.io/github/license/PlatformRelay/assent" alt="License: Apache-2.0"></a>
  <a href="https://pkg.go.dev/github.com/PlatformRelay/assent"><img src="https://img.shields.io/github/go-mod/go-version/PlatformRelay/assent" alt="Go version"></a>
</p>

> **Canonical repo:** GitHub ([PlatformRelay/assent](https://github.com/PlatformRelay/assent)).
> **Status: pre-alpha / design phase.** All APIs, schemas, and commands are drafts.

**assent** is a deterministic, policy-driven **auto-merge gate** for self-service
configuration repositories. Drop it into a repo's CI pipeline and it turns merge requests /
pull requests into decisions: **approve, comment, request changes, or block** — based on
rules *you* write in a **Kyverno-style declarative YAML** with CEL predicates (a Rego
escape hatch is designed and unlocks with the first consumer who needs it — D-012).

The goal: make any config repo automerge-capable with deterministic, testable, reviewable
rules — without writing a custom bot per repo.

## Why

Most changes to config repos (topic definitions, service catalogs, tfvars, tenant onboarding
files) are routine: a team edits *their own* entries within safe bounds. Yet a human still has
to review every MR, reconstructing the same context each time — what changed, who owns it, is
it destructive, which policy applies. assent encodes that reasoning as policy so the routine
90% merges itself and reviewers spend their attention on the risky 10%.

## What it does (intended scope)

- **Runs in the pipeline** — GitLab CI in v1; GitHub Actions is a designed seam that unlocks
  per D-012. One process per MR/PR, no long-lived service required.
- **Understands structured changes** — parses JSON, YAML, and HCL/tfvars into a canonical
  change model (field-level adds / modifies / deletes), so policies reason about *semantics*,
  not diff lines.
- **Policies in Rego or declarative YAML** — both frontends compile to the same decision
  engine; pick whichever your team reads best.
- **Pluggable permission & fact providers** — "is the author allowed to touch this entry?"
  can be answered by Keycloak, LDAP, GitLab/GitHub group membership, a CODEOWNERS-style file,
  or your own plugin.
- **Acts like a reviewer** — posts findings as resolvable review threads, comments,
  approves/denies, and (when everything is green) merges. Same behaviour on GitLab and GitHub.
- **Testable by design** — repo owners get a test harness: fixture changes in, expected
  decision out. Policies without tests are a lint error, not a style choice.

See [`docs/vision.md`](docs/vision.md) for the full intended use case and
[`docs/planning/meta-plan.md`](docs/planning/meta-plan.md) for how we get from here to a
precise implementation plan.

## Repository layout

| Path | Purpose |
| --- | --- |
| `docs/vision.md` | Intended use case, personas, north-star |
| `docs/adr/` | Architecture decision records (template + numbered ADRs) |
| `docs/architecture/` | C4 diagrams (mermaid) |
| `docs/decisions/` | Lightweight operator decision log (D-nnn) |
| `docs/planning/` | Meta-plan and open questions |
| `openspec/` | Spec-driven development: specs (Given/When/Then, REQ IDs) and change proposals |
| `cmd/assent/` | CLI entry point (stub) |
| `internal/` | Go packages (hexagonal: core + ports + adapters) — see `internal/README.md` |
| `examples/` | Sample policies (Rego + declarative) and sample self-service repo layouts |
| `test/e2e/` | End-to-end strategy: kind-hosted GitLab / testcontainers |
| `hack/kind/` | Local kind cluster setup for e2e |

## Development

Spec/test-driven, gates in the [`Taskfile`](Taskfile.yml):

```bash
task check   # fmt + vet + lint + test
```

## License

[Apache-2.0](LICENSE) — © 2026 Konrad Heimel. Same license family as Kubernetes and Argo CD:
permissive, with an explicit patent grant.
