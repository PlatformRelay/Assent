# Local kind cluster for e2e / demo

Hosts the test GitLab instance ([test/e2e](../../test/e2e/README.md), path 1).
Spike B / OQ-6: **CI default remains the testcontainer profile**; kind is the long-lived
local/demo path.

## Status (D-038)

**Authorized, not implemented yet.** Operator approved adding a durable local kind lab
(`task kind-up` / setup·status·teardown, CE-in-pod from Spike B’s `boot-kind.sh`) when a
lane picks it up — **do not treat the missing scripts as a gap to rush**. Until then:

```bash
# Spike measurement harness only (cold-boot; tears down each run):
bash hack/spikes/e2e/boot-kind.sh
bash hack/spikes/e2e/boot-kind.sh --teardown
```

Scaffold already here: `kind-config.yaml` (cluster `assent`, host HTTP `8929` / SSH `2224`).

Planned lab contents (when implemented): promote Spike-B CE-in-pod into idempotent
`setup.sh` / `status.sh` / `teardown.sh` + Task targets; optional `seed/` for
`examples/repos/` projects and fixture MRs. Reuse Omnibus slim config +
`kind load image-archive` (not `kind load docker-image`) from Spike B.
