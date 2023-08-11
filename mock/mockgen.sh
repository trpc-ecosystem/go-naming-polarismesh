#!/usr/bin/env bash

mockgen -destination mock_api/api_mock.go \
  github.com/polarismesh/polaris-go/api \
  SDKContext,ConsumerAPI,ProviderAPI

mockgen -destination mock_loadbalancer/loadbalancer_mock.go \
  github.com/polarismesh/polaris-go/pkg/plugin/loadbalancer \
  LoadBalancer

mockgen -destination mock_model/model_mock.go \
  github.com/polarismesh/polaris-go/pkg/model \
  Instance,CircuitBreakerStatus,ServiceInstances,ValueContext,ServiceClusters

mockgen -destination mock_plugin/plugin_mock.go \
  github.com/polarismesh/polaris-go/pkg/plugin \
  Manager,Plugin

mockgen -destination mock_servicerouter/servicerouter_mock.go \
  github.com/polarismesh/polaris-go/pkg/plugin/servicerouter \
  ServiceRouter
