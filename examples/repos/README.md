# Sample self-service repos

Generic, **generated** sample repos (never copied from any private codebase — D-002). They
serve three purposes: e2e seeds (created in the test GitLab, see `test/e2e/`), adopter
documentation, and fixtures for the policy test harness.

Planned shapes (built in meta-plan Phase 1.1, refined as the operator provides real-world
repo shapes to generalize). **Committed now:** the three rows below. **Deferred (D-029):**
further private-shape generalization (shapes 4–5) — kept in the corpus plan, not cancelled;
does not block Phase 4.

| Sample | Format | Exercises |
| --- | --- | --- |
| `topic-registry/` | YAML (one file per topic: name, owner, partitions, retention) | ownership, bounded change, no-destruction |
| `service-catalog/` | JSON (single catalog file, many entries) | allow-listed fields, schema validity, multi-entry diffs |
| `infra-vars/` | tfvars/HCL (per-env variable files) | environment split, HCL parsing, opaque-change fallback |

| Deferred (corpus plan) | Notes |
| --- | --- |
| Shapes 4–5 (D-008/D-019/D-029) | Additional generalized private self-service shapes when the operator provides sources; sanitize per D-002; not near-term |

## Open-source corpus candidates (OQ-16)

Real public repos whose *shapes* we can test against — and potentially use as live demos —
without any sanitization concerns:

| Repo / format | Why it fits |
| --- | --- |
| [kubernetes/org](https://github.com/kubernetes/org) | the canonical public self-service repo: people PR themselves into orgs/teams (YAML), automation validates & merges — practically our use case, run by Prow today |
| [JulieOps](https://github.com/kafka-ops/julie) / [kafka-gitops](https://github.com/devshawn/kafka-gitops) topology files | public Kafka topic/ACL descriptor formats — realistic topic-registry shapes |
| [octoDNS](https://github.com/octodns/octodns) zone configs | YAML DNS records: ownership + bounded-change + no-destruction map perfectly |
| [Backstage](https://backstage.io) `catalog-info.yaml` | service-catalog JSON/YAML entries with owner fields |
| [GitHub safe-settings](https://github.com/github/safe-settings) org config | org/repo settings as YAML, reviewed via PR |
| [conda-forge](https://github.com/conda-forge) feedstocks / Homebrew formula bumps | public precedent for bot-automerged config PRs (prior art for Phase 1.4 too) |
