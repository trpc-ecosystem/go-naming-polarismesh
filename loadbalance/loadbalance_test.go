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

package loadbalance

import (
	"context"
	"testing"
	"time"

	"trpc.group/trpc-go/trpc-go"
	"trpc.group/trpc-go/trpc-go/codec"
	thttp "trpc.group/trpc-go/trpc-go/http"
	tlb "trpc.group/trpc-go/trpc-go/naming/loadbalance"
	"trpc.group/trpc-go/trpc-go/naming/registry"
	"trpc.group/trpc-go/trpc-naming-polarismesh/internal/mock/mock_api"
	"trpc.group/trpc-go/trpc-naming-polarismesh/internal/mock/mock_loadbalancer"
	"trpc.group/trpc-go/trpc-naming-polarismesh/internal/mock/mock_model"
	"trpc.group/trpc-go/trpc-naming-polarismesh/internal/mock/mock_plugin"
	"trpc.group/trpc-go/trpc-naming-polarismesh/internal/polaris"

	"github.com/golang/mock/gomock"
	"github.com/polarismesh/polaris-go/pkg/config"
	"github.com/polarismesh/polaris-go/pkg/model"
	"github.com/polarismesh/polaris-go/pkg/model/local"
	"github.com/polarismesh/polaris-go/pkg/model/pb"
	"github.com/polarismesh/polaris-go/pkg/plugin/common"
	"github.com/polarismesh/polaris-go/pkg/sdk"
	apiservice "github.com/polarismesh/specification/source/go/api/v1/service_manage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"gopkg.in/yaml.v3"
)

func TestSetup(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	inst := mock_model.NewMockInstance(ctrl)

	plugin := mock_loadbalancer.NewMockLoadBalancer(ctrl)
	plugin.EXPECT().ChooseInstance(gomock.Any(), gomock.Any()).Return(inst, nil).AnyTimes()

	pluginer := mock_plugin.NewMockManager(ctrl)
	pluginer.EXPECT().GetPlugin(common.TypeLoadBalancer, config.DefaultLoadBalancerWR).Return(plugin, nil).AnyTimes()
	m := mock_api.NewMockSDKContext(ctrl)
	m.EXPECT().GetPlugins().Return(pluginer).AnyTimes()

	assert.Nil(t, Setup(m, "polaris_wr", true))
}

func TestSelect(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	inst := mock_model.NewMockInstance(ctrl)
	inst.EXPECT().GetMetadata().Return(map[string]string{}).AnyTimes()
	inst.EXPECT().GetWeight().Return(100).AnyTimes()
	inst.EXPECT().GetHost().Return("host").AnyTimes()
	inst.EXPECT().GetPort().Return(uint32(1003)).AnyTimes()
	inst.EXPECT().GetNamespace().Return("default").AnyTimes()
	inst.EXPECT().GetService().Return("service").AnyTimes()
	inst.EXPECT().GetProtocol().Return("grpc").AnyTimes()
	inst.EXPECT().GetCircuitBreakerStatus().Return(nil).AnyTimes()

	plugin := mock_loadbalancer.NewMockLoadBalancer(ctrl)
	plugin.EXPECT().ChooseInstance(gomock.Any(), gomock.Any()).Return(inst, nil).AnyTimes()

	m := mock_api.NewMockSDKContext(ctrl)
	sdkConfig := config.NewDefaultConfiguration([]string{"127.0.0.1:0"})
	sdkConfig.GetConsumer().GetCircuitBreaker().SetEnable(false)
	m.EXPECT().GetConfig().Return(sdkConfig).AnyTimes()
	m.EXPECT().GetEngine().Return(&permissionEngine{
		checkResult:      &model.CheckResult{Pass: true},
		instancePass8000: true,
	}).AnyTimes()
	m.EXPECT().GetValueContext().Return(mock_model.NewMockValueContext(ctrl)).AnyTimes()

	clustersMock := mock_model.NewMockServiceClusters(ctrl)
	clustersMock.EXPECT().GetServiceInstances().Return(mock_model.NewMockServiceInstances(ctrl)).AnyTimes()

	lb := &WRLoadBalancer{
		sdkCtx: m,
		lb:     plugin,
	}

	list := []*registry.Node{
		{
			Metadata: map[string]interface{}{
				"cluster":          model.NewCluster(clustersMock, nil),
				"serviceInstances": mock_model.NewMockServiceInstances(ctrl),
			},
		},
	}

	node, err := lb.Select("service", list)
	assert.Nil(t, err)
	assert.Equal(t, node.Weight, 100)

	_, err = lb.Select("service", nil)
	assert.NotNil(t, err)
}

