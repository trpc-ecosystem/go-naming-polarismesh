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

// Package selector is a selector.
package selector

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"trpc.group/trpc-go/trpc-go/codec"
	thttp "trpc.group/trpc-go/trpc-go/http"
	"trpc.group/trpc-go/trpc-go/log"
	"trpc.group/trpc-go/trpc-go/naming/registry"
	"trpc.group/trpc-go/trpc-go/naming/selector"
	"trpc.group/trpc-go/trpc-naming-polarismesh/circuitbreaker"
	"trpc.group/trpc-go/trpc-naming-polarismesh/internal/polaris"
	"trpc.group/trpc-go/trpc-naming-polarismesh/servicerouter"

	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/config"
	"github.com/polarismesh/polaris-go/pkg/model"
	"github.com/polarismesh/polaris-go/pkg/plugin/common"
	"github.com/polarismesh/polaris-go/pkg/plugin/loadbalancer"
)

const (
	// DefaultConnectTimeout Default connection timeout.
	DefaultConnectTimeout = time.Second
	// DefaultMessageTimeout Default message timeout.
	DefaultMessageTimeout = time.Second
)

var once = &sync.Once{}

// Setup is for setting up.
func Setup(sdkCtx api.SDKContext, cfg *Config) error {
	s := &Selector{
		consumer: api.NewConsumerAPIByContext(sdkCtx),
		cfg:      cfg,
	}
	const defaultName = "polarismesh"
	if cfg.Name == "" {
		cfg.Name = defaultName
	}
	selector.Register(cfg.Name, s)
	return nil
}

// Register registers selector according to parameters.
func Register(cfg *Config) {
	once.Do(func() {
		s, err := New(cfg)
		if err != nil {
			panic(err)
		}
		selector.Register("polarismesh", s)
	})
}

// New new instance.
func New(cfg *Config) (*Selector, error) {
	var c *config.ConfigurationImpl
	if cfg.UseBuildin {
		c = config.NewDefaultConfigurationWithDomain()
	} else {
		c = config.NewDefaultConfiguration(cfg.ServerAddrs)
	}
	if cfg.Protocol == "" {
		cfg.Protocol = "grpc"
	}
	c.Global.ServerConnector.Protocol = cfg.Protocol
	if cfg.RefreshInterval != 0 {
		refreshInterval := time.Duration(cfg.RefreshInterval) * time.Millisecond
		c.Consumer.LocalCache.ServiceRefreshInterval = &refreshInterval
	}
	if cfg.Timeout != 0 {
		timeout := time.Duration(cfg.Timeout) * time.Millisecond
		c.Global.API.Timeout = &timeout
		// If timeout is set, the maximum number of retries needs to be set to 0.
		c.Global.API.MaxRetryTimes = 0
	}
	// Set local IP
	if cfg.BindIP != "" {
		c.Global.API.BindIP = cfg.BindIP
		c.Global.API.BindIPValue = cfg.BindIP
	}

	connectTimeout := DefaultConnectTimeout
	if cfg.ConnectTimeout != 0 {
		connectTimeout = time.Millisecond * time.Duration(cfg.ConnectTimeout)
	}
	c.Global.ServerConnector.ConnectTimeout = model.ToDurationPtr(connectTimeout)

	// Add a plugin to filter according to the called service env.
	c.Consumer.ServiceRouter.Chain = append([]string{config.DefaultServiceRouterDstMeta},
		c.Consumer.ServiceRouter.Chain...)

	// Add canary routing chain.
	if cfg.EnableCanary {
		c.Consumer.ServiceRouter.Chain = append(c.Consumer.ServiceRouter.Chain,
			config.DefaultServiceRouterCanary)
	}
	// Configure the local cache storage address.
	if cfg.LocalCachePersistDir != "" {
		c.Consumer.LocalCache.PersistDir = cfg.LocalCachePersistDir
	}
	sdkCtx, err := api.InitContextByConfig(c)
	if err != nil {
		return nil, err
	}
	return &Selector{
		consumer: api.NewConsumerAPIByContext(sdkCtx),
		cfg:      cfg,
	}, nil
}

// Selector is route selector.
type Selector struct {
	consumer api.ConsumerAPI
	cfg      *Config
}

