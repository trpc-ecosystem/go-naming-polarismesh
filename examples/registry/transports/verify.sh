#!/bin/bash

set -euo pipefail

COMMON_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../internal" && pwd)"
source "${COMMON_DIR}/e2e_common.sh"

feature_common_init registry-transports
feature_usage() {
    echo "Usage: ./examples/registry/transports/verify.sh --polaris-server <host> [--debug]"
}
feature_parse_transport_args "$@"

SERVICE_MULTI="trpc.polaris.registry.multi.Transport"
TRPC_PORT="${TRPC_PORT:-28351}"
HTTP_PORT="${HTTP_PORT:-28352}"
TTL_SECONDS="${TTL_SECONDS:-6}"
HEALTH_CHECK_TTL="${HEALTH_CHECK_TTL:-6}"
INSTANCE_MULTI_TRPC="multi-trpc-stable"
INSTANCE_MULTI_HTTP="multi-http-stable"

start_transport_provider() {
    local workdir="${BUILD_DIR}/multi-run"
    mkdir -p "${workdir}"
    cp "${SCRIPT_DIR}/provider/trpc_go-multi.yaml" "${workdir}/trpc_go.yaml"
    local extra=()
    if [[ "${DEBUG_MODE:-false}" == "true" ]]; then extra+=(-debug); fi
    (
        cd "${workdir}"
        exec nohup "${BUILD_DIR}/provider" \
            -conf trpc_go.yaml -polaris-server "${POLARIS_SERVER}" \
            -polaris-port "${POLARIS_GRPC_PORT}" -namespace "${NAMESPACE}" \
            -service "${SERVICE_MULTI}" -trpc-port "${TRPC_PORT}" -http-port "${HTTP_PORT}" \
            -version-a v1.0.0 -version-b v2.0.0 -weight-a 80 -weight-b 120 \
            -instance-a "${INSTANCE_MULTI_TRPC}" -instance-b "${INSTANCE_MULTI_HTTP}" \
            ${extra[@]+"${extra[@]}"}
    ) >> "${LOG_DIR}/multi.log" 2>&1 &
    local pid=$!
    disown "${pid}" 2>/dev/null || true
    printf '%s|multi|%s\n' "${pid}" "${TRPC_PORT}" >> "${PID_FILE}"
}

stop_transport_provider() {
    local pid
    pid="$(awk -F'|' '$2 == "multi" {print $1; exit}' "${PID_FILE}")"
    if [[ -z "${pid}" ]]; then echo "multi process is not recorded" >&2; return 1; fi
    stop_pid "${pid}"
    awk -F'|' -v pid="${pid}" '$1 != pid' "${PID_FILE}" > "${PID_FILE}.tmp"
    mv "${PID_FILE}.tmp" "${PID_FILE}"
}

wait_for_instances_zero() {
    local response
    for _ in $(seq 1 "${INSTANCE_QUERY_ATTEMPTS:-80}"); do
        response="$(query_instances "${SERVICE_MULTI}")"
        if instance_response_valid "${response}" &&
            jq -e '[.instances[]? | select(. != null)] | length == 0' >/dev/null <<<"${response}"; then
            return 0
        fi
        sleep 0.25
    done
    echo "instances did not disappear: ${response}" >&2
    return 1
}

