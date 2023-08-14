// Tencent is pleased to support the open source community by making tRPC available.
// Copyright (C) 2023 THL A29 Limited, a Tencent company. All rights reserved.
// If you have downloaded a copy of the tRPC source code from Tencent,
// please note that tRPC source code is licensed under the Apache 2.0 License,
// A copy of the Apache 2.0 License is included in this file.

// Package naming is a naming configuration.
package naming

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"trpc.group/trpc-go/trpc-go/plugin"
	"trpc.group/trpc-go/trpc-naming-polarismesh/circuitbreaker"
	"trpc.group/trpc-go/trpc-naming-polarismesh/discovery"
	"trpc.group/trpc-go/trpc-naming-polarismesh/loadbalance"
	_ "trpc.group/trpc-go/trpc-naming-polarismesh/registry" // 初始化注册模块
	"trpc.group/trpc-go/trpc-naming-polarismesh/selector"
	"trpc.group/trpc-go/trpc-naming-polarismesh/servicerouter"

	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/config"
	plog "github.com/polarismesh/polaris-go/pkg/log"
	"github.com/polarismesh/polaris-go/pkg/model"
	"github.com/polarismesh/polaris-go/plugin/location"
	"gopkg.in/yaml.v3"
)

func init() {
	plugin.Register("polarismesh", &SelectorFactory{})
}

// Config framework configuration.
type Config struct {
	Name                string               `yaml:"-"` // Name is the current name of plugin.
	Debug               bool                 `yaml:"debug"`
	Default             *bool                `yaml:"default"`
	Protocol            string               `yaml:"protocol"`
	ReportTimeout       *time.Duration       `yaml:"report_timeout"`
	EnableServiceRouter *bool                `yaml:"enable_servicerouter"`
	EnableCanary        *bool                `yaml:"enable_canary"`
	PersistDir          *string              `yaml:"persistDir"`
	ServiceExpireTime   *time.Duration       `yaml:"service_expire_time"`
	LogDir              *string              `yaml:"log_dir"`
	Logs                *Logs                `yaml:"logs"`
	Timeout             int                  `yaml:"timeout"`
	ConnectTimeout      int                  `yaml:"connect_timeout"`
	MessageTimeout      *time.Duration       `yaml:"message_timeout"`
	AddressList         string               `yaml:"address_list"`
	Discovery           DiscoveryConfig      `yaml:"discovery"`
	Loadbalance         LoadbalanceConfig    `yaml:"loadbalance"`
	CircuitBreaker      CircuitBreakerConfig `yaml:"circuitbreaker"`
	ServiceRouter       ServiceRouterConfig  `yaml:"service_router"`
	ClusterService      ClusterService       `yaml:"cluster_service"`
	EnableTransMeta     bool                 `yaml:"enable_trans_meta"`
	BindIP              string               `yaml:"bind_ip"`
	InstanceLocation    *model.Location      `yaml:"instance_location"`
	PolarisConfig       config.Configuration
}

// UnmarshalYAML is the customized unmarshal function to ensure the default value of the config.
func (c *Config) UnmarshalYAML(value *yaml.Node) error {
	type plain Config
	if err := value.Decode((*plain)(c)); err != nil {
		return err
	}
	if c.LogDir != nil {
		c.Logs = &Logs{
			DirPath:    *c.LogDir,
			Level:      "default",
			MaxBackups: plog.DefaultRotationMaxBackups,
			MaxSize:    plog.DefaultRotationMaxSize,
		}
	}
	if c.Logs != nil {
		if c.Logs.MaxBackups == 0 {
			c.Logs.MaxBackups = plog.DefaultRotationMaxBackups
		}
		if c.Logs.MaxSize == 0 {
			c.Logs.MaxSize = plog.DefaultRotationMaxSize
		}
		if c.Logs.DirPath == "" {
			c.Logs.DirPath = plog.DefaultLogRotationRootDir
		}
	}
	return nil
}

// Logs log configuration.
type Logs struct {
	DirPath    string `yaml:"dir_path"`
	Level      string `yaml:"level"`
	MaxBackups int    `yaml:"max_backups"`
	MaxSize    int    `yaml:"max_size"`
}