func getMetadata(opts *selector.Options, enableTransMeta bool) map[string]string {
	metadata := make(map[string]string)
	if len(opts.SourceEnvName) > 0 {
		metadata["env"] = opts.SourceEnvName
	}
	// To solve the problem that the transparent transmission field of
	// the request cannot be passed to polaris mesh for meta matching,
	// agree on the transparent transmission field with the prefix 'selector-meta-',
	// remove the prefix and fill in meta, and use it for polaris mesh matching.
	if enableTransMeta {
		setTransSelectorMeta(opts, metadata)
	}
	for key, value := range opts.SourceMetadata {
		if len(key) > 0 && len(value) > 0 {
			metadata[key] = value
		}
	}
	return metadata
}

func extractSourceServiceRequestInfo(opts *selector.Options, enableTransMeta bool) *model.ServiceInfo {
	if opts.DisableServiceRouter {
		return nil
	}
	metadata := getMetadata(opts, enableTransMeta)

	if opts.SourceServiceName != "" || opts.SourceNamespace != "" || len(metadata) > 0 {
		// When the calling service is not empty, or metadata is not empty, return ServiceInfo.
		return &model.ServiceInfo{
			Service:   opts.SourceServiceName,
			Namespace: opts.SourceNamespace,
			Metadata:  metadata,
		}
	}
	return nil
}

func extractCallerServiceRequestInfo(opts *selector.Options) *model.ServiceKey {
	if opts.SourceNamespace == "" && opts.SourceServiceName == "" {
		return model.EmptyServiceKey
	}
	return &model.ServiceKey{
		Namespace: opts.SourceNamespace,
		Service:   opts.SourceServiceName,
	}
}