func TestSelectAcquiresHalfOpenPermission(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	inst := mock_model.NewMockInstance(ctrl)
	inst.EXPECT().GetMetadata().Return(map[string]string{}).AnyTimes()
	inst.EXPECT().GetWeight().Return(100).AnyTimes()
	inst.EXPECT().GetHost().Return("127.0.0.1").AnyTimes()
	inst.EXPECT().GetPort().Return(uint32(8000)).AnyTimes()
	inst.EXPECT().GetNamespace().Return("default").AnyTimes()
	inst.EXPECT().GetService().Return("service").AnyTimes()
	inst.EXPECT().GetProtocol().Return("grpc").AnyTimes()
	status := model.NewHalfOpenStatus("probe", time.Now(), 1).(*model.HalfOpenStatus)
	inst.EXPECT().GetCircuitBreakerStatus().Return(status).AnyTimes()

	sdkConfig := config.NewDefaultConfiguration([]string{"127.0.0.1:0"})
	sdkConfig.GetConsumer().GetCircuitBreaker().SetEnable(true)
	m := mock_api.NewMockSDKContext(ctrl)
	pluginManager := mock_plugin.NewMockManager(ctrl)
	pluginManager.EXPECT().
		GetPlugin(common.TypeCircuitBreaker, config.DefaultCircuitBreaker).
		Return(&fakeCircuitBreakerPlugin{}, nil).AnyTimes()
	m.EXPECT().GetPlugins().Return(pluginManager).AnyTimes()
	m.EXPECT().GetConfig().Return(sdkConfig).AnyTimes()
	m.EXPECT().GetEngine().Return(&permissionEngine{
		checkResult:      &model.CheckResult{Pass: true},
		instancePass8000: true,
	}).AnyTimes()

	plugin := mock_loadbalancer.NewMockLoadBalancer(ctrl)
	plugin.EXPECT().ChooseInstance(gomock.Any(), gomock.Any()).Return(inst, nil).Times(1)

	clustersMock := mock_model.NewMockServiceClusters(ctrl)
	clustersMock.EXPECT().GetServiceInstances().Return(mock_model.NewMockServiceInstances(ctrl)).AnyTimes()
	lb := &WRLoadBalancer{sdkCtx: m, lb: plugin}
	list := []*registry.Node{{
		Metadata: map[string]interface{}{
			"cluster":          model.NewCluster(clustersMock, nil),
			"serviceInstances": mock_model.NewMockServiceInstances(ctrl),
		},
	}}

	node, err := lb.Select("service", list)
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:8000", node.Address)
	assert.True(t, status.AcquirePermission(),
		"an endpoint-shared status must not consume caller-specific quota")
}

