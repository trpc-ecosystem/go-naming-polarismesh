#!/bin/bash

set -euo pipefail

COMMON_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../internal" && pwd)"
source "${COMMON_DIR}/e2e_common.sh"

feature_common_init registry-metadata
feature_usage() {
    echo "Usage: ./examples/registry/metadata/verify.sh --polaris-server <host> [--debug]"
}
feature_parse_transport_args "$@"

SERVICE_NAME="trpc.polaris.registry.metadata.Echo"
PROVIDER_VERSION="${PROVIDER_VERSION:-v1.2.3}"
SECOND_PROVIDER_VERSION="${SECOND_PROVIDER_VERSION:-v1.3.0}"
ALT_HOST_VERSION="${ALT_HOST_VERSION:-v1.4.0}"
REPLACEMENT_VERSION="${REPLACEMENT_VERSION:-v1.2.4}"
PROVIDER_PORT="${PROVIDER_PORT:-28271}"
SECOND_PROVIDER_PORT="${SECOND_PROVIDER_PORT:-28273}"
ALT_HOST_PORT="${ALT_HOST_PORT:-28279}"
REPLACEMENT_PORT="${REPLACEMENT_PORT:-28275}"
FAILURE_PORT="${FAILURE_PORT:-28277}"
REGISTRATION_PORT_PRIMARY="${REGISTRATION_PORT_PRIMARY:-28272}"
REGISTRATION_PORT_SECONDARY="${REGISTRATION_PORT_SECONDARY:-28274}"
REGISTRATION_PORT_ALT_HOST="${REGISTRATION_PORT_ALT_HOST:-28280}"
TTL_WAIT_SECONDS="${TTL_WAIT_SECONDS:-7}"
HEALTH_CHECK_TTL="${HEALTH_CHECK_TTL:-6}"

start_dynamic_provider() {
    local name="$1" version="$2" registration_port="$3" listen_port="$4"
    local bind_host="${5:-}" mode="${6:-dynamic}"
    local workdir="${BUILD_DIR}/${name}-run"
    mkdir -p "${workdir}"
    cp "${SCRIPT_DIR}/provider/trpc_go.yaml" "${workdir}/trpc_go.yaml"
    local extra=()
    if [[ "${DEBUG_MODE:-false}" == "true" ]]; then extra+=(-debug); fi
    if [[ -n "${bind_host}" ]]; then extra+=(-host "${bind_host}"); fi
    (
        cd "${workdir}"
        exec nohup "${BUILD_DIR}/provider" \
            -conf trpc_go.yaml -polaris-server "${POLARIS_SERVER}" \
            -polaris-port "${POLARIS_GRPC_PORT}" -namespace "${NAMESPACE}" \
            -service "${SERVICE_NAME}" -version "${version}" \
            -registration-port "${registration_port}" -metadata-mode "${mode}" \
            -port "${listen_port}" ${extra[@]+"${extra[@]}"}
    ) > "${LOG_DIR}/${name}.log" 2>&1 &
    local pid=$!
    disown "${pid}" 2>/dev/null || true
    printf '%s|%s|%s\n' "${pid}" "${name}" "${listen_port}" >> "${PID_FILE}"
}

