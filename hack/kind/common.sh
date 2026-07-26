#!/usr/bin/env bash
# Shared helpers for the local assent kind lab (GitLab CE in-cluster).
# Source from setup/teardown/status — do not execute directly.
set -euo pipefail

KIND_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${KIND_DIR}/../.." && pwd)"

readonly CLUSTER_NAME="${ASSENT_KIND_CLUSTER:-assent}"
readonly KIND_CONFIG="${ASSENT_KIND_CONFIG:-${KIND_DIR}/kind-config.yaml}"
readonly GITLAB_IMAGE="${ASSENT_GITLAB_IMAGE:-gitlab/gitlab-ce:19.2.0-ce.0}"
readonly HTTP_HOST_PORT="${ASSENT_GITLAB_HTTP_PORT:-8929}"
readonly READY_TIMEOUT="${ASSENT_KIND_READY_TIMEOUT:-900}"
readonly KCTL=(kubectl --context "kind-${CLUSTER_NAME}")

_kind_log() { printf 'assent-kind: %s\n' "$*"; }

_kind_require_tools() {
  local missing=()
  command -v kind >/dev/null 2>&1 || missing+=(kind)
  command -v docker >/dev/null 2>&1 || missing+=(docker)
  command -v kubectl >/dev/null 2>&1 || missing+=(kubectl)
  command -v curl >/dev/null 2>&1 || missing+=(curl)
  if ((${#missing[@]} > 0)); then
    echo "ERROR: missing required tools: ${missing[*]}" >&2
    exit 1
  fi
}

_kind_platform() {
  case "$(uname -m)" in
    arm64 | aarch64) printf 'linux/arm64' ;;
    *) printf 'linux/amd64' ;;
  esac
}

_kind_cluster_exists() {
  kind get clusters 2>/dev/null | grep -qx "${CLUSTER_NAME}"
}

_kind_ensure_cluster() {
  if _kind_cluster_exists; then
    _kind_log "cluster ${CLUSTER_NAME} already exists"
    return 0
  fi
  _kind_log "creating cluster ${CLUSTER_NAME}"
  kind create cluster --name "${CLUSTER_NAME}" --config "${KIND_CONFIG}" --wait 120s
}

_kind_load_gitlab_image() {
  docker image inspect "${GITLAB_IMAGE}" >/dev/null 2>&1 || {
    _kind_log "pulling ${GITLAB_IMAGE}"
    docker pull "${GITLAB_IMAGE}"
  }
  # kind load docker-image breaks against Docker's containerd image store
  # (Spike B): export a single-platform archive, then load that.
  local archive platform
  platform="$(_kind_platform)"
  archive="$(mktemp -t assent-kind-gitlab-image).tar"
  _kind_log "side-loading ${GITLAB_IMAGE} (${platform}) into kind"
  docker save --platform "${platform}" "${GITLAB_IMAGE}" -o "${archive}" 2>/dev/null ||
    docker save "${GITLAB_IMAGE}" -o "${archive}"
  kind load image-archive "${archive}" --name "${CLUSTER_NAME}"
  rm -f "${archive}"
}

# Slim Omnibus — same shape as Spike B (hack/spikes/e2e/boot-*.sh).
_kind_omnibus_config() {
  cat <<EOF
external_url 'http://localhost:${HTTP_HOST_PORT}'
prometheus_monitoring['enable'] = false
registry['enable'] = false
gitlab_kas['enable'] = false
gitlab_rails['gitlab_email_enabled'] = false
gitlab_rails['monitoring_whitelist'] = ['0.0.0.0/0']
puma['worker_processes'] = 0
sidekiq['concurrency'] = 5
EOF
}

_kind_apply_gitlab() {
  local omnibus indented
  omnibus="$(_kind_omnibus_config)"
  indented="$(printf '%s\n' "${omnibus}" | sed 's/^/                /')"
  _kind_log "applying GitLab CE-in-pod manifests"
  "${KCTL[@]}" apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gitlab
  labels:
    app.kubernetes.io/name: gitlab
    app.kubernetes.io/part-of: assent-kind-lab
spec:
  replicas: 1
  selector:
    matchLabels: {app: gitlab}
  template:
    metadata:
      labels: {app: gitlab}
    spec:
      containers:
        - name: gitlab
          image: ${GITLAB_IMAGE}
          imagePullPolicy: IfNotPresent
          env:
            - name: GITLAB_OMNIBUS_CONFIG
              value: |
${indented}
          ports:
            - {name: http, containerPort: ${HTTP_HOST_PORT}}
            - {name: ssh, containerPort: 22}
          volumeMounts:
            - {name: shm, mountPath: /dev/shm}
      volumes:
        - name: shm
          emptyDir: {medium: Memory, sizeLimit: 256Mi}
---
apiVersion: v1
kind: Service
metadata:
  name: gitlab
  labels:
    app.kubernetes.io/name: gitlab
    app.kubernetes.io/part-of: assent-kind-lab
spec:
  type: NodePort
  selector: {app: gitlab}
  ports:
    - name: http
      port: ${HTTP_HOST_PORT}
      targetPort: ${HTTP_HOST_PORT}
      nodePort: 30080
    - name: ssh
      port: 22
      targetPort: 22
      nodePort: 30022
EOF
}

_kind_wait_ready() {
  local deadline code
  deadline=$(($(date +%s) + READY_TIMEOUT))
  _kind_log "waiting for http://localhost:${HTTP_HOST_PORT}/-/readiness (timeout ${READY_TIMEOUT}s)"
  while true; do
    code="$(curl -s -o /dev/null -w '%{http_code}' "http://localhost:${HTTP_HOST_PORT}/-/readiness" || true)"
    if [[ "${code}" == "200" ]]; then
      _kind_log "GitLab ready on http://localhost:${HTTP_HOST_PORT}"
      return 0
    fi
    if (($(date +%s) > deadline)); then
      echo "ERROR: readiness timeout after ${READY_TIMEOUT}s (last HTTP ${code})" >&2
      "${KCTL[@]}" get pods -o wide >&2 || true
      "${KCTL[@]}" logs deploy/gitlab --tail 40 >&2 || true
      return 1
    fi
    sleep 5
  done
}
