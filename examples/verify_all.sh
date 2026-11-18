#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

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

for feature in "${features[@]}"; do
    echo "===== ${feature} ====="
    "${SCRIPT_DIR}/${feature}/verify.sh" "$@"
done

echo "All examples passed."
echo "Processes and services were retained for inspection."
echo "Run ./examples/cleanup.sh to stop processes and remove local artifacts."
echo "Add --delete-service --polaris-server <host> to delete remote resources."
