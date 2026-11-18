#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
BUILD_ROOT="${SCRIPT_DIR}/.build"
OUTPUT_DIR="${SCRIPT_DIR}/dist"
PACKAGE_NAME=""
PACKAGE_NAME_EXPLICIT=false
GOOS_TARGET="linux"
GOARCH_TARGET="amd64"
KEEP_OLD=false

usage() {
    cat <<EOF
Usage: $0 [options]

Build a deployment package with precompiled example binaries for a remote host.

Options:
  --output-dir DIR    Archive output directory; default: ${OUTPUT_DIR}
  --build-root DIR    Temporary build directory; default: ${BUILD_ROOT}
  --name NAME         Package basename; default is generated from version/git/time
  --goos OS           Target operating system; default: ${GOOS_TARGET}
  --goarch ARCH       Target architecture; default: ${GOARCH_TARGET}
  --keep-old          Keep older generated packages
  --clean             Remove build and output directories, then exit
  -h, --help          Show this help
EOF
}

die() {
    echo "ERROR: $*" >&2
    exit 1
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --output-dir)
            [[ $# -ge 2 ]] || die "$1 requires a value"
            OUTPUT_DIR="$2"
            shift 2
            ;;
        --build-root)
            [[ $# -ge 2 ]] || die "$1 requires a value"
            BUILD_ROOT="$2"
            shift 2
            ;;
        --name)
            [[ $# -ge 2 ]] || die "$1 requires a value"
            PACKAGE_NAME="$2"
            PACKAGE_NAME_EXPLICIT=true
            shift 2
            ;;
        --goos)
            [[ $# -ge 2 ]] || die "$1 requires a value"
            GOOS_TARGET="$2"
            shift 2
            ;;
        --goarch)
            [[ $# -ge 2 ]] || die "$1 requires a value"
            GOARCH_TARGET="$2"
            shift 2
            ;;
        --keep-old)
            KEEP_OLD=true
            shift
            ;;
        --clean)
            rm -rf -- "${BUILD_ROOT}" "${OUTPUT_DIR}"
            echo "Removed ${BUILD_ROOT} and ${OUTPUT_DIR}"
            exit 0
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

[[ -f "${REPO_ROOT}/go.mod" ]] || die "go.mod not found in ${REPO_ROOT}"
command -v go >/dev/null 2>&1 || die "Go is required to build the deployment package"

VERSION="$(
    sed -n 's/^const Version = "\(.*\)"$/\1/p' "${REPO_ROOT}/version.go" |
        head -n1
)"
[[ -n "${VERSION}" ]] || die "cannot determine version from version.go"

GIT_COMMIT="$(git -C "${REPO_ROOT}" rev-parse --short=12 HEAD 2>/dev/null || echo unknown)"
if [[ -n "$(git -C "${REPO_ROOT}" status --porcelain 2>/dev/null || true)" ]]; then
    GIT_DIRTY=true
else
    GIT_DIRTY=false
fi

STAMP="$(date '+%Y%m%d_%H%M%S')"
PACKAGE_NAME="${PACKAGE_NAME:-go-naming-polarismesh-cloud-${VERSION}-${GIT_COMMIT}-${STAMP}-${GOOS_TARGET}-${GOARCH_TARGET}}"
STAGE_DIR="${BUILD_ROOT}/${PACKAGE_NAME}"
BIN_DIR="${STAGE_DIR}/bin"

rm -rf -- "${STAGE_DIR}"
mkdir -p "${STAGE_DIR}" "${BIN_DIR}" "${OUTPUT_DIR}"

echo "[1/4] Copying source, examples and cloud scripts"
cp -R "${REPO_ROOT}/examples" "${STAGE_DIR}/examples"
mkdir -p "${STAGE_DIR}/integration/cloud"
cp "${SCRIPT_DIR}"/*.sh "${SCRIPT_DIR}"/*.md "${STAGE_DIR}/integration/cloud/"
for file in go.mod go.sum version.go README.md README.zh_CN.md CHANGELOG.md \
    COMPATIBILITY.md COMPATIBILITY.zh_CN.md LICENSE CONTRIBUTING.md CONTRIBUTING.zh_CN.md; do
    [[ -f "${REPO_ROOT}/${file}" ]] && cp "${REPO_ROOT}/${file}" "${STAGE_DIR}/${file}"
done

find "${STAGE_DIR}/examples" -type d \
    \( -name .build -o -name .logs -o -name .state -o -name polaris \) \
    -prune -exec rm -rf -- {} +
find "${STAGE_DIR}/examples" -type f \
    \( -name '.*-pids' -o -name '*.log' -o -name '*.out' \) \
    -delete

for script in run-e2e.sh pack-logs.sh import-logs.sh summarize-results.sh; do
    cp "${SCRIPT_DIR}/${script}" "${STAGE_DIR}/${script}"
    chmod 0755 "${STAGE_DIR}/${script}"
done
cp "${SCRIPT_DIR}/README.md" "${STAGE_DIR}/CLOUD-README.md"
cp "${SCRIPT_DIR}/README.zh_CN.md" "${STAGE_DIR}/CLOUD-README.zh_CN.md"

echo "[2/4] Building ${GOOS_TARGET}/${GOARCH_TARGET} example binaries"
artifacts=(
    e2e-provider
    e2e-client
    multi-context
    helloworld-server
    helloworld-client
    registry-metadata-provider
    registry-transports-provider
)
packages=(
    ./examples/internal/e2eapp/provider
    ./examples/internal/e2eapp/client
    ./examples/multiContext
    ./examples/helloworld/server
    ./examples/helloworld/client
    ./examples/registry/metadata/provider
    ./examples/registry/transports/provider
)
for index in "${!artifacts[@]}"; do
    (
        cd "${REPO_ROOT}"
        env \
            CGO_ENABLED=0 \
            GOOS="${GOOS_TARGET}" \
            GOARCH="${GOARCH_TARGET}" \
            GOCACHE="${GOCACHE:-${TMPDIR:-/tmp}/go-naming-polarismesh-cloud-cache}" \
            go build -trimpath -ldflags="-s -w" \
            -o "${BIN_DIR}/${artifacts[$index]}" "${packages[$index]}"
    )
done

echo "[3/4] Writing manifest and checksums"
BUILD_AT="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
cat > "${STAGE_DIR}/manifest.json" <<EOF
{
  "package": "${PACKAGE_NAME}",
  "version": "${VERSION}",
  "git_commit": "${GIT_COMMIT}",
  "git_dirty": ${GIT_DIRTY},
  "built_at": "${BUILD_AT}",
  "goos": "${GOOS_TARGET}",
  "goarch": "${GOARCH_TARGET}",
  "binaries": [
    "bin/e2e-provider",
    "bin/e2e-client",
    "bin/multi-context",
    "bin/helloworld-server",
    "bin/helloworld-client",
    "bin/registry-metadata-provider",
    "bin/registry-transports-provider"
  ]
}
EOF

sha256_file() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    else
        shasum -a 256 "$1" | awk '{print $1}'
    fi
}

(
    cd "${STAGE_DIR}"
    : > SHA256SUMS
    while IFS= read -r file; do
        relative="${file#./}"
        printf '%s  %s\n' "$(sha256_file "${file}")" "${relative}"
    done < <(find . -type f ! -name SHA256SUMS -print | LC_ALL=C sort) >> SHA256SUMS
)

echo "[4/4] Creating archive"
if [[ "${KEEP_OLD}" == "false" && "${PACKAGE_NAME_EXPLICIT}" == "false" ]]; then
    rm -f -- "${OUTPUT_DIR}/go-naming-polarismesh-cloud-${VERSION}-${GIT_COMMIT}-"*.tar.gz \
        "${OUTPUT_DIR}/go-naming-polarismesh-cloud-${VERSION}-${GIT_COMMIT}-"*.sha256
fi

ARCHIVE="${OUTPUT_DIR}/${PACKAGE_NAME}.tar.gz"
COPYFILE_DISABLE=1 tar -czf "${ARCHIVE}" -C "${BUILD_ROOT}" "${PACKAGE_NAME}"
ARCHIVE_NAME="$(basename "${ARCHIVE}")"
printf '%s  %s\n' "$(sha256_file "${ARCHIVE}")" "${ARCHIVE_NAME}" > "${ARCHIVE}.sha256"
rm -rf -- "${STAGE_DIR}"

cat <<EOF

Package:  ${ARCHIVE}
Checksum: ${ARCHIVE}.sha256
Target:   ${GOOS_TARGET}/${GOARCH_TARGET}

Upload:
  scp ${ARCHIVE} ${ARCHIVE}.sha256 <user>@<cloud-host>:~/

On the cloud host:
  tar xzf ${ARCHIVE_NAME}
  cd ${PACKAGE_NAME}
  ./run-e2e.sh --server <polaris-host>:8091 --only registry
EOF
