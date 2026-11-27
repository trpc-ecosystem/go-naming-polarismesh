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
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"trpc.group/trpc-go/trpc-go"
	"trpc.group/trpc-go/trpc-go/healthcheck"
	"trpc.group/trpc-go/trpc-go/naming/registry"
	"trpc.group/trpc-go/trpc-go/plugin"
	"trpc.group/trpc-go/trpc-naming-polarismesh/internal/mock/mock_api"

	"github.com/golang/mock/gomock"
	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestFactoryRegistry(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	err := register(
		mock_api.NewMockProviderAPI(c),
		nil,
		&FactoryConfig{
			Services: []Service{{
				ServiceName: t.Name(),
				Token:       "token",
			}},
		})
	require.NoError(t, err)
}

func TestFactoryRegistrationIsAtomic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	firstService := t.Name() + "-first"
	err := register(
		mock_api.NewMockProviderAPI(ctrl),
		nil,
		&FactoryConfig{
			Services: []Service{
				{ServiceName: firstService},
				{},
			},
		})
	require.Error(t, err)
	require.Nil(t, registry.Get(firstService))
}

func TestFactoryRejectsDuplicateServiceName(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service := t.Name()
	err := register(
		mock_api.NewMockProviderAPI(ctrl),
		nil,
		&FactoryConfig{
			Services: []Service{
				{ServiceName: service},
				{ServiceName: service},
			},
		})
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate registry service name")
	require.Nil(t, registry.Get(service))
}

