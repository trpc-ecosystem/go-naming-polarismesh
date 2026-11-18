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
	"strings"
	"sync/atomic"
	"testing"

	"trpc.group/trpc-go/trpc-go/naming/registry"
	"trpc.group/trpc-go/trpc-go/plugin"
	"trpc.group/trpc-go/trpc-naming-polarismesh/internal/mock/mock_api"

	"github.com/golang/mock/gomock"
	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/model"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type countingSnapshotProvider struct {
	snapshot      RegistrationSnapshot
	snapshotCalls int32
	metadataCalls int32
	err           error
	validate      func(*Config, RegistrationSnapshot) error
}

func (p *countingSnapshotProvider) Snapshot() (RegistrationSnapshot, error) {
	atomic.AddInt32(&p.snapshotCalls, 1)
	return p.snapshot, p.err
}

func (p *countingSnapshotProvider) Metadata() (map[string]string, error) {
	atomic.AddInt32(&p.metadataCalls, 1)
	return p.snapshot.Metadata, p.err
}

func (p *countingSnapshotProvider) ValidateRegistration(cfg *Config, snapshot RegistrationSnapshot) error {
	if p.validate != nil {
		return p.validate(cfg, snapshot)
	}
	return nil
}

func TestSnapshotCompleteAddressAndProtocolPresence(t *testing.T) {
	for _, tt := range []struct {
		name         string
		host         string
		protocol     string
		setProtocol  bool
		wantProtocol string
	}{
		{name: "preserve", host: "10.0.0.1", wantProtocol: "trpc"},
		{name: "explicit empty", host: "10.0.0.1", setProtocol: true},
		{name: "old nonempty", host: "10.0.0.1", protocol: "http", wantProtocol: "http"},
		{name: "IPv6", host: "2001:db8::1", setProtocol: true},
		{name: "hostname", host: "example.internal", protocol: "tcp", wantProtocol: "tcp"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			provider := &countingSnapshotProvider{snapshot: RegistrationSnapshot{
				Host: tt.host, RegistrationPort: 9000, Version: "dynamic-v2",
				Protocol: tt.protocol, SetProtocol: tt.setProtocol,
				Metadata: map[string]string{"payload": "  {not json  "},
			}}
			require.NoError(t, RegisterMetadataProvider(t.Name(), provider))
			t.Cleanup(func() { UnregisterMetadataProvider(t.Name()) })
			p := mock_api.NewMockProviderAPI(ctrl)
			p.EXPECT().Register(gomock.Any()).DoAndReturn(func(req *api.InstanceRegisterRequest) (*model.InstanceRegisterResponse, error) {
				require.Equal(t, tt.host, req.Host)
				require.Equal(t, 9000, req.Port)
				require.Equal(t, "dynamic-v2", *req.Version)
				require.NotNil(t, req.Protocol)
				require.Equal(t, tt.wantProtocol, *req.Protocol)
				require.Equal(t, "  {not json  ", req.Metadata["payload"])
				req.Metadata["payload"] = "mutated"
				return &model.InstanceRegisterResponse{InstanceID: "snapshot-id"}, nil
			}).Times(1)
			p.EXPECT().Deregister(gomock.Any()).Return(nil).Times(1)
			r, err := NewRegistry(p, &Config{
				EnableRegister: true, ServiceName: t.Name(), DisableHealthCheck: true,
				InstanceProtocol: "trpc", Version: "static-v1",
			})
			require.NoError(t, err)
			require.NoError(t, r.Register(""))
			require.Equal(t, int32(1), atomic.LoadInt32(&provider.snapshotCalls))
			require.Zero(t, atomic.LoadInt32(&provider.metadataCalls))
			require.Equal(t, "  {not json  ", provider.snapshot.Metadata["payload"])
			require.NoError(t, r.Deregister(""))
		})
	}
}

