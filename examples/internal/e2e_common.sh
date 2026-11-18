#!/bin/bash

set -euo pipefail

COMMON_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${COMMON_DIR}/../.." && pwd)"

feature_common_init() {
    FEATURE_NAME="$1"
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[1]}")" && pwd)"
    BUILD_DIR="${SCRIPT_DIR}/.build"
    LOG_DIR="${SCRIPT_DIR}/.logs"
    PID_FILE="${SCRIPT_DIR}/.${FEATURE_NAME}-pids"
    NAMESPACE="${POLARIS_NAMESPACE:-go-naming-polarismesh-e2e}"
    POLARIS_SERVER="${POLARIS_SERVER:-}"
    POLARIS_GRPC_PORT="${POLARIS_GRPC_PORT:-8091}"
    POLARIS_HTTP_PORT="${POLARIS_HTTP_PORT:-8090}"
    POLARIS_TOKEN="${POLARIS_TOKEN:-}"
    local running
    running="$(collect_feature_processes)"
    if [[ -n "${running}" ]]; then
        echo "${FEATURE_NAME} processes are already running; run cleanup.sh first" >&2
        show_feature_processes "${running}" >&2
        return 2
    fi
    mkdir -p "${BUILD_DIR}" "${LOG_DIR}"
    : > "${PID_FILE}"
}

feature_cleanup_init() {
    FEATURE_NAME="$1"
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[1]}")" && pwd)"
    BUILD_DIR="${SCRIPT_DIR}/.build"
    LOG_DIR="${SCRIPT_DIR}/.logs"
    PID_FILE="${SCRIPT_DIR}/.${FEATURE_NAME}-pids"
    NAMESPACE="${POLARIS_NAMESPACE:-go-naming-polarismesh-e2e}"
    POLARIS_SERVER="${POLARIS_SERVER:-}"
    POLARIS_GRPC_PORT="${POLARIS_GRPC_PORT:-8091}"
    POLARIS_HTTP_PORT="${POLARIS_HTTP_PORT:-8090}"
    POLARIS_TOKEN="${POLARIS_TOKEN:-}"
    DELETE_SERVICE=false
    KEEP_LOGS=false
    DRY_RUN=false
}

feature_parse_transport_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --polaris-server) POLARIS_SERVER="$2"; shift 2 ;;
            --polaris-grpc-port) POLARIS_GRPC_PORT="$2"; shift 2 ;;
            --polaris-http-port) POLARIS_HTTP_PORT="$2"; shift 2 ;;
            --polaris-token) POLARIS_TOKEN="$2"; shift 2 ;;
            --debug) DEBUG_MODE=true; shift ;;
            -h|--help) feature_usage; exit 0 ;;
            *) echo "unknown option: $1" >&2; exit 2 ;;
        esac
    done
    if [[ -z "${POLARIS_SERVER}" ]]; then
        echo "--polaris-server or POLARIS_SERVER is required" >&2
        exit 2
    fi
    CONSOLE="http://${POLARIS_SERVER}:${POLARIS_HTTP_PORT}"
    api_headers=(-H "Content-Type: application/json")
    if [[ -n "${POLARIS_TOKEN}" ]]; then
        api_headers+=(-H "X-Polaris-Token: ${POLARIS_TOKEN}")
    fi
}

feature_parse_cleanup_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --delete-service|--delete-services|--remote)
                DELETE_SERVICE=true
                shift
                ;;
            --polaris-server) POLARIS_SERVER="$2"; shift 2 ;;
            --polaris-grpc-port) POLARIS_GRPC_PORT="$2"; shift 2 ;;
            --polaris-http-port) POLARIS_HTTP_PORT="$2"; shift 2 ;;
            --polaris-token) POLARIS_TOKEN="$2"; shift 2 ;;
            --keep-logs|--processes-only) KEEP_LOGS=true; shift ;;
            --dry-run) DRY_RUN=true; shift ;;
            -f|--force|--debug) shift ;;
            -h|--help)
                cat <<'EOF'
Usage: ./cleanup.sh [options]

Options:
  --delete-service            Delete example services from Polaris
  --delete-services, --remote Alias for --delete-service
  --polaris-server <host>     Required with --delete-service
  --polaris-grpc-port <port>  Polaris gRPC port, default 8091
  --polaris-http-port <port>  Polaris OpenAPI port, default 8090
  --polaris-token <token>     Optional Polaris token
  --keep-logs                 Stop processes but keep build and log artifacts
  --dry-run                   Show the cleanup plan without changing anything
  -f, --force                 Compatibility flag for noninteractive cleanup