assert_multi_instances() {
    local response
    response="$(query_instances "${SERVICE_MULTI}")"
    if ! jq -e --argjson trpc_port "${TRPC_PORT}" --argjson http_port "${HTTP_PORT}" \
        --argjson ttl "${HEALTH_CHECK_TTL}" --arg trpc_id "${INSTANCE_MULTI_TRPC}" \
        --arg http_id "${INSTANCE_MULTI_HTTP}" '
        def raw_value: if type == "object" and has("value") then .value else . end;
        [.instances[]? | select(. != null)] as $instances
        | ($instances | length) == 2
        and ([$instances[] | select(
            .host != "" and .port == $trpc_port and .id == $trpc_id
            and .protocol == "trpc" and .version == "v1.0.0" and .weight == 80
            and ((.healthy | raw_value) == true)
            and ((.enable_health_check // .enableHealthCheck | raw_value) == true)
            and (((.health_check // .healthCheck).heartbeat.ttl | raw_value) == $ttl)
            and .metadata.transport == "trpc" and .metadata.example == "registry-transports"
        )] | length == 1)
        and ([$instances[] | select(
            .host != "" and .port == $http_port and .id == $http_id
            and .protocol == "http" and .version == "v2.0.0" and .weight == 120
            and ((.healthy | raw_value) == true)
            and ((.enable_health_check // .enableHealthCheck | raw_value) == true)
            and (((.health_check // .healthCheck).heartbeat.ttl | raw_value) == $ttl)
            and .metadata.transport == "http" and .metadata.example == "registry-transports"
        )] | length == 1)
    ' >/dev/null <<<"${response}"; then
        echo "multi endpoint assertion failed: ${response}" >&2
        return 1
    fi
}

assert_service_offline() {
    local response
    response="$(api_request GET "/naming/v1/services?namespace=${NAMESPACE}&name=${SERVICE_MULTI}")"
    if ! jq -e --arg service "${SERVICE_MULTI}" '
        .code == 200000 and
        ([.services[]? | select(.name == $service)] as $services
        | ($services | length) == 1 and $services[0].service_status == "OFFLINE"
        and $services[0].total_instance_count == 0)
    ' >/dev/null <<<"${response}"; then
        echo "service did not become offline: ${response}" >&2
        return 1
    fi
}

instance_revision() {
    local response
    response="$(query_instances "${SERVICE_MULTI}")"
    jq -r --arg id "$1" '.instances[]? | select(.id == $id) | .revision // empty' <<<"${response}"
}

build_example_binary "${BUILD_DIR}/provider" registry-transports-provider \
    ./examples/registry/transports/provider
ensure_namespace
response="$(api_request GET "/naming/v1/services?namespace=${NAMESPACE}&name=${SERVICE_MULTI}")"
if ! jq -e --arg service "${SERVICE_MULTI}" '
    .code == 200000 and ([.services[]? | select(.name == $service)] | length) == 0
' >/dev/null <<<"${response}"; then
    echo "service is not absent; inspect or explicitly clean it first: ${response}" >&2
    exit 1
fi
start_transport_provider
wait_for_instances "${SERVICE_MULTI}" 2
wait_for_instance_health "${SERVICE_MULTI}" true "${HEALTH_CHECK_TTL}" \
    "${INSTANCE_MULTI_TRPC}" "${INSTANCE_MULTI_HTTP}"
assert_multi_instances
echo "[case 1.1 multiple endpoints] PASS"
response="$(api_request GET "/naming/v1/services?namespace=${NAMESPACE}&name=${SERVICE_MULTI}")"
if ! jq -e --arg service "${SERVICE_MULTI}" '
    [.services[]? | select(.name == $service)] as $services
    | ($services | length) == 1
    and (($services[0].service_type | tostring) == "SERVICE_TYPE_MICROSERVICE"
        or $services[0].service_type == 0)
' >/dev/null <<<"${response}"; then
    echo "microservice type assertion failed: ${response}" >&2
    exit 1
fi
echo "[case 1.2 microservice service type] PASS"

sleep "${TTL_SECONDS}"
wait_for_instance_health "${SERVICE_MULTI}" true "${HEALTH_CHECK_TTL}" \
    "${INSTANCE_MULTI_TRPC}" "${INSTANCE_MULTI_HTTP}"
pid="$(awk -F'|' '$2 == "multi" {print $1; exit}' "${PID_FILE}")"
if [[ -z "${pid}" ]]; then echo "multi process is not recorded" >&2; exit 1; fi
kill -STOP "${pid}"
negative_status=0
set +e
wait_for_instance_health "${SERVICE_MULTI}" false "${HEALTH_CHECK_TTL}" \
    "${INSTANCE_MULTI_TRPC}" "${INSTANCE_MULTI_HTTP}" || negative_status=$?
kill -CONT "${pid}"
set -e
if [[ "${negative_status}" -ne 0 ]]; then exit "${negative_status}"; fi
wait_for_instance_health "${SERVICE_MULTI}" true "${HEALTH_CHECK_TTL}" \
    "${INSTANCE_MULTI_TRPC}" "${INSTANCE_MULTI_HTTP}"
echo "[case 1.3 TTL and heartbeat] PASS"

revision_trpc_before="$(instance_revision "${INSTANCE_MULTI_TRPC}")"
revision_http_before="$(instance_revision "${INSTANCE_MULTI_HTTP}")"
if [[ -z "${revision_trpc_before}" || -z "${revision_http_before}" ]]; then
    echo "instance revision is empty before deregistration" >&2
    exit 1
fi
stop_transport_provider
wait_for_instances_zero
assert_service_offline
echo "[case 2.1 deregistration and offline service] PASS"
start_transport_provider
wait_for_instances "${SERVICE_MULTI}" 2
wait_for_instance_health "${SERVICE_MULTI}" true "${HEALTH_CHECK_TTL}" \
    "${INSTANCE_MULTI_TRPC}" "${INSTANCE_MULTI_HTTP}"
assert_multi_instances
echo "[case 2.2 stable instance IDs] PASS"
revision_trpc_after="$(instance_revision "${INSTANCE_MULTI_TRPC}")"
revision_http_after="$(instance_revision "${INSTANCE_MULTI_HTTP}")"
if [[ -z "${revision_trpc_after}" || "${revision_trpc_after}" == "${revision_trpc_before}" ||
      -z "${revision_http_after}" || "${revision_http_after}" == "${revision_http_before}" ]]; then
    echo "instance revisions did not change after re-registration" >&2
    exit 1
fi
echo "[case 2.3 revision after re-registration] PASS"
stop_transport_provider
wait_for_instances_zero
echo "[case 2.4 repeat deregistration] PASS"