func TestHeartbeatIntervalValidation(t *testing.T) {
	tests := []struct {
		name              string
		heartbeatInterval int
		endpointInterval  int
		wantError         bool
	}{
		{name: "negative", heartbeatInterval: -1, wantError: true},
		{name: "sub second", heartbeatInterval: 500, wantError: true},
		{name: "not whole second", heartbeatInterval: 1500, wantError: true},
		{name: "zero inherits", heartbeatInterval: 0},
		{name: "whole second", heartbeatInterval: 2000},
		{
			name:             "invalid endpoint",
			endpointInterval: 2500,
			wantError:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			service := Service{
				ServiceName:       t.Name(),
				HeartbeatInterval: tt.heartbeatInterval,
			}
			if tt.endpointInterval != 0 {
				service.Endpoints = []EndpointConfig{{
					Address:           "127.0.0.1:8080",
					Protocol:          "trpc",
					HeartbeatInterval: tt.endpointInterval,
				}}
			}
			err := register(
				mock_api.NewMockProviderAPI(ctrl),
				nil,
				&FactoryConfig{
					EnableRegister: true,
					Services:       []Service{service},
				})
			if tt.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestSetup(t *testing.T) {
	factory := &RegistryFactory{}
	assert.Equal(t, factory.Type(), "registry")
	assert.NotNil(t, factory.Setup("registry", nil))
}

func TestConfigSetup(t *testing.T) {
	cfgstr := `
plugins:
  registry:
    polarismesh:
      connect_timeout: 1000
      message_timeout: 1s
      join_point: default
      disable_health_check: true
      address_list: "not_exist"
`
	cfg := trpc.Config{}
	err := yaml.Unmarshal([]byte(cfgstr), &cfg)
	require.NoError(t, err)
	polarisCfg := cfg.Plugins["registry"]["polarismesh"]
	pluginFac := &RegistryFactory{}
	err = pluginFac.Setup("polarismesh", &polarisCfg)
	require.NoError(t, err)
	require.NotNil(t, pluginFac.GetSDKCtx())
}

func TestNew(t *testing.T) {
	_, err := newRegistry(nil, &Config{ServiceName: t.Name()})
	require.NoError(t, err)

	_, err = newRegistry(nil, &Config{})
	require.Error(t, err)

	_, err = newRegistry(nil, nil)
	require.Error(t, err)
}

func TestRegistryRemainsComparable(t *testing.T) {
	first := Registry{}
	second := Registry{}
	require.True(t, first == second)
}

func TestRegisterDeRegister(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	p := mock_api.NewMockProviderAPI(ctrl)
	p.EXPECT().Register(gomock.Any()).DoAndReturn(
		func(req *api.InstanceRegisterRequest) (*model.InstanceRegisterResponse, error) {
			require.Equal(t, "127.0.0.1", req.Host)
			require.Equal(t, 8080, req.Port)
			return &model.InstanceRegisterResponse{InstanceID: "instance-id"}, nil
		}).Times(1)
	p.EXPECT().Heartbeat(gomock.Any()).Return(nil).AnyTimes()
	p.EXPECT().Deregister(gomock.Any()).DoAndReturn(
		func(req *api.InstanceDeRegisterRequest) error {
			require.Equal(t, "instance-id", req.InstanceID)
			return nil
		}).Times(1)

	r, err := NewRegistry(p, &Config{
		EnableRegister: true,
		ServiceName:    t.Name(),
		BindAddress:    "127.0.0.1:8080",
		HeartBeat:      1,
	})
	require.NoError(t, err)
	require.NoError(t, r.Register(""))
	require.NoError(t, r.Deregister(""))
	require.Empty(t, r.state.instances)
}

func TestFlexDepends(t *testing.T) {
	require.Equal(t, []string{"selector-polarismesh"}, (&RegistryFactory{}).FlexDependsOn())
}

func TestRegistryInstanceLocation(t *testing.T) {
	const region, zone, campus = "China", "Guangzhou", "Shenzhen"

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	newProviderAPIByContext := api.NewProviderAPIByContext
	api.NewProviderAPIByContext = func(_ api.SDKContext) api.ProviderAPI {
		mocked := mock_api.NewMockProviderAPI(ctrl)
		mocked.EXPECT().Heartbeat(gomock.Any()).Return(nil).AnyTimes()
		mocked.EXPECT().Deregister(gomock.Any()).Return(nil).AnyTimes()
		mocked.EXPECT().Register(gomock.Not(gomock.Nil())).DoAndReturn(
			func(instance *api.InstanceRegisterRequest) (*model.InstanceRegisterResponse, error) {
				require.Equal(t, region, instance.Location.Region)
				require.Equal(t, zone, instance.Location.Zone)
				require.Equal(t, campus, instance.Location.Campus)
				require.NotNil(t, instance.Version)
				require.Equal(t, "v1.2.3", *instance.Version)
				return &model.InstanceRegisterResponse{InstanceID: "instance_id"}, nil
			}).Times(1)
		return mocked
	}
	defer func() { api.NewProviderAPIByContext = newProviderAPIByContext }()

	node := yaml.Node{}
	require.NoError(t, yaml.Unmarshal([]byte(fmt.Sprintf(`
register_self: true
instance_location:
  region: %s
  zone: %s
  campus: %s
address_list: "not_exist"
service:
  - name: %s
    token: "xxx"
    namespace: Development
    version: v1.2.3
    bind_address: 127.0.0.1:8080
`, region, zone, campus, t.Name())), &node))
	require.NoError(t, (&RegistryFactory{}).Setup("polarismesh", &plugin.YamlNodeDecoder{Node: &node}))

	r := registry.Get(t.Name())
	require.NotNil(t, r)
	require.NoError(t, r.Register(""))
	require.NoError(t, r.Deregister(""))
}

func TestHealthCheckHeartbeat(t *testing.T) {
	t.Run("service unregistered to healthcheck heart beat immediately", func(t *testing.T) {
		heartbeat := make(chan struct{}, 4)
		service := t.Name() + "-unregistered"
		r, err := NewRegistry(&provider{heartbeat: heartbeat}, &Config{
			ServiceName:  service,
			BindAddress:  "127.0.0.1:8080",
			HeartBeat:    10,
			ServiceToken: "token",
			InstanceID:   "existing-instance",
		})
		require.NoError(t, err)
		require.NoError(t, r.Register(""))
		select {
		case <-time.After(time.Second):
			require.FailNow(t, "heartbeat must be called immediately")
		case <-heartbeat:
		}
		require.NoError(t, r.Deregister(""))
	})

	t.Run("service registered to healthcheck heart beat on serving", func(t *testing.T) {
		heartbeat := make(chan struct{}, 4)
		service := t.Name() + "-serving"
		r, err := NewRegistry(&provider{heartbeat: heartbeat}, &Config{
			ServiceName:  service,
			BindAddress:  "127.0.0.1:8080",
			HeartBeat:    1,
			ServiceToken: "token",
			InstanceID:   "existing-instance",
		})
		require.NoError(t, err)

		hc := healthcheck.New(healthcheck.WithStatusWatchers(healthcheck.GetWatchers()))
		update, err := hc.Register(service)
		require.NoError(t, err)
		require.NoError(t, r.Register(""))

		select {
		case <-time.After(time.Second * 2):
		case <-heartbeat:
			require.FailNow(t, "the first heartbeat should not start before service serving")
		}

		update(healthcheck.Serving)
		select {
		case <-time.After(time.Second):
			require.FailNow(t, "heartbeat should start immediately when service start serving")
		case <-heartbeat:
		}
		require.NoError(t, r.Deregister(""))
	})
}

func TestAutoHeartbeatFollowsHealthStatus(t *testing.T) {
	heartbeat := make(chan *api.InstanceHeartbeatRequest, 4)
	service := t.Name()
	r, err := NewRegistry(&recordingProvider{heartbeat: heartbeat}, &Config{
		EnableRegister: true,
		ServiceName:    service,
		BindAddress:    "127.0.0.1:8080",
		HeartBeat:      10,
		InstanceID:     "auto-instance",
		AutoHeartbeat:  true,
	})
	require.NoError(t, err)

	hc := healthcheck.New(healthcheck.WithStatusWatchers(healthcheck.GetWatchers()))
	update, err := hc.Register(service)
	require.NoError(t, err)
	update(healthcheck.NotServing)
	require.NoError(t, r.Register(""))

	select {
	case <-heartbeat:
		require.FailNow(t, "auto heartbeat must not start before service serving")
	case <-time.After(200 * time.Millisecond):
	}

	update(healthcheck.Serving)
	select {
	case req := <-heartbeat:
		require.Equal(t, "auto-instance", req.InstanceID)
	case <-time.After(time.Second):
		require.FailNow(t, "auto heartbeat must resume immediately when service serving")
	}

	update(healthcheck.NotServing)
	select {
	case <-heartbeat:
		require.FailNow(t, "auto heartbeat must pause when service is not serving")
	case <-time.After(200 * time.Millisecond):
	}

	update(healthcheck.Serving)
	select {
	case req := <-heartbeat:
		require.Equal(t, "auto-instance", req.InstanceID)
	case <-time.After(time.Second):
		require.FailNow(t, "auto heartbeat must resume after service recovery")
	}
	require.NoError(t, r.Deregister(""))
}

func TestAutoHeartbeatUsesInstanceIDOnSharedAddress(t *testing.T) {
	p := &sharedAddressProvider{
		registered:   make(chan string, 2),
		heartbeat:    make(chan string, 2),
		deregistered: make(chan string, 2),
	}
	r, err := NewRegistry(p, &Config{
		EnableRegister: true,
		ServiceName:    t.Name(),
		HeartBeat:      10,
		AutoHeartbeat:  true,
		Endpoints: []Endpoint{
			{Address: "127.0.0.1:8080", Protocol: "trpc", InstanceID: "trpc-instance"},
			{Address: "127.0.0.1:8080", Protocol: "http", InstanceID: "http-instance"},
		},
	})
	require.NoError(t, err)
	require.NoError(t, r.Register(""))

	got := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case id := <-p.registered:
			got[id] = true
		case <-time.After(time.Second):
			require.FailNow(t, "expected both shared-address instances to register")
		}
	}
	require.True(t, got["trpc-instance"])
	require.True(t, got["http-instance"])

	got = map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case id := <-p.heartbeat:
			got[id] = true
		case <-time.After(time.Second):
			require.FailNow(t, "expected both explicit IDs to receive heartbeat")
		}
	}
	require.True(t, got["trpc-instance"])
	require.True(t, got["http-instance"])

	require.NoError(t, r.Deregister(""))
	for i := 0; i < 2; i++ {
		select {
		case <-p.deregistered:
		case <-time.After(time.Second):
			require.FailNow(t, "expected both explicit IDs to deregister")
		}
	}
}