// ServiceRouterConfig service routing configuration.
type ServiceRouterConfig struct {
	// NearbyMatchLevel is the minimum matching level of the nearest route,
	// including region (big area),
	// zone (area), campus (campus), and the default is zone.
	NearbyMatchLevel string `yaml:"nearby_matchlevel"`
	// PercentOfMinInstances # is the minimum healthy instance judgment threshold of all dead and all alive,
	// the value range is between [0,1], and the default is 0.
	// which means, only when all instances are unhealthy, all dead and all alive will be enabled.
	PercentOfMinInstances float64 `yaml:"percent_of_min_instances"`
	// NeedReturnAllNodes indicates whether to expand all nodes into registry.Node return.
	NeedReturnAllNodes bool `yaml:"need_return_all_nodes"`
}

// DiscoveryConfig configuration.
type DiscoveryConfig struct {
	RefreshInterval int `yaml:"refresh_interval"`
}

// LoadbalanceConfig loads balancing configuration.
type LoadbalanceConfig struct {
	Name []string `yaml:"name"` // load balancing type.
	// Detailed configuration of each load balancing strategy.
	Details map[string]yaml.Node `yaml:"details"`
}

// CircuitBreakerConfig circuit breaker configuration.
type CircuitBreakerConfig struct {
	CheckPeriod               *time.Duration `yaml:"checkPeriod"`
	RequestCountAfterHalfOpen *int           `yaml:"requestCountAfterHalfOpen"`
	SleepWindow               *time.Duration `yaml:"sleepWindow"`
	SuccessCountAfterHalfOpen *int           `yaml:"successCountAfterHalfOpen"`
	Chain                     []string       `yaml:"chain"`
	ErrorCount                *struct {
		ContinuousErrorThreshold *int           `yaml:"continuousErrorThreshold"`
		MetricNumBuckets         *int           `yaml:"metricNumBuckets"`
		MetricStatTimeWindow     *time.Duration `yaml:"metricStatTimeWindow"`
	} `yaml:"errorCount"`
	ErrorRate *struct {
		ErrorRateThreshold     *float64       `yaml:"errorRateThreshold"`
		MetricNumBuckets       *int           `yaml:"metricNumBuckets"`
		MetricStatTimeWindow   *time.Duration `yaml:"metricStatTimeWindow"`
		RequestVolumeThreshold *int           `yaml:"requestVolumeThreshold"`
	} `yaml:"errorRate"`
}

// ClusterService cluster service.
type ClusterService struct {
	Discover    string `yaml:"discover"`
	HealthCheck string `yaml:"health_check"`
	Monitor     string `yaml:"monitor"`
}

// SelectorFactory implements the name service plugin for trpc.
type SelectorFactory struct {
	sdkCtx api.SDKContext
}

// Type plugin type.
func (f *SelectorFactory) Type() string {
	return "selector"
}

func (c *Config) getSetDefault() bool {
	setDefault := true
	if c.Default != nil {
		setDefault = *c.Default
	}
	return setDefault
}

func (c *Config) getEnableCanary() bool {
	var isEnable bool
	if c.EnableCanary != nil {
		isEnable = *c.EnableCanary
	}
	return isEnable
}

func (c *Config) getEnableServiceRouter() bool {
	isEnable := true
	if c.EnableServiceRouter != nil {
		isEnable = *c.EnableServiceRouter
	}
	return isEnable
}

func (c *Config) setLog() {
	if l := c.Logs; l != nil {
		newLogOptions := func(path string) *plog.Options {
			o := plog.CreateDefaultLoggerOptions(filepath.Join(l.DirPath, path), getLogLevel(l.Level))
			o.RotationMaxBackups = l.MaxBackups
			o.RotationMaxSize = l.MaxSize
			return o
		}
		_ = plog.ConfigBaseLogger(plog.DefaultLogger, newLogOptions(plog.DefaultBaseLogRotationPath))
		_ = plog.ConfigStatLogger(plog.DefaultLogger, newLogOptions(plog.DefaultStatLogRotationPath))
		_ = plog.ConfigDetectLogger(plog.DefaultLogger, newLogOptions(plog.DefaultDetectLogRotationPath))
		_ = plog.ConfigStatReportLogger(plog.DefaultLogger, newLogOptions(plog.DefaultStatReportLogRotationPath))
		_ = plog.ConfigNetworkLogger(plog.DefaultLogger, newLogOptions(plog.DefaultNetworkLogRotationPath))
	}
	if c.Debug {
		plog.GetBaseLogger().SetLogLevel(plog.DebugLog)
	}
}

