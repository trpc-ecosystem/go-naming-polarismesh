#!/bin/bash

set -euo pipefail

COMMON_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../internal" && pwd)"
source "${COMMON_DIR}/e2e_common.sh"

feature_common_init circuitbreaker
feature_usage() {
    echo "Usage: ./examples/circuitbreaker/verify.sh --polaris-server <host> [--debug]"
}
feature_parse_transport_args "$@"

SERVICE_NAME="trpc.polaris.e2e.CircuitBreaker"
CALLER_SERVICE="trpc.polaris.e2e.CircuitBreakerCaller"
PORT_A="${PORT_A:-28241}"
PORT_B="${PORT_B:-28242}"
CLIENT_LISTEN="${CLIENT_LISTEN:-18241}"
CLIENT_URL="http://127.0.0.1:${CLIENT_LISTEN}"
PROVIDER_B_PID=""

rule_id() {
    local name="$1"
    api_request GET "/naming/v1/circuitbreaker/rules?name=${name}&limit=20" |
        jq -r --arg name "${name}" '.data[]? | select(.name == $name) | .id' |
        head -n1
}

set_rule_enabled() {
    local name="$1"
    local enabled="$2"
    local id response
    id="$(rule_id "${name}")"
    [[ -n "${id}" ]] || return 0
    response="$(api_request PUT /naming/v1/circuitbreaker/rules/enable \
        "[{\"id\":\"${id}\",\"enable\":${enabled}}]")"
    if ! api_success "${response}"; then
        echo "set rule ${name} enabled=${enabled} failed: ${response}" >&2
        return 1
    fi
}

ensure_rule() {
    local name="$1"
    local level="$2"
    local path="${3:-}"
    local id response payload
    id="$(rule_id "${name}")"
    if [[ -n "${id}" ]]; then
        set_rule_enabled "${name}" true
        echo "reuse circuit-breaker rule ${name}" >&2
        return 0
    fi
    payload="$(jq -cn \
        --arg name "${name}" \
        --arg level "${level}" \
        --arg namespace "${NAMESPACE}" \
        --arg service "${SERVICE_NAME}" \
        --arg caller "${CALLER_SERVICE}" \
        --arg path "${path}" '
        {
          name: $name,
          namespace: $namespace,
          enable: true,
          level: $level,
          description: "go-naming-polarismesh circuit breaker example",
          rule_matcher: {
            source: {namespace: $namespace, service: $caller},
            destination: {
              namespace: $namespace,
              service: $service,
              method: {type: "EXACT", value: "*"}
            }
          },
          error_conditions: [{
            inputType: "RET_CODE",
            condition: {type: "RANGE", value: "500~599"}
          }],
          trigger_condition: [{
            triggerType: "CONSECUTIVE_ERROR",
            errorCount: 1
          }],
          recoverCondition: {sleep_window: 2, consecutiveSuccess: 1},
          block_configs: [{
            name: "main",
            api: (if $path == "" then {path: {value: ""}} else {
              protocol: "*",
              method: "*",
              path: {type: "EXACT", value: $path}
            } end),
            error_conditions: [{
              inputType: "RET_CODE",
              condition: {type: "RANGE", value: "500~599"}
            }],
            trigger_conditions: [{
              triggerType: "CONSECUTIVE_ERROR",
              errorCount: 1
            }]
          }]
        }')"
    response="$(api_request POST /naming/v1/circuitbreaker/rules "[${payload}]")"
    if create_success "${response}"; then
        echo "create circuit-breaker rule ${name}" >&2
        return 0
    fi
    # Another run may have created the rule between the query and create.
    id="$(rule_id "${name}")"
    if [[ -n "${id}" ]]; then
        set_rule_enabled "${name}" true
        echo "reuse circuit-breaker rule ${name}" >&2
        return 0
    fi
    echo "create rule ${name} failed: ${response}" >&2
    return 1
}

activate_rule() {
    local active="$1"
    local rule
    for rule in \
        go-naming-polarismesh-e2e-cb-instance \
        go-naming-polarismesh-e2e-cb-service \
        go-naming-polarismesh-e2e-cb-method; do
        [[ "${rule}" == "${active}" ]] && continue
        set_rule_enabled "${rule}" false
    done
    ensure_rule "$@"
}

find_bad_key() {
    local result index
    result="$(curl -fsS --max-time 60 \
        "${CLIENT_URL}/sequence?mode=service&lb=polaris_hash&key=cb-key&count=1024")"
    index="$(jq -r --arg port ":${PORT_B}" '
        [.calls | to_entries[]
         | select(.value.success and (.value.address | endswith($port)))
         | .key][0] // empty
    ' <<<"${result}")"
    if [[ -z "${index}" ]]; then
        echo "no provider-b selection in hash sequence: ${result}" >&2
        return 1
    fi
    echo "cb-key-${index}"
}

wait_for_provider_b() {
    wait_for_instances "${SERVICE_NAME}" 2
}

start_provider_b() {
    PROVIDER_B_PID="$(start_provider provider-b-${1} "${SERVICE_NAME}" "${PORT_B}" 100 "")"
    wait_for_provider_b
}

kill_provider_b() {
    if [[ -n "${PROVIDER_B_PID}" ]]; then
        kill -KILL "${PROVIDER_B_PID}" 2>/dev/null || true
        wait "${PROVIDER_B_PID}" 2>/dev/null || true
        PROVIDER_B_PID=""
    fi
}

build_shared_binaries
ensure_namespace
ensure_service "${SERVICE_NAME}"
start_provider provider-a "${SERVICE_NAME}" "${PORT_A}" 100 "" >/dev/null
start_provider_b initial
start_client "${SERVICE_NAME}" "${CALLER_SERVICE}" >/dev/null
wait_for_http "${CLIENT_URL}/health"

# Case 1: INSTANCE failover and half-open recovery.
activate_rule "go-naming-polarismesh-e2e-cb-instance" INSTANCE
key="$(find_bad_key)" || {
    echo "could not select provider-b" >&2
    exit 1
}
kill_provider_b
sleep 1
failure_seen=false
streak=0
for _ in $(seq 1 20); do
    result="$(curl -fsS --max-time 15 \
        "${CLIENT_URL}/call?mode=service&lb=polaris_hash&key=${key}")"
    if jq -e '.success == false' >/dev/null <<<"${result}"; then
        failure_seen=true
        streak=0
        sleep 0.3
        continue
    fi
    address="$(jq -r '.address' <<<"${result}")"
    [[ "${address##*:}" == "${PORT_A}" ]] && streak=$((streak + 1))
    [[ "${streak}" -ge 5 ]] && break
done
[[ "${failure_seen}" == "true" && "${streak}" -ge 5 ]] || {
    echo "instance failover failed: ${result}" >&2
    exit 1
}
echo "[case 1.1 instance failover] PASS"

start_provider_b recovery
sleep 4
recovered=false
for _ in $(seq 1 30); do
    result="$(curl -fsS --max-time 15 \
        "${CLIENT_URL}/call?mode=service&lb=polaris_hash&key=${key}")"
    address="$(jq -r '.address' <<<"${result}")"
    if jq -e '.success == true' >/dev/null <<<"${result}" &&
        [[ "${address##*:}" == "${PORT_B}" ]]; then
        recovered=true
        break
    fi
    sleep 0.5
done
[[ "${recovered}" == "true" ]] || {
    echo "half-open recovery did not return provider-b: ${result}" >&2
    exit 1
}
echo "[case 1.2 half-open recovery] PASS"

# Case 2: SERVICE-level rule opens the whole service.
activate_rule "go-naming-polarismesh-e2e-cb-service" SERVICE
kill_provider_b
sleep 1
service_opened=false
for _ in $(seq 1 40); do
    result="$(curl -fsS --max-time 15 \
        "${CLIENT_URL}/call?mode=service&lb=polaris_hash&key=${key}")"
    if jq -e '.success == false' >/dev/null <<<"${result}" &&
        jq -r '.error' <<<"${result}" | grep -q "service .* denied by circuit breaker"; then
        service_opened=true
        break
    fi
    sleep 0.5
done
[[ "${service_opened}" == "true" ]] || {
    echo "service rule did not open: ${result}" >&2
    exit 1
}
echo "[case 1.3 service-level rule] PASS"
start_provider_b service-recovery

# Case 3: METHOD-level rule opens only the configured method.
activate_rule "go-naming-polarismesh-e2e-cb-method" METHOD "/${SERVICE_NAME}/Echo"
kill_provider_b
sleep 1
method_opened=false
for _ in $(seq 1 40); do
    result="$(curl -fsS --max-time 15 \
        "${CLIENT_URL}/call?mode=service&lb=polaris_hash&key=${key}")"
    if jq -e '.success == false' >/dev/null <<<"${result}" &&
        jq -r '.error' <<<"${result}" | grep -q "method .* denied by circuit breaker"; then
        method_opened=true
        break
    fi
    sleep 0.5
done
[[ "${method_opened}" == "true" ]] || {
    echo "method rule did not open: ${result}" >&2
    exit 1
}
echo "[case 1.4 method-level rule] PASS"
