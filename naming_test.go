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

package naming

import (
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"trpc.group/trpc-go/trpc-go"
	"trpc.group/trpc-go/trpc-go/errs"
	tlb "trpc.group/trpc-go/trpc-go/naming/loadbalance"
	trpcregistry "trpc.group/trpc-go/trpc-go/naming/registry"
	adaptercb "trpc.group/trpc-go/trpc-naming-polarismesh/circuitbreaker"
	adapterlb "trpc.group/trpc-go/trpc-naming-polarismesh/loadbalance"
	_ "trpc.group/trpc-go/trpc-naming-polarismesh/registry"

	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/config"
	plog "github.com/polarismesh/polaris-go/pkg/log"
	"github.com/polarismesh/polaris-go/pkg/model"
	"github.com/polarismesh/polaris-go/pkg/model/local"
	"github.com/polarismesh/polaris-go/pkg/model/pb"
	polarisplugin "github.com/polarismesh/polaris-go/pkg/plugin"
	"github.com/polarismesh/polaris-go/pkg/plugin/common"
	"github.com/polarismesh/polaris-go/plugin/loadbalancer/ringhash"
	apiservice "github.com/polarismesh/specification/source/go/api/v1/service_manage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"gopkg.in/yaml.v3"
)

func TestLogPluginVersionOnce(t *testing.T) {
	var calls int
	logVersion := newVersionLogger(func() {
		calls++
	})

	logVersion()
	logVersion()
	require.Equal(t, 1, calls)
}

func TestSelectorFactory_Setup(t *testing.T) {
	cfgstr := `
plugins:
  selector:
    polarismesh:
      address_list: 127.0.0.1:0
      enable_servicerouter: true
      persistDir: /tmp/polarismesh/backup
      log_dir: /tmp/polarismesh/log
      debug: false
      discovery:
        refresh_interval: 10000
`
	cfg := trpc.Config{}
	err := yaml.Unmarshal([]byte(cfgstr), &cfg)
	assert.Nil(t, err)
	polarisCfg := cfg.Plugins["selector"]["polarismesh"]
	pluginFac := &SelectorFactory{}
	err = pluginFac.Setup("polarismesh", &polarisCfg)
	assert.Nil(t, err)
	assert.NotNil(t, pluginFac.GetSDKCtx())
}

func Test_SetupWithConfig(t *testing.T) {
	var (
		enableServiceRouter = true
		persisDir           = "/tmp/polarismesh/backup"
		logDir              = "/tmp/polarismesh/log"
	)
	require.Nil(t, SetupWithConfig(&Config{
		AddressList:         "127.0.0.1:0",
		EnableServiceRouter: &enableServiceRouter,
		PersistDir:          &persisDir,
		LogDir:              &logDir,
		Debug:               false,
		Discovery:           DiscoveryConfig{RefreshInterval: 10000},
	}))
}

func Test_SetupWithPolarisConfig(t *testing.T) {
	var (
		enableServiceRouter = true
		persisDir           = "/tmp/polarismesh/backup"
		logDir              = "/tmp/polarismesh/log/polaris_config"
		address             = "not_exist"
	)
	require.NoError(t, os.RemoveAll(logDir))
	defer func() {
		assert.NoError(t, os.RemoveAll(logDir))
	}()
	require.NoError(t, api.SetLoggersDir(logDir))

	// 北极星配置文件
	cfg := api.NewConfiguration()
	addresses := []string{address}
	cfg.GetGlobal().GetServerConnector().SetAddresses(addresses)

	require.Nil(t, SetupWithConfig(&Config{
		EnableServiceRouter: &enableServiceRouter,
		PersistDir:          &persisDir,
		Debug:               false,
		Discovery:           DiscoveryConfig{RefreshInterval: 10000},
		PolarisConfig:       cfg,
	}))

	// 日志异步书写
	time.Sleep(3 * time.Second)
	startLog, err := os.ReadFile(logDir + "/base/polaris.log")
	assert.Equal(t, err, nil)
	index := strings.Index(string(startLog), address)
	assert.True(t, index >= 0)
}

