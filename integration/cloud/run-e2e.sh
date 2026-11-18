#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "${SCRIPT_DIR}/go.mod" ]]; then
    ROOT="${SCRIPT_DIR}"
elif [[ -f "${SCRIPT_DIR}/../../go.mod" ]]; then
    ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
else
    echo "ERROR: cannot locate deployment package root" >&2
    exit 1
fi

SERVER="${POLARIS_SERVER_ADDRESS:-}"
SERVER_HOST=""
GRPC_PORT="${POLARIS_GRPC_PORT:-8091}"
HTTP_PORT="${POLARIS_HTTP_PORT:-8090}"
NAMESPACE="${POLARIS_NAMESPACE:-go-naming-polarismesh-e2e}"
ONLY=""
TIMEOUT="${CLOUD_SCENARIO_TIMEOUT:-600}"
RUN_ID=""
RUNS_ROOT="${CLOUD_RUNS_DIR:-${ROOT}/cloud-runs}"
CREATE_ARCHIVE=true
EXTRA_ARGS=()

ALL_SCENARIOS=(
    helloworld
    loadbalancer
    circuitbreaker
    registry/metadata
    registry/transports
    route/metadata
    route/rule
    multiContext
)

usage() {
    cat <<EOF
Usage: $0 --server HOST[:8091] [options] [-- extra verify.sh args]

Options:
  --server ADDRESS       Polaris host or host:grpc-port; default: POLARIS_SERVER_ADDRESS
  --grpc-port PORT       Polaris gRPC port when --server has no port; default: ${GRPC_PORT}
  --http-port PORT       Polaris OpenAPI port; default: ${HTTP_PORT}
  --namespace NAME       E2E namespace; default: ${NAMESPACE}
  --only LIST            Comma-separated scenarios or "registry"
  --timeout SECONDS      Per-scenario timeout; default: ${TIMEOUT}
  --run-id ID            Stable run directory name
  --runs-root DIR        Output root; default: ${RUNS_ROOT}
  --no-archive           Do not create a log archive automatically
  -h, --help             Show this help

Scenarios:
  ${ALL_SCENARIOS[*]}

"registry" expands to registry/metadata and registry/transports.
POLARIS_TOKEN is inherited from the environment and is never written to logs.
The script does not delete retained processes or remote resources.
EOF
}

die() {
    echo "ERROR: $*" >&2
    exit 1
}

