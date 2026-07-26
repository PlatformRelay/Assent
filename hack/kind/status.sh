#!/usr/bin/env bash
# Print local assent kind lab status (cluster, pod, readiness).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
source "${SCRIPT_DIR}/common.sh"

_kind_require_tools

if ! _kind_cluster_exists; then
  echo "cluster: absent (${CLUSTER_NAME})"
  exit 1
fi

echo "cluster: kind-${CLUSTER_NAME}"
if ! "${KCTL[@]}" get deploy/gitlab >/dev/null 2>&1; then
  echo "GitLab deploy: absent (run task kind-up)"
  exit 1
fi
"${KCTL[@]}" get deploy/gitlab
"${KCTL[@]}" get pods -l app=gitlab -o wide

code="$(curl -s -o /dev/null -w '%{http_code}' "http://localhost:${HTTP_HOST_PORT}/-/readiness" || true)"
echo "readiness: HTTP ${code} (http://localhost:${HTTP_HOST_PORT}/-/readiness)"
[[ "${code}" == "200" ]]
