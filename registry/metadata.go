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
	"fmt"
	"reflect"
	"sync"
	"unicode/utf8"
)

const (
	maxRegisterMetadataEntries = 64
	maxRegisterMetadataValue   = 65535
)

var metadataProviders sync.Map

// MetadataProvider supplies dynamic instance metadata when a service is registered.
type MetadataProvider interface {
	Metadata() (map[string]string, error)
}

// MetadataProviderFunc adapts a function to MetadataProvider.
type MetadataProviderFunc func() (map[string]string, error)

// Metadata calls f.
func (f MetadataProviderFunc) Metadata() (map[string]string, error) {
	if f == nil {
		return nil, fmt.Errorf("metadata provider func must not be nil")
	}
	return f()
}

// RegistrationSnapshot is the atomic set of registration fields supplied by a provider.
type RegistrationSnapshot struct {
	// Metadata is merged into the instance registration request.
	Metadata map[string]string
	// Host overrides the host of exactly one endpoint when non-empty.
	// Together with RegistrationPort it permits registration without a listening address.
	Host string
	// Version overrides the configured instance version when non-empty.
	Version string
	// RegistrationPort overrides the port of exactly one endpoint when non-zero.
	// A supplied port must be in 1..65535; zero preserves the endpoint port.
	RegistrationPort int
	// Protocol overrides the endpoint protocol when non-empty or SetProtocol is true.
	Protocol string
	// SetProtocol permits explicitly overriding the endpoint protocol with an empty value.
	SetProtocol bool
}

// RegistrationSnapshotProvider atomically supplies all dynamic registration fields.
type RegistrationSnapshotProvider interface {
	Snapshot() (RegistrationSnapshot, error)
}

// RegistrationConfigValidator optionally validates a provider's snapshot against
// the registry configuration before any instance is registered. Both arguments
// are defensive copies; validators must not register instances or start heartbeats.
type RegistrationConfigValidator interface {
	ValidateRegistration(config *Config, snapshot RegistrationSnapshot) error
}

// RegistrationSnapshotProviderFunc adapts a function to RegistrationSnapshotProvider.
type RegistrationSnapshotProviderFunc func() (RegistrationSnapshot, error)

// Snapshot calls f.
func (f RegistrationSnapshotProviderFunc) Snapshot() (RegistrationSnapshot, error) {
	if f == nil {
		return RegistrationSnapshot{}, fmt.Errorf("registration snapshot provider func must not be nil")
	}
	return f()
}

// Metadata provides source compatibility with MetadataProvider.
func (f RegistrationSnapshotProviderFunc) Metadata() (map[string]string, error) {
	snapshot, err := f.Snapshot()
	if err != nil {
		return nil, err
	}
	return snapshot.Metadata, nil
}

// RegisterMetadataProvider registers a dynamic metadata provider for one service.
func RegisterMetadataProvider(service string, provider MetadataProvider) error {
	if service == "" {
		return fmt.Errorf("metadata provider service must not be empty")
	}
	if isNilMetadataProvider(provider) {
		return fmt.Errorf("metadata provider for service %q must not be nil", service)
	}
	if _, loaded := metadataProviders.LoadOrStore(service, provider); loaded {
		return fmt.Errorf("metadata provider for service %q is already registered", service)
	}
	return nil
}

// UnregisterMetadataProvider removes the dynamic metadata provider for one service.
func UnregisterMetadataProvider(service string) {
	metadataProviders.Delete(service)
}

func getMetadataProvider(service string) (MetadataProvider, bool) {
	provider, ok := metadataProviders.Load(service)
	if !ok {
		return nil, false
	}
	return provider.(MetadataProvider), true
}

func isNilMetadataProvider(provider MetadataProvider) bool {
	if provider == nil {
		return true
	}
	value := reflect.ValueOf(provider)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if metadata == nil {
		return nil
	}
	cloned := make(map[string]string, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func mergeMetadata(base, override map[string]string) map[string]string {
	if base == nil && override == nil {
		return nil
	}
	merged := make(map[string]string, len(base)+len(override))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range override {
		merged[key] = value
	}
	return merged
}

func validateRegisterMetadata(metadata map[string]string) error {
	if len(metadata) > maxRegisterMetadataEntries {
		return fmt.Errorf("register metadata contains %d entries, maximum is %d",
			len(metadata), maxRegisterMetadataEntries)
	}
	for key, value := range metadata {
		if key == "" {
			return fmt.Errorf("register metadata key must not be empty")
		}
		if length := utf8.RuneCountInString(value); length > maxRegisterMetadataValue {
			return fmt.Errorf("register metadata value for key %q contains %d characters, maximum is %d",
				key, length, maxRegisterMetadataValue)
		}
	}
	return nil
}

type metadataProviderSnapshot struct {
	metadata         map[string]string
	host             string
	version          string
	registrationPort int
	instanceProtocol string
	setProtocol      bool
	atomic           bool
	validator        RegistrationConfigValidator
}

func loadMetadataProviderSnapshot(service string) (*metadataProviderSnapshot, error) {
	provider, ok := getMetadataProvider(service)
	if !ok {
		return nil, nil
	}
	validator, _ := provider.(RegistrationConfigValidator)
	if snapshotProvider, ok := provider.(RegistrationSnapshotProvider); ok {
		snapshot, err := snapshotProvider.Snapshot()
		if err != nil {
			return nil, fmt.Errorf("registration snapshot provider for service %q failed: %w", service, err)
		}
		return &metadataProviderSnapshot{
			metadata:         cloneMetadata(snapshot.Metadata),
			host:             snapshot.Host,
			version:          snapshot.Version,
			registrationPort: snapshot.RegistrationPort,
			instanceProtocol: snapshot.Protocol,
			setProtocol:      snapshot.SetProtocol,
			atomic:           true,
			validator:        validator,
		}, nil
	}
	dynamic, err := provider.Metadata()
	if err != nil {
		return nil, fmt.Errorf("metadata provider for service %q failed: %w", service, err)
	}
	return &metadataProviderSnapshot{
		metadata:  cloneMetadata(dynamic),
		validator: validator,
	}, nil
}

func (s *metadataProviderSnapshot) validateConfig(cfg *Config) error {
	if s == nil || s.validator == nil {
		return nil
	}
	snapshot := RegistrationSnapshot{
		Metadata: cloneMetadata(s.metadata), Host: s.host, Version: s.version,
		RegistrationPort: s.registrationPort, Protocol: s.instanceProtocol, SetProtocol: s.setProtocol,
	}
	if err := s.validator.ValidateRegistration(cloneConfig(cfg), snapshot); err != nil {
		return fmt.Errorf("validate registration provider for service %q: %w", cfg.ServiceName, err)
	}
	return nil
}

func buildRegisterMetadata(
	service string,
	static, endpoint map[string]string,
	snapshot *metadataProviderSnapshot,
) (map[string]string, error) {
	metadata := mergeMetadata(static, endpoint)
	if snapshot != nil {
		metadata = mergeMetadata(metadata, snapshot.metadata)
	}
	if err := validateRegisterMetadata(metadata); err != nil {
		return nil, fmt.Errorf("validate register metadata for service %q: %w", service, err)
	}
	return metadata, nil
}

func (r *Registry) buildRegisterMetadata() (map[string]string, error) {
	snapshot, err := loadMetadataProviderSnapshot(r.cfg.ServiceName)
	if err != nil {
		return nil, err
	}
	return buildRegisterMetadata(r.cfg.ServiceName, r.cfg.Metadata, nil, snapshot)
}
