# Spike B — e2e boot profiles (P2-E2)

Measured comparison of two GitLab CE boot paths. **Decision D-026:** testcontainer is the **CI
default**; kind is for **local/demo** (D-083). Full spike report:
[`docs/planning/spikes/spike-b-e2e.md`](../../../docs/planning/spikes/spike-b-e2e.md).

## Scripts

| Script | Profile | Purpose |
| --- | --- | --- |
| [`boot-testcontainer.sh`](boot-testcontainer.sh) | testcontainer | Boot GitLab CE as a plain Docker container (CI default) |
| [`boot-kind.sh`](boot-kind.sh) | kind | Boot GitLab CE inside the `assent` kind cluster |
| [`smoke.sh`](smoke.sh) | both | Product-surface smoke: seed → MR → thread → approve → SHA merge |

All boot scripts are idempotent and accept `--teardown`. On success each prints one machine-readable
line: `RESULT boot_seconds=<n> rss_mb=<n>`.

## Quick start

**Testcontainer (CI default):**

```bash
./hack/spikes/e2e/boot-testcontainer.sh
export ASSENT_E2E_GITLAB=http://localhost:8980
export ASSENT_SPIKE_PROFILE=testcontainer
./hack/spikes/e2e/smoke.sh
```

**Kind (local/demo):**

```bash
./hack/spikes/e2e/boot-kind.sh
export ASSENT_E2E_GITLAB=http://localhost:8929
export ASSENT_SPIKE_PROFILE=kind
./hack/spikes/e2e/smoke.sh
```

Teardown: `./hack/spikes/e2e/boot-testcontainer.sh --teardown` or
`./hack/spikes/e2e/boot-kind.sh --teardown`.

## Related docs

- [`test/e2e/README.md`](../../../test/e2e/README.md) — L3 e2e env vars, skip behaviour, task targets
- [`hack/kind/`](../../kind/) — kind cluster scaffold used by `boot-kind.sh`
