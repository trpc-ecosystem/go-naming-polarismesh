#!/bin/bash
set -euo pipefail
COMMON_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../internal" && pwd)"
source "${COMMON_DIR}/e2e_common.sh"
feature_cleanup_init multi-context
feature_parse_cleanup_args "$@"
feature_common_cleanup "trpc.polaris.e2e.MultiContextA"
feature_common_cleanup "trpc.polaris.e2e.MultiContextB"