func TestSnapshotAddressPrecedenceAndPartialOverrides(t *testing.T) {
	for _, tt := range []struct {
		name       string
		snapshot   RegistrationSnapshot
		preferBind bool
		wantHost   string
		wantPort   int
	}{
		{name: "complete", snapshot: RegistrationSnapshot{Host: "10.0.0.3", RegistrationPort: 9000}, preferBind: true, wantHost: "10.0.0.3", wantPort: 9000},
		{name: "host only", snapshot: RegistrationSnapshot{Host: "10.0.0.3"}, wantHost: "10.0.0.3", wantPort: 8000},
		{name: "port only", snapshot: RegistrationSnapshot{RegistrationPort: 9000}, wantHost: "10.0.0.2", wantPort: 9000},
		{name: "bind", snapshot: RegistrationSnapshot{}, preferBind: true, wantHost: "10.0.0.1", wantPort: 7000},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			require.NoError(t, RegisterMetadataProvider(t.Name(), RegistrationSnapshotProviderFunc(func() (RegistrationSnapshot, error) { return tt.snapshot, nil })))
			t.Cleanup(func() { UnregisterMetadataProvider(t.Name()) })
			p := mock_api.NewMockProviderAPI(ctrl)
			p.EXPECT().Register(gomock.Any()).DoAndReturn(func(req *api.InstanceRegisterRequest) (*model.InstanceRegisterResponse, error) {
				require.Equal(t, tt.wantHost, req.Host)
				require.Equal(t, tt.wantPort, req.Port)
				return &model.InstanceRegisterResponse{InstanceID: "id"}, nil
			})
			p.EXPECT().Deregister(gomock.Any()).Return(nil)
			r, err := NewRegistry(p, &Config{ServiceName: t.Name(), EnableRegister: true, DisableHealthCheck: true,
				BindAddress: "10.0.0.1:7000", PreferBindAddress: tt.preferBind})
			require.NoError(t, err)
			require.NoError(t, r.Register("", registry.WithAddress("10.0.0.2:8000")))
			require.NoError(t, r.Deregister(""))
		})
	}
}

func TestSnapshotInvalidFieldsFailBeforeRPC(t *testing.T) {
	for _, snapshot := range []RegistrationSnapshot{
		{Host: "10.0.0.1:8080", RegistrationPort: 9000},
		{Host: "http://10.0.0.1", RegistrationPort: 9000},
		{Host: "[::1]", RegistrationPort: 9000},
		{Host: "bad host", RegistrationPort: 9000},
		{Host: " ::1", RegistrationPort: 9000},
		{Host: "::invalid", RegistrationPort: 9000},
		{Host: "10.0.0.1", RegistrationPort: -1},
		{Host: "10.0.0.1", RegistrationPort: 65536},
		{Host: "10.0.0.1"},
		{RegistrationPort: 9000},
		{Host: "10.0.0.1", RegistrationPort: 9000, Protocol: " http"},
		{Host: "10.0.0.1", RegistrationPort: 9000, Protocol: strings.Repeat("x", 33)},
	} {
		t.Run("invalid", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			require.NoError(t, RegisterMetadataProvider(t.Name(), RegistrationSnapshotProviderFunc(func() (RegistrationSnapshot, error) { return snapshot, nil })))
			t.Cleanup(func() { UnregisterMetadataProvider(t.Name()) })
			r, err := NewRegistry(mock_api.NewMockProviderAPI(ctrl), &Config{ServiceName: t.Name(), EnableRegister: true})
			require.NoError(t, err)
			require.Error(t, r.Register(""))
			require.Empty(t, r.state.instances)
		})
	}
}

func TestSnapshotMultipleEndpointsAcceptOnlySharedFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	provider := &countingSnapshotProvider{snapshot: RegistrationSnapshot{
		Metadata: map[string]string{"generation": "2"}, Version: "v2", SetProtocol: true,
	}}
	require.NoError(t, RegisterMetadataProvider(t.Name(), provider))
	t.Cleanup(func() { UnregisterMetadataProvider(t.Name()) })
	p := mock_api.NewMockProviderAPI(ctrl)
	p.EXPECT().Register(gomock.Any()).DoAndReturn(func(req *api.InstanceRegisterRequest) (*model.InstanceRegisterResponse, error) {
		require.Empty(t, *req.Protocol)
		require.Equal(t, "v2", *req.Version)
		require.Equal(t, "2", req.Metadata["generation"])
		return &model.InstanceRegisterResponse{InstanceID: req.Host}, nil
	}).Times(2)
	p.EXPECT().Deregister(gomock.Any()).Return(nil).Times(2)
	r, err := NewRegistry(p, &Config{ServiceName: t.Name(), EnableRegister: true, DisableHealthCheck: true,
		Endpoints: []Endpoint{{Address: "10.0.0.1:8000", Protocol: "trpc"}, {Address: "10.0.0.2:8001", Protocol: "http"}}})
	require.NoError(t, err)
	require.NoError(t, r.Register(""))
	require.Equal(t, int32(1), provider.snapshotCalls)
	require.NoError(t, r.Deregister(""))
	provider.snapshot.Host = "10.0.0.3"
	require.ErrorContains(t, r.Register(""), "exactly one endpoint")
}

