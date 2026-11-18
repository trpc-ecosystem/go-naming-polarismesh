#!/bin/bash

set -euo pipefail

COMMON_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../internal" && pwd)"
source "${COMMON_DIR}/e2e_common.sh"

feature_common_init route-metadata
feature_usage() {
    echo "Usage: ./examples/route/metadata/verify.sh --polaris-server <host> [--debug]"
}
feature_parse_transport_args "$@"

SERVICE_NAME="trpc.polaris.e2e.RouteMetadata"
CALLER_SERVICE="trpc.polaris.e2e.RouteMetadataCaller"
PORT_RED="${PORT_RED:-28221}"
PORT_BLUE="${PORT_BLUE:-28222}"
CLIENT_LISTEN="${CLIENT_LISTEN:-18221}"
CLIENT_URL="http://127.0.0.1:${CLIENT_LISTEN}"

build_shared_binaries
ensure_namespace
ensure_service "${SERVICE_NAME}"
start_provider provider-red "${SERVICE_NAME}" "${PORT_RED}" 100 "env=red,zone=red" >/dev/null
start_provider provider-blue "${SERVICE_NAME}" "${PORT_BLUE}" 100 "env=blue,zone=blue" >/dev/null
wait_for_instances "${SERVICE_NAME}" 2
start_client "${SERVICE_NAME}" "${CALLER_SERVICE}" >/dev/null
wait_for_http "${CLIENT_URL}/health"

assert_metadata_route() {
    local query="$1"
    local expected_port="$2"
    local name="$3"
    local case_id="$4"
    local result
    result="$(curl -fsS --max-time 60 \
        "${CLIENT_URL}/sequence?mode=target&lb=polaris_wr&count=16&${query}")"
    if ! jq -e --arg port ":${expected_port}" '
        .successes == 16
        and (.addresses | length) == 1
        and ([.addresses | keys[] | select(endswith($port))] | length) == 1
    ' >/dev/null <<<"${result}"; then
        echo "${name} failed: ${result}" >&2
        exit 1
    fi
    echo "[case ${case_id} ${name}] PASS"
}

assert_metadata_route "dest_meta=env%3Dred" "${PORT_RED}" "destination metadata red" "1.1"
assert_metadata_route "dest_meta=env%3Dblue" "${PORT_BLUE}" "destination metadata blue" "1.1"
assert_metadata_route "disable_router=true&callee_env=red" "${PORT_RED}" "callee env red" "1.2"

result="$(curl -fsS --max-time 30 \
    "${CLIENT_URL}/sequence?mode=target&lb=polaris_wr&count=5&dest_meta=env%3Dmissing")"
if ! jq -e '.successes == 0 and .failures == 5' >/dev/null <<<"${result}"; then
    echo "negative metadata route failed: ${result}" >&2
    exit 1
fi
echo "[case 1.3 missing metadata has no match] PASS"