func requestMethodPath(opts *selector.Options) string {
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

func requestHTTPMethod(opts *selector.Options) string {
	if opts.Ctx == nil {
		return ""
	}
	msg := codec.Message(opts.Ctx)
	switch msg.ClientReqHead().(type) {
	case *thttp.ClientReqHeader:
		return requestHTTPMethodFromMessage(msg)
	}
	return ""
}

func requestHTTPMethodFromMessage(msg codec.Msg) string {
	if header, ok := msg.ClientReqHead().(*thttp.ClientReqHeader); ok {
		return header.Method
	}
	return ""
}

func isHTTPRequest(opts *selector.Options) bool {
	if opts.Ctx == nil {
		return false
	}
	_, ok := codec.Message(opts.Ctx).ClientReqHead().(*thttp.ClientReqHeader)
	return ok
}

func isInstanceNotFound(err error) bool {
	var sdkErr model.SDKError
	return errors.As(err, &sdkErr) && sdkErr.ErrorCode() == model.ErrCodeAPIInstanceNotFound
}

func getDestMetadata(opts *selector.Options) map[string]string {
	destMeta := make(map[string]string)
	// Support environment selection when service routing is not enabled.
	if opts.DisableServiceRouter {
		if len(opts.DestinationEnvName) > 0 {
			destMeta["env"] = opts.DestinationEnvName
		}
	}
	// Support custom metadata tag key passed to polaris mesh for addressing.
	for key, value := range opts.DestinationMetadata {
		if len(key) > 0 && len(value) > 0 {
			destMeta[key] = value
		}
	}
	return destMeta
}

// Select selects service node.
func (s *Selector) Select(serviceName string, opt ...selector.Option) (*registry.Node, error) {
	opts := &selector.Options{}
	for _, o := range opt {
		o(opts)
	}
	log.Tracef("[NAMING-POLARISMESH] select options: %+v", opts)

	namespace := opts.Namespace
	var sourceService *model.ServiceInfo

	if s.cfg.Enable {
		sourceService = extractSourceServiceRequestInfo(opts, s.cfg.EnableTransMeta)
	}
	callerService := extractCallerServiceRequestInfo(opts)
	if opts.LoadBalanceType == "" {
		opts.LoadBalanceType = LoadBalanceWR
	}
	name, ok := loadBalanceMap[opts.LoadBalanceType]
	if !ok {
		// May fallback to the original name defined in polaris-go.
		name = opts.LoadBalanceType
	}
	destMeta := getDestMetadata(opts)
	var hashKey []byte
	if opts.Key != "" {
		hashKey = []byte(opts.Key)
	}
	canary := getCanaryValue(opts)
	resp, err := s.consumer.GetOneInstance(&api.GetOneInstanceRequest{
		GetOneInstanceRequest: model.GetOneInstanceRequest{
			Service:        serviceName,
			Namespace:      namespace,
			SourceService:  sourceService,
			Metadata:       destMeta,
			LbPolicy:       name,
			ReplicateCount: opts.Replicas,
			Canary:         canary,
			HashKey:        hashKey,
		},
	})
	var (
		inst      model.Instance
		allowOpen bool
	)
	switch {
	case err != nil && isInstanceNotFound(err), err == nil && (resp == nil || len(resp.Instances) == 0):
		inst, allowOpen, err = s.selectInstanceCandidate(name, serviceName, namespace, sourceService,
			destMeta, canary, hashKey, opts.Replicas)
		if err != nil {
			return nil, fmt.Errorf("select instance candidate err: %w", err)
		}
	case err != nil:
		return nil, fmt.Errorf("get one instance err: %s", err.Error())
	default:
		inst = resp.Instances[0]
		allowOpen = resp.Cluster != nil && resp.Cluster.HasLimitedInstances
	}

	serviceResource, err := polaris.InstanceServiceResource(inst, callerService)
	if err != nil {
		return nil, fmt.Errorf("build service circuit breaker resource err: %w", err)
	}
	methodResource, err := polaris.MethodResource(
		serviceResource.GetService(), callerService, inst.GetProtocol(),
		requestHTTPMethod(opts), requestMethodPath(opts))
	if err != nil {
		return nil, fmt.Errorf("build method circuit breaker resource err: %w", err)
	}
	instanceAvailable, err := polaris.InstancePermissionAvailable(
		s.consumer.SDKContext(), inst, allowOpen, callerService)
	if err != nil {
		return nil, fmt.Errorf("check instance circuit breaker err: %w", err)
	}
	if !instanceAvailable {
		inst, allowOpen, err = s.selectAvailableInstance(name, serviceName, namespace, sourceService,
			callerService, destMeta, canary, hashKey, opts.Replicas, inst)
		if err != nil {
			return nil, fmt.Errorf("select available instance err: %w", err)
		}
		fallbackMethodResource, err := polaris.MethodResource(
			serviceResource.GetService(), callerService, inst.GetProtocol(),
			requestHTTPMethod(opts), requestMethodPath(opts))
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
	if !polaris.IsRecoverAllOpenInstance(inst, allowOpen) {
		resources = append(resources, instanceResource)
	}
	permission, err := polaris.AcquireResourcePermissions(s.consumer.SDKContext(), resources...)
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
			return nil, fmt.Errorf("service %s/%s denied by circuit breaker", namespace, serviceName)
		}
	}
	var setName, containerName string
	if inst.GetMetadata() != nil {
		containerName = inst.GetMetadata()[containerKey]
		if enable := inst.GetMetadata()[setEnableKey]; enable == setEnableValue {
			setName = inst.GetMetadata()[setNameKey]
		}
	}
	return &registry.Node{
		ContainerName: containerName,
		SetName:       setName,
		Address:       net.JoinHostPort(inst.GetHost(), strconv.Itoa(int(inst.GetPort()))),
		ServiceName:   serviceName,
		Weight:        inst.GetWeight(),
		Metadata: map[string]interface{}{
			"instance":                        inst,
			"service":                         serviceName,
			"namespace":                       namespace,
			polaris.SourceServiceMetadataKey:  sourceService,
			polaris.CallerServiceMetadataKey:  callerService,
			polaris.MethodResourceMetadataKey: methodResource,
			polaris.AllowOpenMetadataKey:      allowOpen,
		},
	}, nil
}

