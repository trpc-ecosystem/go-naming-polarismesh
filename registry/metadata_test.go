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
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMetadataProviderLifecycle(t *testing.T) {
	service := t.Name()
	provider := MetadataProviderFunc(func() (map[string]string, error) {
		return map[string]string{"source": "dynamic"}, nil
	})

	require.NoError(t, RegisterMetadataProvider(service, provider))
	require.Error(t, RegisterMetadataProvider(service, provider))

	registered, ok := getMetadataProvider(service)
	require.True(t, ok)
	metadata, err := registered.Metadata()
	require.NoError(t, err)
	require.Equal(t, map[string]string{"source": "dynamic"}, metadata)

	UnregisterMetadataProvider(service)
	_, ok = getMetadataProvider(service)
	require.False(t, ok)

	require.NoError(t, RegisterMetadataProvider(service, provider))
	UnregisterMetadataProvider(service)
}

func TestRegisterMetadataProviderRejectsInvalidArguments(t *testing.T) {
	require.Error(t, RegisterMetadataProvider("", MetadataProviderFunc(func() (map[string]string, error) {
		return nil, nil
	})))
	require.Error(t, RegisterMetadataProvider(t.Name(), nil))
	require.Error(t, RegisterMetadataProvider(t.Name(), MetadataProviderFunc(nil)))
}

func TestMetadataProviderConcurrent(t *testing.T) {
	const goroutines = 32

	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			service := fmt.Sprintf("%s-%d", t.Name(), i)
			provider := MetadataProviderFunc(func() (map[string]string, error) {
				return map[string]string{"worker": fmt.Sprint(i)}, nil
			})
			if err := RegisterMetadataProvider(service, provider); err != nil {
				errs <- err
				return
			}
			defer UnregisterMetadataProvider(service)
			if _, ok := getMetadataProvider(service); !ok {
				errs <- fmt.Errorf("provider for service %q was not found", service)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	const sameServiceGoroutines = 16
	service := t.Name() + "-same-service"
	var successfulRegistrations int32
	for i := 0; i < sameServiceGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			provider := MetadataProviderFunc(func() (map[string]string, error) {
				return nil, nil
			})
			if err := RegisterMetadataProvider(service, provider); err == nil {
				atomic.AddInt32(&successfulRegistrations, 1)
			}
		}()
	}
	wg.Wait()
	defer UnregisterMetadataProvider(service)
	require.Equal(t, int32(1), successfulRegistrations)
}

func TestBuildRegisterMetadataStaticOnly(t *testing.T) {
	static := map[string]string{"env": "test"}
	r := &Registry{cfg: &Config{ServiceName: t.Name(), Metadata: static}}

	metadata, err := r.buildRegisterMetadata()
	require.NoError(t, err)
	require.Equal(t, static, metadata)

	metadata["env"] = "changed"
	require.Equal(t, "test", static["env"])
}

func TestBuildRegisterMetadataMergesProvider(t *testing.T) {
	service := t.Name()
	require.NoError(t, RegisterMetadataProvider(service, MetadataProviderFunc(func() (map[string]string, error) {
		return map[string]string{
			"env":   "dynamic",
			"owner": "customer-agent",
		}, nil
	})))
	defer UnregisterMetadataProvider(service)

	static := map[string]string{"env": "static", "business": "orders"}
	r := &Registry{cfg: &Config{ServiceName: service, Metadata: static}}
	metadata, err := r.buildRegisterMetadata()
	require.NoError(t, err)
	require.Equal(t, "dynamic", metadata["env"])
	require.Equal(t, "orders", metadata["business"])
	require.Equal(t, "customer-agent", metadata["owner"])
	require.Equal(t, "static", static["env"])
}

func TestBuildRegisterMetadataPreservesOpaqueValues(t *testing.T) {
	service := t.Name()
	require.NoError(t, RegisterMetadataProvider(service, MetadataProviderFunc(func() (map[string]string, error) {
		return map[string]string{
			"descriptor":    "  {\"unknown\":true,\"signature\":\"unchanged\"}  ",
			"invalid-json":  "{not json",
			"arbitrary-key": "opaque",
		}, nil
	})))
	defer UnregisterMetadataProvider(service)

	r := &Registry{cfg: &Config{
		ServiceName: service,
		Metadata:    map[string]string{"env": "test"},
	}}
	metadata, err := r.buildRegisterMetadata()
	require.NoError(t, err)
	require.Equal(t, "  {\"unknown\":true,\"signature\":\"unchanged\"}  ", metadata["descriptor"])
	require.Equal(t, "{not json", metadata["invalid-json"])
	require.Equal(t, "opaque", metadata["arbitrary-key"])
	require.Equal(t, "test", metadata["env"])
}