// Setup initialization.
func (f *SelectorFactory) Setup(name string, dec plugin.Decoder) error {
	if dec == nil {
		return errors.New("selector config decoder empty")
	}
	conf := &Config{Name: name}
	if err := dec.Decode(conf); err != nil {
		return err
	}
	sdkCtx, err := setupWithConfig(conf)
	f.sdkCtx = sdkCtx
	return err
}

// GetSDKCtx returns the stored sdk context.
func (f *SelectorFactory) GetSDKCtx() api.SDKContext {
	return f.sdkCtx
}

// SetupWithConfig executes setup using the given config.
// Remember to give a proper config name to conf.Name.
func SetupWithConfig(conf *Config) error {
	_, err := setupWithConfig(conf)
	return err
}

func setupWithConfig(conf *Config) (api.SDKContext, error) {
	// 如果没设置协议默认使用 grpc 协议
	if len(conf.Protocol) == 0 {
		conf.Protocol = "grpc"
	}

	// Initialization log.
	conf.setLog()
	sdkCtx, err := newSDKContext(conf)
	if err != nil {
		return nil, fmt.Errorf("new sdk ctx err: %w", err)
	}
	return sdkCtx, setupComponents(sdkCtx, conf)
}

func setupComponents(sdkCtx api.SDKContext, conf *Config) error {
	setDefault := conf.getSetDefault()
	enableServiceRouter := conf.getEnableServiceRouter()
	enableCanary := conf.getEnableCanary()
	if err := discovery.Setup(sdkCtx, &discovery.Config{Name: conf.Name}, setDefault); err != nil {
		return err
	}

	// Initialize service routing.
	if err := servicerouter.Setup(
		sdkCtx,
		&servicerouter.Config{
			Name:               conf.Name,
			Enable:             enableServiceRouter,
			EnableCanary:       enableCanary,
			NeedReturnAllNodes: conf.ServiceRouter.NeedReturnAllNodes,
		},
		setDefault,
	); err != nil {
		return err
	}
	if err := setupLoadbalance(sdkCtx, conf, setDefault); err != nil {
		return err
	}
	if err := circuitbreaker.Setup(
		sdkCtx,
		&circuitbreaker.Config{
			Name:          conf.Name,
			ReportTimeout: conf.ReportTimeout,
		},
		setDefault,
	); err != nil {
		return err
	}
	if err := selector.Setup(sdkCtx,
		&selector.Config{
			Name:            conf.Name,
			Enable:          enableServiceRouter,
			EnableCanary:    enableCanary,
			ReportTimeout:   conf.ReportTimeout,
			EnableTransMeta: conf.EnableTransMeta,
		}); err != nil {
		return err
	}
	return nil
}

func setupLoadbalance(sdkCtx api.SDKContext, conf *Config, setDefault bool) error {
	if len(conf.Loadbalance.Name) == 0 {
		conf.Loadbalance.Name = append(
			conf.Loadbalance.Name,
			loadbalance.LoadBalancerWR,
			loadbalance.LoadBalancerHash,
			loadbalance.LoadBalancerRingHash,
			loadbalance.LoadBalancerL5CST,
			loadbalance.LoadBalancerMaglev,
		)
	}
	for index, balanceType := range conf.Loadbalance.Name {
		// Under the premise that polaris mesh is set as the addressing method by default,
		// the first load balancing method is set as the default load balancing method.
		isDefault := setDefault && index == 0
		if err := loadbalance.Setup(sdkCtx, balanceType, isDefault); err != nil {
			return err
		}
	}
	return nil
}

