#!/bin/bash

set -euo pipefail

COMMON_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../internal" && pwd)"
source "${COMMON_DIR}/e2e_common.sh"

feature_common_init loadbalancer
feature_usage() {
    echo "Usage: ./examples/loadbalancer/verify.sh --polaris-server <host> [--debug]"
}
feature_parse_transport_args "$@"

SERVICE_NAME="trpc.polaris.e2e.LoadBalancer"
CALLER_SERVICE="trpc.polaris.e2e.LoadBalancerCaller"
PORT_A="${PORT_A:-28211}"
PORT_B="${PORT_B:-28212}"
CLIENT_LISTEN="${CLIENT_LISTEN:-18211}"
CLIENT_URL="http://127.0.0.1:${CLIENT_LISTEN}"

build_shared_binaries
ensure_namespace
ensure_service "${SERVICE_NAME}"
start_provider provider-a "${SERVICE_NAME}" "${PORT_A}" 100 "" >/dev/null
start_provider provider-b "${SERVICE_NAME}" "${PORT_B}" 20 "" >/dev/null
wait_for_instances "${SERVICE_NAME}" 2
start_client "${SERVICE_NAME}" "${CALLER_SERVICE}" >/dev/null
wait_for_http "${CLIENT_URL}/health"

for lb in polaris_hash polaris_ring_hash polaris_maglev; do
    result="$(curl -fsS --max-time 90 \
        "${CLIENT_URL}/sequence?mode=service&lb=${lb}&key=${lb}-distribution&count=48")"
    if ! jq -e --arg a ":${PORT_A}" --arg b ":${PORT_B}" '
        .successes == 48
        and ([.addresses | keys[] | select(endswith($a))] | length > 0)
        and ([.addresses | keys[] | select(endswith($b))] | length > 0)
    ' >/dev/null <<<"${result}"; then
        echo "${lb} distribution assertion failed: ${result}" >&2
        exit 1
    fi
    echo "[case 1.1 ${lb} distribution] PASS"
done

for lb in polaris_hash polaris_ring_hash polaris_maglev; do
    first=""
    for _ in $(seq 1 8); do
        result="$(curl -fsS --max-time 20 \
            "${CLIENT_URL}/call?mode=service&lb=${lb}&key=stable-key")"
        address="$(jq -r '.address' <<<"${result}")"
        if [[ -z "${first}" ]]; then
            first="${address}"
        elif [[ "${address}" != "${first}" ]]; then
            echo "${lb} is not stable for one hash key: ${first} vs ${address}" >&2
            exit 1
        fi
    done
    echo "[case 1.2 ${lb} hash stability] PASS"
done

result="$(curl -fsS --max-time 120 \
    "${CLIENT_URL}/sequence?mode=service&lb=polaris_wr&count=200")"
if ! jq -e --arg a ":${PORT_A}" --arg b ":${PORT_B}" '
    ([.addresses | to_entries[] | select(.key | endswith($a)) | .value] | add // 0) as $countA
    | ([.addresses | to_entries[] | select(.key | endswith($b)) | .value] | add // 0) as $countB
    | .successes == 200 and ($countA > $countB) and ($countB > 0)
' >/dev/null <<<"${result}"; then
    echo "weighted random assertion failed: ${result}" >&2
    exit 1
fi
echo "[case 1.3 weighted random distribution] PASS"