EOF
                exit 0
                ;;
            *) echo "unknown option: $1" >&2; exit 2 ;;
        esac
    done
    if [[ "${DELETE_SERVICE}" == "true" ]]; then
        if [[ -z "${POLARIS_SERVER}" ]]; then
            echo "--delete-service requires --polaris-server or POLARIS_SERVER" >&2
            exit 2
        fi
        CONSOLE="http://${POLARIS_SERVER}:${POLARIS_HTTP_PORT}"
        api_headers=(-H "Content-Type: application/json")
        if [[ -n "${POLARIS_TOKEN}" ]]; then
            api_headers+=(-H "X-Polaris-Token: ${POLARIS_TOKEN}")
        fi
    fi
}

feature_usage() {
    echo "Usage: ./verify.sh --polaris-server <host> [--polaris-token <token>] [--debug]"
}

api_request() {
    local method="$1"
    local path="$2"
    local payload="${3:-}"
    local response
    local attempt
    for attempt in 1 2 3; do
        if [[ -n "${payload}" ]]; then
            if response="$(curl -sS --max-time 60 -X "${method}" "${api_headers[@]}" \
                -d "${payload}" "${CONSOLE}${path}")"; then
                printf '%s\n' "${response}"
                return 0
            fi
        else
            if response="$(curl -sS --max-time 60 -X "${method}" "${api_headers[@]}" \
                "${CONSOLE}${path}")"; then
                printf '%s\n' "${response}"
                return 0
            fi
        fi
        if [[ "${attempt}" -lt 3 ]]; then
            echo "Polaris API ${method} ${path} failed on attempt ${attempt}; retrying" >&2
            sleep 1
        fi
    done
    echo "Polaris API ${method} ${path} failed after 3 attempts" >&2
    return 1
}

api_success() {
    jq -e \
        '.code == 200000 and ([.responses[]? | select(.code != 200000 and .code != 200002)] | length == 0)' \
        >/dev/null 2>&1 <<<"$1"
}

instance_response_valid() {
    jq -e '.code == 200000 and ((.instances // []) | type == "array")' \
        >/dev/null 2>&1 <<<"$1"
}

query_instances() {
    local service="$1"
    local response
    response="$(api_request GET \
        "/naming/v1/instances?namespace=${NAMESPACE}&service=${service}")" || return 1
    if ! instance_response_valid "${response}"; then
        echo "query instances for ${service} returned an invalid response: ${response}" >&2
        return 1
    fi
    printf '%s\n' "${response}"
}

create_success() {
    jq -e \
        '(.code == 200000 or .code == 400201) and ([.responses[]? | select(.code != 200000 and .code != 200002 and .code != 400201)] | length == 0)' \
        >/dev/null 2>&1 <<<"$1"
}

