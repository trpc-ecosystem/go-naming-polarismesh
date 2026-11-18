#!/bin/bash

set -euo pipefail

COMMON_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../internal" && pwd)"
source "${COMMON_DIR}/e2e_common.sh"

feature_common_init helloworld
feature_usage() {
    echo "Usage: ./examples/helloworld/verify.sh --polaris-server <host> [--debug]"
}
feature_parse_transport_args "$@"

SERVICE_NAME="trpc.examples.helloworld.Greeter"
CALLER_SERVICE="trpc.examples.helloworld.Client"
PROVIDER_PORT="${PROVIDER_PORT:-28261}"
PROVIDER_VERSION="${PROVIDER_VERSION:-v1.0.0}"

build_helloworld_binaries() {
    build_example_binary "${BUILD_DIR}/server" helloworld-server \
        ./examples/helloworld/server
    build_example_binary "${BUILD_DIR}/client" helloworld-client \
        ./examples/helloworld/client
}

start_helloworld_server() {
    local workdir="${BUILD_DIR}/server-run"
    rm -rf "${workdir}"
    mkdir -p "${workdir}"
    cp "${REPO_ROOT}/examples/helloworld/server/trpc_go.yaml" "${workdir}/trpc_go.yaml"
    local debug_flag=()
    if [[ "${DEBUG_MODE:-false}" == "true" ]]; then
        debug_flag=(-debug)
    fi
    (
        cd "${workdir}"
        exec nohup "${BUILD_DIR}/server" \
            -conf trpc_go.yaml \
            -polaris-server "${POLARIS_SERVER}" \
            -polaris-port "${POLARIS_GRPC_PORT}" \
            -namespace "${NAMESPACE}" \
            -service "${SERVICE_NAME}" \
            -version "${PROVIDER_VERSION}" \
            -port "${PROVIDER_PORT}" \
            ${debug_flag[@]+"${debug_flag[@]}"}
    ) > "${LOG_DIR}/server.log" 2>&1 &
    local pid=$!
    disown "${pid}" 2>/dev/null || true
    printf '%s|server|%s\n' "${pid}" "${PROVIDER_PORT}" >> "${PID_FILE}"
    echo "${pid}"
}

run_helloworld_client() {
    local workdir="${BUILD_DIR}/client-run"
    rm -rf "${workdir}"
    mkdir -p "${workdir}"
    cp "${REPO_ROOT}/examples/helloworld/client/trpc_go.yaml" "${workdir}/trpc_go.yaml"
    local debug_flag=()
    if [[ "${DEBUG_MODE:-false}" == "true" ]]; then
        debug_flag=(-debug)
    fi
    (
        cd "${workdir}"
        "${BUILD_DIR}/client" \
            -conf trpc_go.yaml \
            -polaris-server "${POLARIS_SERVER}" \
            -polaris-port "${POLARIS_GRPC_PORT}" \
            -namespace "${NAMESPACE}" \
            -service "${SERVICE_NAME}" \
            -caller "${CALLER_SERVICE}" \
            -message "tRPC-Go" \
            ${debug_flag[@]+"${debug_flag[@]}"}
    ) > "${LOG_DIR}/client.out" 2> "${LOG_DIR}/client.log"
}

run_helloworld_client_with_retry() {
    local attempt
    for attempt in $(seq 1 5); do
        if run_helloworld_client; then
            return 0
        fi
        echo "HelloWorld client attempt ${attempt} failed; retrying" >&2
        sleep 1
    done
    return 1
}

registered_host() {
    local response
    response="$(api_request GET \
        "/naming/v1/instances?namespace=${NAMESPACE}&service=${SERVICE_NAME}")"
    jq -r '.instances[0].host // empty' <<<"${response}"
}

wait_for_logged_host() {
    local host
    for _ in $(seq 1 40); do
        host="$(sed -n "s/.* listening on \(.*\):${PROVIDER_PORT} as .*/\1/p" \
            "${LOG_DIR}/server.log" | head -n1)"
        if [[ -n "${host}" ]]; then
            printf '%s\n' "${host}"
            return 0
        fi
        sleep 0.1
    done
    return 1
}

assert_call() {
    local output="$1"
    local path="$2"
    local expected_response="$3"
    local line
    line="$(grep -F "${path}" "${output}" | head -n1)"
    if [[ -z "${line}" ]]; then
        echo "missing ${path} result: $(cat "${output}")" >&2
        return 1
    fi
    if [[ "${line}" != *"address="*":${PROVIDER_PORT}"* ]]; then
        echo "${path} did not select port ${PROVIDER_PORT}: ${line}" >&2
        return 1
    fi
    if [[ "${line}" != *"${expected_response}"* ]]; then
        echo "${path} response mismatch: ${line}" >&2
        return 1
    fi
}

build_helloworld_binaries
ensure_namespace
ensure_service "${SERVICE_NAME}"
start_helloworld_server >/dev/null
wait_for_instances "${SERVICE_NAME}" 1

expected_host="$(wait_for_logged_host)" || {
    echo "server log did not contain the connection-derived host" >&2
    exit 1
}
actual_host="$(registered_host)"
if [[ -z "${expected_host}" || "${expected_host}" != "${actual_host}" ]]; then
    echo "registered host mismatch: connection-derived=${expected_host}, Polaris=${actual_host}" >&2
    exit 1
fi
echo "[case 1.1 connection-derived registration host] PASS"

run_helloworld_client_with_retry || {
    cat "${LOG_DIR}/client.log" >&2
    exit 1
}
assert_call "${LOG_DIR}/client.out" "target://Hello" \
    "Hello, tRPC-Go! (server ${PROVIDER_VERSION})"
echo "[case 1.2 target discovery] PASS"

assert_call "${LOG_DIR}/client.out" "service://Hello" \
    "Hello, tRPC-Go! (server ${PROVIDER_VERSION})"
assert_call "${LOG_DIR}/client.out" "service://Hi" \
    "Hi, tRPC-Go! (server ${PROVIDER_VERSION})"
echo "[case 1.3 service discovery] PASS"
