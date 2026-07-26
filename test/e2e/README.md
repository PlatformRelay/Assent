# End-to-end tests (L3)

Real-forge e2e per [ADR-0006](../../docs/adr/0006-testing-strategy.md). Two scaffolded paths;
Spike B (meta-plan Phase 2, OQ-6) measures both and picks the CI default:

## Path 1 — GitLab in kind (`hack/kind/`)

Long-lived local instance: kind cluster + GitLab Helm chart (or the lighter GitLab CE
docker-in-kind variant). Best for local development, demos, and repeated runs — boot cost is
paid once. Also the home of the generated sample repos (`examples/repos/`), seeded via a
`seed` script that creates projects, users/groups, and opens fixture MRs.

## Path 2 — GitLab CE testcontainer

Self-contained per test run via testcontainers-go and the `gitlab/gitlab-ce` image. Fully
hermetic for CI, but GitLab CE boots slowly (minutes) and is memory-hungry — hence the spike
before committing CI to it.

## Shape of an e2e case (both paths)

1. Seed: create sample project + policy dir, configure bot user/token.
2. Act: open an MR with a fixture change; run `assent run` as the pipeline would.
3. Assert against the **forge API**: threads created/resolvable, approval state, merge state,
   and the emitted JSON report — this doubles as the forge-port conformance suite (ADR-0005).

GitHub e2e (dedicated test org) lands with epic E8.

E2E code is build-tagged (`//go:build e2e`) and excluded from `task check`; run via `task e2e`.