func (s *Selector) selectAvailableInstance(
	lbName string,
	serviceName string,
	namespace string,
	sourceService *model.ServiceInfo,
	caller *model.ServiceKey,
	metadata map[string]string,
	canary string,
	hashKey []byte,
	replicateCount int,
	excluded model.Instance,
) (model.Instance, bool, error) {
	resp, err := s.consumer.GetInstances(&api.GetInstancesRequest{
		GetInstancesRequest: model.GetInstancesRequest{
			Service:       serviceName,
			Namespace:     namespace,
			SourceService: sourceService,
			Metadata:      metadata,
			Canary:        canary,
		},
	})
	if err != nil {
		return nil, false, err
	}
	if resp == nil || resp.Cluster == nil {
		return nil, false, fmt.Errorf("get instances return empty cluster")
	}
	allowOpen := resp.Cluster.HasLimitedInstances

	sdkCtx := s.consumer.SDKContext()
	plugin, err := sdkCtx.GetPlugins().GetPlugin(common.TypeLoadBalancer, lbName)
	if err != nil {
		return nil, false, err
	}
	lb, ok := plugin.(loadbalancer.LoadBalancer)
	if !ok {
		return nil, false, fmt.Errorf("plugin %s is not a load balancer", lbName)
	}
	if proxy, ok := lb.(*loadbalancer.Proxy); ok {
		lb = proxy.LoadBalancer
	}

	criteria := &loadbalancer.Criteria{
		HashKey: hashKey,
		ReplicateInfo: loadbalancer.ReplicateInfo{
			Count: replicateCount,
		},
	}
	inst, err := polaris.ChoosePermittedInstance(
		sdkCtx, lb, resp.Cluster, resp, *criteria, caller, excluded)
	if err != nil {
		return nil, false, err
	}
	return inst, allowOpen, nil
}

func (s *Selector) selectInstanceCandidate(
	lbName string,
	serviceName string,
	namespace string,
	sourceService *model.ServiceInfo,
	metadata map[string]string,
	canary string,
	hashKey []byte,
	replicateCount int,
) (model.Instance, bool, error) {
	resp, err := s.consumer.GetInstances(&api.GetInstancesRequest{
		GetInstancesRequest: model.GetInstancesRequest{
			Service:       serviceName,
			Namespace:     namespace,
			SourceService: sourceService,
			Metadata:      metadata,
			Canary:        canary,
		},
	})
	if err != nil {
		return nil, false, err
	}
	if resp == nil || resp.Cluster == nil {
		return nil, false, fmt.Errorf("get instances return empty cluster")
	}
	allowOpen := resp.Cluster.HasLimitedInstances

	sdkCtx := s.consumer.SDKContext()
	plugin, err := sdkCtx.GetPlugins().GetPlugin(common.TypeLoadBalancer, lbName)
	if err != nil {
		return nil, false, err
	}
	lb, ok := plugin.(loadbalancer.LoadBalancer)
	if !ok {
		return nil, false, fmt.Errorf("plugin %s is not a load balancer", lbName)
	}
	if proxy, ok := lb.(*loadbalancer.Proxy); ok {
		lb = proxy.LoadBalancer
	}

	criteria := &loadbalancer.Criteria{
		HashKey: hashKey,
		ReplicateInfo: loadbalancer.ReplicateInfo{
			Count: replicateCount,
		},
	}
	inst, err := polaris.ChooseInstanceCandidate(sdkCtx, lb, resp.Cluster, resp, *criteria)
	if err != nil {
		return nil, false, err
	}
	return inst, allowOpen, nil
}

// GetConsumer gets the consumerAPI instance of the selector.
func (s *Selector) GetConsumer() api.ConsumerAPI {
	return s.consumer
}

// GetCfg gets selector Config configuration.
func (s *Selector) GetCfg() *Config {
	return s.cfg
}

// Report reports the service status.
func (s *Selector) Report(node *registry.Node, cost time.Duration, err error) error {
	return circuitbreaker.Report(s.consumer, node, s.cfg.ReportTimeout, cost, err)
}

func getCanaryValue(opts *selector.Options) string {
	if opts.Ctx == nil {
		return ""
	}
	ctx := opts.Ctx
	msg := codec.Message(ctx)
	metaData := msg.ClientMetaData()
	if metaData == nil {
		return ""
	}
	return string(metaData[servicerouter.CanaryKey])
}

func setTransSelectorMeta(opts *selector.Options, selectorMeta map[string]string) {
	if opts.Ctx == nil {
		return
	}
	msg := codec.Message(opts.Ctx)
	for k, v := range msg.ServerMetaData() {
		if strings.HasPrefix(k, selectorMetaPrefix) {
			trimmedKey := strings.TrimPrefix(k, selectorMetaPrefix)
			selectorMeta[trimmedKey] = string(v)
		}
	}
}