func TestRegistryZeroWeight(t *testing.T) {
	const weight = 0
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	newProviderAPIByContext := api.NewProviderAPIByContext
	api.NewProviderAPIByContext = func(_ api.SDKContext) api.ProviderAPI {
		mocked := mock_api.NewMockProviderAPI(ctrl)
		mocked.EXPECT().Heartbeat(gomock.Any()).Return(nil).AnyTimes()
		mocked.EXPECT().Deregister(gomock.Any()).Return(nil).AnyTimes()
		mocked.EXPECT().Register(gomock.Not(gomock.Nil())).DoAndReturn(
			func(req *api.InstanceRegisterRequest) (*model.InstanceRegisterResponse, error) {
				require.NotNil(t, req.Weight)
				require.Equal(t, weight, *req.Weight)
				return &model.InstanceRegisterResponse{InstanceID: "instance-id"}, nil
			}).Times(1)
		return mocked
	}
	defer func() { api.NewProviderAPIByContext = newProviderAPIByContext }()

	node := yaml.Node{}
	require.NoError(t, yaml.Unmarshal([]byte(fmt.Sprintf(`
register_self: true
heartbeat_interval: 3000
protocol: grpc
address_list: "not_exist"
service:
  - name: %s
    namespace: Development
    token: xxxxxxxxxxxxxxxxxxx
    instance_id: xxxxxxxxxxxxxxxx
    weight: %d
    bind_address: 127.0.0.1:8080
`, t.Name(), weight)), &node))
	require.NoError(t, (&RegistryFactory{}).Setup("polarismesh", &plugin.YamlNodeDecoder{Node: &node}))
	r := registry.Get(t.Name())
	require.NoError(t, r.Register(t.Name()))
	require.NoError(t, r.Deregister(t.Name()))
}

func TestRegisterMergesMetadataAndVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service := t.Name()
	require.NoError(t, RegisterMetadataProvider(service, MetadataProviderFunc(func() (map[string]string, error) {
		return map[string]string{
			"env":   "dynamic",
			"owner": "customer-agent",
		}, nil
	})))
	defer UnregisterMetadataProvider(service)

	providerAPI := mock_api.NewMockProviderAPI(ctrl)
	providerAPI.EXPECT().Register(gomock.Any()).DoAndReturn(
		func(req *api.InstanceRegisterRequest) (*model.InstanceRegisterResponse, error) {
			require.Equal(t, "dynamic", req.Metadata["env"])
			require.Equal(t, "orders", req.Metadata["business"])
			require.Equal(t, "customer-agent", req.Metadata["owner"])
			require.NotNil(t, req.Version)
			require.Equal(t, "v2.0.0", *req.Version)
			return &model.InstanceRegisterResponse{InstanceID: "instance-id"}, nil
		}).Times(1)
	providerAPI.EXPECT().Heartbeat(gomock.Any()).Return(nil).AnyTimes()
	providerAPI.EXPECT().Deregister(gomock.Any()).Return(nil).Times(1)

	static := map[string]string{"env": "static", "business": "orders"}
	r, err := NewRegistry(providerAPI, &Config{
		EnableRegister: true,
		ServiceName:    service,
		BindAddress:    "127.0.0.1:8080",
		Version:        "v2.0.0",
		Metadata:       static,
	})
	require.NoError(t, err)
	require.NoError(t, r.Register(""))
	require.Equal(t, "static", static["env"])
	require.NoError(t, r.Deregister(""))
}

func TestRegisterProviderFailurePreventsRegistrationAndHeartbeat(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service := t.Name()
	providerErr := errors.New("agent card unavailable")
	require.NoError(t, RegisterMetadataProvider(service, MetadataProviderFunc(func() (map[string]string, error) {
		return nil, providerErr
	})))
	defer UnregisterMetadataProvider(service)

	providerAPI := mock_api.NewMockProviderAPI(ctrl)
	r, err := NewRegistry(providerAPI, &Config{
		EnableRegister: true,
		ServiceName:    service,
		BindAddress:    "127.0.0.1:8080",
	})
	require.NoError(t, err)
	err = r.Register("")
	require.ErrorIs(t, err, providerErr)
	require.Empty(t, r.state.instances)
}