parse_server() {
    local value="$1"
    if [[ "${value}" =~ ^\[([0-9A-Fa-f:]+)\]:([0-9]+)$ ]]; then
        SERVER_HOST="${BASH_REMATCH[1]}"
        GRPC_PORT="${BASH_REMATCH[2]}"
    elif [[ "${value}" =~ ^([^:]+):([0-9]+)$ ]]; then
        SERVER_HOST="${BASH_REMATCH[1]}"
        GRPC_PORT="${BASH_REMATCH[2]}"
    elif [[ "${value}" == *:* ]]; then
        die "--server must be HOST, HOST:PORT or [IPv6]:PORT"
    else
        SERVER_HOST="${value}"
    fi
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --server)
            [[ $# -ge 2 ]] || die "$1 requires a value"
            SERVER="$2"
            shift 2
            ;;
        --grpc-port)
            [[ $# -ge 2 ]] || die "$1 requires a value"
            GRPC_PORT="$2"
            shift 2
            ;;
        --http-port)
            [[ $# -ge 2 ]] || die "$1 requires a value"
            HTTP_PORT="$2"
            shift 2
            ;;
        --namespace)
            [[ $# -ge 2 ]] || die "$1 requires a value"
            NAMESPACE="$2"
            shift 2
            ;;
        --only)
            [[ $# -ge 2 ]] || die "$1 requires a value"
            ONLY="$2"
            shift 2
            ;;
        --timeout)
            [[ $# -ge 2 ]] || die "$1 requires a value"
            TIMEOUT="$2"
            shift 2
            ;;
        --run-id)
            [[ $# -ge 2 ]] || die "$1 requires a value"
            RUN_ID="$2"
            shift 2
            ;;
        --runs-root)
            [[ $# -ge 2 ]] || die "$1 requires a value"
            RUNS_ROOT="$2"
            shift 2
            ;;
        --no-archive)
            CREATE_ARCHIVE=false
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        --)
            shift
            EXTRA_ARGS=("$@")
            break
            ;;
        *)
            die "unknown argument: $1"
            ;;
    esac
done

[[ -n "${SERVER}" ]] || die "--server or POLARIS_SERVER_ADDRESS is required"
parse_server "${SERVER}"
[[ -n "${SERVER_HOST}" ]] || die "server host is empty"
[[ "${NAMESPACE}" =~ ^[A-Za-z0-9_.-]+$ ]] || die "namespace contains unsupported characters"
[[ "${NAMESPACE}" != "default" ]] || die "POLARIS_NAMESPACE must not be default"
[[ "${GRPC_PORT}" =~ ^[0-9]+$ ]] || die "gRPC port must be numeric"
[[ "${HTTP_PORT}" =~ ^[0-9]+$ ]] || die "HTTP port must be numeric"
[[ "${TIMEOUT}" =~ ^[1-9][0-9]*$ ]] || die "timeout must be a positive integer"

selected=()
if [[ -z "${ONLY}" ]]; then
    selected=("${ALL_SCENARIOS[@]}")
else
    requested=()
    IFS=',' read -r -a raw_requested <<<"${ONLY}"
    for item in "${raw_requested[@]}"; do
        item="${item//[[:space:]]/}"
        [[ -n "${item}" ]] || continue
        if [[ "${item}" == "registry" ]]; then
            requested+=(registry/metadata registry/transports)
        else
            requested+=("${item}")
        fi
    done
    [[ ${#requested[@]} -gt 0 ]] || die "--only did not select any scenario"
    for item in "${requested[@]}"; do
        found=false
        for scenario in "${ALL_SCENARIOS[@]}"; do
            if [[ "${item}" == "${scenario}" ]]; then
                found=true
                break
            fi
        done
        [[ "${found}" == "true" ]] || die "unknown scenario: ${item}"
        duplicate=false
        for existing in ${selected[@]+"${selected[@]}"}; do
            if [[ "${existing}" == "${item}" ]]; then
                duplicate=true
                break
            fi
        done
        [[ "${duplicate}" == "true" ]] || selected+=("${item}")
    done
fi

TIMEOUT_BIN="$(command -v timeout || command -v gtimeout || true)"
[[ -n "${TIMEOUT_BIN}" ]] || die "timeout or gtimeout is required"
for command_name in curl jq ps tar; do
    command -v "${command_name}" >/dev/null 2>&1 ||
        die "${command_name} is required on the cloud host"
done

for scenario in "${selected[@]}"; do
    [[ -x "${ROOT}/examples/${scenario}/verify.sh" ]] ||
        die "verification script not found: examples/${scenario}/verify.sh"
done

if [[ -z "${RUN_ID}" ]]; then
    RUN_ID="run-$(date '+%Y%m%d_%H%M%S')-$$"
fi
[[ "${RUN_ID}" =~ ^[A-Za-z0-9_.-]+$ ]] ||
    die "--run-id may contain only letters, digits, dot, underscore and dash"

RUN_DIR="${RUNS_ROOT}/${RUN_ID}"
[[ ! -e "${RUN_DIR}" ]] || die "run directory already exists: ${RUN_DIR}"
mkdir -p "${RUN_DIR}/logs" "${RUN_DIR}/runtime-records"
RUN_LOG="${RUN_DIR}/run.log"
RESULTS_FILE="${RUN_DIR}/results.tsv"
STARTED_AT="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"

log() {
    printf '%s\n' "$*" | tee -a "${RUN_LOG}"
}

{
    echo "date=${STARTED_AT}"
    echo "hostname=$(hostname 2>/dev/null || true)"
    echo "uname=$(uname -a 2>/dev/null || true)"
    echo "server=${SERVER_HOST}"
    echo "grpc_port=${GRPC_PORT}"
    echo "http_port=${HTTP_PORT}"
    echo "namespace=${NAMESPACE}"
    echo "only=${ONLY}"
    echo "timeout=${TIMEOUT}"
    echo "POLARIS_TOKEN_set=$([[ -n "${POLARIS_TOKEN:-}" ]] && echo true || echo false)"
} > "${RUN_DIR}/environment.txt"

scenarios_json=""
for scenario in "${selected[@]}"; do
    [[ -n "${scenarios_json}" ]] && scenarios_json+=", "
    scenarios_json+="\"${scenario}\""
done

export POLARIS_SERVER="${SERVER_HOST}"
export POLARIS_GRPC_PORT="${GRPC_PORT}"
export POLARIS_HTTP_PORT="${HTTP_PORT}"
export POLARIS_NAMESPACE="${NAMESPACE}"
if [[ -d "${ROOT}/bin" ]]; then
    export POLARIS_E2E_BINARY_DIR="${ROOT}/bin"
fi

log "Run ID: ${RUN_ID}"
log "Root: ${ROOT}"
log "Server: ${SERVER_HOST}:${GRPC_PORT}"
log "HTTP: ${SERVER_HOST}:${HTTP_PORT}"
log "Namespace: ${NAMESPACE}"
log "Scenarios: ${selected[*]}"
log "Started: ${STARTED_AT}"

printf 'scenario\texit_code\tduration_seconds\n' > "${RESULTS_FILE}"
overall_code=0
for scenario in "${selected[@]}"; do
    safe_scenario="${scenario//\//__}"
    scenario_log="${RUN_DIR}/logs/${safe_scenario}.log"
    scenario_started="$(date +%s)"
    log "===== ${scenario} ====="
    set +e
    (
        cd "${ROOT}"
        "${TIMEOUT_BIN}" --signal=TERM --kill-after=30s "${TIMEOUT}s" \
            "./examples/${scenario}/verify.sh" \
            --polaris-server "${SERVER_HOST}" \
            --polaris-grpc-port "${GRPC_PORT}" \
            --polaris-http-port "${HTTP_PORT}" \
            ${EXTRA_ARGS[@]+"${EXTRA_ARGS[@]}"}
    ) 2>&1 | tee "${scenario_log}"
    scenario_code=${PIPESTATUS[0]}
    set -e
    scenario_finished="$(date +%s)"
    scenario_duration=$((scenario_finished - scenario_started))
    printf '%s\t%s\t%s\n' "${scenario}" "${scenario_code}" "${scenario_duration}" >> "${RESULTS_FILE}"
    if [[ "${scenario_code}" -ne 0 ]]; then
        overall_code=1
    fi

    log_dir="${RUN_DIR}/logs/${safe_scenario}"
    if [[ -d "${ROOT}/examples/${scenario}/.logs" ]]; then
        mkdir -p "${log_dir}"
        cp -R "${ROOT}/examples/${scenario}/.logs/." "${log_dir}/"
    fi
    runtime_dir="${RUN_DIR}/runtime-records/${safe_scenario}"
    mkdir -p "${runtime_dir}"
    while IFS= read -r pid_file; do
        cp "${pid_file}" "${runtime_dir}/"
    done < <(find "${ROOT}/examples/${scenario}" -maxdepth 1 -name '.*-pids' -type f 2>/dev/null)
done

FINISHED_AT="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
cat > "${RUN_DIR}/manifest.json" <<EOF
{
  "run_id": "${RUN_ID}",
  "server": "${SERVER_HOST}",
  "grpc_port": ${GRPC_PORT},
  "http_port": ${HTTP_PORT},
  "namespace": "${NAMESPACE}",
  "only": "${ONLY}",
  "timeout_seconds": ${TIMEOUT},
  "started_at": "${STARTED_AT}",
  "finished_at": "${FINISHED_AT}",
  "exit_code": ${overall_code},
  "scenarios": [${scenarios_json}]
}
EOF

"${ROOT}/summarize-results.sh" "${RUN_DIR}" \
    --output "${RUN_DIR}/analysis.txt" >> "${RUN_LOG}" 2>&1 || true
log "Finished: ${FINISHED_AT}"
log "Overall exit code: ${overall_code}"
log "Run directory: ${RUN_DIR}"

if [[ "${CREATE_ARCHIVE}" == "true" ]]; then
    ARCHIVE="${RUNS_ROOT}/${RUN_ID}-logs.tar.gz"
    "${ROOT}/pack-logs.sh" --run-dir "${RUN_DIR}" --output "${ARCHIVE}"
    log "Log archive: ${ARCHIVE}"
fi

if [[ "${overall_code}" -eq 0 ]]; then
    log "Cloud E2E passed."
else
    log "Cloud E2E failed; inspect analysis.txt and scenario logs."
fi
exit "${overall_code}"
