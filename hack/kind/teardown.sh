#!/usr/bin/env bash
# Delete the local assent kind lab cluster.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
source "${SCRIPT_DIR}/common.sh"

_kind_require_tools

if ! _kind_cluster_exists; then
  _kind_log "cluster ${CLUSTER_NAME} not present — nothing to do"
  exit 0
fi

_kind_log "deleting cluster ${CLUSTER_NAME}"
kind delete cluster --name "${CLUSTER_NAME}"
_kind_log "teardown complete"
