#!/bin/bash

set -euo pipefail

COMMON_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../internal" && pwd)"
source "${COMMON_DIR}/e2e_common.sh"

feature_common_init route-rule
feature_usage() {
    echo "Usage: ./examples/route/rule/verify.sh --polaris-server <host> [--debug]"
}
feature_parse_transport_args "$@"

SERVICE_NAME="trpc.polaris.e2e.RouteRule"
CALLER_SERVICE="trpc.polaris.e2e.RouteRuleCaller"
RULE_NAME="go-naming-polarismesh-e2e-route-rule"
PORT_STABLE="${PORT_STABLE:-28231}"
PORT_VIP="${PORT_VIP:-28232}"
CLIENT_LISTEN="${CLIENT_LISTEN:-18231}"
CLIENT_URL="http://127.0.0.1:${CLIENT_LISTEN}"

route_rule_id() {
    local response
    response="$(api_request GET \
        "/naming/v2/routings?name=${RULE_NAME}&namespace=${NAMESPACE}&service=${SERVICE_NAME}&offset=0&limit=20")" ||
        return 1
    jq -r --arg name "${RULE_NAME}" \
        '.data[]? | select(.name == $name) | .id' <<<"${response}" 2>/dev/null |
        head -n1
}

route_rule_payload() {
    local id="${1:-}"
    jq -cn \
        --arg id "${id}" \
        --arg name "${RULE_NAME}" \
        --arg namespace "${NAMESPACE}" \
        --arg service "${SERVICE_NAME}" \
        --arg caller "${CALLER_SERVICE}" '
        [{
          name: $name,
          namespace: $namespace,
          enable: true,
          routing_policy: "RulePolicy",
          priority: 0,
          description: "go-naming-polarismesh source/destination route",
          routing_config: {
            "@type": "type.googleapis.com/v1.RuleRoutingConfig",
            rules: [{
              name: "source-segment-blue-to-vip",
              sources: [{
                service: $caller,
                namespace: $namespace
              }],
              destinations: [{
                service: $service,
                namespace: $namespace,
                labels: {
                  route: {type: "EXACT", value: "vip", value_type: "TEXT"}
                },
                priority: 0,
                weight: 100,
                name: "vip"
              }]
            }]
          }
        }]
        | if $id == "" then . else .[0].id = $id end'
}

build_shared_binaries
ensure_namespace
ensure_service "${SERVICE_NAME}"
start_provider provider-stable "${SERVICE_NAME}" "${PORT_STABLE}" 100 "route=stable" >/dev/null
start_provider provider-vip "${SERVICE_NAME}" "${PORT_VIP}" 100 "route=vip" >/dev/null
wait_for_instances "${SERVICE_NAME}" 2
start_client "${SERVICE_NAME}" "${CALLER_SERVICE}" >/dev/null
wait_for_http "${CLIENT_URL}/health"

rule_id="$(route_rule_id)"
if [[ -n "${rule_id}" ]]; then
    payload="$(route_rule_payload "${rule_id}")"
    response="$(api_request PUT /naming/v2/routings "${payload}")"
    if ! api_success "${response}"; then
        echo "update route rule ${RULE_NAME} failed: ${response}" >&2
        exit 1
    fi
    echo "reuse route rule ${RULE_NAME}" >&2
else
    payload="$(route_rule_payload)"
    response="$(api_request POST /naming/v2/routings "${payload}")"
    if ! api_success "${response}"; then
        # Another run may have created the rule between the query and create.
        rule_id="$(route_rule_id)"
        if [[ -n "${rule_id}" ]]; then
            payload="$(route_rule_payload "${rule_id}")"
            response="$(api_request PUT /naming/v2/routings "${payload}")"
            if ! api_success "${response}"; then
                echo "update route rule ${RULE_NAME} failed: ${response}" >&2
                exit 1
            fi
            echo "reuse route rule ${RULE_NAME}" >&2
        else
            echo "create route rule failed: ${response}" >&2
            exit 1
        fi
    else
        echo "create route rule ${RULE_NAME}" >&2
    fi
fi

vip_port=":${PORT_VIP}"
for _ in $(seq 1 30); do
    result="$(curl -fsS --max-time 60 \
        "${CLIENT_URL}/sequence?mode=service&lb=polaris_wr&count=12")"
    if jq -e --arg vipPort "${vip_port}" '
        .successes == 12
        and (.addresses | length) == 1
        and ([.addresses | keys[] | select(endswith($vipPort))] | length) == 1
    ' >/dev/null <<<"${result}"; then
        echo "[case 1.1 source and destination rule] PASS"
        break
    fi
    sleep 1
done
if ! jq -e --arg vipPort "${vip_port}" '
    .successes == 12
    and (.addresses | length) == 1
        and ([.addresses | keys[] | select(endswith($vipPort))] | length) == 1
' >/dev/null <<<"${result}"; then
    echo "route rule did not select vip: ${result}" >&2
    exit 1
fi

result="$(curl -fsS --max-time 60 \
    "${CLIENT_URL}/sequence?mode=service&lb=polaris_wr&count=8&source_svc=trpc.polaris.e2e.RouteRuleOtherCaller")"
if ! jq -e '.successes == 8 and (.addresses | length) > 1' >/dev/null <<<"${result}"; then
    echo "negative source rule failed: ${result}" >&2
    exit 1
fi
echo "[case 1.2 unmatched source falls back to normal routing] PASS"
