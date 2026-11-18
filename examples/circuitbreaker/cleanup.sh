#!/bin/bash
set -euo pipefail
COMMON_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../internal" && pwd)"
source "${COMMON_DIR}/e2e_common.sh"
feature_cleanup_init circuitbreaker
feature_parse_cleanup_args "$@"

SERVICE_NAME="trpc.polaris.e2e.CircuitBreaker"
if [[ "${DELETE_SERVICE}" == "true" && "${DRY_RUN}" == "true" ]]; then
    echo "Remote rules retained: go-naming-polarismesh-e2e-cb-{instance,service,method}"
fi
feature_common_cleanup "${SERVICE_NAME}"
