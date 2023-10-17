//
//
// Tencent is pleased to support the open source community by making tRPC available.
//
// Copyright (C) 2023 THL A29 Limited, a Tencent company.
// All rights reserved.
//
// If you have downloaded a copy of the tRPC source code from Tencent,
// please note that tRPC source code is licensed under the Apache 2.0 License,
// A copy of the Apache 2.0 License is included in this file.
//
//

package registry

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"trpc.group/trpc-go/trpc-go/log"
	"trpc.group/trpc-go/trpc-go/naming/registry"
	"trpc.group/trpc-go/trpc-go/plugin"

	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/config"
	plog "github.com/polarismesh/polaris-go/pkg/log"
	"github.com/polarismesh/polaris-go/pkg/model"
)

const (
	defaultConnectTimeout = time.Second
	defaultMessageTimeout = time.Second
	defaultProtocol       = "grpc"
)

// FactoryConfig is factory configuration.
type FactoryConfig struct {
	EnableRegister     bool            `yaml:"register_self"`
	Protocol           string          `yaml:"protocol"`
	HeartbeatInterval  int             `yaml:"heartbeat_interval"`
	Services           []Service       `yaml:"service"`
	Debug              bool            `yaml:"debug"`
	AddressList        string          `yaml:"address_list"`
	ClusterService     ClusterService  `yaml:"cluster_service"`
	ConnectTimeout     int             `yaml:"connect_timeout"`
	MessageTimeout     *time.Duration  `yaml:"message_timeout"`
	DisableHealthCheck bool            `yaml:"disable_health_check"`
	InstanceLocation   *model.Location `yaml:"instance_location"`
}

// ClusterService is cluster service.
type ClusterService struct {
	Discover    string `yaml:"discover"`
	HealthCheck string `yaml:"health_check"`
	Monitor     string `yaml:"monitor"`
}

// Service is service configuration.
type Service struct {
	Namespace   string            `yaml:"namespace"`
	ServiceName string            `yaml:"name"`
	Token       string            `yaml:"token"`
	InstanceID  string            `yaml:"instance_id"`
	Weight      *int              `yaml:"weight"`
	BindAddress string            `yaml:"bind_address"`
	MetaData    map[string]string `yaml:"metadata"`
}

func init() {
	plugin.Register("polarismesh", &RegistryFactory{})
}

// RegistryFactory is registered factory.
type RegistryFactory struct {
	sdkCtx api.SDKContext
}

// Type returns registration type.
func (f *RegistryFactory) Type() string {
	return "registry"
}

// Setup starts loading configuration and registers log.
func (f *RegistryFactory) Setup(name string, configDec plugin.Decoder) error {
	if configDec == nil {
		return errors.New("registry config decoder empty")
	}
	conf := &FactoryConfig{}
	if err := configDec.Decode(conf); err != nil {
		return err
	}
	if conf.Debug {
		log.Debug("set polaris mesh log level debug")
		plog.GetBaseLogger().SetLogLevel(plog.DebugLog)
	}
	sdkCtx, err := newSDKCtx(conf)
	if err != nil {
		return fmt.Errorf("create new provider failed: err %w", err)
	}
	f.sdkCtx = sdkCtx
	return register(api.NewProviderAPIByContext(sdkCtx), conf)
}

// FlexDependsOn makes sure that register is initialized after selector,
// which may set some global status of SDK, such as log directories.
func (f *RegistryFactory) FlexDependsOn() []string {
	return []string{"selector-polarismesh"}
}

// GetSDKCtx returns the stored sdk context.
func (f *RegistryFactory) GetSDKCtx() api.SDKContext {
	return f.sdkCtx
}

func newSDKCtx(cfg *FactoryConfig) (api.SDKContext, error) {
	var c *config.ConfigurationImpl
	if len(cfg.AddressList) > 0 {
		addressList := strings.Split(cfg.AddressList, ",")
		c = config.NewDefaultConfiguration(addressList)
	} else {
		c = config.NewDefaultConfigurationWithDomain()
	}
	// Config cluster
	if cfg.ClusterService.Discover != "" {
		c.Global.GetSystem().GetDiscoverCluster().SetService(cfg.ClusterService.Discover)
	}
	if cfg.ClusterService.HealthCheck != "" {
		c.Global.GetSystem().GetHealthCheckCluster().SetService(cfg.ClusterService.HealthCheck)
	}
	if cfg.ClusterService.Monitor != "" {
		c.Global.GetSystem().GetMonitorCluster().SetService(cfg.ClusterService.Monitor)
	}
	if cfg.Protocol == "" {
		cfg.Protocol = defaultProtocol
	}
	c.Global.ServerConnector.Protocol = cfg.Protocol
	if cfg.ConnectTimeout != 0 {
		c.GetGlobal().GetServerConnector().SetConnectTimeout(time.Duration(cfg.ConnectTimeout) * time.Millisecond)
	} else {
		c.GetGlobal().GetServerConnector().SetConnectTimeout(defaultConnectTimeout)
	}
	// Set message timeout.
	messageTimeout := defaultMessageTimeout
	if cfg.MessageTimeout != nil {
		messageTimeout = *cfg.MessageTimeout
	}
	c.GetGlobal().GetServerConnector().SetMessageTimeout(messageTimeout)
	return api.InitContextByConfig(c)
}

func register(provider api.ProviderAPI, conf *FactoryConfig) error {
	for _, service := range conf.Services {
		cfg := &Config{
			Protocol:           conf.Protocol,
			EnableRegister:     conf.EnableRegister,
			HeartBeat:          conf.HeartbeatInterval / 1000,
			ServiceName:        service.ServiceName,
			Namespace:          service.Namespace,
			ServiceToken:       service.Token,
			InstanceID:         service.InstanceID,
			Metadata:           service.MetaData,
			BindAddress:        service.BindAddress,
			Weight:             service.Weight,
			DisableHealthCheck: conf.DisableHealthCheck,
			InstanceLocation:   conf.InstanceLocation,
		}
		reg, err := newRegistry(provider, cfg)
		if err != nil {
			return fmt.Errorf("create new registry for service %s failed: err %w", service.ServiceName, err)
		}
		registry.Register(service.ServiceName, reg)
	}
	return nil
}
