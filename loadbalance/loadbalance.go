//
//
// Tencent is pleased to support the open source community by making tRPC available.
//
// Copyright (C) 2023 Tencent.
// All rights reserved.
//
// If you have downloaded a copy of the tRPC source code from Tencent,
// please note that tRPC source code is licensed under the Apache 2.0 License,
// A copy of the Apache 2.0 License is included in this file.
//
//

// Package loadbalance is for loading balance configuration.
package loadbalance

import (
	"fmt"
	"net"
	"strconv"

	"trpc.group/trpc-go/trpc-go/codec"
	thttp "trpc.group/trpc-go/trpc-go/http"
	"trpc.group/trpc-go/trpc-go/naming/loadbalance"
	"trpc.group/trpc-go/trpc-go/naming/registry"
	"trpc.group/trpc-go/trpc-naming-polarismesh/internal/polaris"

	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/config"
	"github.com/polarismesh/polaris-go/pkg/model"
	"github.com/polarismesh/polaris-go/pkg/plugin/common"
	"github.com/polarismesh/polaris-go/pkg/plugin/loadbalancer"
	"github.com/polarismesh/polaris-go/plugin/loadbalancer/ringhash"
	"gopkg.in/yaml.v3"
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
	lb := unwrapLoadBalancer(loadBalancer.(loadbalancer.LoadBalancer))
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
	hashKey := []byte(opts.Key)
	sourceService := list[0].Metadata[polaris.SourceServiceMetadataKey]
	callerValue, hasCaller := list[0].Metadata[polaris.CallerServiceMetadataKey]
	callerService := polaris.SourceServiceKey(callerValue)
	if !hasCaller {
		callerService = polaris.SourceServiceKey(sourceService)
	}

	lb := unwrapLoadBalancer(wr.lb)
	criteria := loadbalancer.Criteria{HashKey: hashKey}
	inst, err := polaris.ChooseInstanceCandidate(wr.sdkCtx, lb, cluster, serviceInstances, criteria)
	if err != nil {
		return nil, fmt.Errorf("choose instance err: %w", err)
	}
	serviceResource, err := polaris.InstanceServiceResource(inst, callerService)
	if err != nil {
		return nil, fmt.Errorf("build service circuit breaker resource err: %w", err)
	}
	methodResource, err := polaris.MethodResource(
		serviceResource.GetService(), callerService, inst.GetProtocol(),
		loadBalanceHTTPMethod(opts), loadBalanceMethodPath(opts))
	if err != nil {
		return nil, fmt.Errorf("build method circuit breaker resource err: %w", err)
	}
	instanceAvailable, err := polaris.InstancePermissionAvailable(
		wr.sdkCtx, inst, cluster.HasLimitedInstances, callerService)
	if err != nil {
		return nil, fmt.Errorf("check instance circuit breaker err: %w", err)
	}
	if !instanceAvailable {
		inst, err = polaris.ChooseAvailableInstance(
			wr.sdkCtx, lb, cluster, serviceInstances, criteria, callerService)
		if err != nil {
			return nil, fmt.Errorf("choose available instance err: %w", err)
		}
		fallbackMethodResource, err := polaris.MethodResource(
			serviceResource.GetService(), callerService, inst.GetProtocol(),
			loadBalanceHTTPMethod(opts), loadBalanceMethodPath(opts))
		if err != nil {
			return nil, fmt.Errorf("build fallback method circuit breaker resource err: %w", err)
		}
		methodResource = fallbackMethodResource
	}
	instanceResource, err := polaris.InstanceResource(inst, callerService)
	if err != nil {
		return nil, fmt.Errorf("build instance circuit breaker resource err: %w", err)
	}
	resources := []model.Resource{serviceResource}
	if methodResource != nil {
		resources = append(resources, methodResource)
	}
	if !polaris.IsRecoverAllOpenInstance(inst, cluster.HasLimitedInstances) {
		resources = append(resources, instanceResource)
	}
	permission, err := polaris.AcquireResourcePermissions(wr.sdkCtx, resources...)
	if err != nil {
		return nil, fmt.Errorf("check circuit breaker err: %w", err)
	}
	if !permission.Pass {
		switch resource := permission.Resource.(type) {
		case *model.MethodResource:
			return nil, fmt.Errorf("method %s denied by circuit breaker", resource.Path)
		case *model.InstanceResource:
			return nil, fmt.Errorf(
				"instance %s:%d denied by circuit breaker",
				resource.GetNode().Host, resource.GetNode().Port)
		default:
			return nil, fmt.Errorf("service %s/%s denied by circuit breaker",
				inst.GetNamespace(), inst.GetService())
		}
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
			"instance":                        inst,
			polaris.SourceServiceMetadataKey:  sourceService,
			polaris.CallerServiceMetadataKey:  callerService,
			polaris.MethodResourceMetadataKey: methodResource,
			polaris.AllowOpenMetadataKey:      cluster.HasLimitedInstances,
		},
	}

	return node, nil
}

func loadBalanceMethodPath(opts *loadbalance.Options) string {
	if opts.Ctx == nil {
		return ""
	}
	msg := codec.Message(opts.Ctx)
	if isHTTPRequest(opts) {
		if path := msg.CalleeMethod(); path != "" {
			return path
		}
	}
	if path := msg.ClientRPCName(); path != "" {
		return path
	}
	return msg.CalleeMethod()
}

func loadBalanceHTTPMethod(opts *loadbalance.Options) string {
	if opts.Ctx == nil {
		return ""
	}
	msg := codec.Message(opts.Ctx)
	switch msg.ClientReqHead().(type) {
	case *thttp.ClientReqHeader:
		return loadBalanceHTTPMethodFromMessage(msg)
	}
	return ""
}

func loadBalanceHTTPMethodFromMessage(msg codec.Msg) string {
	if header, ok := msg.ClientReqHead().(*thttp.ClientReqHeader); ok {
		return header.Method
	}
	return ""
}

func isHTTPRequest(opts *loadbalance.Options) bool {
	if opts.Ctx == nil {
		return false
	}
	_, ok := codec.Message(opts.Ctx).ClientReqHead().(*thttp.ClientReqHeader)
	return ok
}

func unwrapLoadBalancer(lb loadbalancer.LoadBalancer) loadbalancer.LoadBalancer {
	if proxy, ok := lb.(*loadbalancer.Proxy); ok {
		return proxy.LoadBalancer
	}
	return lb
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
