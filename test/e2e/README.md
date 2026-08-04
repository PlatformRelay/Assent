# End-to-end tests (L3)

Real-forge e2e per [ADR-0006](../../docs/adr/0006-testing-strategy.md). Spike B (P2-E2, **D-026**)
measured two boot profiles and chose **testcontainer as the CI default**; **kind** remains the
local/demo path (D-083). Boot scripts and smoke live under [`hack/spikes/e2e/`](../../hack/spikes/e2e/README.md).

## Profiles

| Profile | Role | Boot |
| --- | --- | --- |
| **testcontainer** | CI default — self-contained GitLab CE Docker container per run | `hack/spikes/e2e/boot-testcontainer.sh` |
| **kind** | Local/demo — durable kind cluster + CE-in-pod (boot cost paid once) | `hack/spikes/e2e/boot-kind.sh` |

After boot, the Spike-B product smoke (`hack/spikes/e2e/smoke.sh`) exercises the forge primitives
Reconcile needs: seed sample repo → open MR → resolvable thread → resolve → approve → SHA-pinned
merge. Select the profile with `ASSENT_SPIKE_PROFILE` (`testcontainer` default, or `kind`).

## Environment variables

| Variable | Purpose |
| --- | --- |
| `ASSENT_E2E_GITLAB` | GitLab base URL (e.g. `http://localhost:8980` for testcontainer). **Arms** the L3 e2e tests. |
| `ASSENT_SPIKE_PROFILE` | Spike-B boot profile passed to smoke scripts: `testcontainer` (default) or `kind`. |
| `ASSENT_SPIKE_HTTP_PORT` | Host port for testcontainer profile (default `8980`). |
| `ASSENT_SPIKE_GITLAB_IMAGE` | GitLab CE image tag (default `gitlab/gitlab-ce:19.2.0-ce.0`). |

## Skip behaviour (autonomous gate)

E2E code is build-tagged (`//go:build e2e`) and **excluded from `task check`**. When
`ASSENT_E2E_GITLAB` is **unset**, `go test -tags e2e ./test/e2e/...` (and `task e2e`) **skip**
before touching live infra — the autonomous PR gate stays green without Docker or kind.

Compile drift is still caught on every PR via `go vet -tags e2e ./...` in verify and locally via
`task e2e-vet`.

## Shape of an e2e case (both profiles)

1. Seed: create sample project + policy dir, configure bot user/token.
2. Act: open an MR with a fixture change; run `assent run` as the pipeline would.
3. Assert against the **forge API**: threads created/resolvable, approval state, merge state,
   and the emitted JSON report — this doubles as the forge-port conformance suite (ADR-0005).

GitHub e2e (dedicated test org) lands with epic E10.

## Task entry points

| Task | When |
| --- | --- |
| `task e2e-vet` | Every commit — compile+vet e2e-tagged wiring (no live infra) |
| `task e2e` | Operator/CI with GitLab booted and `ASSENT_E2E_GITLAB` set |
