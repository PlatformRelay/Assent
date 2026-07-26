# End-to-end tests (L3)

Real-forge e2e per [ADR-0006](../../docs/adr/0006-testing-strategy.md). Two paths;
Spike B (OQ-6) measured both and picked the CI default:

## Path 1 — GitLab in kind (`hack/kind/`) — local/demo lab

Long-lived local instance: `task kind-up` creates cluster `assent`, side-loads GitLab CE,
and waits on `http://localhost:8929`. Boot cost is paid once. See [`hack/kind/README.md`](../../hack/kind/README.md).

```sh
task kind-up
task kind-status
ASSENT_SPIKE_HTTP_PORT=8929 bash hack/spikes/e2e/smoke.sh
task kind-down
```

## Path 2 — GitLab CE testcontainer — CI default

Self-contained per run via `hack/spikes/e2e/boot-testcontainer.sh` (and, later,
testcontainers-go). Hermetic for CI; GitLab CE boots slowly and is memory-hungry — hence
Spike B before committing CI to it.

## Shape of an e2e case (both paths)

1. Seed: create sample project + policy dir, configure bot user/token.
2. Act: open an MR with a fixture change; run `assent run` as the pipeline would.
3. Assert against the **forge API**: threads created/resolvable, approval state, merge state,
   and the emitted JSON report — this doubles as the forge-port conformance suite (ADR-0005).

GitHub e2e (dedicated test org) lands with epic E8.

E2E code is build-tagged (`//go:build e2e`) and excluded from `task check`; run via `task e2e`.

## gitlab.com real-repo lab (D-012 / S11)

Private project `assent-lab` on gitlab.com — credential via workspace `GITLAB_TOKEN`
(direnv). Pointers live in gitignored `agent-context/LOCAL-INFRA.md` (D-037).
