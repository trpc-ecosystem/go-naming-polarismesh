#!/bin/bash

set -euo pipefail

COMMON_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../internal" && pwd)"
source "${COMMON_DIR}/e2e_common.sh"

feature_common_init multi-context
feature_usage() {
    echo "Usage: ./examples/multiContext/verify.sh --polaris-server <host> [--debug]"
}
feature_parse_transport_args "$@"

SERVICE_A="trpc.polaris.e2e.MultiContextA"
SERVICE_B="trpc.polaris.e2e.MultiContextB"
CALLER_SERVICE="trpc.polaris.e2e.MultiContextCaller"
PORT_A="${PORT_A:-28251}"
PORT_B="${PORT_B:-28252}"

build_shared_binaries
ensure_namespace
ensure_service "${SERVICE_A}"
ensure_service "${SERVICE_B}"
start_provider provider-a "${SERVICE_A}" "${PORT_A}" 100 "" >/dev/null
start_provider provider-b "${SERVICE_B}" "${PORT_B}" 100 "" >/dev/null
wait_for_instances "${SERVICE_A}" 1
wait_for_instances "${SERVICE_B}" 1

workdir="${BUILD_DIR}/multi-context-run"
rm -rf "${workdir}"
mkdir -p "${workdir}"
render_client_config "${workdir}/trpc_go.yaml"
debug_flag=()
if [[ "${DEBUG_MODE:-false}" == "true" ]]; then
    debug_flag=(-debug)
fi
(
    cd "${workdir}"
    "${BUILD_DIR}/multi-context" \
        -polaris-server "${POLARIS_SERVER}:${POLARIS_GRPC_PORT}" \
        -namespace "${NAMESPACE}" \
        -caller "${CALLER_SERVICE}" \
        -service-a "${SERVICE_A}" \
        -service-b "${SERVICE_B}" \
        ${debug_flag[@]+"${debug_flag[@]}"}
) > "${LOG_DIR}/multi-context.result.json" 2> "${LOG_DIR}/multi-context.log"

result="$(sed -n '/^\[/,$p' "${LOG_DIR}/multi-context.result.json")"
if ! jq -e \
    --arg serviceA "${SERVICE_A}" \
    --arg serviceB "${SERVICE_B}" \
    --arg portA ":${PORT_A}" \
    --arg portB ":${PORT_B}" '
    length == 2
    and all(.[]; .success == true)
    and any(.[];
        .selector == "polaris-a"
        and .service == $serviceA
        and (.address | endswith($portA)))
    and any(.[];
        .selector == "polaris-b"
        and .service == $serviceB
        and (.address | endswith($portB)))
' >/dev/null <<<"${result}"; then
    echo "multi-context isolation assertion failed: ${result}" >&2
    cat "${LOG_DIR}/multi-context.log" >&2
    exit 1
fi

echo "[case 1.1 named selector isolation] PASS"
echo "[case 1.2 independent SDKContext discovery] PASS"