func setSdkCircuitBreaker(c config.Configuration, cfg *Config) {
	if len(cfg.CircuitBreaker.Chain) > 0 {
		c.GetConsumer().GetCircuitBreaker().SetChain(cfg.CircuitBreaker.Chain)
	}
	if cfg.CircuitBreaker.CheckPeriod != nil {
		c.GetConsumer().GetCircuitBreaker().SetCheckPeriod(*cfg.CircuitBreaker.CheckPeriod)
	}
	if cfg.CircuitBreaker.RequestCountAfterHalfOpen != nil {
		c.GetConsumer().GetCircuitBreaker().SetRequestCountAfterHalfOpen(*cfg.CircuitBreaker.RequestCountAfterHalfOpen)
	}
	if cfg.CircuitBreaker.SleepWindow != nil {
		c.GetConsumer().GetCircuitBreaker().SetSleepWindow(*cfg.CircuitBreaker.SleepWindow)
	}
	if cfg.CircuitBreaker.SuccessCountAfterHalfOpen != nil {
		c.GetConsumer().GetCircuitBreaker().SetSuccessCountAfterHalfOpen(*cfg.CircuitBreaker.SuccessCountAfterHalfOpen)
	}
	setErrorCount(c, cfg)
	setErrorRate(c, cfg)
}

func setErrorCount(c config.Configuration, cfg *Config) {
	if cfg.CircuitBreaker.ErrorCount == nil {
		return
	}
	errorCount := cfg.CircuitBreaker.ErrorCount
	if errorCount.ContinuousErrorThreshold != nil {
		c.GetConsumer().GetCircuitBreaker().GetErrorCountConfig().
			SetContinuousErrorThreshold(*errorCount.ContinuousErrorThreshold)
	}
	if errorCount.MetricNumBuckets != nil {
		c.GetConsumer().GetCircuitBreaker().GetErrorCountConfig().SetMetricNumBuckets(*errorCount.MetricNumBuckets)
	}
	if errorCount.MetricStatTimeWindow != nil {
		c.GetConsumer().GetCircuitBreaker().GetErrorCountConfig().SetMetricStatTimeWindow(*errorCount.MetricStatTimeWindow)
	}
}

func setErrorRate(c config.Configuration, cfg *Config) {
	if cfg.CircuitBreaker.ErrorRate == nil {
		return
	}
	errorRate := cfg.CircuitBreaker.ErrorRate
	if errorRate.ErrorRateThreshold != nil {
		c.GetConsumer().GetCircuitBreaker().GetErrorRateConfig().SetErrorRatePercent(int(*errorRate.ErrorRateThreshold * 100))
	}
	if errorRate.MetricNumBuckets != nil {
		c.GetConsumer().GetCircuitBreaker().GetErrorRateConfig().SetMetricNumBuckets(*errorRate.MetricNumBuckets)
	}
	if errorRate.RequestVolumeThreshold != nil {
		c.GetConsumer().GetCircuitBreaker().GetErrorRateConfig().SetRequestVolumeThreshold(*errorRate.RequestVolumeThreshold)
	}
	if errorRate.MetricStatTimeWindow != nil {
		c.GetConsumer().GetCircuitBreaker().GetErrorRateConfig().SetMetricStatTimeWindow(*errorRate.MetricStatTimeWindow)
	}
}

func setSdkProperty(c config.Configuration, cfg *Config) {
	if cfg.Timeout != 0 {
		timeout := time.Duration(cfg.Timeout) * time.Millisecond
		c.GetGlobal().GetAPI().SetTimeout(timeout)
		// If a timeout is set, the maximum number of retries needs to be set to 0.
		c.GetGlobal().GetAPI().SetMaxRetryTimes(0)
	}
	if cfg.Discovery.RefreshInterval != 0 {
		refreshInterval := time.Duration(cfg.Discovery.RefreshInterval) * time.Millisecond
		c.GetConsumer().GetLocalCache().SetServiceRefreshInterval(refreshInterval)
	}
	//Set the service cache as a persistent directory.
	if cfg.PersistDir != nil {
		c.GetConsumer().GetLocalCache().SetPersistDir(*cfg.PersistDir)
	}
	// Set the sdk cache retention time.
	if cfg.ServiceExpireTime != nil {
		c.GetConsumer().GetLocalCache().SetServiceExpireTime(*cfg.ServiceExpireTime)
	}
	if cfg.ClusterService.Discover != "" {
		c.GetGlobal().GetSystem().GetDiscoverCluster().SetService(cfg.ClusterService.Discover)
	}
	if cfg.ClusterService.HealthCheck != "" {
		c.GetGlobal().GetSystem().GetHealthCheckCluster().SetService(cfg.ClusterService.HealthCheck)
	}
	if cfg.ClusterService.Monitor != "" {
		c.GetGlobal().GetSystem().GetMonitorCluster().SetService(cfg.ClusterService.Monitor)
	}
	// Set service routing.
	if cfg.ServiceRouter.NearbyMatchLevel != "" {
		c.GetConsumer().GetServiceRouter().GetNearbyConfig().SetMatchLevel(cfg.ServiceRouter.NearbyMatchLevel)
	}
	c.GetConsumer().GetServiceRouter().SetPercentOfMinInstances(cfg.ServiceRouter.PercentOfMinInstances)
}

