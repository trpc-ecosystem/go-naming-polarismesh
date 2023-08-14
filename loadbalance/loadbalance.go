// Tencent is pleased to support the open source community by making tRPC available.
// Copyright (C) 2023 THL A29 Limited, a Tencent company. All rights reserved.
// If you have downloaded a copy of the tRPC source code from Tencent,
// please note that tRPC source code is licensed under the Apache 2.0 License,
// A copy of the Apache 2.0 License is included in this file.

// Package loadbalance is for loading balance configuration.
package loadbalance

import (
	"fmt"
	"net"
	"strconv"

	"gopkg.in/yaml.v3"
	"trpc.group/trpc-go/trpc-go/naming/loadbalance"
	"trpc.group/trpc-go/trpc-go/naming/registry"

	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/config"
	"github.com/polarismesh/polaris-go/pkg/model"
	"github.com/polarismesh/polaris-go/pkg/plugin/common"
	"github.com/polarismesh/polaris-go/pkg/plugin/loadbalancer"
	"github.com/polarismesh/polaris-go/plugin/loadbalancer/ringhash"
)

// Deprecated: you can always use the original string defined in polaris-go.
const (
	// LoadBalancerWR weighted rotation training
	LoadBalancerWR = "polaris_wr"
	// LoadBalancerHash hash
	LoadBalancerHash = "polaris_hash"
	// LoadBalancerRingHash ring hash
	LoadBalancerRingHash = "polaris_ring_hash"
	// LoadBalancerMaglev maglev
	LoadBalancerMaglev = "polaris_maglev"
	// LoadBalancerL5CST l5cst
	LoadBalancerL5CST = "polaris_l5cst"
)

// Deprecated: you can always use the original string defined in polaris-go.
var loadBalanceMap map[string]string = map[string]string{
	LoadBalancerWR:       config.DefaultLoadBalancerWR,
	LoadBalancerHash:     config.DefaultLoadBalancerHash,
	LoadBalancerRingHash: config.DefaultLoadBalancerRingHash,
	LoadBalancerMaglev:   config.DefaultLoadBalancerMaglev,
	LoadBalancerL5CST:    config.DefaultLoadBalancerL5CST,
}

var loadBalanceCfgMap = map[string]func() config.BaseConfig{
	config.DefaultLoadBalancerRingHash: func() config.BaseConfig { return &ringhash.Config{} },
}

const (
	setEnableKey   string = "internal-enable-set"
	setNameKey     string = "internal-set-name"
	setEnableValue string = "Y"
	containerKey   string = "container_name"
)

// Setup is for setting up
func Setup(ctx api.SDKContext, loadBalanceType string, setDefault bool) error {
	name, ok := loadBalanceMap[loadBalanceType]
	if !ok {
		// May fallback to the original name defined in polaris-go.
		name = loadBalanceType
	}

	lb, err := New(ctx, name)
	if err != nil {
		return fmt.Errorf("load balancer %s initialize err: %w", name, err)
	}

	loadbalance.Register(loadBalanceType, lb)
	if setDefault {
		loadbalance.SetDefaultLoadBalancer(lb)
	}
	return nil
}

// New creates a new WRLoadBalancer.
func New(ctx api.SDKContext, name string) (*WRLoadBalancer, error) {
	loadBalancer, err := ctx.GetPlugins().GetPlugin(common.TypeLoadBalancer, name)
	if err != nil {
		return nil, fmt.Errorf("api sdk ctx get plugin for %s err: %w", name, err)
	}
	lb := loadBalancer.(loadbalancer.LoadBalancer)
	return &WRLoadBalancer{
		sdkCtx: ctx,
		lb:     lb,
	}, nil
}

// WRLoadBalancer is a struct for a load balancing object.
type WRLoadBalancer struct {
	sdkCtx api.SDKContext
	lb     loadbalancer.LoadBalancer
}

// Select selects a load balancing node.
func (wr *WRLoadBalancer) Select(serviceName string,
	list []*registry.Node, opt ...loadbalance.Option) (*registry.Node, error) {
	opts := &loadbalance.Options{}
	for _, o := range opt {
		o(opts)
	}

	if len(list) == 0 {
		return nil, loadbalance.ErrNoServerAvailable
	}
	cluster := list[0].Metadata["cluster"].(*model.Cluster)
	serviceInstances := list[0].Metadata["serviceInstances"].(model.ServiceInstances)
	envKey := list[0].EnvKey

	criteria := &loadbalancer.Criteria{
		Cluster: cluster,
		HashKey: []byte(opts.Key),
	}
	inst, err := loadbalancer.ChooseInstance(wr.sdkCtx.GetValueContext(), wr.lb, criteria, serviceInstances)
	if err != nil {
		return nil, fmt.Errorf("choose instance err: %s", err.Error())
	}
	var (
		setName       string
		containerName string
	)
	if inst.GetMetadata() != nil {
		containerName = inst.GetMetadata()[containerKey]
		if enable := inst.GetMetadata()[setEnableKey]; enable == setEnableValue {
			setName = inst.GetMetadata()[setNameKey]
		}
	}

	node := &registry.Node{
		ContainerName: containerName,
		SetName:       setName,
		ServiceName:   serviceName,
		Address:       net.JoinHostPort(inst.GetHost(), strconv.Itoa(int(inst.GetPort()))),
		Weight:        inst.GetWeight(),
		EnvKey:        envKey,
		Metadata: map[string]interface{}{
			"instance": inst,
		},
	}

	return node, nil
}

// AsPluginCfgs parses yaml node to polaris mesh load balance configures.
func AsPluginCfgs(yamlCfgs map[string]yaml.Node) (map[string]config.BaseConfig, error) {
	cfgs := make(map[string]config.BaseConfig)
	for trpcName, node := range yamlCfgs {
		polarisName, ok := loadBalanceMap[trpcName]
		if !ok {
			return nil, fmt.Errorf("loadbalance %s is not implemented", trpcName)
		}
		newCfg, ok := loadBalanceCfgMap[polarisName]
		if !ok {
			return nil, fmt.Errorf("loadbalance %s does not support detailed config", trpcName)
		}
		cfg := newCfg()
		if err := node.Decode(cfg); err != nil {
			return nil, fmt.Errorf("failed to decode cfg of loadbalance %s: %w", trpcName, err)
		}
		cfgs[polarisName] = cfg
	}
	return cfgs, nil
}
