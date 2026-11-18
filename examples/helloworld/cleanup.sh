#!/bin/bash

set -euo pipefail

COMMON_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../internal" && pwd)"
source "${COMMON_DIR}/e2e_common.sh"

feature_cleanup_init helloworld
feature_parse_cleanup_args "$@"
feature_common_cleanup "trpc.examples.helloworld.Greeter"
if [[ "${KEEP_LOGS}" != "true" && "${DRY_RUN}" != "true" ]]; then
    rm -rf "${SCRIPT_DIR}/client/.polaris-cache" "${SCRIPT_DIR}/client/polaris" \
        "${SCRIPT_DIR}/server/polaris"
fi
