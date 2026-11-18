#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

mockgen -destination mock_api/api_mock.go \
  github.com/polarismesh/polaris-go/api \
  SDKContext,ConsumerAPI,ProviderAPI

mockgen -destination mock_loadbalancer/loadbalancer_mock.go \
  github.com/polarismesh/polaris-go/pkg/plugin/loadbalancer \
  LoadBalancer

mockgen -destination mock_model/model_mock.go \
  github.com/polarismesh/polaris-go/pkg/model \
  Instance,CircuitBreakerStatus,ServiceInstances,ServiceClusters

mockgen -destination mock_model/value_context_mock.go -package mock_model \
  github.com/polarismesh/polaris-go/pkg/sdk \
  ValueContext

mockgen -destination mock_plugin/plugin_mock.go \
  github.com/polarismesh/polaris-go/pkg/plugin \
  Manager,Plugin

mockgen -destination mock_servicerouter/servicerouter_mock.go \
  github.com/polarismesh/polaris-go/pkg/plugin/servicerouter \
  ServiceRouter
