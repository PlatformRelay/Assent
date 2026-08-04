# hack/compare — PCS exit gate harness

Autonomous exit gate for [p5-pcs-policy-comparison](../../openspec/specs/p5-pcs-policy-comparison/spec.md)
(PCS-S09). Proves the full `PolicyComparisonSuite` runner shipped and closes the D-057 deferred
scope logged at E6-S09.

| Script / task | Purpose |
| --- | --- |
| `hack/compare/exitgate_test.sh` | REQ-PCS-S09-01..03: schema drift guard, `assent compare --suite` on committed corpus, E6 single-dir seed regression, backlog/later-phases/decisions pins |
| `task compare-exitgate-test` | Same check via Taskfile |
| `task check` | Includes `compare-exitgate-test` after `dogfood-comparison` |

**Residual (documented in D-118):** `assent compare` does not replace human review of
`acceptedDeltas` rationale text — it enforces per-delta identity allowlisting only.
