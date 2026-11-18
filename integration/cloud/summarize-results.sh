#!/bin/bash

set -euo pipefail

usage() {
    cat <<'EOF'
Usage: ./summarize-results.sh RUN_DIR [--output FILE] [--strict]

Summarize results.tsv and manifest.json from one cloud E2E run.
--strict returns non-zero when at least one scenario failed.
EOF
}

die() {
    echo "ERROR: $*" >&2
    exit 1
}

RUN_DIR=""
OUTPUT=""
STRICT=false
while [[ $# -gt 0 ]]; do
    case "$1" in
        --output)
            [[ $# -ge 2 ]] || die "$1 requires a value"
            OUTPUT="$2"
            shift 2
            ;;
        --strict)
            STRICT=true
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
            [[ -z "${RUN_DIR}" ]] || die "only one run directory may be supplied"
            RUN_DIR="$1"
            shift
            ;;
    esac
done

[[ -n "${RUN_DIR}" ]] || die "RUN_DIR is required"
[[ -d "${RUN_DIR}" ]] || die "run directory not found: ${RUN_DIR}"
RUN_DIR="$(cd "${RUN_DIR}" && pwd)"
OUTPUT="${OUTPUT:-${RUN_DIR}/analysis.txt}"
RESULTS_FILE="${RUN_DIR}/results.tsv"
MANIFEST_FILE="${RUN_DIR}/manifest.json"

manifest_value() {
    local key="$1"
    [[ -f "${MANIFEST_FILE}" ]] || {
        printf '<unknown>'
        return
    }
    local value
    value="$(sed -n "s/.*\"${key}\"[[:space:]]*:[[:space:]]*\"\\([^\"]*\\)\".*/\\1/p" \
        "${MANIFEST_FILE}" | head -n1)"
    if [[ -z "${value}" ]]; then
        value="$(sed -n "s/.*\"${key}\"[[:space:]]*:[[:space:]]*\\([^,]*\\).*/\\1/p" \
            "${MANIFEST_FILE}" | head -n1 | tr -d '[:space:]')"
    fi
    printf '%s' "${value:-<unknown>}"
}

passed=0
failed=0
total=0
failed_names=()
table_rows=""
if [[ -f "${RESULTS_FILE}" ]]; then
    while IFS=$'\t' read -r scenario code duration; do
        [[ "${scenario}" == "scenario" ]] && continue
        [[ -n "${scenario}" ]] || continue
        total=$((total + 1))
        if [[ "${code}" == "0" ]]; then
            passed=$((passed + 1))
        else
            failed=$((failed + 1))
            failed_names+=("${scenario}")
        fi
        table_rows+="| ${scenario} | ${code} | ${duration} |"$'\n'
    done < "${RESULTS_FILE}"
fi

mkdir -p "$(dirname "${OUTPUT}")"
{
    echo "# go-naming-polarismesh Cloud E2E Result"
    echo
    echo "- Run directory: ${RUN_DIR}"
    echo "- Run ID: $(manifest_value run_id)"
    echo "- Server: $(manifest_value server)"
    echo "- Namespace: $(manifest_value namespace)"
    echo "- Started: $(manifest_value started_at)"
    echo "- Finished: $(manifest_value finished_at)"
    echo "- Overall exit code: $(manifest_value exit_code)"
    echo
    if [[ "${total}" -eq 0 ]]; then
        echo "No scenario results were found. Inspect run.log and logs/."
    else
        echo "Result: ${passed} passed, ${failed} failed"
        echo
        echo "| Scenario | Exit code | Duration (s) |"
        echo "| --- | ---: | ---: |"
        printf '%s' "${table_rows}"
        if [[ ${#failed_names[@]} -gt 0 ]]; then
            printf '\nFailed scenarios: %s\n' "$(
                IFS=', '
                echo "${failed_names[*]}"
            )"
        fi
    fi
    echo
    echo "Inspect order: analysis.txt -> scenario log -> retained .logs -> polaris/log/."
} > "${OUTPUT}"

cat "${OUTPUT}"
if [[ "${STRICT}" == "true" && "${failed}" -gt 0 ]]; then
    exit 1
fi