func TestBuildRegisterMetadataMergesDistinctKeys(t *testing.T) {
	service := t.Name()
	require.NoError(t, RegisterMetadataProvider(service, MetadataProviderFunc(func() (map[string]string, error) {
		return map[string]string{"dynamic": "value"}, nil
	})))
	defer UnregisterMetadataProvider(service)

	r := &Registry{cfg: &Config{
		ServiceName: service,
		Metadata:    map[string]string{"static": "value"},
	}}
	metadata, err := r.buildRegisterMetadata()
	require.NoError(t, err)
	require.Equal(t, map[string]string{"dynamic": "value", "static": "value"}, metadata)
}

func TestBuildRegisterMetadataDoesNotClassifyKeys(t *testing.T) {
	service := t.Name()
	require.NoError(t, RegisterMetadataProvider(service, MetadataProviderFunc(func() (map[string]string, error) {
		return map[string]string{
			"first-family-name":  "customer",
			"second-family-name": "other",
		}, nil
	})))
	defer UnregisterMetadataProvider(service)

	r := &Registry{cfg: &Config{ServiceName: service}}
	metadata, err := r.buildRegisterMetadata()
	require.NoError(t, err)
	require.Len(t, metadata, 2)
}

func TestBuildRegisterMetadataPreservesDifferentSources(t *testing.T) {
	service := t.Name()
	require.NoError(t, RegisterMetadataProvider(service, MetadataProviderFunc(func() (map[string]string, error) {
		return map[string]string{"kind": "dynamic"}, nil
	})))
	defer UnregisterMetadataProvider(service)

	r := &Registry{cfg: &Config{
		ServiceName: service,
		Metadata:    map[string]string{"owner": "static"},
	}}
	metadata, err := r.buildRegisterMetadata()
	require.NoError(t, err)
	require.Equal(t, map[string]string{"owner": "static", "kind": "dynamic"}, metadata)
}

func TestBuildRegisterMetadataRefreshesProvider(t *testing.T) {
	service := t.Name()
	var calls int32
	require.NoError(t, RegisterMetadataProvider(service, MetadataProviderFunc(func() (map[string]string, error) {
		call := atomic.AddInt32(&calls, 1)
		return map[string]string{"refresh": fmt.Sprint(call)}, nil
	})))
	defer UnregisterMetadataProvider(service)

	r := &Registry{cfg: &Config{ServiceName: service}}
	first, err := r.buildRegisterMetadata()
	require.NoError(t, err)
	second, err := r.buildRegisterMetadata()
	require.NoError(t, err)
	require.Equal(t, "1", first["refresh"])
	require.Equal(t, "2", second["refresh"])
}

func TestBuildRegisterMetadataDynamicOverridesStaticKey(t *testing.T) {
	service := t.Name()
	require.NoError(t, RegisterMetadataProvider(service, MetadataProviderFunc(func() (map[string]string, error) {
		return map[string]string{"name": "dynamic"}, nil
	})))
	defer UnregisterMetadataProvider(service)

	r := &Registry{cfg: &Config{
		ServiceName: service,
		Metadata:    map[string]string{"name": "static"},
	}}
	metadata, err := r.buildRegisterMetadata()
	require.NoError(t, err)
	require.Equal(t, "dynamic", metadata["name"])
}

func TestBuildRegisterMetadataProviderError(t *testing.T) {
	service := t.Name()
	providerErr := errors.New("metadata unavailable")
	require.NoError(t, RegisterMetadataProvider(service, MetadataProviderFunc(func() (map[string]string, error) {
		return nil, providerErr
	})))
	defer UnregisterMetadataProvider(service)

	r := &Registry{cfg: &Config{ServiceName: service}}
	_, err := r.buildRegisterMetadata()
	require.ErrorIs(t, err, providerErr)
	require.Contains(t, err.Error(), service)
}

func TestValidateRegisterMetadata(t *testing.T) {
	tooMany := make(map[string]string, maxRegisterMetadataEntries+1)
	for i := 0; i <= maxRegisterMetadataEntries; i++ {
		tooMany[fmt.Sprintf("key-%d", i)] = "value"
	}

	tests := []struct {
		name     string
		metadata map[string]string
		wantErr  bool
	}{
		{name: "nil", metadata: nil},
		{name: "at limit", metadata: map[string]string{"key": strings.Repeat("界", maxRegisterMetadataValue)}},
		{name: "too many entries", metadata: tooMany, wantErr: true},
		{name: "empty key", metadata: map[string]string{"": "value"}, wantErr: true},
		{
			name:     "value too long",
			metadata: map[string]string{"key": strings.Repeat("界", maxRegisterMetadataValue+1)},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRegisterMetadata(tt.metadata)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
