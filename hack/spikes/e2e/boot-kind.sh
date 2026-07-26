#!/usr/bin/env bash
# Spike B (P2-E2-S01) — boot GitLab CE inside a kind cluster ("kind" profile).
#
# Uses the existing hack/kind/kind-config.yaml scaffold (cluster `assent`, NodePort 30080
# mapped to host port 8929) and a plain CE-in-pod manifest — no Helm, to keep the two
# profiles comparable (same image, same Omnibus config as boot-testcontainer.sh).
#
# Idempotent: tears down any previous instance before booting. `--teardown` deletes the
# kind cluster. On success prints exactly one machine-readable line:
#   RESULT boot_seconds=<n> rss_mb=<n>
#
# boot_seconds includes kind cluster creation (that IS the profile's cost in CI); the
# GitLab image is pre-pulled into Docker and side-loaded into the node with `kind load`
# outside the timer, mirroring the cached-image comparison in boot-testcontainer.sh.
# rss_mb is the GitLab container's own cgroup-v2 memory.current after a settle period.
set -euo pipefail

IMAGE="${ASSENT_SPIKE_GITLAB_IMAGE:-gitlab/gitlab-ce:19.2.0-ce.0}"
CLUSTER="assent"
HTTP_PORT=8929 # fixed by hack/kind/kind-config.yaml extraPortMappings
READY_TIMEOUT="${ASSENT_SPIKE_READY_TIMEOUT:-900}"
SETTLE_SECONDS="${ASSENT_SPIKE_SETTLE_SECONDS:-60}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KIND_CONFIG="${SCRIPT_DIR}/../../kind/kind-config.yaml"
KCTL=(kubectl --context "kind-${CLUSTER}")

OMNIBUS_CONFIG="
external_url 'http://localhost:${HTTP_PORT}'
prometheus_monitoring['enable'] = false
registry['enable'] = false
gitlab_kas['enable'] = false
gitlab_rails['gitlab_email_enabled'] = false
gitlab_rails['monitoring_whitelist'] = ['0.0.0.0/0']
puma['worker_processes'] = 0
sidekiq['concurrency'] = 5
"

teardown() {
  kind delete cluster --name "${CLUSTER}" >/dev/null 2>&1 || true
}

if [[ "${1:-}" == "--teardown" ]]; then
  teardown
  echo "teardown complete"
  exit 0
fi

teardown # idempotent: always start from a clean slate

docker image inspect "${IMAGE}" >/dev/null 2>&1 || docker pull "${IMAGE}" >/dev/null

start=$(date +%s)
kind create cluster --name "${CLUSTER}" --config "${KIND_CONFIG}" --wait 120s >/dev/null

# Side-load the cached image; the clock pauses around this because load time is a
# host-cache artifact (see header — parity with the testcontainer profile).
# `kind load docker-image` breaks against Docker's containerd image store (it imports
# --all-platforms but only the host platform's blobs exist locally), so export the
# single-platform archive explicitly and load that.
load_start=$(date +%s)
archive=$(mktemp -t assent-spike-gitlab-image).tar
trap 'rm -f "${archive}"' EXIT
docker save --platform linux/arm64 "${IMAGE}" -o "${archive}" 2>/dev/null ||
  docker save "${IMAGE}" -o "${archive}"
kind load image-archive "${archive}" --name "${CLUSTER}" >/dev/null
rm -f "${archive}"
load_seconds=$(($(date +%s) - load_start))

"${KCTL[@]}" apply -f - >/dev/null <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gitlab
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
          image: ${IMAGE}
          imagePullPolicy: IfNotPresent
          env:
            - name: GITLAB_OMNIBUS_CONFIG
              value: |
$(printf '%s\n' "${OMNIBUS_CONFIG}" | sed 's/^/                /')
          ports:
            - containerPort: ${HTTP_PORT}
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
spec:
  type: NodePort
  selector: {app: gitlab}
  ports:
    - port: ${HTTP_PORT}
      targetPort: ${HTTP_PORT}
      nodePort: 30080
EOF

deadline=$((start + load_seconds + READY_TIMEOUT))
while true; do
  code=$(curl -s -o /dev/null -w '%{http_code}' "http://localhost:${HTTP_PORT}/-/readiness" || true)
  [[ "${code}" == "200" ]] && break
  if (($(date +%s) > deadline)); then
    echo "ERROR: readiness timeout after ${READY_TIMEOUT}s (last code: ${code})" >&2
    "${KCTL[@]}" get pods -o wide >&2 || true
    "${KCTL[@]}" logs deploy/gitlab --tail 30 >&2 || true
    exit 1
  fi
  sleep 5
done
boot_seconds=$(($(date +%s) - start - load_seconds))

sleep "${SETTLE_SECONDS}"
rss_bytes=$("${KCTL[@]}" exec deploy/gitlab -- cat /sys/fs/cgroup/memory.current)
rss_mb=$((rss_bytes / 1024 / 1024))

echo "RESULT boot_seconds=${boot_seconds} rss_mb=${rss_mb}"
