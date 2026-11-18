#!/bin/bash

set -euo pipefail

COMMON_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../internal" && pwd)"
source "${COMMON_DIR}/e2e_common.sh"

feature_cleanup_init registry-transports
feature_parse_cleanup_args "$@"
# Retain the cleanup scope for resources created by older example versions.
feature_common_cleanup \
    "trpc.polaris.registry.multi.Transport" \
    "trpc.polaris.registry.mcp.Transport"