func TestSelectRebuildsMethodResourceAfterInstanceFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	svcKey := &model.ServiceKey{Namespace: "default", Service: "method-fallback-lb"}
	newInstance := func(protocol string, port uint32) model.Instance {
		return pb.NewInstanceInProto(&apiservice.Instance{
			Id:       wrapperspb.String(protocol),
			Host:     wrapperspb.String("127.0.0.1"),
			Port:     wrapperspb.UInt32(port),
			Protocol: wrapperspb.String(protocol),
			Weight:   wrapperspb.UInt32(100),
			Healthy:  wrapperspb.Bool(true),
		}, svcKey, local.NewInstanceLocalValue())
	}
	grpcInst := newInstance("grpc", 8000)
	httpInst := newInstance("http", 8001)
	instances := model.NewDefaultServiceInstances(
		model.ServiceInfo{Service: svcKey.Service, Namespace: svcKey.Namespace},
		[]model.Instance{grpcInst, httpInst},
	)
	cluster := model.NewCluster(instances.GetServiceClusters(), nil)

	sdkConfig := config.NewDefaultConfiguration([]string{"127.0.0.1:0"})
	sdkConfig.GetConsumer().GetCircuitBreaker().SetEnable(true)
	engine := &permissionEngine{checkResult: &model.CheckResult{Pass: true}}
	m := mock_api.NewMockSDKContext(ctrl)
	grpcInstanceResource, err := polaris.InstanceResource(grpcInst, model.EmptyServiceKey)
	require.NoError(t, err)
	pluginManager := mock_plugin.NewMockManager(ctrl)
	pluginManager.EXPECT().
		GetPlugin(common.TypeCircuitBreaker, config.DefaultCircuitBreaker).
		Return(&fakeCircuitBreakerPlugin{
			statuses: map[string]model.CircuitBreakerStatus{
				grpcInstanceResource.String(): model.NewCircuitBreakerStatus(
					"grpc-open", model.Open, time.Now()),
			},
		}, nil).AnyTimes()
	m.EXPECT().GetPlugins().Return(pluginManager).AnyTimes()
	m.EXPECT().GetConfig().Return(sdkConfig).AnyTimes()
	m.EXPECT().GetEngine().Return(engine).AnyTimes()

	plugin := mock_loadbalancer.NewMockLoadBalancer(ctrl)
	plugin.EXPECT().ChooseInstance(gomock.Any(), gomock.Any()).Return(grpcInst, nil).Times(1)
	plugin.EXPECT().ChooseInstance(gomock.Any(), gomock.Any()).Return(httpInst, nil).Times(1)

	lb := &WRLoadBalancer{sdkCtx: m, lb: plugin}
	msg := trpc.Message(context.Background())
	msg.WithClientRPCName("/review.Service/Call")
	requestCtx := context.WithValue(context.Background(), codec.ContextKeyMessage, msg)
	node, err := lb.Select(svcKey.Service, []*registry.Node{{
		Metadata: map[string]interface{}{
			"cluster":          cluster,
			"serviceInstances": instances,
		},
	}}, tlb.WithContext(requestCtx))
	require.NoError(t, err)

	methodResource, ok := node.Metadata[polaris.MethodResourceMetadataKey].(*model.MethodResource)
	require.True(t, ok)
	require.Equal(t, "http", methodResource.Protocol)
	require.Equal(t, "http", methodResource.Protocol)
}

func TestLoadBalanceHTTPMethodPathDropsQuery(t *testing.T) {
	msg := trpc.Message(context.Background())
	msg.WithClientRPCName("/orders?id=1")
	msg.WithCalleeMethod("/orders")
	msg.WithClientReqHead(&thttp.ClientReqHeader{Method: "GET"})
	ctx := context.WithValue(context.Background(), codec.ContextKeyMessage, msg)

	require.Equal(t, "/orders", loadBalanceMethodPath(&tlb.Options{Ctx: ctx}))
}

