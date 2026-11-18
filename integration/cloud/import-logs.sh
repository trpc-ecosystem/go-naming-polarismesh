#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
OUTPUT_ROOT="${REPO_ROOT}/integration/cloud/.logs/imported"
FORCE=false

usage() {
    cat <<EOF
Usage: $0 ARCHIVE [options]

Options:
  --output-dir DIR   Extraction directory; default: ${OUTPUT_ROOT}/<archive-name>
  --force            Replace an existing extraction directory
  -h, --help         Show this help

The adjacent ARCHIVE.sha256 file is verified when present.
Python 3 is used to reject absolute paths, traversal and archive links.
EOF
}

die() {
    echo "ERROR: $*" >&2
    exit 1
}

ARCHIVE=""
OUTPUT_DIR=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --output-dir)
            [[ $# -ge 2 ]] || die "$1 requires a value"
            OUTPUT_DIR="$2"
            shift 2
            ;;
        --force)
            FORCE=true
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        -*)
            die "unknown argument: $1"
            ;;
        *)
            [[ -z "${ARCHIVE}" ]] || die "only one archive may be imported"
            ARCHIVE="$1"
            shift
            ;;
    esac
done

[[ -n "${ARCHIVE}" ]] || die "ARCHIVE is required"
[[ -f "${ARCHIVE}" ]] || die "archive not found: ${ARCHIVE}"
command -v python3 >/dev/null 2>&1 || die "python3 is required for safe archive import"

ARCHIVE="$(cd "$(dirname "${ARCHIVE}")" && pwd)/$(basename "${ARCHIVE}")"
archive_name="$(basename "${ARCHIVE}")"
archive_name="${archive_name%.tar.gz}"
archive_name="${archive_name%.tgz}"
OUTPUT_DIR="${OUTPUT_DIR:-${OUTPUT_ROOT}/${archive_name}}"

if [[ -e "${OUTPUT_DIR}" ]]; then
    [[ "${FORCE}" == "true" ]] || die "output exists: ${OUTPUT_DIR} (use --force)"
    rm -rf -- "${OUTPUT_DIR}"
fi
mkdir -p "${OUTPUT_DIR}"

python3 - "${ARCHIVE}" "${OUTPUT_DIR}" <<'PY'
import hashlib
import pathlib
import sys
import tarfile

archive = pathlib.Path(sys.argv[1])
destination = pathlib.Path(sys.argv[2]).resolve()
sidecar = archive.with_name(archive.name + ".sha256")
if sidecar.is_file():
    fields = sidecar.read_text(encoding="utf-8").split()
    if not fields:
        raise SystemExit(f"invalid checksum file: {sidecar}")
    expected = fields[0].lower()
    actual = hashlib.sha256(archive.read_bytes()).hexdigest()
    if actual != expected:
        raise SystemExit(
            f"checksum mismatch for {archive}: expected {expected}, got {actual}"
        )

with tarfile.open(archive, "r:gz") as package:
    for member in package.getmembers():
        target = (destination / member.name).resolve()
        if not target.is_relative_to(destination):
            raise SystemExit(f"unsafe archive path: {member.name}")
        if member.issym() or member.islnk():
            raise SystemExit(f"archive links are not accepted: {member.name}")
    package.extractall(destination, filter="data")
PY

entries=()
while IFS= read -r path; do
    entries+=("${path}")
done < <(
    find "${OUTPUT_DIR}" -mindepth 1 -maxdepth 1 \
        \( -type f -o -type d \) \
        ! -name '._*' \
        ! -name '.DS_Store' | sort
)

RUN_DIR="${OUTPUT_DIR}"
if [[ ${#entries[@]} -eq 1 && -d "${entries[0]}" ]]; then
    RUN_DIR="${entries[0]}"
fi

"${REPO_ROOT}/integration/cloud/summarize-results.sh" \
    "${RUN_DIR}" --output "${RUN_DIR}/analysis.txt" || true

echo "Imported: ${OUTPUT_DIR}"
echo "Analysis: ${RUN_DIR}/analysis.txt"