func TestRegistryMultipleEndpoints(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	autoHeartbeat := true
	weightA, weightB := 80, 120
	p := mock_api.NewMockProviderAPI(ctrl)
	p.EXPECT().Register(gomock.Any()).DoAndReturn(
		func(req *api.InstanceRegisterRequest) (*model.InstanceRegisterResponse, error) {
			require.Equal(t, "service-host-a", req.Host)
			require.Equal(t, 8001, req.Port)
			require.Equal(t, "trpc", *req.Protocol)
			require.Equal(t, "v1", *req.Version)
			require.Equal(t, 80, *req.Weight)
			require.Equal(t, 6, *req.TTL)
			require.Equal(t, "token-a", req.ServiceToken)
			require.Equal(t, "service-metadata", req.Metadata["scope"])
			require.Equal(t, "trpc", req.Metadata["transport"])
			require.Empty(t, req.InstanceId)
			return &model.InstanceRegisterResponse{InstanceID: "instance-a"}, nil
		}).Times(1)
	p.EXPECT().Register(gomock.Any()).DoAndReturn(
		func(req *api.InstanceRegisterRequest) (*model.InstanceRegisterResponse, error) {
			require.Equal(t, "service-host-a", req.Host)
			require.Equal(t, 8002, req.Port)
			require.Equal(t, "http", *req.Protocol)
			require.Equal(t, "v2", *req.Version)
			require.Equal(t, 120, *req.Weight)
			require.Equal(t, 7, *req.TTL)
			require.Equal(t, "token-b", req.ServiceToken)
			require.Equal(t, "http-stable", req.InstanceId)
			require.Equal(t, "http", req.Metadata["transport"])
			return &model.InstanceRegisterResponse{InstanceID: "http-stable"}, nil
		}).Times(1)
	p.EXPECT().Heartbeat(gomock.Any()).Return(nil).AnyTimes()
	p.EXPECT().Deregister(gomock.Any()).Return(nil).Times(2)

	r, err := NewRegistry(p, &Config{
		EnableRegister: true,
		ServiceName:    t.Name(),
		Namespace:      "test",
		Metadata:       map[string]string{"scope": "service-metadata"},
		Endpoints: []Endpoint{
			{
				Address:      "service-host-a:8001",
				Protocol:     "trpc",
				Version:      "v1",
				ServiceToken: "token-a",
				Weight:       &weightA,
				TTL:          6,
				HeartBeat:    2,
				Metadata:     map[string]string{"transport": "trpc"},
			},
			{
				Address:       "service-host-a:8002",
				Protocol:      "http",
				Version:       "v2",
				ServiceToken:  "token-b",
				InstanceID:    "http-stable",
				Weight:        &weightB,
				TTL:           7,
				AutoHeartbeat: &autoHeartbeat,
				Metadata:      map[string]string{"transport": "http"},
			},
		},
	})
	require.NoError(t, err)
	require.NoError(t, r.Register(""))
	require.Len(t, r.state.instances, 2)
	require.Equal(t, "instance-a", r.state.instances[0].instanceID)
	require.Equal(t, "http-stable", r.state.instances[1].instanceID)
	require.NoError(t, r.Deregister(""))
	require.Empty(t, r.state.instances)
}

func TestRegisterPartialFailureRollsBack(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	registerErr := errors.New("second endpoint unavailable")
	p := mock_api.NewMockProviderAPI(ctrl)
	p.EXPECT().Register(gomock.Any()).Return(
		&model.InstanceRegisterResponse{InstanceID: "instance-a"}, nil).Times(1)
	p.EXPECT().Register(gomock.Any()).Return(nil, registerErr).Times(1)
	p.EXPECT().Deregister(gomock.Any()).DoAndReturn(
		func(req *api.InstanceDeRegisterRequest) error {
			require.Equal(t, "instance-a", req.InstanceID)
			return nil
		}).Times(1)

	r, err := NewRegistry(p, &Config{
		EnableRegister: true,
		ServiceName:    t.Name(),
		Endpoints: []Endpoint{
			{Address: "service-host-a:8001", Protocol: "trpc", HeartBeat: 2},
			{Address: "service-host-a:8002", Protocol: "http", HeartBeat: 2},
		},
	})
	require.NoError(t, err)
	err = r.Register("")
	require.ErrorIs(t, err, registerErr)
	require.Empty(t, r.state.instances)
}

func TestRegisterRollbackFailureRetainsRetryableInstance(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	registerErr := errors.New("second endpoint unavailable")
	rollbackErr := errors.New("deregister unavailable")
	p := mock_api.NewMockProviderAPI(ctrl)
	p.EXPECT().Register(gomock.Any()).Return(
		&model.InstanceRegisterResponse{InstanceID: "instance-a"}, nil).Times(1)
	p.EXPECT().Register(gomock.Any()).Return(nil, registerErr).Times(1)
	p.EXPECT().Deregister(gomock.Any()).Return(rollbackErr).Times(1)
	p.EXPECT().Deregister(gomock.Any()).Return(nil).Times(1)

	r, err := NewRegistry(p, &Config{
		EnableRegister: true,
		ServiceName:    t.Name(),
		Endpoints: []Endpoint{
			{Address: "service-host-a:8001", Protocol: "trpc", HeartBeat: 2},
			{Address: "service-host-a:8002", Protocol: "http", HeartBeat: 2},
		},
	})
	require.NoError(t, err)
	err = r.Register("")
	require.ErrorIs(t, err, registerErr)
	require.Contains(t, err.Error(), rollbackErr.Error())
	require.Len(t, r.state.instances, 1)
	require.NoError(t, r.Deregister(""))
	require.Empty(t, r.state.instances)
}