func TestSelectDoesNotLeakServiceQuotaWhenMethodIsOpen(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	svcKey := &model.ServiceKey{Namespace: "default", Service: "lb-quota-leak"}
	inst := pb.NewInstanceInProto(&apiservice.Instance{
		Id:       wrapperspb.String("instance"),
		Host:     wrapperspb.String("127.0.0.1"),
		Port:     wrapperspb.UInt32(8000),
		Protocol: wrapperspb.String("grpc"),
		Weight:   wrapperspb.UInt32(100),
		Healthy:  wrapperspb.Bool(true),
	}, svcKey, local.NewInstanceLocalValue())
	serviceResource, err := polaris.InstanceServiceResource(inst, model.EmptyServiceKey)
	require.NoError(t, err)
	methodResource, err := polaris.MethodResource(
		svcKey, model.EmptyServiceKey, "grpc", "GET", "/review.Service/Call")
	require.NoError(t, err)
	serviceHalfOpen := model.NewHalfOpenStatus(
		"service-probe", time.Now(), 2).(*model.HalfOpenStatus)
	circuitBreaker := &fakeCircuitBreakerPlugin{
		statuses: map[string]model.CircuitBreakerStatus{
			serviceResource.String(): serviceHalfOpen,
			methodResource.String(): model.NewCircuitBreakerStatus(
				"method-open", model.Open, time.Now()),
		},
	}

	sdkConfig := config.NewDefaultConfiguration([]string{"127.0.0.1:0"})
	sdkConfig.GetConsumer().GetCircuitBreaker().SetEnable(true)
	m := mock_api.NewMockSDKContext(ctrl)
	pluginManager := mock_plugin.NewMockManager(ctrl)
	pluginManager.EXPECT().
		GetPlugin(common.TypeCircuitBreaker, config.DefaultCircuitBreaker).
		Return(circuitBreaker, nil).AnyTimes()
	m.EXPECT().GetPlugins().Return(pluginManager).AnyTimes()
	m.EXPECT().GetConfig().Return(sdkConfig).AnyTimes()
	m.EXPECT().GetEngine().Return(&permissionEngine{}).AnyTimes()

	lbPlugin := mock_loadbalancer.NewMockLoadBalancer(ctrl)
	lbPlugin.EXPECT().ChooseInstance(gomock.Any(), gomock.Any()).
		Return(inst, nil).Times(1)
	clustersMock := mock_model.NewMockServiceClusters(ctrl)
	clustersMock.EXPECT().GetServiceInstances().
		Return(mock_model.NewMockServiceInstances(ctrl)).AnyTimes()
	lb := &WRLoadBalancer{sdkCtx: m, lb: lbPlugin}

	msg := trpc.Message(context.Background())
	msg.WithClientRPCName("/review.Service/Call")
	msg.WithCalleeMethod("/review.Service/Call")
	msg.WithClientReqHead(&thttp.ClientReqHeader{Method: "GET"})
	requestCtx := context.WithValue(context.Background(), codec.ContextKeyMessage, msg)
	_, err = lb.Select(svcKey.Service, []*registry.Node{{
		Metadata: map[string]interface{}{
			"cluster":          model.NewCluster(clustersMock, nil),
			"serviceInstances": mock_model.NewMockServiceInstances(ctrl),
		},
	}}, tlb.WithContext(requestCtx))
	require.Error(t, err)
	require.Contains(t, err.Error(), "method /review.Service/Call denied")
	require.Equal(t, int64(0), serviceHalfOpen.AllocatedCount())
}