ensure_namespace() {
    local response
    response="$(api_request POST /naming/v1/namespaces \
        "[{\"name\":\"${NAMESPACE}\",\"comment\":\"go-naming-polarismesh examples\"}]")"
    create_success "${response}" || {
        echo "create namespace failed: ${response}" >&2
        return 1
    }
}

ensure_service() {
    local service="$1"
    local response
    response="$(api_request POST /naming/v1/services \
        "[{\"name\":\"${service}\",\"namespace\":\"${NAMESPACE}\",\"comment\":\"go-naming-polarismesh example\"}]")"
    create_success "${response}" || {
        echo "create service ${service} failed: ${response}" >&2
        return 1
    }
    response="$(api_request GET \
        "/naming/v1/instances?namespace=${NAMESPACE}&service=${service}")"
    if ! jq -e '.code == 200000' >/dev/null <<<"${response}"; then
        echo "query instances for ${service} failed: ${response}" >&2
        return 1
    fi
    if ! jq -e '[.instances[]? | select(. != null)] | length == 0' \
        >/dev/null <<<"${response}"; then
        echo "service ${service} already has registered instances; " \
            "run cleanup.sh --delete-service before verification" >&2
        return 1
    fi
}

render_provider_config() {
    local output="$1"
    local service="$2"
    local port="$3"
    local weight="$4"
    sed \
        -e "s|__NAMESPACE__|${NAMESPACE}|g" \
        -e "s|__SERVICE__|${service}|g" \
        -e "s|__PORT__|${port}|g" \
        -e "s|__POLARIS__|${POLARIS_SERVER}|g" \
        -e "s|__POLARIS_GRPC_PORT__|${POLARIS_GRPC_PORT}|g" \
        -e "s|__VERSION__|${PROVIDER_VERSION:-v1.0.0}|g" \
        -e "s|__WEIGHT__|${weight}|g" \
        "${REPO_ROOT}/examples/internal/e2eapp/config/provider.yaml.tmpl" > "${output}"
}

render_client_config() {
    local output="$1"
    sed \
        -e "s|__NAMESPACE__|${NAMESPACE}|g" \
        -e "s|__POLARIS__|${POLARIS_SERVER}|g" \
        -e "s|__POLARIS_GRPC_PORT__|${POLARIS_GRPC_PORT}|g" \
        "${REPO_ROOT}/examples/internal/e2eapp/config/client.yaml.tmpl" > "${output}"
}

build_example_binary() {
    local output="$1"
    local artifact="$2"
    local package="$3"
    mkdir -p "$(dirname "${output}")"
    if [[ -n "${POLARIS_E2E_BINARY_DIR:-}" ]]; then
        local source="${POLARIS_E2E_BINARY_DIR}/${artifact}"
        if [[ ! -x "${source}" ]]; then
            echo "prebuilt example binary is missing or not executable: ${source}" >&2
            return 1
        fi
        cp "${source}" "${output}"
        chmod 0755 "${output}"
        return 0
    fi
    (
        cd "${REPO_ROOT}"
        env GOCACHE="${GOCACHE:-${TMPDIR:-/tmp}/go-naming-polarismesh-cache}" \
            go build -o "${output}" "${package}"
    )
}

build_shared_binaries() {
    build_example_binary "${BUILD_DIR}/provider" e2e-provider \
        ./examples/internal/e2eapp/provider
    build_example_binary "${BUILD_DIR}/client" e2e-client \
        ./examples/internal/e2eapp/client
    build_example_binary "${BUILD_DIR}/multi-context" multi-context \
        ./examples/multiContext
}

start_provider() {
    local name="$1"
    local service="$2"
    local port="$3"
    local weight="$4"
    local labels="$5"
    local workdir="${BUILD_DIR}/${name}-run"
    rm -rf "${workdir}"
    mkdir -p "${workdir}"
    render_provider_config "${workdir}/trpc_go.yaml" "${service}" "${port}" "${weight}"
    local debug_flag=()
    if [[ "${DEBUG_MODE:-false}" == "true" ]]; then
        debug_flag=(-debug)
    fi
    (
        cd "${workdir}"
        exec nohup "${BUILD_DIR}/provider" \
            -service "${service}" \
            -node-id "${name}" \
            -namespace "${NAMESPACE}" \
            -port "${port}" \
            -version "${PROVIDER_VERSION:-v1.0.0}" \
            -polaris-server "${POLARIS_SERVER}" \
            -polaris-port "${POLARIS_GRPC_PORT}" \
            -labels "${labels}" \
            ${debug_flag[@]+"${debug_flag[@]}"}
    ) > "${LOG_DIR}/${name}.log" 2>&1 &
    local pid=$!
    disown "${pid}" 2>/dev/null || true
    printf '%s|%s|%s\n' "${pid}" "${name}" "${port}" >> "${PID_FILE}"
    echo "${pid}"
}

start_client() {
    local service="$1"
    local caller="$2"
    local workdir="${BUILD_DIR}/client-run"
    rm -rf "${workdir}"
    mkdir -p "${workdir}"
    render_client_config "${workdir}/trpc_go.yaml"
    local debug_flag=()
    if [[ "${DEBUG_MODE:-false}" == "true" ]]; then
        debug_flag=(-debug)
    fi
    (
        cd "${workdir}"
        exec nohup "${BUILD_DIR}/client" \
            -listen "127.0.0.1:${CLIENT_LISTEN}" \
            -namespace "${NAMESPACE}" \
            -service "${service}" \
            -caller "${caller}" \
            ${debug_flag[@]+"${debug_flag[@]}"}
    ) > "${LOG_DIR}/client.log" 2>&1 &
    local pid=$!
    disown "${pid}" 2>/dev/null || true
    printf '%s|%s|%s\n' "${pid}" client "${CLIENT_LISTEN}" >> "${PID_FILE}"
    echo "${pid}"
}

wait_for_http() {
    local url="$1"
    for _ in $(seq 1 80); do
        if curl -fsS --max-time 2 "${url}" >/dev/null 2>&1; then
            return 0
        fi
        sleep 0.25
    done
    echo "HTTP endpoint not ready: ${url}" >&2
    return 1
}

wait_for_instances() {
    local service="$1"
    local expected="$2"
    local response count
    local attempts="${INSTANCE_QUERY_ATTEMPTS:-80}"
    for _ in $(seq 1 "${attempts}"); do
        response="$(api_request GET "/naming/v1/instances?namespace=${NAMESPACE}&service=${service}")"
        if ! instance_response_valid "${response}"; then
            sleep 0.25
            continue
        fi
        count="$(jq '[.instances[]? | select(. != null)] | length' <<<"${response}" 2>/dev/null || echo 0)"
        if [[ "${count}" -ge "${expected}" ]]; then
            sleep "${DISCOVERY_SETTLE_SECONDS:-1}"
            return 0
        fi
        sleep 0.25
    done
    echo "timed out waiting for ${expected} instances of ${service}: ${response}" >&2
    return 1
}

instance_id_by_port() {
    local service="$1"
    local port="$2"
    local response
    response="$(query_instances "${service}")" || return 1
    jq -r --argjson port "${port}" '
        [.instances[]? | select(. != null and .port == $port)] as $instances
        | if ($instances | length) == 1 then $instances[0].id // empty else empty end
    ' <<<"${response}"
}

wait_for_instance_health() {
    local service="$1"
    local expected_health="$2"
    local expected_ttl="$3"
    shift 3
    local ids=("$@")
    local attempts="${INSTANCE_QUERY_ATTEMPTS:-80}"
    local response
    if [[ ${#ids[@]} -eq 0 ]]; then
        echo "wait_for_instance_health requires at least one instance id" >&2
        return 1
    fi
    for _ in $(seq 1 "${attempts}"); do
        response="$(api_request GET \
            "/naming/v1/instances?namespace=${NAMESPACE}&service=${service}")"
        if instance_response_valid "${response}" && jq -e \
            --argjson expected_health "${expected_health}" \
            --argjson expected_ttl "${expected_ttl}" \
            --args '
            def raw_value:
                if type == "object" and has("value") then .value else . end;
            [.instances[]? | select(. != null)] as $instances
            | ($ARGS.positional | length) > 0
            and ([$ARGS.positional[] as $id
                | [$instances[] | select(
                    .id == $id
                    and ((.healthy | raw_value) == $expected_health)
                    and ((.enable_health_check // .enableHealthCheck | raw_value) == true)
                    and (((.health_check // .healthCheck).heartbeat.ttl | raw_value) == $expected_ttl)
                )]
                | length == 1]
                | all)
        ' "${ids[@]}" >/dev/null <<<"${response}"; then
            return 0
        fi
        sleep 0.5
    done
    echo "timed out waiting for instances in ${service} to become healthy=${expected_health}, ttl=${expected_ttl}: ${response}" >&2
    return 1
}

stop_pid() {
    local pid="$1"
    if [[ -n "${pid}" ]] && kill -0 "${pid}" 2>/dev/null && feature_pid_owned "${pid}"; then
        kill -TERM "${pid}" 2>/dev/null || true
        for _ in $(seq 1 40); do
            kill -0 "${pid}" 2>/dev/null || return 0
            sleep 0.25
        done
        feature_pid_owned "${pid}" && kill -KILL "${pid}" 2>/dev/null || true
    fi
}

feature_command_owned() {
    local command="$1"
    [[ "${command}" == "${BUILD_DIR}/provider" ||
       "${command}" == "${BUILD_DIR}/provider "* ||
       "${command}" == "${BUILD_DIR}/client" ||
       "${command}" == "${BUILD_DIR}/client "* ||
       "${command}" == "${BUILD_DIR}/server" ||
       "${command}" == "${BUILD_DIR}/server "* ||
       "${command}" == "${BUILD_DIR}/multi-context" ||
       "${command}" == "${BUILD_DIR}/multi-context "* ]]
}

feature_pid_owned() {
    local pid="$1"
    local command
    command="$(ps -p "${pid}" -o command= 2>/dev/null || true)"
    feature_command_owned "${command}"
}

collect_feature_processes() {
    {
        if [[ -f "${PID_FILE}" ]]; then
            while IFS='|' read -r pid _ _; do
                [[ "${pid}" =~ ^[0-9]+$ ]] || continue
                feature_pid_owned "${pid}" && printf '%s\n' "${pid}"
            done < "${PID_FILE}"
        fi
        while read -r pid command; do
            if feature_command_owned "${command}"; then
                printf '%s\n' "${pid}"
            fi
        done < <(ps -axo pid=,command= 2>/dev/null || true)
        true
    } | awk -v self="$$" -v parent="${PPID}" \
        '$1 != self && $1 != parent {print $1}' | sort -u
}

show_feature_processes() {
    local processes="$1"
    if [[ -z "${processes}" ]]; then
        echo "Processes: none"
        return
    fi
    local lines=() line
    while IFS= read -r pid; do
        line="$(ps -p "${pid}" -o pid=,command= 2>/dev/null || true)"
        [[ -n "${line}" ]] && lines+=("  ${line}")
    done <<<"${processes}"
    if [[ ${#lines[@]} -eq 0 ]]; then
        echo "Processes: none"
        return
    fi
    echo "Processes:"
    printf '%s\n' "${lines[@]}"
}

stop_feature_processes() {
    local processes="$1"
    [[ -n "${processes}" ]] || return 0
    while IFS= read -r pid; do
        feature_pid_owned "${pid}" && stop_pid "${pid}"
    done <<<"${processes}"
}

delete_instances_and_service() {
    local service="$1"
    local response instance_id
    response="$(api_request GET \
        "/naming/v1/services?namespace=${NAMESPACE}&name=${service}")"
    if ! jq -e --arg service "${service}" '
        .code == 200000 and ([.services[]? | select(.name == $service)] | length > 0)
    ' >/dev/null <<<"${response}"; then
        if jq -e '.code == 200000' >/dev/null <<<"${response}"; then
            return 0
        fi
        echo "query service ${service} failed: ${response}" >&2
        return 1
    fi
    response="$(api_request GET \
        "/naming/v1/instances?namespace=${NAMESPACE}&service=${service}")" || {
        echo "query instances for service ${service} failed" >&2
        return 1
    }
    while IFS= read -r instance_id; do
        [[ -n "${instance_id}" ]] || continue
        api_request POST /naming/v1/instances/delete \
            "[{\"id\":\"${instance_id}\",\"namespace\":\"${NAMESPACE}\",\"service\":\"${service}\"}]" \
            >/dev/null || true
    done < <(jq -r '.instances[]?.id // empty' <<<"${response}" 2>/dev/null || true)
    for _ in $(seq 1 10); do
        response="$(api_request POST /naming/v1/services/delete \
            "[{\"name\":\"${service}\",\"namespace\":\"${NAMESPACE}\"}]")"
        if api_success "${response}"; then
            return 0
        fi
        sleep 0.5
    done
    echo "delete service ${service} failed: ${response}" >&2
    return 1
}

feature_common_cleanup() {
    local service="$1"
    local extra_services=()
    if [[ $# -gt 1 ]]; then
        extra_services=("${@:2}")
    fi
    local processes
    processes="$(collect_feature_processes)"
    if [[ "${DRY_RUN}" == "true" ]]; then
        echo "Cleanup plan for ${FEATURE_NAME}:"
        show_feature_processes "${processes}"
        echo "Local artifacts: ${BUILD_DIR}, ${LOG_DIR}, ${PID_FILE}"
        if [[ "${DELETE_SERVICE}" == "true" ]]; then
            echo "Remote service: ${NAMESPACE}/${service}"
            local extra
            if [[ ${#extra_services[@]} -gt 0 ]]; then
                for extra in "${extra_services[@]}"; do
                    [[ -n "${extra}" ]] && echo "Remote service: ${NAMESPACE}/${extra}"
                done
            fi
        else
            echo "Remote resources: keep"
        fi
        return 0
    fi
    stop_feature_processes "${processes}"
    rm -f "${PID_FILE}"
    if [[ "${DELETE_SERVICE}" == "true" ]]; then
        local remote_failed=false
        if ! delete_instances_and_service "${service}"; then
            echo "delete service ${service} failed" >&2
            remote_failed=true
        fi
        local extra
        if [[ ${#extra_services[@]} -gt 0 ]]; then
            for extra in "${extra_services[@]}"; do
                [[ -n "${extra}" ]] || continue
                if ! delete_instances_and_service "${extra}"; then
                    echo "delete service ${extra} failed" >&2
                    remote_failed=true
                fi
            done
        fi
        if [[ "${remote_failed}" == "true" ]]; then
            return 1
        fi
    fi
    if [[ "${KEEP_LOGS}" != "true" ]]; then
        rm -rf "${BUILD_DIR}" "${LOG_DIR}"
    fi
}

json_file_result() {
    curl -fsS --max-time 60 "$@"
}
