#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
    cat <<'EOF'
Usage: ./examples/cleanup.sh [options]

Default:
  Stop all example processes and remove local build/log artifacts.
  Remote services and rules are retained.

Remote resources:
  --delete-service --polaris-server <host>
      Delete example services from Polaris. Test rules are retained.

Other options:
  --keep-logs, --processes-only   Stop processes and keep local artifacts
  --dry-run                       Show cleanup scope without changing anything
  -f, --force                     Compatibility flag for noninteractive cleanup
EOF
    exit 0
fi

features=(
    helloworld
    loadbalancer
    circuitbreaker
    registry/metadata
    registry/transports
    route/metadata
    route/rule
    multiContext
)

status=0
for feature in "${features[@]}"; do
    echo "===== cleanup ${feature} ====="
    "${SCRIPT_DIR}/${feature}/cleanup.sh" "$@" || status=1
done

exit "${status}"