assert_snapshot_instance() {
    local port="$1" version="$2" expected_host="${3:-}"
    local response
    response="$(query_instances "${SERVICE_NAME}")"
    if ! jq -e --argjson port "${port}" --arg version "${version}" \
        --arg host "${expected_host}" --arg service "${SERVICE_NAME}" \
        --argjson ttl "${HEALTH_CHECK_TTL}" '
        def raw_value: if type == "object" and has("value") then .value else . end;
        [.instances[]? | select(. != null and .port == $port)] as $instances
        | ($instances | length) == 1
        and ($instances[0] as $instance
            | $instance.host != ""
            and ($host == "" or $instance.host == $host)
            and (($instance.protocol // "") == "")
            and $instance.version == $version
            and (($instance.healthy | raw_value) == true)
            and (($instance.enable_health_check // $instance.enableHealthCheck | raw_value) == true)
            and ((($instance.health_check // $instance.healthCheck).heartbeat.ttl | raw_value) == $ttl)
            and $instance.metadata.deployment == "green"
            and (($instance.metadata.descriptor | fromjson) as $descriptor
                | $descriptor.service == $service and $descriptor.version == $version))
    ' >/dev/null <<<"${response}"; then
        echo "snapshot assertion failed for port ${port}: ${response}" >&2
        return 1
    fi
}

wait_for_replacement() {
    local response
    for _ in $(seq 1 "${INSTANCE_QUERY_ATTEMPTS:-80}"); do
        response="$(query_instances "${SERVICE_NAME}")"
        if jq -e --argjson port "${REGISTRATION_PORT_PRIMARY}" \
            --arg version "${REPLACEMENT_VERSION}" '
            [.instances[]? | select(. != null and .port == $port and .version == $version)]
            | length == 1
        ' >/dev/null <<<"${response}"; then return 0; fi
        sleep 0.25
    done
    echo "replacement snapshot did not appear: ${response}" >&2
    return 1
}

build_example_binary "${BUILD_DIR}/provider" registry-metadata-provider \
    ./examples/registry/metadata/provider
ensure_namespace
response="$(api_request GET "/naming/v1/services?namespace=${NAMESPACE}&name=${SERVICE_NAME}")"
if ! jq -e --arg service "${SERVICE_NAME}" '
    .code == 200000 and ([.services[]? | select(.name == $service)] | length) == 0
' >/dev/null <<<"${response}"; then
    echo "service is not absent; inspect or explicitly clean it first: ${response}" >&2
    exit 1
fi

start_dynamic_provider snapshot-primary "${PROVIDER_VERSION}" \
    "${REGISTRATION_PORT_PRIMARY}" "${PROVIDER_PORT}"
start_dynamic_provider snapshot-secondary "${SECOND_PROVIDER_VERSION}" \
    "${REGISTRATION_PORT_SECONDARY}" "${SECOND_PROVIDER_PORT}"
start_dynamic_provider snapshot-alt-host "${ALT_HOST_VERSION}" \
    "${REGISTRATION_PORT_ALT_HOST}" "${ALT_HOST_PORT}" 127.0.0.1
wait_for_instances "${SERVICE_NAME}" 3
primary_id="$(instance_id_by_port "${SERVICE_NAME}" "${REGISTRATION_PORT_PRIMARY}")"
secondary_id="$(instance_id_by_port "${SERVICE_NAME}" "${REGISTRATION_PORT_SECONDARY}")"
alternate_id="$(instance_id_by_port "${SERVICE_NAME}" "${REGISTRATION_PORT_ALT_HOST}")"
if [[ -z "${primary_id}" || -z "${secondary_id}" || -z "${alternate_id}" ]]; then
    echo "cannot resolve all snapshot instance IDs" >&2
    exit 1
fi
wait_for_instance_health "${SERVICE_NAME}" true "${HEALTH_CHECK_TTL}" \
    "${primary_id}" "${secondary_id}" "${alternate_id}"
assert_snapshot_instance "${REGISTRATION_PORT_PRIMARY}" "${PROVIDER_VERSION}"
echo "[case 1.1 complete registration snapshot] PASS"
assert_snapshot_instance "${REGISTRATION_PORT_SECONDARY}" "${SECOND_PROVIDER_VERSION}"
echo "[case 1.2 independent registration version] PASS"
assert_snapshot_instance "${REGISTRATION_PORT_ALT_HOST}" "${ALT_HOST_VERSION}" 127.0.0.1
echo "[case 1.3 independent registration host] PASS"

sleep "${TTL_WAIT_SECONDS}"
wait_for_instance_health "${SERVICE_NAME}" true "${HEALTH_CHECK_TTL}" \
    "${primary_id}" "${secondary_id}" "${alternate_id}"
pid="$(awk -F'|' '$2 == "snapshot-primary" {print $1; exit}' "${PID_FILE}")"
if [[ -z "${pid}" ]]; then echo "primary process is not recorded" >&2; exit 1; fi
kill -STOP "${pid}"
negative_status=0
set +e
wait_for_instance_health "${SERVICE_NAME}" false "${HEALTH_CHECK_TTL}" "${primary_id}" || negative_status=$?
if [[ "${negative_status}" -eq 0 ]]; then
    wait_for_instance_health "${SERVICE_NAME}" true "${HEALTH_CHECK_TTL}" \
        "${secondary_id}" "${alternate_id}" || negative_status=$?
fi
kill -CONT "${pid}"
set -e
if [[ "${negative_status}" -ne 0 ]]; then exit "${negative_status}"; fi
wait_for_instance_health "${SERVICE_NAME}" true "${HEALTH_CHECK_TTL}" \
    "${primary_id}" "${secondary_id}" "${alternate_id}"
echo "[case 1.4 TTL and auto heartbeat] PASS"

response="$(api_request GET "/naming/v1/services?namespace=${NAMESPACE}&name=${SERVICE_NAME}")"
if ! jq -e --arg service "${SERVICE_NAME}" '
    [.services[]? | select(.name == $service)] as $services
    | ($services | length) == 1
    and (($services[0].service_type | tostring) == "SERVICE_TYPE_MICROSERVICE"
        or $services[0].service_type == 0)
' >/dev/null <<<"${response}"; then
    echo "microservice type assertion failed: ${response}" >&2
    exit 1
fi
echo "[case 1.5 microservice service type] PASS"

workdir="${BUILD_DIR}/failure-run"
mkdir -p "${workdir}"
cp "${SCRIPT_DIR}/provider/trpc_go.yaml" "${workdir}/trpc_go.yaml"
set +e
(
    cd "${workdir}"
    timeout 20s "${BUILD_DIR}/provider" \
        -conf trpc_go.yaml -polaris-server "${POLARIS_SERVER}" \
        -polaris-port "${POLARIS_GRPC_PORT}" -namespace "${NAMESPACE}" \
        -service "${SERVICE_NAME}" -version "${PROVIDER_VERSION}" \
        -registration-port "${REGISTRATION_PORT_PRIMARY}" -metadata-mode failure \
        -port "${FAILURE_PORT}"
) > "${LOG_DIR}/provider-failure.log" 2>&1
exit_code=$?
set -e
if [[ "${exit_code}" -eq 0 || "${exit_code}" -eq 124 ]] ||
    ! grep -q "snapshot unavailable" "${LOG_DIR}/provider-failure.log"; then
    echo "provider did not fail with the expected snapshot error" >&2
    cat "${LOG_DIR}/provider-failure.log" >&2
    exit 1
fi
wait_for_instances "${SERVICE_NAME}" 3
echo "[case 1.6 provider failure before registration] PASS"

start_dynamic_provider snapshot-replacement "${REPLACEMENT_VERSION}" \
    "${REGISTRATION_PORT_PRIMARY}" "${REPLACEMENT_PORT}"
wait_for_replacement
new_id="$(instance_id_by_port "${SERVICE_NAME}" "${REGISTRATION_PORT_PRIMARY}")"
if [[ "${new_id}" != "${primary_id}" ]]; then echo "replacement changed instance ID" >&2; exit 1; fi
wait_for_instance_health "${SERVICE_NAME}" true "${HEALTH_CHECK_TTL}" \
    "${new_id}" "${secondary_id}" "${alternate_id}"
assert_snapshot_instance "${REGISTRATION_PORT_PRIMARY}" "${REPLACEMENT_VERSION}"
assert_snapshot_instance "${REGISTRATION_PORT_SECONDARY}" "${SECOND_PROVIDER_VERSION}"
assert_snapshot_instance "${REGISTRATION_PORT_ALT_HOST}" "${ALT_HOST_VERSION}" 127.0.0.1
response="$(query_instances "${SERVICE_NAME}")"
if ! jq -e --argjson port "${REGISTRATION_PORT_PRIMARY}" \
    --arg replacement ":${REPLACEMENT_PORT}" '
    [.instances[]? | select(. != null and .port == $port)] as $instances
    | ($instances | length) == 1
    and (($instances[0].metadata.descriptor | fromjson).endpoint | contains($replacement))
' >/dev/null <<<"${response}"; then
    echo "replacement descriptor was not updated: ${response}" >&2
    exit 1
fi
echo "[case 1.7 same host and registration port replacement] PASS"
