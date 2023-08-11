// Tencent is pleased to support the open source community by making tRPC available.
// Copyright (C) 2023 THL A29 Limited, a Tencent company. All rights reserved.
// If you have downloaded a copy of the tRPC source code from Tencent,
// please note that tRPC source code is licensed under the Apache 2.0 License,
// A copy of the Apache 2.0 License is included in this file.

// Package selector is a selector.
package selector

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"trpc.group/trpc-go/trpc-go/codec"
	"trpc.group/trpc-go/trpc-go/log"
	"trpc.group/trpc-go/trpc-go/naming/registry"
	"trpc.group/trpc-go/trpc-go/naming/selector"
	"trpc.group/trpc-go/trpc-naming-polaris/circuitbreaker"
	"trpc.group/trpc-go/trpc-naming-polaris/servicerouter"

	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/config"
	"github.com/polarismesh/polaris-go/pkg/model"
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
	const defaultName = "polaris"
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
		selector.Register("polaris", s)
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
	// the request cannot be passed to Polaris for meta matching,
	// agree on the transparent transmission field with the prefix 'selector-meta-',
	// remove the prefix and fill in meta, and use it for Polaris matching.
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

func getDestMetadata(opts *selector.Options) map[string]string {
	destMeta := make(map[string]string)
	// Support environment selection when service routing is not enabled.
	if opts.DisableServiceRouter {
		if len(opts.DestinationEnvName) > 0 {
			destMeta["env"] = opts.DestinationEnvName
		}
	}
	// Support custom metadata tag key passed to polaris for addressing.
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
	log.Tracef("[NAMING-POLARIS] select options: %+v", opts)

	namespace := opts.Namespace
	var sourceService *model.ServiceInfo

	if s.cfg.Enable {
		sourceService = extractSourceServiceRequestInfo(opts, s.cfg.EnableTransMeta)
	}
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
	resp, err := s.consumer.GetOneInstance(&api.GetOneInstanceRequest{
		GetOneInstanceRequest: model.GetOneInstanceRequest{
			Service:        serviceName,
			Namespace:      namespace,
			SourceService:  sourceService,
			Metadata:       destMeta,
			LbPolicy:       name,
			ReplicateCount: opts.Replicas,
			Canary:         getCanaryValue(opts),
			HashKey:        hashKey,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get one instance err: %s", err.Error())
	}
	if len(resp.Instances) == 0 {
		return nil, fmt.Errorf("get one instance return empty")
	}
	inst := resp.Instances[0]
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
			"instance":  inst,
			"service":   serviceName,
			"namespace": namespace,
		},
	}, nil
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
