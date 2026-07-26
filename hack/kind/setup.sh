#!/usr/bin/env bash
# Idempotent local kind lab: cluster `assent` + GitLab CE (long-lived local/demo).
# CI continues to use the Spike-B testcontainer profile; this lab is for operators.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
source "${SCRIPT_DIR}/common.sh"

_kind_require_tools
_kind_ensure_cluster
_kind_load_gitlab_image
_kind_apply_gitlab
_kind_wait_ready

cat <<EOF
assent-kind: lab up

  cluster:  kind-${CLUSTER_NAME}
  GitLab:   http://localhost:${HTTP_HOST_PORT}
  image:    ${GITLAB_IMAGE}

  status:   task kind-status
  teardown: task kind-down
  smoke:    bash hack/spikes/e2e/smoke.sh   # expects ASSENT_SPIKE_HTTP_PORT=${HTTP_HOST_PORT}
EOF
