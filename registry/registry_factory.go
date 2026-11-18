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

package registry

import (
	"bytes"
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
	"gopkg.in/yaml.v3"
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
	Namespace          string            `yaml:"namespace"`
	ServiceName        string            `yaml:"name"`
	Version            string            `yaml:"version"`
	InstanceProtocol   string            `yaml:"instance_protocol"`
	Token              string            `yaml:"token"`
	InstanceID         string            `yaml:"instance_id"`
	Weight             *int              `yaml:"weight"`
	BindAddress        string            `yaml:"bind_address"`
	PreferBindAddress  bool              `yaml:"prefer_bind_address"`
	TTL                int               `yaml:"ttl"`
	HeartbeatInterval  int               `yaml:"heartbeat_interval"`
	AutoHeartbeat      *bool             `yaml:"auto_heartbeat"`
	DisableHealthCheck *bool             `yaml:"disable_health_check"`
	InstanceLocation   *model.Location   `yaml:"instance_location"`
	MetaData           map[string]string `yaml:"metadata"`
	Endpoints          []EndpointConfig  `yaml:"endpoints"`
}

// UnmarshalYAML rejects unknown service fields instead of silently changing registration behavior.
func (s *Service) UnmarshalYAML(node *yaml.Node) error {
	type serviceFields Service
	raw, err := yaml.Marshal(node)
	if err != nil {
		return err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	var decoded serviceFields
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*s = Service(decoded)
	return nil
}

// EndpointConfig is one endpoint declaration in registry service YAML.
type EndpointConfig struct {
	Address            string            `yaml:"address"`
	Protocol           string            `yaml:"protocol"`
	Version            string            `yaml:"version"`
	Token              string            `yaml:"token"`
	InstanceID         string            `yaml:"instance_id"`
	Weight             *int              `yaml:"weight"`
	TTL                int               `yaml:"ttl"`
	HeartbeatInterval  int               `yaml:"heartbeat_interval"`
	AutoHeartbeat      *bool             `yaml:"auto_heartbeat"`
	DisableHealthCheck *bool             `yaml:"disable_health_check"`
	InstanceLocation   *model.Location   `yaml:"instance_location"`
	MetaData           map[string]string `yaml:"metadata"`
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
		if err := plog.GetBaseLogger().SetLogLevel(plog.DebugLog); err != nil {
			return fmt.Errorf("set polaris mesh log level debug: %w", err)
		}
	}
	sdkCtx, err := newSDKCtx(conf)
	if err != nil {
		return fmt.Errorf("create new provider failed: err %w", err)
	}
	f.sdkCtx = sdkCtx
	return register(api.NewProviderAPIByContext(sdkCtx), sdkCtx, conf)
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

func register(provider api.ProviderAPI, sdkCtx api.SDKContext, conf *FactoryConfig) error {
	type preparedService struct {
		name string
		cfg  *Config
	}
	prepared := make([]preparedService, 0, len(conf.Services))
	serviceNames := make(map[string]struct{}, len(conf.Services))
	for _, service := range conf.Services {
		if _, exists := serviceNames[service.ServiceName]; exists {
			return fmt.Errorf("duplicate registry service name %q", service.ServiceName)
		}
		serviceNames[service.ServiceName] = struct{}{}
		autoHeartbeat := service.AutoHeartbeat
		if autoHeartbeat == nil {
			disabled := false
			autoHeartbeat = &disabled
		}
		disableHealthCheck := conf.DisableHealthCheck
		if service.DisableHealthCheck != nil {
			disableHealthCheck = *service.DisableHealthCheck
		}
		heartbeatInterval := conf.HeartbeatInterval
		if service.HeartbeatInterval != 0 {
			heartbeatInterval = service.HeartbeatInterval
		}
		heartbeatSeconds, err := millisecondsToSeconds("heartbeat_interval", heartbeatInterval)
		if err != nil {
			return fmt.Errorf("create new registry for service %s failed: err %w", service.ServiceName, err)
		}
		instanceLocation := conf.InstanceLocation
		if service.InstanceLocation != nil {
			instanceLocation = service.InstanceLocation
		}
		cfg := &Config{
			Protocol:           conf.Protocol,
			EnableRegister:     conf.EnableRegister,
			HeartBeat:          heartbeatSeconds,
			ServiceName:        service.ServiceName,
			Version:            service.Version,
			InstanceProtocol:   service.InstanceProtocol,
			Namespace:          service.Namespace,
			ServiceToken:       service.Token,
			InstanceID:         service.InstanceID,
			Metadata:           service.MetaData,
			BindAddress:        service.BindAddress,
			PreferBindAddress:  service.PreferBindAddress,
			Weight:             service.Weight,
			TTL:                service.TTL,
			AutoHeartbeat:      *autoHeartbeat,
			DisableHealthCheck: disableHealthCheck,
			InstanceLocation:   instanceLocation,
		}
		endpoints, err := buildEndpoints(service, conf, *autoHeartbeat, disableHealthCheck, heartbeatInterval)
		if err != nil {
			return fmt.Errorf("create new registry for service %s failed: err %w", service.ServiceName, err)
		}
		cfg.Endpoints = endpoints
		preparedCfg, err := prepareConfig(cfg)
		if err != nil {
			return fmt.Errorf("create new registry for service %s failed: err %w", service.ServiceName, err)
		}
		prepared = append(prepared, preparedService{name: service.ServiceName, cfg: preparedCfg})
	}
	registries := make([]*Registry, 0, len(prepared))
	for _, service := range prepared {
		reg := newRegistryPrepared(provider, service.cfg)
		reg.sdkCtx = sdkCtx
		registries = append(registries, reg)
	}
	for i, service := range prepared {
		registry.Register(service.name, registries[i])
	}
	return nil
}

func buildEndpoints(
	service Service,
	conf *FactoryConfig,
	serviceAutoHeartbeat bool,
	serviceDisableHealthCheck bool,
	serviceHeartbeatInterval int,
) ([]Endpoint, error) {
	if len(service.Endpoints) == 0 {
		return nil, nil
	}
	endpoints := make([]Endpoint, 0, len(service.Endpoints))
	for i, endpoint := range service.Endpoints {
		protocol := endpoint.Protocol
		if protocol == "" {
			protocol = service.InstanceProtocol
		}
		version := endpoint.Version
		if version == "" {
			version = service.Version
		}
		token := endpoint.Token
		if token == "" {
			token = service.Token
		}
		instanceID := endpoint.InstanceID
		if instanceID == "" {
			instanceID = service.InstanceID
		}
		weight := endpoint.Weight
		if weight == nil {
			weight = service.Weight
		}
		ttl := endpoint.TTL
		if ttl == 0 {
			ttl = service.TTL
		}
		heartbeatInterval := endpoint.HeartbeatInterval
		if heartbeatInterval == 0 {
			heartbeatInterval = serviceHeartbeatInterval
		}
		heartbeatSeconds, err := millisecondsToSeconds(
			fmt.Sprintf("endpoint %d heartbeat_interval", i), heartbeatInterval)
		if err != nil {
			return nil, err
		}
		autoHeartbeat := serviceAutoHeartbeat
		if endpoint.AutoHeartbeat != nil {
			autoHeartbeat = *endpoint.AutoHeartbeat
		}
		disableHealthCheck := serviceDisableHealthCheck
		if endpoint.DisableHealthCheck != nil {
			disableHealthCheck = *endpoint.DisableHealthCheck
		}
		instanceLocation := service.InstanceLocation
		if endpoint.InstanceLocation != nil {
			instanceLocation = endpoint.InstanceLocation
		}
		if instanceLocation == nil {
			instanceLocation = conf.InstanceLocation
		}
		autoHeartbeatValue := autoHeartbeat
		disableHealthCheckValue := disableHealthCheck
		endpoints = append(endpoints, Endpoint{
			Address:            endpoint.Address,
			Protocol:           protocol,
			Version:            version,
			ServiceToken:       token,
			InstanceID:         instanceID,
			Weight:             weight,
			TTL:                ttl,
			HeartBeat:          heartbeatSeconds,
			AutoHeartbeat:      &autoHeartbeatValue,
			DisableHealthCheck: &disableHealthCheckValue,
			Metadata:           endpoint.MetaData,
			InstanceLocation:   instanceLocation,
		})
	}
	return endpoints, nil
}

func millisecondsToSeconds(name string, milliseconds int) (int, error) {
	if milliseconds < 0 {
		return 0, fmt.Errorf("%s must not be negative", name)
	}
	if milliseconds == 0 {
		return 0, nil
	}
	if milliseconds < 1000 || milliseconds%1000 != 0 {
		return 0, fmt.Errorf("%s must be a whole number of seconds in milliseconds", name)
	}
	return milliseconds / 1000, nil
}
