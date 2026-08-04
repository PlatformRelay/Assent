# internal/

Shipped library packages for the `assent` CLI. Hexagonal layout: **pure decision core**
(no I/O) at the centre; adapters and hosts at the edges. See
[C4 containers](../docs/architecture/c4-container.md) for the product sketch; this file
lists what actually exists today (`go list ./internal/...`).

## Pure (no filesystem, network, or forge I/O)

| Package | Role |
| --- | --- |
| `internal/core/aggregate` | Obligations aggregator — CEL assert trees, coverage, tri-state |
| `internal/core/decision` | Decision / findings / pins model and report emission |
| `internal/core/classify` | Change-set classification helpers |
| `internal/core/policy` | Policy envelope types (profiles, bindings, prove blocks) |
| `internal/core/hash` | Canonical JSON hashing (ADR-0017 digest vectors) |
| `internal/change` | Value tree, structural differ, canonical `ChangeSet` |
| `internal/compare` | Policy comparison suite loader, classifiers, gates, records |
| `internal/catalogue` | Catalogue load/combine and profile→pack activation |
| `internal/evaldecode` | Strict decode of evaluation inputs from JSON/YAML |
| `internal/glob` | Path glob matching for policy selectors |
| `internal/lint` | Policy and presentation lint (hard-error fixtures) |
| `internal/schemadrift` | Presentation/config drift detection |
| `internal/render` | Markdown renderer, redaction, locale chrome, summary |
| `internal/render/locale` | Locale string catalog |

## I/O and adapter edges

| Package | Role |
| --- | --- |
| `internal/forge` | Forge port (snapshot, resolve, reconcile) + shared helpers |
| `internal/forge/gitlab` | GitLab HTTP adapter |
| `internal/forge/fake` | In-memory forge fake for tests |
| `internal/forge/conformance` | SHA-guarded reconciliation conformance suite |
| `internal/provider` | Provider host (HTTP/exec transport, sensitive handling) |
| `internal/provider/builtin` | Built-in providers (repo-file, resource-owner, GitLab groups) |

## Test/support

| Package | Role |
| --- | --- |
| `internal/adoptertest` | Adopter-facing policy test helpers (ADR-0014 fixtures) |

## Architecture rule

`internal/core/aggregate`, `internal/core/decision`, `internal/core/classify`, and
`internal/change` must not import forge, provider, `cmd/`, or anything that performs I/O.
Compare and catalogue stay pure (filesystem reads happen in `cmd/assent` before calling in).
Enforcement: manual review today; depguard/purity lint is tracked separately (AUD-01).