func TestSnapshotValidatorUsesCopiesAndSameGeneration(t *testing.T) {
	ctrl := gomock.NewController(t)
	provider := &countingSnapshotProvider{snapshot: RegistrationSnapshot{Host: "10.0.0.1", RegistrationPort: 9000,
		Metadata: map[string]string{"generation": "one"}}}
	provider.validate = func(cfg *Config, snapshot RegistrationSnapshot) error {
		require.Equal(t, "one", snapshot.Metadata["generation"])
		cfg.Metadata["static"] = "changed"
		cfg.Endpoints[0].Metadata["endpoint"] = "changed"
		snapshot.Metadata["generation"] = "changed"
		return nil
	}
	require.NoError(t, RegisterMetadataProvider(t.Name(), provider))
	t.Cleanup(func() { UnregisterMetadataProvider(t.Name()) })
	p := mock_api.NewMockProviderAPI(ctrl)
	p.EXPECT().Register(gomock.Any()).DoAndReturn(func(req *api.InstanceRegisterRequest) (*model.InstanceRegisterResponse, error) {
		require.Equal(t, map[string]string{"generation": "one", "static": "value", "endpoint": "value"}, req.Metadata)
		return &model.InstanceRegisterResponse{InstanceID: "id"}, nil
	})
	p.EXPECT().Deregister(gomock.Any()).Return(nil)
	r, err := NewRegistry(p, &Config{ServiceName: t.Name(), EnableRegister: true, DisableHealthCheck: true,
		Metadata: map[string]string{"static": "value"}, Endpoints: []Endpoint{{Address: "10.0.0.2:8000", Metadata: map[string]string{"endpoint": "value"}}}})
	require.NoError(t, err)
	require.NoError(t, r.Register(""))
	require.Equal(t, int32(1), provider.snapshotCalls)
	require.NoError(t, r.Deregister(""))
	validationErr := errors.New("provider policy rejected configuration")
	provider.validate = func(*Config, RegistrationSnapshot) error { return validationErr }
	require.ErrorIs(t, r.Register(""), validationErr)
	require.Empty(t, r.state.instances)
}

func TestSnapshotCancelledRegistrationRetainsSafeRollback(t *testing.T) {
	ctrl := gomock.NewController(t)
	require.NoError(t, RegisterMetadataProvider(t.Name(), RegistrationSnapshotProviderFunc(func() (RegistrationSnapshot, error) {
		return RegistrationSnapshot{Host: "10.0.0.1", RegistrationPort: 9000, SetProtocol: true}, nil
	})))
	t.Cleanup(func() { UnregisterMetadataProvider(t.Name()) })
	p := mock_api.NewMockProviderAPI(ctrl)
	p.EXPECT().Register(gomock.Any()).Return(nil, context.Canceled).Times(1)
	p.EXPECT().Deregister(gomock.Any()).DoAndReturn(func(req *api.InstanceDeRegisterRequest) error {
		require.Equal(t, "owned-id", req.InstanceID)
		require.Equal(t, 9000, req.Port)
		return nil
	}).Times(1)
	r, err := NewRegistry(p, &Config{ServiceName: t.Name(), EnableRegister: true, InstanceID: "owned-id"})
	require.NoError(t, err)
	require.ErrorIs(t, r.Register(""), context.Canceled)
	require.Empty(t, r.state.instances)
}

func TestRegistryConfigReturnsDeepCopy(t *testing.T) {
	r, err := NewRegistry(nil, &Config{ServiceName: t.Name(), Metadata: map[string]string{"key": "value"},
		Endpoints: []Endpoint{{Address: "10.0.0.1:8000", Metadata: map[string]string{"nested": "value"}}}})
	require.NoError(t, err)
	copy := r.Config()
	copy.Metadata["key"] = "changed"
	copy.Endpoints[0].Metadata["nested"] = "changed"
	copy.Endpoints[0].Address = "changed"
	require.Equal(t, "value", r.Config().Metadata["key"])
	require.Equal(t, "value", r.Config().Endpoints[0].Metadata["nested"])
	require.Equal(t, "10.0.0.1:8000", r.Config().Endpoints[0].Address)
	var nilRegistry *Registry
	require.Nil(t, nilRegistry.Config())
}

func TestServiceYAMLRejectsUnknownFields(t *testing.T) {
	for _, input := range []string{
		"service:\n  - name: example\n    removed_option: 1\n",
		"service:\n  - name: example\n    registration_port: 1\n",
		"service:\n  - name: example\n    endpoints:\n      - address: 10.0.0.1:8000\n        unknown: true\n",
	} {
		var node yaml.Node
		require.NoError(t, yaml.Unmarshal([]byte(input), &node))
		decoder := &plugin.YamlNodeDecoder{Node: &node}
		require.ErrorContains(t, decoder.Decode(&FactoryConfig{}), "not found")
	}
}