func TestRegisterAmbiguousFailureCleansKnownInstance(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	innerErr := model.NewSDKError(model.ErrCodeNetworkError, context.DeadlineExceeded, "network timeout")
	registerErr := model.NewSDKError(model.ErrCodeAPITimeoutError, innerErr, "register timeout")
	require.False(t, errors.Is(registerErr, context.DeadlineExceeded))

	p := mock_api.NewMockProviderAPI(ctrl)
	p.EXPECT().Register(gomock.Any()).Return(nil, registerErr).Times(1)
	p.EXPECT().Deregister(gomock.Any()).DoAndReturn(
		func(req *api.InstanceDeRegisterRequest) error {
			require.Equal(t, "known-instance", req.InstanceID)
			return nil
		}).Times(1)

	r, err := NewRegistry(p, &Config{
		EnableRegister:     true,
		ServiceName:        t.Name(),
		BindAddress:        "127.0.0.1:8080",
		InstanceID:         "known-instance",
		DisableHealthCheck: true,
	})
	require.NoError(t, err)
	err = r.Register("")
	require.ErrorIs(t, err, registerErr)
	require.Empty(t, r.state.instances)
}

func TestRegisterValidationFailureDoesNotDeregisterStableInstance(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	weight := -1
	p := mock_api.NewMockProviderAPI(ctrl)
	p.EXPECT().Register(gomock.Any()).Times(0)
	p.EXPECT().Deregister(gomock.Any()).Times(0)

	r, err := NewRegistry(p, &Config{
		EnableRegister:     true,
		ServiceName:        t.Name(),
		BindAddress:        "127.0.0.1:8080",
		InstanceID:         "stable-instance",
		Weight:             &weight,
		DisableHealthCheck: true,
	})
	require.NoError(t, err)
	err = r.Register("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "weight should be in range")
	require.Empty(t, r.state.instances)
}

func TestRegisterAmbiguousFailureCleanupFailureRetainsPendingInstance(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	innerErr := model.NewSDKError(model.ErrCodeNetworkError, context.DeadlineExceeded, "network timeout")
	registerErr := model.NewSDKError(model.ErrCodeAPITimeoutError, innerErr, "register timeout")
	cleanupErr := errors.New("deregister unavailable")
	p := mock_api.NewMockProviderAPI(ctrl)
	p.EXPECT().Register(gomock.Any()).Return(nil, registerErr).Times(1)
	p.EXPECT().Deregister(gomock.Any()).DoAndReturn(
		func(req *api.InstanceDeRegisterRequest) error {
			require.Equal(t, "known-instance", req.InstanceID)
			return cleanupErr
		}).Times(1)
	p.EXPECT().Deregister(gomock.Any()).DoAndReturn(
		func(req *api.InstanceDeRegisterRequest) error {
			require.Equal(t, "known-instance", req.InstanceID)
			return nil
		}).Times(1)

	r, err := NewRegistry(p, &Config{
		EnableRegister:     true,
		ServiceName:        t.Name(),
		BindAddress:        "127.0.0.1:8080",
		InstanceID:         "known-instance",
		DisableHealthCheck: true,
	})
	require.NoError(t, err)
	err = r.Register("")
	require.ErrorIs(t, err, registerErr)
	require.Contains(t, err.Error(), cleanupErr.Error())
	require.Len(t, r.state.instances, 1)
	require.Nil(t, r.state.instances[0].cancel)
	require.Error(t, r.Register(""))

	require.NoError(t, r.Deregister(""))
	require.Empty(t, r.state.instances)
}

func TestRegisterRejectsDuplicateEndpointIdentity(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	p := mock_api.NewMockProviderAPI(ctrl)
	_, err := NewRegistry(p, &Config{
		EnableRegister: true,
		ServiceName:    t.Name(),
		Endpoints: []Endpoint{
			{Address: "service-host-a:8001", Protocol: "trpc"},
			{Address: "service-host-a:8001", Protocol: "trpc"},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate identity")
}

func TestRegisterRejectsSharedAddressWithoutExplicitIDs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	p := mock_api.NewMockProviderAPI(ctrl)
	_, err := NewRegistry(p, &Config{
		EnableRegister: true,
		ServiceName:    t.Name(),
		Endpoints: []Endpoint{
			{Address: "service-host-a:8001", Protocol: "trpc"},
			{Address: "service-host-a:8001", Protocol: "http"},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "explicit instance_id")
}

func TestRegistryAllowsSharedAddressWithExplicitIDs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	p := mock_api.NewMockProviderAPI(ctrl)
	p.EXPECT().Register(gomock.Any()).DoAndReturn(
		func(req *api.InstanceRegisterRequest) (*model.InstanceRegisterResponse, error) {
			require.Equal(t, "service-host-a:8001", req.Host+":"+fmt.Sprint(req.Port))
			require.NotEmpty(t, req.InstanceId)
			return &model.InstanceRegisterResponse{InstanceID: req.InstanceId}, nil
		}).Times(2)
	p.EXPECT().Deregister(gomock.Any()).Return(nil).Times(2)

	r, err := NewRegistry(p, &Config{
		EnableRegister:     true,
		ServiceName:        t.Name(),
		DisableHealthCheck: true,
		Endpoints: []Endpoint{
			{Address: "service-host-a:8001", Protocol: "trpc", InstanceID: "trpc-id"},
			{Address: "service-host-a:8001", Protocol: "http", InstanceID: "http-id"},
		},
	})
	require.NoError(t, err)
	require.NoError(t, r.Register(""))
	require.Len(t, r.state.instances, 2)
	require.NoError(t, r.Deregister(""))
	require.Empty(t, r.state.instances)
}

func TestRegisterRejectsReturnedInstanceIDCollision(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	p := mock_api.NewMockProviderAPI(ctrl)
	p.EXPECT().Register(gomock.Any()).Return(
		&model.InstanceRegisterResponse{InstanceID: "duplicate-id"}, nil).Times(2)
	p.EXPECT().Deregister(gomock.Any()).DoAndReturn(
		func(req *api.InstanceDeRegisterRequest) error {
			require.Equal(t, "duplicate-id", req.InstanceID)
			return nil
		}).Times(1)

	r, err := NewRegistry(p, &Config{
		EnableRegister:     true,
		ServiceName:        t.Name(),
		DisableHealthCheck: true,
		Endpoints: []Endpoint{
			{Address: "service-host-a:8001", Protocol: "trpc"},
			{Address: "service-host-a:8002", Protocol: "http"},
		},
	})
	require.NoError(t, err)
	err = r.Register("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate instance id")
	require.Empty(t, r.state.instances)
}

func TestRegisterRejectsReturnedIDReservedByLaterEndpoint(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	p := mock_api.NewMockProviderAPI(ctrl)
	p.EXPECT().Register(gomock.Any()).Return(
		&model.InstanceRegisterResponse{InstanceID: "reserved-id"}, nil).Times(1)
	p.EXPECT().Deregister(gomock.Any()).DoAndReturn(
		func(req *api.InstanceDeRegisterRequest) error {
			require.Equal(t, "reserved-id", req.InstanceID)
			return nil
		}).Times(1)

	r, err := NewRegistry(p, &Config{
		EnableRegister:     true,
		ServiceName:        t.Name(),
		DisableHealthCheck: true,
		Endpoints: []Endpoint{
			{Address: "service-host-a:8001", Protocol: "trpc"},
			{Address: "service-host-a:8002", Protocol: "http", InstanceID: "reserved-id"},
		},
	})
	require.NoError(t, err)
	err = r.Register("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "reserved by endpoint")
	require.Empty(t, r.state.instances)
}

func TestRegisterPreservesOpaqueMetadataWithEndpoints(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service := t.Name()
	metadata := map[string]string{
		"descriptor": "{not json",
	}
	p := mock_api.NewMockProviderAPI(ctrl)
	p.EXPECT().Register(gomock.Any()).DoAndReturn(func(req *api.InstanceRegisterRequest) (*model.InstanceRegisterResponse, error) {
		require.Equal(t, metadata, req.Metadata)
		return &model.InstanceRegisterResponse{InstanceID: fmt.Sprint(req.Port)}, nil
	}).Times(2)
	p.EXPECT().Deregister(gomock.Any()).Return(nil).Times(2)
	r, err := NewRegistry(p, &Config{
		EnableRegister:     true,
		ServiceName:        service,
		DisableHealthCheck: true,
		Endpoints: []Endpoint{
			{Address: "service-host-a:8001", Protocol: "trpc", Metadata: metadata},
			{Address: "service-host-a:8002", Protocol: "http", Metadata: metadata},
		},
	})
	require.NoError(t, err)
	require.NoError(t, r.Register(""))
	require.NoError(t, r.Deregister(""))
}

func TestRegistrationSnapshotAppliesAtomicFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service := t.Name()
	require.NoError(t, RegisterMetadataProvider(service, RegistrationSnapshotProviderFunc(
		func() (RegistrationSnapshot, error) {
			return RegistrationSnapshot{
				Metadata:         map[string]string{"deployment": "green"},
				Version:          "dynamic-v2",
				RegistrationPort: 9000,
				Protocol:         "http",
			}, nil
		})))
	defer UnregisterMetadataProvider(service)

	p := mock_api.NewMockProviderAPI(ctrl)
	p.EXPECT().Register(gomock.Any()).DoAndReturn(
		func(req *api.InstanceRegisterRequest) (*model.InstanceRegisterResponse, error) {
			require.Equal(t, 9000, req.Port)
			require.Equal(t, "dynamic-v2", *req.Version)
			require.Equal(t, "http", *req.Protocol)
			require.Equal(t, "green", req.Metadata["deployment"])
			return &model.InstanceRegisterResponse{InstanceID: "ordinary-id"}, nil
		}).Times(1)
	p.EXPECT().Deregister(gomock.Any()).Return(nil).Times(1)

	r, err := NewRegistry(p, &Config{
		EnableRegister:     true,
		ServiceName:        service,
		BindAddress:        "service-host-a:8000",
		Version:            "static-v1",
		InstanceProtocol:   "trpc",
		DisableHealthCheck: true,
	})
	require.NoError(t, err)
	require.NoError(t, r.Register(""))
	require.Equal(t, "dynamic-v2", r.Version())
	require.NoError(t, r.Deregister(""))
}

func TestRegistrationSnapshotRejectsUnsupportedAtomicFields(t *testing.T) {
	tests := []struct {
		name      string
		snapshot  RegistrationSnapshot
		endpoints []Endpoint
		want      string
	}{
		{
			name: "port out of range",
			snapshot: RegistrationSnapshot{
				Metadata:         map[string]string{"deployment": "green"},
				RegistrationPort: 65536,
			},
			want: "1..65535",
		},
		{
			name: "shared snapshot port",
			snapshot: RegistrationSnapshot{
				Metadata:         map[string]string{"deployment": "green"},
				RegistrationPort: 9000,
			},
			endpoints: []Endpoint{
				{Address: "service-host-a:8001", Protocol: "trpc"},
				{Address: "service-host-a:8002", Protocol: "http"},
			},
			want: "exactly one endpoint",
		},
		{
			name: "whitespace protocol",
			snapshot: RegistrationSnapshot{
				Metadata: map[string]string{"deployment": "green"},
				Protocol: " http",
			},
			want: "surrounding whitespace",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			service := t.Name()
			require.NoError(t, RegisterMetadataProvider(service, RegistrationSnapshotProviderFunc(
				func() (RegistrationSnapshot, error) {
					return tt.snapshot, nil
				})))
			defer UnregisterMetadataProvider(service)

			endpoints := tt.endpoints
			if len(endpoints) == 0 {
				endpoints = []Endpoint{{Address: "service-host-a:8000", Protocol: "trpc"}}
			}
			r, err := NewRegistry(mock_api.NewMockProviderAPI(ctrl), &Config{
				EnableRegister:     true,
				ServiceName:        service,
				DisableHealthCheck: true,
				Endpoints:          endpoints,
			})
			require.NoError(t, err)
			err = r.Register("")
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestRegisterAcceptsDifferentEndpointMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	p := mock_api.NewMockProviderAPI(ctrl)
	p.EXPECT().Register(gomock.Any()).DoAndReturn(func(req *api.InstanceRegisterRequest) (*model.InstanceRegisterResponse, error) {
		return &model.InstanceRegisterResponse{InstanceID: fmt.Sprint(req.Port)}, nil
	}).Times(2)
	p.EXPECT().Deregister(gomock.Any()).Return(nil).Times(2)
	r, err := NewRegistry(p, &Config{
		EnableRegister:     true,
		ServiceName:        t.Name(),
		DisableHealthCheck: true,
		Endpoints: []Endpoint{
			{
				Address:  "service-host-a:8001",
				Protocol: "http",
				Metadata: map[string]string{
					"endpoint-kind": "search",
				},
			},
			{
				Address:  "service-host-a:8002",
				Protocol: "trpc",
			},
		},
	})
	require.NoError(t, err)
	require.NoError(t, r.Register(""))
	require.NoError(t, r.Deregister(""))
}

func TestNewRegistryRejectsOversizedInstanceProtocol(t *testing.T) {
	_, err := NewRegistry(nil, &Config{
		ServiceName: t.Name(),
		Endpoints: []Endpoint{{
			Address:  "127.0.0.1:8080",
			Protocol: strings.Repeat("p", maxInstanceProtocolLength+1),
		}},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds maximum")
}

func TestNewRegistryRejectsEndpointsWithBindAddress(t *testing.T) {
	_, err := NewRegistry(nil, &Config{
		ServiceName: t.Name(),
		BindAddress: "127.0.0.1:8080",
		Endpoints: []Endpoint{{
			Address:  "127.0.0.1:8081",
			Protocol: "trpc",
		}},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot be combined")
}

func TestHeartbeatOnlyRequiresInstanceID(t *testing.T) {
	r, err := NewRegistry(&recordingProvider{heartbeat: make(chan *api.InstanceHeartbeatRequest, 1)}, &Config{
		ServiceName: t.Name(),
		BindAddress: "127.0.0.1:8080",
	})
	require.NoError(t, err)
	err = r.Register("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires instance_id")
}

func TestConcurrentNewRegistryHealthcheckWatch(t *testing.T) {
	const count = 32
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := NewRegistry(nil, &Config{
				ServiceName: fmt.Sprintf("%s-%d", t.Name(), i),
			})
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}

func TestRegisterFactoryParsesMultipleEndpoints(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	newProviderAPIByContext := api.NewProviderAPIByContext
	api.NewProviderAPIByContext = func(_ api.SDKContext) api.ProviderAPI {
		mocked := mock_api.NewMockProviderAPI(ctrl)
		mocked.EXPECT().Register(gomock.Any()).Return(
			&model.InstanceRegisterResponse{InstanceID: "instance-a"}, nil).Times(1)
		mocked.EXPECT().Register(gomock.Any()).Return(
			&model.InstanceRegisterResponse{InstanceID: "instance-b"}, nil).Times(1)
		mocked.EXPECT().Heartbeat(gomock.Any()).Return(nil).AnyTimes()
		mocked.EXPECT().Deregister(gomock.Any()).Return(nil).Times(2)
		return mocked
	}
	defer func() { api.NewProviderAPIByContext = newProviderAPIByContext }()

	node := yaml.Node{}
	require.NoError(t, yaml.Unmarshal([]byte(fmt.Sprintf(`
register_self: true
heartbeat_interval: 2000
address_list: "not_exist"
service:
  - name: %s
    namespace: Development
    version: v1
    instance_protocol: trpc
    metadata:
      service: metadata
    endpoints:
      - address: 127.0.0.1:8081
        protocol: trpc
      - address: 127.0.0.1:8082
        protocol: http
        version: v2
        weight: 120
        metadata:
          transport: http
`, t.Name())), &node))
	require.NoError(t, (&RegistryFactory{}).Setup("polarismesh", &plugin.YamlNodeDecoder{Node: &node}))
	r := registry.Get(t.Name())
	require.NoError(t, r.Register(t.Name()))
	require.NoError(t, r.Deregister(t.Name()))
}

func TestRegistrySDKContextAndFromFramework(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	sdkCtx := mock_api.NewMockSDKContext(ctrl)
	r := &Registry{sdkCtx: sdkCtx, cfg: &Config{Version: "1.2.3"}}
	require.Same(t, sdkCtx, r.SDKContext())
	require.Equal(t, "1.2.3", r.Version())

	service := t.Name()
	registry.Register(service, r)
	got, ok := FromFramework(service)
	require.True(t, ok)
	require.Same(t, r, got)
	require.Same(t, sdkCtx, got.SDKContext())

	missing, ok := FromFramework(service + "-missing")
	require.False(t, ok)
	require.Nil(t, missing)

	otherService := service + "-other"
	registry.Register(otherService, &registry.NoopRegistry{})
	other, ok := FromFramework(otherService)
	require.False(t, ok)
	require.Nil(t, other)
	require.Nil(t, (*Registry)(nil).SDKContext())
}

func TestLegacyHeartbeatOnlyRegistration(t *testing.T) {
	heartbeat := make(chan *api.InstanceHeartbeatRequest, 1)
	r, err := NewRegistry(&recordingProvider{heartbeat: heartbeat}, &Config{
		ServiceName:  t.Name(),
		BindAddress:  "127.0.0.1:8080",
		InstanceID:   "existing-instance",
		ServiceToken: "token",
	})
	require.NoError(t, err)
	require.NoError(t, r.Register(""))
	select {
	case req := <-heartbeat:
		require.Equal(t, "existing-instance", req.InstanceID)
		require.Equal(t, "127.0.0.1", req.Host)
		require.Equal(t, 8080, req.Port)
	case <-time.After(time.Second):
		require.FailNow(t, "heartbeat was not reported")
	}
	require.NoError(t, r.Deregister(""))
}

func TestDisableHealthCheckSkipsTTLAndHeartbeat(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	p := mock_api.NewMockProviderAPI(ctrl)
	p.EXPECT().Register(gomock.Any()).DoAndReturn(
		func(req *api.InstanceRegisterRequest) (*model.InstanceRegisterResponse, error) {
			require.Nil(t, req.TTL)
			return &model.InstanceRegisterResponse{InstanceID: "instance-id"}, nil
		}).Times(1)
	p.EXPECT().Heartbeat(gomock.Any()).Times(0)
	p.EXPECT().Deregister(gomock.Any()).Return(nil).Times(1)

	r, err := NewRegistry(p, &Config{
		EnableRegister:     true,
		ServiceName:        t.Name(),
		BindAddress:        "127.0.0.1:8080",
		DisableHealthCheck: true,
	})
	require.NoError(t, err)
	require.NoError(t, r.Register(""))
	require.NoError(t, r.Deregister(""))
}

func TestHeartbeatFailureStateAndRecovery(t *testing.T) {
	heartbeatErr := errors.New("heartbeat unavailable")
	shouldFail := true
	p := &heartbeatFuncProvider{
		heartbeat: func(*api.InstanceHeartbeatRequest) error {
			if shouldFail {
				return heartbeatErr
			}
			return nil
		},
	}
	r, err := NewRegistry(p, &Config{
		ServiceName:  t.Name(),
		BindAddress:  "127.0.0.1:8080",
		InstanceID:   "existing-instance",
		ServiceToken: "token",
	})
	require.NoError(t, err)
	instance := &registeredInstance{
		spec: instanceSpec{
			host:         "127.0.0.1",
			port:         8080,
			serviceToken: "token",
		},
		instanceID: "existing-instance",
	}

	r.heartBeat(instance)
	r.heartBeat(instance)
	require.Equal(t, 2, instance.heartbeatFailures)
	require.False(t, instance.lastHeartbeatFailLog.IsZero())

	shouldFail = false
	r.heartBeat(instance)
	require.Zero(t, instance.heartbeatFailures)
	require.True(t, instance.lastHeartbeatFailLog.IsZero())
}

func TestRegisterSkipsNilOption(t *testing.T) {
	r, err := NewRegistry(nil, &Config{
		ServiceName:        t.Name(),
		BindAddress:        "127.0.0.1:8080",
		DisableHealthCheck: true,
	})
	require.NoError(t, err)
	require.NoError(t, r.Register("", nil))
	require.NoError(t, r.Deregister(""))
}

type provider struct {
	api.ProviderAPI
	heartbeat chan struct{}
}

func (p *provider) Heartbeat(*api.InstanceHeartbeatRequest) error {
	select {
	case p.heartbeat <- struct{}{}:
	default:
	}
	return nil
}

type recordingProvider struct {
	api.ProviderAPI
	heartbeat chan *api.InstanceHeartbeatRequest
}

func (p *recordingProvider) Register(
	req *api.InstanceRegisterRequest,
) (*model.InstanceRegisterResponse, error) {
	return &model.InstanceRegisterResponse{InstanceID: req.InstanceId}, nil
}

func (p *recordingProvider) Heartbeat(req *api.InstanceHeartbeatRequest) error {
	p.heartbeat <- req
	return nil
}

func (p *recordingProvider) Deregister(*api.InstanceDeRegisterRequest) error {
	return nil
}

type sharedAddressProvider struct {
	api.ProviderAPI
	registered   chan string
	heartbeat    chan string
	deregistered chan string
}

func (p *sharedAddressProvider) Register(
	req *api.InstanceRegisterRequest,
) (*model.InstanceRegisterResponse, error) {
	p.registered <- req.InstanceId
	return &model.InstanceRegisterResponse{InstanceID: req.InstanceId}, nil
}

func (p *sharedAddressProvider) Heartbeat(req *api.InstanceHeartbeatRequest) error {
	p.heartbeat <- req.InstanceID
	return nil
}

func (p *sharedAddressProvider) Deregister(req *api.InstanceDeRegisterRequest) error {
	p.deregistered <- req.InstanceID
	return nil
}

type heartbeatFuncProvider struct {
	api.ProviderAPI
	heartbeat func(*api.InstanceHeartbeatRequest) error
}

func (p *heartbeatFuncProvider) Heartbeat(req *api.InstanceHeartbeatRequest) error {
	return p.heartbeat(req)
}