func TestLoadBalanceAppliesAllHalfOpenResourceQuotas(t *testing.T) {
	for _, tc := range []struct {
		name         string
		serviceQuota int
		methodQuota  int
		denied       string
	}{
		{name: "service exhausted", serviceQuota: 1, methodQuota: 10, denied: "service"},
		{name: "method exhausted", serviceQuota: 2, methodQuota: 1, denied: "method /review.Service/Call denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			svcKey := &model.ServiceKey{Namespace: "default", Service: "lb-all-half-open"}
			inst := pb.NewInstanceInProto(&apiservice.Instance{
				Id:       wrapperspb.String("instance"),
				Host:     wrapperspb.String("127.0.0.1"),
				Port:     wrapperspb.UInt32(8000),
				Protocol: wrapperspb.String("grpc"),
				Weight:   wrapperspb.UInt32(100),
				Healthy:  wrapperspb.Bool(true),
			}, svcKey, local.NewInstanceLocalValue())
			serviceResource, err := polaris.InstanceServiceResource(inst, model.EmptyServiceKey)
			require.NoError(t, err)
			methodResource, err := polaris.MethodResource(
				svcKey, model.EmptyServiceKey, "grpc", "GET", "/review.Service/Call")
			require.NoError(t, err)
			serviceHalfOpen := model.NewHalfOpenStatus(
				"service-probe", time.Now(), tc.serviceQuota).(*model.HalfOpenStatus)
			methodHalfOpen := model.NewHalfOpenStatus(
				"method-probe", time.Now(), tc.methodQuota).(*model.HalfOpenStatus)
			circuitBreaker := &fakeCircuitBreakerPlugin{
				statuses: map[string]model.CircuitBreakerStatus{
					serviceResource.String(): serviceHalfOpen,
					methodResource.String():  methodHalfOpen,
				},
			}

			sdkConfig := config.NewDefaultConfiguration([]string{"127.0.0.1:0"})
			sdkConfig.GetConsumer().GetCircuitBreaker().SetEnable(true)
			m := mock_api.NewMockSDKContext(ctrl)
			pluginManager := mock_plugin.NewMockManager(ctrl)
			pluginManager.EXPECT().
				GetPlugin(common.TypeCircuitBreaker, config.DefaultCircuitBreaker).
				Return(circuitBreaker, nil).AnyTimes()
			m.EXPECT().GetPlugins().Return(pluginManager).AnyTimes()
			m.EXPECT().GetConfig().Return(sdkConfig).AnyTimes()
			m.EXPECT().GetEngine().Return(&permissionEngine{}).AnyTimes()

			lbPlugin := mock_loadbalancer.NewMockLoadBalancer(ctrl)
			lbPlugin.EXPECT().ChooseInstance(gomock.Any(), gomock.Any()).
				Return(inst, nil).Times(2)
			clustersMock := mock_model.NewMockServiceClusters(ctrl)
			clustersMock.EXPECT().GetServiceInstances().
				Return(mock_model.NewMockServiceInstances(ctrl)).AnyTimes()
			lb := &WRLoadBalancer{sdkCtx: m, lb: lbPlugin}

			msg := trpc.Message(context.Background())
			msg.WithClientRPCName("/review.Service/Call")
			msg.WithCalleeMethod("/review.Service/Call")
			msg.WithClientReqHead(&thttp.ClientReqHeader{Method: "GET"})
			requestCtx := context.WithValue(context.Background(), codec.ContextKeyMessage, msg)
			_, err = lb.Select(svcKey.Service, []*registry.Node{{
				Metadata: map[string]interface{}{
					"cluster":          model.NewCluster(clustersMock, nil),
					"serviceInstances": mock_model.NewMockServiceInstances(ctrl),
				},
			}}, tlb.WithContext(requestCtx))
			require.NoError(t, err)
			require.Equal(t, int64(1), serviceHalfOpen.AllocatedCount())
			require.Equal(t, int64(1), methodHalfOpen.AllocatedCount())

			_, err = lb.Select(svcKey.Service, []*registry.Node{{
				Metadata: map[string]interface{}{
					"cluster":          model.NewCluster(clustersMock, nil),
					"serviceInstances": mock_model.NewMockServiceInstances(ctrl),
				},
			}}, tlb.WithContext(requestCtx))
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.denied)
			require.Equal(t, int64(1), serviceHalfOpen.AllocatedCount())
			require.Equal(t, int64(1), methodHalfOpen.AllocatedCount())
		})
	}
}

