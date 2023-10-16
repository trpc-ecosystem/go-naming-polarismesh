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

package naming

import (
	"os"
	"strings"
	"testing"
	"time"

	_ "trpc.group/trpc-go/trpc-naming-polarismesh/registry"

	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/config"
	plog "github.com/polarismesh/polaris-go/pkg/log"
	"github.com/polarismesh/polaris-go/plugin/loadbalancer/ringhash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

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
	os.RemoveAll(logDir)
	defer os.RemoveAll(logDir)
	api.SetLoggersDir(logDir)

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
    metricStatTimeWindow: 2m
  errorRate:
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
	errorCount := sdkCtx.GetConfig().GetConsumer().GetCircuitBreaker().GetErrorCountConfig()
	assert.Equal(t, 20, errorCount.GetContinuousErrorThreshold())
	assert.Equal(t, 12, errorCount.GetMetricNumBuckets())
	assert.Equal(t, 2*time.Minute, errorCount.GetMetricStatTimeWindow())
	errRate := sdkCtx.GetConfig().GetConsumer().GetCircuitBreaker().GetErrorRateConfig()
	assert.Equal(t, 1*time.Minute, errRate.GetMetricStatTimeWindow())
	assert.Equal(t, 6, errRate.GetMetricNumBuckets())
	assert.Equal(t, 12, errRate.GetRequestVolumeThreshold())
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
