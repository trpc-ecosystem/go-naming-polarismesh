// Tencent is pleased to support the open source community by making tRPC available.
// Copyright (C) 2023 THL A29 Limited, a Tencent company. All rights reserved.
// If you have downloaded a copy of the tRPC source code from Tencent,
// please note that tRPC source code is licensed under the Apache 2.0 License,
// A copy of the Apache 2.0 License is included in this file.

// Package discovery is a package for service discovery.
package discovery

import (
	"fmt"

	"trpc.group/trpc-go/trpc-go/naming/discovery"
	"trpc.group/trpc-go/trpc-go/naming/registry"

	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/model"
)

// Setup is for setting up.
func Setup(sdkCtx api.SDKContext, cfg *Config, setDefault bool) error {
	d := &Discovery{
		consumer: api.NewConsumerAPIByContext(sdkCtx),
		cfg:      cfg,
	}

	name := "polarismesh"
	if cfg != nil && cfg.Name != "" {
		name = cfg.Name
	}
	discovery.Register(name, d)
	if setDefault {
		discovery.SetDefaultDiscovery(d)
	}
	return nil
}

// Discovery is service discovery.
type Discovery struct {
	consumer api.ConsumerAPI
	cfg      *Config
}

func checkOpts(serviceName string, opt ...discovery.Option) (*discovery.Options, error) {
	opts := &discovery.Options{}

	for _, o := range opt {
		o(opts)
	}

	if len(serviceName) == 0 || len(opts.Namespace) == 0 {
		return nil, fmt.Errorf("service or namespace is empty, namespace: %s, service: %s",
			opts.Namespace, serviceName)
	}

	return opts, nil
}

// List gets a list of instances of a service.
func (d *Discovery) List(serviceName string, opt ...discovery.Option) (nodes []*registry.Node, err error) {

	opts, err := checkOpts(serviceName, opt...)
	if err != nil {
		return nil, err
	}

	req := &api.GetInstancesRequest{
		GetInstancesRequest: model.GetInstancesRequest{
			Namespace:                    opts.Namespace,
			Service:                      serviceName,
			IncludeCircuitBreakInstances: true,
			IncludeUnhealthyInstances:    true,
			SkipRouteFilter:              true,
		},
	}
	resp, err := d.consumer.GetInstances(req)
	if err != nil {
		return nil, fmt.Errorf("fail to get instances, err is %s", err.Error())
	}

	list := []*registry.Node{}
	for range resp.Instances {
		n := &registry.Node{
			Metadata: map[string]interface{}{
				"service_instances": resp,
			},
		}
		list = append(list, n)
		// The node list is meaningless for the framework.
		// For performance considerations, only one node list information used to store the sdk is reserved.
		break
	}

	return list, nil
}