func TestLoadBalanceRecoverAllOpenInstanceRemainsAvailable(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	svcKey := &model.ServiceKey{Namespace: "default", Service: "lb-recover-all"}
	localValue := local.NewInstanceLocalValue().(*local.DefaultInstanceLocalValue)
	localValue.SetCircuitBreakerStatus(model.NewCircuitBreakerStatus(
		"instance-open", model.Open, time.Now()))
	inst := pb.NewInstanceInProto(&apiservice.Instance{
		Id:       wrapperspb.String("instance"),
		Host:     wrapperspb.String("127.0.0.1"),
		Port:     wrapperspb.UInt32(8000),
		Protocol: wrapperspb.String("grpc"),
		Weight:   wrapperspb.UInt32(100),
		Healthy:  wrapperspb.Bool(true),
	}, svcKey, localValue)
	instances := model.NewDefaultServiceInstances(
		model.ServiceInfo{Service: svcKey.Service, Namespace: svcKey.Namespace},
		[]model.Instance{inst},
	)
	cluster := model.NewCluster(instances.GetServiceClusters(), nil)
	cluster.HasLimitedInstances = true

	sdkConfig := config.NewDefaultConfiguration([]string{"127.0.0.1:0"})
	sdkConfig.GetConsumer().GetCircuitBreaker().SetEnable(true)
	m := mock_api.NewMockSDKContext(ctrl)
	pluginManager := mock_plugin.NewMockManager(ctrl)
	pluginManager.EXPECT().
		GetPlugin(common.TypeCircuitBreaker, config.DefaultCircuitBreaker).
		Return(&fakeCircuitBreakerPlugin{}, nil).AnyTimes()
	m.EXPECT().GetPlugins().Return(pluginManager).AnyTimes()
	m.EXPECT().GetConfig().Return(sdkConfig).AnyTimes()
	m.EXPECT().GetEngine().Return(&permissionEngine{}).AnyTimes()

	lbPlugin := mock_loadbalancer.NewMockLoadBalancer(ctrl)
	lbPlugin.EXPECT().ChooseInstance(gomock.Any(), gomock.Any()).
		Return(inst, nil).Times(1)
	lb := &WRLoadBalancer{sdkCtx: m, lb: lbPlugin}
	node, err := lb.Select(svcKey.Service, []*registry.Node{{
		Metadata: map[string]interface{}{
			"cluster":          cluster,
			"serviceInstances": instances,
		},
	}})
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:8000", node.Address)
}

type permissionEngine struct {
	sdk.Engine
	checkResult      *model.CheckResult
	instancePass8000 bool
	methodResource   []model.Resource
}

type fakeCircuitBreakerPlugin struct {
	mock_plugin.MockPlugin
	statuses map[string]model.CircuitBreakerStatus
}

func (p *fakeCircuitBreakerPlugin) CheckResource(resource model.Resource) model.CircuitBreakerStatus {
	if p.statuses == nil {
		return nil
	}
	return p.statuses[resource.String()]
}

func (p *fakeCircuitBreakerPlugin) Report(*model.ResourceStat) error {
	return nil
}

func (e *permissionEngine) Check(resource model.Resource) (*model.CheckResult, error) {
	switch res := resource.(type) {
	case *model.ServiceResource:
		return &model.CheckResult{Pass: true}, nil
	case *model.InstanceResource:
		return &model.CheckResult{Pass: e.instancePass8000 || res.GetNode().Port != 8000}, nil
	case *model.MethodResource:
		e.methodResource = append(e.methodResource, res)
		return &model.CheckResult{Pass: true}, nil
	}
	return e.checkResult, nil
}

func TestAsPluginCfgs(t *testing.T) {
	newYamlCfgs := func(cfg string) map[string]yaml.Node {
		yamlCfgs := make(map[string]yaml.Node)
		require.Nil(t, yaml.Unmarshal([]byte(cfg), &yamlCfgs))
		return yamlCfgs
	}
	t.Run("unimplemented lb", func(t *testing.T) {
		_, err := AsPluginCfgs(newYamlCfgs(`unimplemented_lb: {}`))
		require.NotNil(t, err)
	})
	t.Run("lb does not support to config", func(t *testing.T) {
		_, err := AsPluginCfgs(newYamlCfgs(`polaris_maglev: {}`))
		require.NotNil(t, err)
	})
	t.Run("ring hash", func(t *testing.T) {
		_, err := AsPluginCfgs(newYamlCfgs(`
polaris_ring_hash:
  vnodeCount: 1024`))
		require.Nil(t, err)
	})
	t.Run("mixed lbs", func(t *testing.T) {
		_, err := AsPluginCfgs(newYamlCfgs(`
polaris_ring_hash:
  vnodeCount: 1024
unimplemented_lb: {}`))
		require.NotNil(t, err)
	})
}
