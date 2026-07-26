# Local kind lab — GitLab for assent

Long-lived **local/demo** GitLab CE inside kind (Spike B / OQ-6: CI stays on the
**testcontainer** profile; kind is paid once for interactive work).

Cluster name: `assent` · GitLab HTTP: `http://localhost:8929` · SSH host port: `2224`.

## Quick start

```sh
task kind-up       # create cluster if needed, load image, apply GitLab, wait ready
task kind-status   # deploy + readiness probe
task kind-down     # delete the kind cluster
```

Requires `kind`, `docker`, `kubectl`, and `curl` on `PATH`. First boot pulls
`gitlab/gitlab-ce:19.2.0-ce.0` (~minutes); later `kind-up` is idempotent.

Overrides (optional):

| Env | Default | Purpose |
| --- | --- | --- |
| `ASSENT_KIND_CLUSTER` | `assent` | kind cluster name |
| `ASSENT_GITLAB_IMAGE` | `gitlab/gitlab-ce:19.2.0-ce.0` | CE image pin (Spike B) |
| `ASSENT_GITLAB_HTTP_PORT` | `8929` | host port (must match `kind-config.yaml`) |
| `ASSENT_KIND_READY_TIMEOUT` | `900` | readiness wait (seconds) |

## Layout

| Path | Role |
| --- | --- |
| `kind-config.yaml` | kind cluster + NodePort host mappings |
| `common.sh` | shared helpers (preflight, load, apply, wait) |
| `setup.sh` / `teardown.sh` / `status.sh` | lab lifecycle |
| `config_test.go` | contract tests (scripts present, ports pinned) |

Product-surface smoke against this lab (after `kind-up`):

```sh
ASSENT_SPIKE_HTTP_PORT=8929 bash hack/spikes/e2e/smoke.sh
```

(`smoke.sh` was written for the testcontainer default port `8980`; point it at `8929` here.)

## vs testcontainer / gitlab.com

| Profile | When |
| --- | --- |
| **kind lab** (`task kind-up`) | Local/demo, repeated L3 runs, E7 conformance host |
| **testcontainer** (`hack/spikes/e2e/boot-testcontainer.sh`) | CI default (Spike B) |
| **gitlab.com `assent-lab`** | D-012 / P4-E1-S11 real-repo adoption (`agent-context/LOCAL-INFRA.md`) |