func setLocation(c config.Configuration, cfg *Config) {
	il := cfg.InstanceLocation
	if il != nil && il.Region != "" && il.Zone != "" && il.Campus != "" {
		// It seems that the location API of polaris-go is broken.
		// I have no choice but to write the following codes.
		l := c.GetGlobal().GetLocation().(*config.LocationConfigImpl)
		l.Providers = []*config.LocationProviderConfigImpl{
			{Type: location.Local, Options: map[string]interface{}{
				"region": il.Region,
				"zone":   il.Zone,
				"campus": il.Campus,
			}},
		}
	}
}

func newSDKContext(cfg *Config) (api.SDKContext, error) {
	var c config.Configuration
	if cfg.PolarisConfig != nil { // Specific polaris mesh config
		c = cfg.PolarisConfig
	} else { // Default polairs config
		c = api.NewConfiguration()
	}

	cfg.AddressList = strings.TrimSpace(cfg.AddressList)
	if len(cfg.AddressList) > 0 {
		c.GetGlobal().GetServerConnector().SetAddresses(strings.Split(cfg.AddressList, ","))
	}

	// Set local IP.
	if cfg.BindIP != "" {
		c.GetGlobal().GetAPI().SetBindIP(cfg.BindIP)
	}

	c.GetGlobal().GetServerConnector().SetProtocol(cfg.Protocol)
	connectTimeout := selector.DefaultConnectTimeout
	if cfg.ConnectTimeout != 0 {
		connectTimeout = time.Millisecond * time.Duration(cfg.ConnectTimeout)
	}
	c.GetGlobal().GetServerConnector().SetConnectTimeout(connectTimeout)
	messageTimeout := selector.DefaultMessageTimeout
	if cfg.MessageTimeout != nil {
		messageTimeout = *cfg.MessageTimeout
	}

	var routers []string

	// Add a plugin to filter according to the called service env.
	routers = append(routers, config.DefaultServiceRouterDstMeta)
	for _, router := range c.GetConsumer().GetServiceRouter().GetChain() {
		routers = append(routers, router)
	}

	// Add canary routing chain.
	if cfg.EnableCanary != nil && *cfg.EnableCanary {
		routers = append(routers, config.DefaultServiceRouterCanary)
	}

	c.GetConsumer().GetServiceRouter().SetChain(routers)

	confs, err := loadbalance.AsPluginCfgs(cfg.Loadbalance.Details)
	if err != nil {
		return nil, fmt.Errorf("failed to parse yaml loadbalance details: %w", err)
	}
	loadBalanceConfig := c.GetConsumer().GetLoadbalancer()
	for name, conf := range confs {
		if err := loadBalanceConfig.SetPluginConfig(name, conf); err != nil {
			return nil, fmt.Errorf("failed to set load balancer %s's config: %w", name, err)
		}
	}

	c.GetGlobal().GetServerConnector().SetMessageTimeout(messageTimeout)
	// Configure circuit breaker policy.
	setSdkCircuitBreaker(c, cfg)
	// Configure location properties.
	setLocation(c, cfg)
	// Configure other properties.
	setSdkProperty(c, cfg)
	sdkCtx, err := api.InitContextByConfig(c)
	if err != nil {
		return nil, err
	}
	return sdkCtx, nil
}

func getLogLevel(desc string) int {
	switch strings.ToLower(desc) {
	case "debug":
		return plog.DebugLog
	case "info":
		return plog.InfoLog
	case "warn":
		return plog.WarnLog
	case "error":
		return plog.ErrorLog
	case "fatal":
		return plog.FatalLog
	case "none":
		return plog.NoneLog
	case "default":
		fallthrough
	default:
		return plog.DefaultBaseLogLevel
	}
}
