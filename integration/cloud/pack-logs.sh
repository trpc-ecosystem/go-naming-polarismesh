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

RUNS_ROOT="${CLOUD_RUNS_DIR:-${ROOT}/cloud-runs}"
RUN_DIR=""
OUTPUT=""
PACK_ALL=false

usage() {
    cat <<EOF
Usage: $0 [options]

Options:
  --run-dir DIR      Pack one cloud run directory
  --runs-root DIR    Cloud run root; default: ${RUNS_ROOT}
  --all              Pack every run directory below the run root
  --output FILE      Archive path; default: cloud-runs/<run-id>-logs.tar.gz
  -h, --help         Show this help

With no run selection, all directories under cloud-runs are packed. Tokens are
never read by this script and are not added to the archive.
EOF
}

die() {
    echo "ERROR: $*" >&2
    exit 1
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --run-dir)
            [[ $# -ge 2 ]] || die "$1 requires a value"
            RUN_DIR="$2"
            shift 2
            ;;
        --runs-root)
            [[ $# -ge 2 ]] || die "$1 requires a value"
            RUNS_ROOT="$2"
            shift 2
            ;;
        --all)
            PACK_ALL=true
            shift
            ;;
        --output)
            [[ $# -ge 2 ]] || die "$1 requires a value"
            OUTPUT="$2"
            shift 2
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            die "unknown argument: $1"
            ;;
    esac
done

sha256_file() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    else
        shasum -a 256 "$1" | awk '{print $1}'
    fi
}

entries=()
if [[ -n "${RUN_DIR}" && "${PACK_ALL}" == "true" ]]; then
    die "--run-dir and --all are mutually exclusive"
elif [[ -n "${RUN_DIR}" ]]; then
    [[ -d "${RUN_DIR}" ]] || die "run directory not found: ${RUN_DIR}"
    RUN_DIR="$(cd "${RUN_DIR}" && pwd)"
    BASE_DIR="$(dirname "${RUN_DIR}")"
    entries=("$(basename "${RUN_DIR}")")
else
    RUNS_ROOT="$(cd "${RUNS_ROOT}" 2>/dev/null && pwd || true)"
    [[ -n "${RUNS_ROOT}" && -d "${RUNS_ROOT}" ]] ||
        die "cloud run root not found; use --run-dir or create cloud-runs"
    while IFS= read -r directory; do
        entries+=("$(basename "${directory}")")
    done < <(find "${RUNS_ROOT}" -mindepth 1 -maxdepth 1 -type d | sort)
    [[ ${#entries[@]} -gt 0 ]] || die "no run directories found below ${RUNS_ROOT}"
    BASE_DIR="${RUNS_ROOT}"
    PACK_ALL=true
fi

for entry in "${entries[@]}"; do
    directory="${BASE_DIR}/${entry}"
    if [[ -x "${ROOT}/summarize-results.sh" ]]; then
        "${ROOT}/summarize-results.sh" "${directory}" || true
    fi
done

if [[ -z "${OUTPUT}" ]]; then
    if [[ "${PACK_ALL}" == "true" ]]; then
        OUTPUT="${BASE_DIR}/go-naming-polarismesh-e2e-logs-all-$(date '+%Y%m%d_%H%M%S').tar.gz"
    else
        OUTPUT="${BASE_DIR}/${entries[0]}-logs.tar.gz"
    fi
fi
mkdir -p "$(dirname "${OUTPUT}")"
OUTPUT="$(cd "$(dirname "${OUTPUT}")" && pwd)/$(basename "${OUTPUT}")"

COPYFILE_DISABLE=1 tar -czf "${OUTPUT}" -C "${BASE_DIR}" "${entries[@]}"
OUTPUT_NAME="$(basename "${OUTPUT}")"
printf '%s  %s\n' "$(sha256_file "${OUTPUT}")" "${OUTPUT_NAME}" > "${OUTPUT}.sha256"

echo "Log archive: ${OUTPUT}"
echo "Checksum:    ${OUTPUT}.sha256"