func Test_newSDKContext(t *testing.T) {
	cfgstr := `
address_list: 127.0.0.1:0
report_timeout: 1s
timeout: 1000
debug: false
persistDir: /tmp/polarismesh/backup
join_point: default
service_expire_time: 12h
connect_timeout:
message_timeout: 3s
bind_ip: 127.0.0.1
discovery:
  refresh_interval: 10000
circuitbreaker:
  checkPeriod: 10s
  requestCountAfterHalfOpen: 20
  successCountAfterHalfOpen: 18
  sleepWindow: 25s
  errorCount:
    continuousErrorThreshold: 20
    metricNumBuckets: 12
    metricStatTimeWindow: 1m
  errorRate:
    errorRateThreshold: 0.4
    metricStatTimeWindow: 1m
    metricNumBuckets: 6
    requestVolumeThreshold: 12
cluster_service:
  discover: xxx.polaris
  health_check: yyy.polaris
  monitor: zzz.polaris
service_router:
  nearby_matchlevel: region
  percent_of_min_instances: 0.2
  need_return_all_nodes: true
loadbalance:
  name: [polaris_ring_hash]
  details:
    polaris_ring_hash:
      vnodeCount: 1024
instance_location:                # 注册实例的地址位置信息
  region: China
  zone: Guangdong
  campus: Shenzhen
`
	cfg := Config{}
	err := yaml.Unmarshal([]byte(cfgstr), &cfg)
	assert.Nil(t, err)
	sdkCtx, err := newSDKContext(&cfg)
	assert.Nil(t, err)
	// 检查熔断配置
	circuitbreaker := sdkCtx.GetConfig().GetConsumer().GetCircuitBreaker()
	assert.Equal(t, 10*time.Second, circuitbreaker.GetCheckPeriod())
	assert.Equal(t, 20, circuitbreaker.GetRequestCountAfterHalfOpen())
	assert.Equal(t, 18, circuitbreaker.GetSuccessCountAfterHalfOpen())
	assert.Equal(t, 25*time.Second, circuitbreaker.GetSleepWindow())
	assert.Equal(t, 20, circuitbreaker.GetDefaultErrorCount())
	assert.Equal(t, 40, circuitbreaker.GetDefaultErrorPercent())
	assert.Equal(t, time.Minute, circuitbreaker.GetDefaultInterval())
	assert.Equal(t, 12, circuitbreaker.GetDefaultMinimumRequest())
	// 检查LocalCache配置
	discovery := sdkCtx.GetConfig().GetConsumer().GetLocalCache()
	assert.Equal(t, 10000*time.Millisecond, discovery.GetServiceRefreshInterval())
	assert.Equal(t, "/tmp/polarismesh/backup", discovery.GetPersistDir())
	loadBalanceCfg, ok := sdkCtx.GetConfig().GetConsumer().GetLoadbalancer().
		GetPluginConfig(config.DefaultLoadBalancerRingHash).(*ringhash.Config)
	require.True(t, ok)
	require.Equal(t, 1024, loadBalanceCfg.VnodeCount)
	// 其它配置检查
	global := sdkCtx.GetConfig().GetGlobal()
	assert.Equal(t, 1000*time.Millisecond, global.GetAPI().GetTimeout())
	assert.Equal(t, "xxx.polaris", global.GetSystem().GetDiscoverCluster().GetService())
	assert.Equal(t, "yyy.polaris", global.GetSystem().GetHealthCheckCluster().GetService())
	assert.Equal(t, "zzz.polaris", global.GetSystem().GetMonitorCluster().GetService())
	// 检查服务路由配置
	serviceRouter := sdkCtx.GetConfig().GetConsumer().GetServiceRouter()
	assert.Equal(t, "region", serviceRouter.GetNearbyConfig().GetMatchLevel())
	assert.Equal(t, 0.2, serviceRouter.GetPercentOfMinInstances())

	reportTimeout := time.Second
	assert.Equal(t, &reportTimeout, cfg.ReportTimeout)
	// 检查客户端绑定IP配置
	assert.Equal(t, "127.0.0.1", sdkCtx.GetConfig().GetGlobal().GetAPI().GetBindIP())

	// 检查地域信息
	assert.Equal(t, "China", sdkCtx.GetValueContext().GetCurrentLocation().GetLocation().Region)
	assert.Equal(t, "Guangdong", sdkCtx.GetValueContext().GetCurrentLocation().GetLocation().Zone)
	assert.Equal(t, "Shenzhen", sdkCtx.GetValueContext().GetCurrentLocation().GetLocation().Campus)
}

