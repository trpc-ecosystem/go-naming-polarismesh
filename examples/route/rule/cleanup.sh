#!/bin/bash
set -euo pipefail
COMMON_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../internal" && pwd)"
source "${COMMON_DIR}/e2e_common.sh"
feature_cleanup_init route-rule
feature_parse_cleanup_args "$@"
NAMESPACE="go-naming-polarismesh-e2e"
SERVICE_NAME="trpc.polaris.e2e.RouteRule"

if [[ "${DELETE_SERVICE}" == "true" && "${DRY_RUN}" == "true" ]]; then
    echo "Remote rule retained: go-naming-polarismesh-e2e-route-rule"
fi

feature_common_cleanup "${SERVICE_NAME}"