func Test_newSDKContextLegacyCircuitBreakerChain(t *testing.T) {
	cfg := &Config{
		Protocol:    "grpc",
		AddressList: "127.0.0.1:0",
	}
	cfg.CircuitBreaker.Chain = []string{"errorCount", "errorRate"}
	sdkCtx, err := newSDKContext(cfg)
	require.NoError(t, err)
	defer sdkCtx.Destroy()

	require.Equal(t, []string{"composite"},
		sdkCtx.GetConfig().GetConsumer().GetCircuitBreaker().GetChain())
}

func Test_newSDKContextUnknownCircuitBreakerChain(t *testing.T) {
	cfg := &Config{
		Protocol:    "grpc",
		AddressList: "127.0.0.1:0",
	}
	cfg.CircuitBreaker.Chain = []string{"unknownCircuitBreaker"}
	_, err := newSDKContext(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknownCircuitBreaker is not registered")
}

type reportProbeCircuitBreaker struct {
	polarisplugin.PluginBase
}

var reportProbeCalls int32

func (p *reportProbeCircuitBreaker) Type() common.Type {
	return common.TypeCircuitBreaker
}

func (p *reportProbeCircuitBreaker) Name() string {
	return "naming-test-report-probe"
}

func (p *reportProbeCircuitBreaker) CheckResource(model.Resource) model.CircuitBreakerStatus {
	return nil
}

func (p *reportProbeCircuitBreaker) Report(*model.ResourceStat) error {
	atomic.AddInt32(&reportProbeCalls, 1)
	return nil
}

func Test_ReportReachesPolarisCircuitBreaker(t *testing.T) {
	probe := &reportProbeCircuitBreaker{}
	polarisplugin.RegisterPlugin(probe)
	before := atomic.LoadInt32(&reportProbeCalls)

	cfg := &Config{Protocol: "grpc", AddressList: "127.0.0.1:0"}
	cfg.CircuitBreaker.Chain = []string{probe.Name()}
	sdkCtx, err := newSDKContext(cfg)
	require.NoError(t, err)
	defer sdkCtx.Destroy()

	svcKey := &model.ServiceKey{Namespace: "default", Service: "report-service"}
	inst := pb.NewInstanceInProto(&apiservice.Instance{
		Id:     wrapperspb.String("report-instance"),
		Host:   wrapperspb.String("127.0.0.1"),
		Port:   wrapperspb.UInt32(8000),
		Weight: wrapperspb.UInt32(100),
	}, svcKey, local.NewInstanceLocalValue())
	node := &trpcregistry.Node{Metadata: map[string]interface{}{"instance": inst}}
	consumer := api.NewConsumerAPIByContext(sdkCtx)

	require.NoError(t, adaptercb.Report(consumer, node, nil, time.Second,
		errs.NewFrameError(errs.RetClientNetErr, "test network failure")))
	require.Equal(t, before+2, atomic.LoadInt32(&reportProbeCalls))
}

func Test_ReportWhenSDKCircuitBreakerDisabled(t *testing.T) {
	pc := api.NewConfiguration()
	pc.GetConsumer().GetCircuitBreaker().SetEnable(false)
	sdkCtx, err := newSDKContext(&Config{
		Protocol:      "grpc",
		AddressList:   "127.0.0.1:0",
		PolarisConfig: pc,
	})
	require.NoError(t, err)
	defer sdkCtx.Destroy()

	svcKey := &model.ServiceKey{Namespace: "default", Service: "disabled-report"}
	inst := pb.NewInstanceInProto(&apiservice.Instance{
		Id:     wrapperspb.String("disabled-report-instance"),
		Host:   wrapperspb.String("127.0.0.1"),
		Port:   wrapperspb.UInt32(8000),
		Weight: wrapperspb.UInt32(100),
	}, svcKey, local.NewInstanceLocalValue())
	node := &trpcregistry.Node{Metadata: map[string]interface{}{"instance": inst}}

	require.NotPanics(t, func() {
		require.NoError(t, adaptercb.Report(api.NewConsumerAPIByContext(sdkCtx), node, nil,
			time.Millisecond, nil))
	})
}

func Test_LoadBalanceIgnoresOtherCallerExhaustedHalfOpenStatus(t *testing.T) {
	sdkCtx, err := newSDKContext(&Config{Protocol: "grpc", AddressList: "127.0.0.1:0"})
	require.NoError(t, err)
	defer sdkCtx.Destroy()

	halfOpen := model.NewHalfOpenStatus("other-caller", time.Now(), 1).(*model.HalfOpenStatus)
	require.True(t, halfOpen.AcquirePermission())
	require.False(t, halfOpen.AcquirePermission())
	localValue := local.NewInstanceLocalValue().(*local.DefaultInstanceLocalValue)
	localValue.SetCircuitBreakerStatus(halfOpen)
	instances := pb.NewServiceInstancesInProto(&apiservice.DiscoverResponse{
		Service: &apiservice.Service{
			Name:      wrapperspb.String("half-open-lb"),
			Namespace: wrapperspb.String("default"),
		},
		Instances: []*apiservice.Instance{
			{
				Id:      wrapperspb.String("half-open"),
				Host:    wrapperspb.String("127.0.0.1"),
				Port:    wrapperspb.UInt32(8000),
				Weight:  wrapperspb.UInt32(100),
				Healthy: wrapperspb.Bool(true),
			},
			{
				Id:      wrapperspb.String("healthy"),
				Host:    wrapperspb.String("127.0.0.1"),
				Port:    wrapperspb.UInt32(8001),
				Weight:  wrapperspb.UInt32(1),
				Healthy: wrapperspb.Bool(true),
			},
		},
	}, func(id string) local.InstanceLocalValue {
		if id == "half-open" {
			return localValue
		}
		return local.NewInstanceLocalValue()
	}, nil, local.NewServiceLocalValue(), sdkCtx.GetValueContext().GetContextLogger().GetBaseLogger())
	lb, err := adapterlb.New(sdkCtx, config.DefaultLoadBalancerHash)
	require.NoError(t, err)

	cluster := model.NewCluster(instances.GetServiceClusters(), nil)
	cluster.SetReuse(false)
	node, err := lb.Select("half-open-lb", []*trpcregistry.Node{{
		Metadata: map[string]interface{}{
			"cluster":          cluster,
			"serviceInstances": instances,
		},
	}}, tlb.WithKey("review-key-2"))
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:8000", node.Address)
	require.False(t, halfOpen.AcquirePermission())
}

func Test_LoadBalanceDoesNotConsumeSharedHalfOpenQuota(t *testing.T) {
	sdkCtx, err := newSDKContext(&Config{Protocol: "grpc", AddressList: "127.0.0.1:0"})
	require.NoError(t, err)
	defer sdkCtx.Destroy()

	halfOpen := model.NewHalfOpenStatus("shared-status", time.Now(), 1).(*model.HalfOpenStatus)
	localValue := local.NewInstanceLocalValue().(*local.DefaultInstanceLocalValue)
	localValue.SetCircuitBreakerStatus(halfOpen)
	instances := pb.NewServiceInstancesInProto(&apiservice.DiscoverResponse{
		Service: &apiservice.Service{
			Name:      wrapperspb.String("half-open-recovery"),
			Namespace: wrapperspb.String("default"),
		},
		Instances: []*apiservice.Instance{
			{
				Id:      wrapperspb.String("half-open"),
				Host:    wrapperspb.String("127.0.0.1"),
				Port:    wrapperspb.UInt32(8000),
				Weight:  wrapperspb.UInt32(100),
				Healthy: wrapperspb.Bool(true),
			},
			{
				Id:      wrapperspb.String("healthy"),
				Host:    wrapperspb.String("127.0.0.1"),
				Port:    wrapperspb.UInt32(8001),
				Weight:  wrapperspb.UInt32(100),
				Healthy: wrapperspb.Bool(true),
			},
		},
	}, func(id string) local.InstanceLocalValue {
		if id == "half-open" {
			return localValue
		}
		return local.NewInstanceLocalValue()
	}, nil, local.NewServiceLocalValue(), sdkCtx.GetValueContext().GetContextLogger().GetBaseLogger())
	lb, err := adapterlb.New(sdkCtx, config.DefaultLoadBalancerHash)
	require.NoError(t, err)

	cluster := model.NewCluster(instances.GetServiceClusters(), nil)
	cluster.SetReuse(false)
	node, err := lb.Select("half-open-recovery", []*trpcregistry.Node{{
		Metadata: map[string]interface{}{
			"cluster":          cluster,
			"serviceInstances": instances,
		},
	}}, tlb.WithKey("review-key-2"))
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:8000", node.Address)
	require.True(t, halfOpen.AcquirePermission())
}

func Test_LoadBalanceKeepsRecoverAllOpenInstance(t *testing.T) {
	sdkCtx, err := newSDKContext(&Config{Protocol: "grpc", AddressList: "127.0.0.1:0"})
	require.NoError(t, err)
	defer sdkCtx.Destroy()

	localValue := local.NewInstanceLocalValue().(*local.DefaultInstanceLocalValue)
	localValue.SetCircuitBreakerStatus(model.NewCircuitBreakerStatus(
		"recover-all", model.Open, time.Now()))
	instances := pb.NewServiceInstancesInProto(&apiservice.DiscoverResponse{
		Service: &apiservice.Service{
			Name:      wrapperspb.String("recover-all"),
			Namespace: wrapperspb.String("default"),
		},
		Instances: []*apiservice.Instance{{
			Id:      wrapperspb.String("open"),
			Host:    wrapperspb.String("127.0.0.1"),
			Port:    wrapperspb.UInt32(8000),
			Weight:  wrapperspb.UInt32(100),
			Healthy: wrapperspb.Bool(true),
		}},
	}, func(id string) local.InstanceLocalValue {
		return localValue
	}, nil, local.NewServiceLocalValue(), sdkCtx.GetValueContext().GetContextLogger().GetBaseLogger())
	lb, err := adapterlb.New(sdkCtx, config.DefaultLoadBalancerWR)
	require.NoError(t, err)

	cluster := model.NewCluster(instances.GetServiceClusters(), nil)
	cluster.SetReuse(false)
	cluster.HasLimitedInstances = true
	node, err := lb.Select("recover-all", []*trpcregistry.Node{{
		Metadata: map[string]interface{}{
			"cluster":          cluster,
			"serviceInstances": instances,
		},
	}})
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:8000", node.Address)
}

func Test_normalizeCircuitBreakerChain(t *testing.T) {
	for _, tt := range []struct {
		name    string
		chain   []string
		want    []string
		wantErr bool
	}{
		{
			name:    "single error count strategy is rejected",
			chain:   []string{"errorCount"},
			wantErr: true,
		},
		{
			name:    "single error rate strategy is rejected",
			chain:   []string{"errorRate"},
			wantErr: true,
		},
		{
			name:    "legacy strategy mixed with composite is rejected",
			chain:   []string{"errorCount", "composite"},
			wantErr: true,
		},
		{name: "both legacy strategies", chain: []string{"errorCount", "errorRate"}, want: []string{"composite"}},
		{name: "composite", chain: []string{"composite"}, want: []string{"composite"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeCircuitBreakerChain(tt.chain)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_newSDKContextDefaultLoadBalanceAliases(t *testing.T) {
	sdkCtx, err := newSDKContext(&Config{Protocol: "grpc", AddressList: "127.0.0.1:0"})
	require.NoError(t, err)
	defer sdkCtx.Destroy()

	require.NoError(t, setupLoadbalance(sdkCtx, &Config{}, false))
	for _, name := range []string{
		"polaris_wr",
		"polaris_hash",
		"polaris_ring_hash",
		"polaris_l5cst",
		"polaris_maglev",
	} {
		require.NotNil(t, tlb.Get(name), "legacy loadbalance name %s is not registered", name)
	}
}

func Test_setSdkCircuitBreakerWindowSources(t *testing.T) {
	errorCountWindow := func(window time.Duration) *struct {
		ContinuousErrorThreshold *int           `yaml:"continuousErrorThreshold"`
		MetricNumBuckets         *int           `yaml:"metricNumBuckets"`
		MetricStatTimeWindow     *time.Duration `yaml:"metricStatTimeWindow"`
	} {
		return &struct {
			ContinuousErrorThreshold *int           `yaml:"continuousErrorThreshold"`
			MetricNumBuckets         *int           `yaml:"metricNumBuckets"`
			MetricStatTimeWindow     *time.Duration `yaml:"metricStatTimeWindow"`
		}{MetricStatTimeWindow: &window}
	}

	t.Run("default window", func(t *testing.T) {
		cfg := &Config{}
		cfg.CircuitBreaker.ErrorCount = errorCountWindow(config.DefaultInterval)
		c := api.NewConfiguration()
		require.NoError(t, setSdkCircuitBreaker(c, cfg))
	})

	t.Run("default window mismatch", func(t *testing.T) {
		cfg := &Config{}
		cfg.CircuitBreaker.ErrorCount = errorCountWindow(2 * time.Minute)
		c := api.NewConfiguration()
		err := setSdkCircuitBreaker(c, cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "is not supported by polaris-go composite")
	})

	t.Run("existing polaris config window", func(t *testing.T) {
		window := 2 * time.Minute
		cfg := &Config{}
		cfg.CircuitBreaker.ErrorCount = errorCountWindow(window)
		c := api.NewConfiguration()
		c.GetConsumer().GetCircuitBreaker().SetDefaultInterval(window)
		require.NoError(t, setSdkCircuitBreaker(c, cfg))
	})

	t.Run("explicit adapter window", func(t *testing.T) {
		cfg := &Config{}
		cfg.CircuitBreaker.ErrorCount = errorCountWindow(2 * time.Minute)
		window := 2 * time.Minute
		cfg.CircuitBreaker.ErrorRate = &struct {
			ErrorRateThreshold     *float64       `yaml:"errorRateThreshold"`
			MetricNumBuckets       *int           `yaml:"metricNumBuckets"`
			MetricStatTimeWindow   *time.Duration `yaml:"metricStatTimeWindow"`
			RequestVolumeThreshold *int           `yaml:"requestVolumeThreshold"`
		}{MetricStatTimeWindow: &window}
		c := api.NewConfiguration()
		require.NoError(t, setSdkCircuitBreaker(c, cfg))
	})

	t.Run("explicit adapter window mismatch", func(t *testing.T) {
		cfg := &Config{}
		cfg.CircuitBreaker.ErrorCount = errorCountWindow(2 * time.Minute)
		window := 3 * time.Minute
		cfg.CircuitBreaker.ErrorRate = &struct {
			ErrorRateThreshold     *float64       `yaml:"errorRateThreshold"`
			MetricNumBuckets       *int           `yaml:"metricNumBuckets"`
			MetricStatTimeWindow   *time.Duration `yaml:"metricStatTimeWindow"`
			RequestVolumeThreshold *int           `yaml:"requestVolumeThreshold"`
		}{MetricStatTimeWindow: &window}
		c := api.NewConfiguration()
		err := setSdkCircuitBreaker(c, cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "use errorRate.metricStatTimeWindow 3m0s")
	})
}

func Test_getLogLevel(t *testing.T) {
	type args struct {
		desc string
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{
			"convert to debug level",
			args{"debug"},
			plog.DebugLog,
		},
		{
			"convert to info level",
			args{"info"},
			plog.InfoLog,
		},
		{
			"convert to warn level",
			args{"warn"},
			plog.WarnLog,
		},
		{
			"convert to error level",
			args{"error"},
			plog.ErrorLog,
		},
		{
			"convert to fatal level",
			args{"fatal"},
			plog.FatalLog,
		},
		{
			"convert to none level",
			args{"none"},
			plog.NoneLog,
		},
		{
			"convert to default",
			args{"default"},
			plog.DefaultBaseLogLevel,
		},
		{
			"convert to real log level - case insensitive",
			args{"DEBUG"},
			plog.DebugLog,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, getLogLevel(tt.args.desc))
		})
	}
}

func TestConfig_UnmarshalYAML(t *testing.T) {
	String := func(v string) *string { return &v }
	type args struct {
		value *yaml.Node
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
		want    *Config
	}{
		{
			"unmarshal yaml - log_dir",
			args{func() *yaml.Node {
				cfgstr := `
plugins:
  selector:
    polarismesh:
      address_list: 127.0.0.1:0
      log_dir: /tmp/polarismesh/log
      discovery:
        refresh_interval: 10000
`
				cfg := trpc.Config{}
				err := yaml.Unmarshal([]byte(cfgstr), &cfg)
				assert.Nil(t, err)
				polarisCfg := cfg.Plugins["selector"]["polarismesh"]
				return &polarisCfg
			}()},
			false,
			&Config{
				LogDir: String("/tmp/polarismesh/log"),
				Logs: &Logs{
					DirPath:    "/tmp/polarismesh/log",
					Level:      "default",
					MaxBackups: plog.DefaultRotationMaxBackups,
					MaxSize:    plog.DefaultRotationMaxSize,
				},
				AddressList: "127.0.0.1:0",
				Discovery: DiscoveryConfig{
					RefreshInterval: 10000,
				},
			},
		},
		{
			"unmarshal yaml - no log config",
			args{func() *yaml.Node {
				cfgstr := `
plugins:
  selector:
    polarismesh:
      address_list: 127.0.0.1:0
      discovery:
        refresh_interval: 10000
`
				cfg := trpc.Config{}
				err := yaml.Unmarshal([]byte(cfgstr), &cfg)
				assert.Nil(t, err)
				polarisCfg := cfg.Plugins["selector"]["polarismesh"]
				return &polarisCfg
			}()},
			false,
			&Config{
				AddressList: "127.0.0.1:0",
				Discovery: DiscoveryConfig{
					RefreshInterval: 10000,
				},
			},
		},
		{
			"unmarshal yaml - logs, with default value",
			args{func() *yaml.Node {
				cfgstr := `
plugins:
  selector:
    polarismesh:
      address_list: 127.0.0.1:0
      logs:
        level: debug
      discovery:
        refresh_interval: 10000
`
				cfg := trpc.Config{}
				err := yaml.Unmarshal([]byte(cfgstr), &cfg)
				assert.Nil(t, err)
				polarisCfg := cfg.Plugins["selector"]["polarismesh"]
				return &polarisCfg
			}()},
			false,
			&Config{
				Logs: &Logs{
					DirPath:    plog.DefaultLogRotationRootDir,
					Level:      "debug",
					MaxBackups: plog.DefaultRotationMaxBackups,
					MaxSize:    plog.DefaultRotationMaxSize,
				},
				AddressList: "127.0.0.1:0",
				Discovery: DiscoveryConfig{
					RefreshInterval: 10000,
				},
			},
		},
		{
			"unmarshal yaml - no log config",
			args{func() *yaml.Node {
				cfgstr := `
plugins:
  selector:
    polarismesh:
      address_list: 127.0.0.1:0
      discovery:
        refresh_interval: 10000
`
				cfg := trpc.Config{}
				err := yaml.Unmarshal([]byte(cfgstr), &cfg)
				assert.Nil(t, err)
				polarisCfg := cfg.Plugins["selector"]["polarismesh"]
				return &polarisCfg
			}()},
			false,
			&Config{
				AddressList: "127.0.0.1:0",
				Discovery: DiscoveryConfig{
					RefreshInterval: 10000,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{}
			err := c.UnmarshalYAML(tt.args.value)
			assert.Equal(t, tt.wantErr, err != nil)
			assert.Equal(t, tt.want, c)
		})
	}
}
