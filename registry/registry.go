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

// Package registry is for service registry.
package registry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"trpc.group/trpc-go/trpc-go/healthcheck"
	"trpc.group/trpc-go/trpc-go/naming/registry"
	"trpc.group/trpc-go/trpc-naming-polarismesh/internal/metrics"

	"github.com/polarismesh/polaris-go/api"
	plog "github.com/polarismesh/polaris-go/pkg/log"
	"github.com/polarismesh/polaris-go/pkg/model"
)

const (
	defaultHeartBeat                = 5
	defaultTTL                      = 5
	maxInstanceProtocolLength       = 32
	heartbeatErrorLogInterval       = time.Minute
	maxHeartbeatSeconds       int64 = int64((1<<63 - 1) / time.Second)
)

var (
	defaultWeight        = 100
	healthcheckWatchLock sync.Mutex
)

type instanceSpec struct {
	order              int
	host               string
	port               int
	protocol           string
	setProtocol        bool
	version            string
	serviceToken       string
	instanceID         string
	weight             *int
	ttl                int
	heartBeat          int
	autoHeartbeat      bool
	disableHealthCheck bool
	endpointMetadata   map[string]string
	metadata           map[string]string
	location           *model.Location
}

type registeredInstance struct {
	spec       instanceSpec
	instanceID string
	cancel     context.CancelFunc
	statuses   chan healthcheck.Status

	heartbeatFailures    int
	lastHeartbeatFailLog time.Time
}

type registryState struct {
	instances         []*registeredInstance
	heartbeatStatuses map[*registeredInstance]chan healthcheck.Status
}

type registrationValidationError struct {
	err error
}

func (e *registrationValidationError) Error() string {
	return e.err.Error()
}

func (e *registrationValidationError) Unwrap() error {
	return e.err
}

// Registry is service registration.
type Registry struct {
	// Provider: public for user custom.
	Provider api.ProviderAPI

	cfg    *Config
	sdkCtx api.SDKContext

	lifecycleMu sync.Mutex
	state       *registryState

	versionMu         sync.RWMutex
	registeredVersion string

	statusMu      sync.RWMutex
	statusKnown   bool
	serviceStatus healthcheck.Status
	heartbeatWG   sync.WaitGroup
}

// newRegistry is to new an instance.
func newRegistry(provider api.ProviderAPI, cfg *Config) (*Registry, error) {
	prepared, err := prepareConfig(cfg)
	if err != nil {
		return nil, err
	}
	return newRegistryPrepared(provider, prepared), nil
}

func prepareConfig(cfg *Config) (*Config, error) {
	if cfg == nil {
		return nil, fmt.Errorf("registry config must not be nil")
	}
	cloned := cloneConfig(cfg)
	if err := applyConfigDefaults(cloned); err != nil {
		return nil, err
	}
	validator := &Registry{cfg: cloned}
	if len(cloned.Endpoints) > 0 {
		if _, err := validator.resolveInstanceSpecs(""); err != nil {
			return nil, err
		}
	}
	return cloned, nil
}

func newRegistryPrepared(provider api.ProviderAPI, cfg *Config) *Registry {
	r := &Registry{
		Provider: provider,
		cfg:      cfg,
		state: &registryState{
			heartbeatStatuses: make(map[*registeredInstance]chan healthcheck.Status),
		},
	}
	healthcheckWatchLock.Lock()
	healthcheck.Watch(cfg.ServiceName, r.observeHealthStatus)
	healthcheckWatchLock.Unlock()
	return r
}

// NewRegistry provides an externally accessible new instance interface.
func NewRegistry(provider api.ProviderAPI, cfg *Config) (*Registry, error) {
	return newRegistry(provider, cfg)
}

// Register is for registration service.
func (r *Registry) Register(_ string, opt ...registry.Option) error {
	opts := &registry.Options{}
	for _, o := range opt {
		if o != nil {
			o(opts)
		}
	}

	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	if len(r.state.instances) > 0 {
		return fmt.Errorf("service %q already has registered instances; deregister before retrying",
			r.cfg.ServiceName)
	}
	if r.Provider == nil && (!r.cfg.DisableHealthCheck || r.cfg.EnableRegister) {
		return fmt.Errorf("register service %q: provider is nil", r.cfg.ServiceName)
	}

	var snapshot *metadataProviderSnapshot
	if r.cfg.EnableRegister {
		var err error
		snapshot, err = loadMetadataProviderSnapshot(r.cfg.ServiceName)
		if err != nil {
			return err
		}
	}

	if err := snapshot.validateConfig(r.cfg); err != nil {
		return err
	}
	specs, err := r.resolveRegistrationSpecs(opts.Address, snapshot)
	if err != nil {
		return err
	}
	if snapshot != nil && snapshot.atomic {
		if err := r.applyRegistrationSnapshot(specs, snapshot); err != nil {
			return err
		}
		if err := validateInstanceSpecIdentity(specs); err != nil {
			return fmt.Errorf("register service %q: %w", r.cfg.ServiceName, err)
		}
	}
	if r.cfg.EnableRegister {
		for i := range specs {
			specs[i].metadata, err = buildRegisterMetadata(
				r.cfg.ServiceName, r.cfg.Metadata, specs[i].endpointMetadata, snapshot)
			if err != nil {
				return err
			}
		}
	} else {
		for i := range specs {
			specs[i].autoHeartbeat = false
			if !specs[i].disableHealthCheck && specs[i].instanceID == "" {
				return fmt.Errorf(
					"register service %q endpoint %d: heartbeat-only mode requires instance_id",
					r.cfg.ServiceName, specs[i].order)
			}
		}
	}

	registered := make([]*registeredInstance, 0, len(specs))
	for i := range specs {
		if r.cfg.EnableRegister {
			instance, err := r.registerInstance(&specs[i])
			if err != nil {
				remaining, rollbackErr := r.deregisterInstances(registered)
				var failureCleanupErr error
				if shouldCleanupFailedRegistration(&specs[i], err) {
					failureCleanupErr = r.deregisterSpec(&specs[i])
					if failureCleanupErr != nil {
						remaining = append(remaining, newPendingRegisteredInstance(
							specs[i], specs[i].instanceID))
					}
				}
				r.state.instances = remaining
				if rollbackErr != nil {
					failureCleanup := "<nil>"
					if failureCleanupErr != nil {
						failureCleanup = failureCleanupErr.Error()
					}
					return fmt.Errorf(
						"register service %q endpoint %s failed: %w; rollback failed: %s; failure cleanup: %s",
						r.cfg.ServiceName, instanceAddress(specs[i]), err, rollbackErr.Error(), failureCleanup)
				}
				if failureCleanupErr != nil {
					return fmt.Errorf(
						"register service %q endpoint %s failed: %w; failure cleanup failed: %s",
						r.cfg.ServiceName, instanceAddress(specs[i]), err, failureCleanupErr.Error())
				}
				return fmt.Errorf("register service %q endpoint %s failed: %w",
					r.cfg.ServiceName, instanceAddress(specs[i]), err)
			}
			cleanupCandidate, collisionErr := findInstanceIDCollision(
				instance, registered, specs[i+1:])
			if collisionErr != nil {
				rollback := registered
				if cleanupCandidate {
					rollback = append(append([]*registeredInstance(nil), registered...), instance)
				}
				remaining, rollbackErr := r.deregisterInstances(rollback)
				r.state.instances = remaining
				if rollbackErr != nil {
					return fmt.Errorf(
						"register service %q endpoint %s failed: %w; rollback failed: %s",
						r.cfg.ServiceName, instanceAddress(specs[i]), collisionErr, rollbackErr.Error())
				}
				return fmt.Errorf("register service %q endpoint %s failed: %w",
					r.cfg.ServiceName, instanceAddress(specs[i]), collisionErr)
			}
			registered = append(registered, instance)
			continue
		}
		registered = append(registered, &registeredInstance{
			spec:       specs[i],
			instanceID: specs[i].instanceID,
			statuses:   make(chan healthcheck.Status, 1),
		})
	}
	r.state.instances = registered
	if len(specs) == 1 {
		r.setRegisteredVersion(specs[0].version)
	}
	for _, instance := range registered {
		r.startManualHeartbeat(instance)
	}
	return nil
}

func (r *Registry) resolveInstanceSpecs(optsAddress string) ([]instanceSpec, error) {
	return r.resolveRegistrationSpecs(optsAddress, nil)
}

func (r *Registry) resolveRegistrationSpecs(
	optsAddress string,
	snapshot *metadataProviderSnapshot,
) ([]instanceSpec, error) {
	if len(r.cfg.Endpoints) == 0 {
		address := optsAddress
		if address == "" || r.cfg.PreferBindAddress {
			address = os.ExpandEnv(r.cfg.BindAddress)
		}
		if snapshot != nil && snapshot.atomic && snapshot.host != "" && snapshot.registrationPort != 0 {
			address = net.JoinHostPort(snapshot.host, strconv.Itoa(snapshot.registrationPort))
		}
		if address == "" {
			return nil, fmt.Errorf("register service %q: instance address is empty", r.cfg.ServiceName)
		}
		host, port, err := parseInstanceAddress(address)
		if err != nil {
			return nil, fmt.Errorf("register service %q address %q: %w", r.cfg.ServiceName, address, err)
		}
		spec, err := r.buildInstanceSpec(0, host, port, Endpoint{})
		if err != nil {
			return nil, err
		}
		return []instanceSpec{spec}, nil
	}

	specs := make([]instanceSpec, 0, len(r.cfg.Endpoints))
	for i, endpoint := range r.cfg.Endpoints {
		address := os.ExpandEnv(endpoint.Address)
		if address == "" {
			return nil, fmt.Errorf("register service %q endpoint %d: address is empty",
				r.cfg.ServiceName, i)
		}
		host, port, err := parseInstanceAddress(address)
		if err != nil {
			return nil, fmt.Errorf("register service %q endpoint %d address %q: %w",
				r.cfg.ServiceName, i, address, err)
		}
		spec, err := r.buildInstanceSpec(i, host, port, endpoint)
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	if err := validateInstanceSpecIdentity(specs); err != nil {
		return nil, fmt.Errorf("register service %q: %w", r.cfg.ServiceName, err)
	}
	return specs, nil
}

func (r *Registry) applyRegistrationSnapshot(
	specs []instanceSpec,
	snapshot *metadataProviderSnapshot,
) error {
	if (snapshot.host != "" || snapshot.registrationPort != 0) && len(specs) != 1 {
		return fmt.Errorf(
			"register service %q registration snapshot host/port requires exactly one endpoint, got %d",
			r.cfg.ServiceName, len(specs))
	}
	if snapshot.host != "" {
		if err := validateSnapshotHost(snapshot.host); err != nil {
			return fmt.Errorf("register service %q registration snapshot host: %w", r.cfg.ServiceName, err)
		}
		specs[0].host = snapshot.host
	}
	if snapshot.registrationPort != 0 {
		if snapshot.registrationPort < 1 || snapshot.registrationPort > 65535 {
			return fmt.Errorf(
				"register service %q registration snapshot port %d must be in 1..65535",
				r.cfg.ServiceName, snapshot.registrationPort)
		}
		specs[0].port = snapshot.registrationPort
	}
	if snapshot.setProtocol || snapshot.instanceProtocol != "" {
		if strings.TrimSpace(snapshot.instanceProtocol) != snapshot.instanceProtocol {
			return fmt.Errorf(
				"register service %q registration snapshot protocol contains surrounding whitespace",
				r.cfg.ServiceName)
		}
		if len(snapshot.instanceProtocol) > maxInstanceProtocolLength {
			return fmt.Errorf(
				"register service %q registration snapshot protocol length %d exceeds maximum %d",
				r.cfg.ServiceName, len(snapshot.instanceProtocol), maxInstanceProtocolLength)
		}
		for i := range specs {
			specs[i].protocol = snapshot.instanceProtocol
			specs[i].setProtocol = true
		}
	}
	if snapshot.version != "" {
		for i := range specs {
			specs[i].version = snapshot.version
		}
	}
	return nil
}

func validateSnapshotHost(host string) error {
	if strings.TrimSpace(host) != host || strings.ContainsAny(host, "[]/\\@?#\x00\r\n\t ") {
		return fmt.Errorf("must be a host without a scheme, path, port, brackets or whitespace")
	}
	if strings.Contains(host, ":") && net.ParseIP(host) == nil {
		return fmt.Errorf("must be an IPv6 address or a host without a port")
	}
	return nil
}

func (r *Registry) buildInstanceSpec(order int, host string, port int, endpoint Endpoint) (instanceSpec, error) {
	protocol := endpoint.Protocol
	if protocol == "" {
		protocol = r.cfg.InstanceProtocol
	}
	version := endpoint.Version
	if version == "" {
		version = r.cfg.Version
	}
	serviceToken := endpoint.ServiceToken
	if serviceToken == "" {
		serviceToken = r.cfg.ServiceToken
	}
	instanceID := endpoint.InstanceID
	if instanceID == "" {
		instanceID = r.cfg.InstanceID
	}
	weight := endpoint.Weight
	if weight == nil {
		weight = r.cfg.Weight
	}
	if weight == nil {
		weight = &defaultWeight
	}
	ttl := endpoint.TTL
	if ttl == 0 {
		ttl = r.cfg.TTL
	}
	heartBeat := endpoint.HeartBeat
	if heartBeat == 0 {
		heartBeat = r.cfg.HeartBeat
	}
	autoHeartbeat := r.cfg.AutoHeartbeat
	if endpoint.AutoHeartbeat != nil {
		autoHeartbeat = *endpoint.AutoHeartbeat
	}
	disableHealthCheck := r.cfg.DisableHealthCheck
	if endpoint.DisableHealthCheck != nil {
		disableHealthCheck = *endpoint.DisableHealthCheck
	}
	location := endpoint.InstanceLocation
	if location == nil {
		location = r.cfg.InstanceLocation
	}
	if r.cfg.EnableRegister && autoHeartbeat && disableHealthCheck {
		return instanceSpec{}, fmt.Errorf(
			"register service %q endpoint %d: auto heartbeat conflicts with disabled health check",
			r.cfg.ServiceName, order)
	}
	if !disableHealthCheck {
		if ttl <= 0 {
			return instanceSpec{}, fmt.Errorf("register service %q endpoint %d: TTL must be positive",
				r.cfg.ServiceName, order)
		}
		if heartBeat <= 0 {
			return instanceSpec{}, fmt.Errorf(
				"register service %q endpoint %d: heartbeat interval must be positive",
				r.cfg.ServiceName, order)
		}
		if int64(heartBeat) > maxHeartbeatSeconds {
			return instanceSpec{}, fmt.Errorf(
				"register service %q endpoint %d: heartbeat interval exceeds duration limit",
				r.cfg.ServiceName, order)
		}
	}
	if strings.TrimSpace(protocol) != protocol {
		return instanceSpec{}, fmt.Errorf("register service %q endpoint %d: protocol contains surrounding whitespace",
			r.cfg.ServiceName, order)
	}
	if len(protocol) > maxInstanceProtocolLength {
		return instanceSpec{}, fmt.Errorf(
			"register service %q endpoint %d: protocol length %d exceeds maximum %d",
			r.cfg.ServiceName, order, len(protocol), maxInstanceProtocolLength)
	}
	return instanceSpec{
		order:              order,
		host:               host,
		port:               port,
		protocol:           protocol,
		setProtocol:        protocol != "",
		version:            version,
		serviceToken:       serviceToken,
		instanceID:         instanceID,
		weight:             cloneInt(weight),
		ttl:                ttl,
		heartBeat:          heartBeat,
		autoHeartbeat:      autoHeartbeat,
		disableHealthCheck: disableHealthCheck,
		endpointMetadata:   cloneMetadata(endpoint.Metadata),
		location:           cloneLocation(location),
	}, nil
}

func validateInstanceSpecIdentity(specs []instanceSpec) error {
	addresses := make(map[string]instanceSpec, len(specs))
	instanceIDs := make(map[string]int, len(specs))
	for _, spec := range specs {
		address := instanceAddress(spec)
		if previous, ok := addresses[address]; ok {
			if previous.instanceID == "" || spec.instanceID == "" {
				return fmt.Errorf(
					"endpoints %d and %d use duplicate identity %q; explicit instance_id is required to share an address",
					previous.order, spec.order, address)
			}
		}
		addresses[address] = spec
		if spec.instanceID == "" {
			continue
		}
		if previous, ok := instanceIDs[spec.instanceID]; ok {
			return fmt.Errorf("endpoints %d and %d use duplicate instance_id %q",
				previous, spec.order, spec.instanceID)
		}
		instanceIDs[spec.instanceID] = spec.order
	}
	return nil
}

func findInstanceIDCollision(
	candidate *registeredInstance,
	registered []*registeredInstance,
	remaining []instanceSpec,
) (bool, error) {
	for _, instance := range registered {
		if instance.instanceID == candidate.instanceID {
			return false, fmt.Errorf(
				"provider returned duplicate instance id %q for endpoints %d and %d",
				candidate.instanceID, instance.spec.order, candidate.spec.order)
		}
	}
	for _, spec := range remaining {
		if spec.instanceID == candidate.instanceID {
			return true, fmt.Errorf(
				"provider returned instance id %q for endpoint %d, but it is reserved by endpoint %d",
				candidate.instanceID, candidate.spec.order, spec.order)
		}
	}
	return false, nil
}

// shouldCleanupFailedRegistration reports whether a failed registration may
// still have reached the server. The Provider API does not expose that outcome,
// so an explicit instance ID remains the only ownership-safe cleanup target.
func shouldCleanupFailedRegistration(spec *instanceSpec, registerErr error) bool {
	var validationErr *registrationValidationError
	if errors.As(registerErr, &validationErr) {
		return false
	}
	return spec.instanceID != ""
}

func newPendingRegisteredInstance(spec instanceSpec, instanceID string) *registeredInstance {
	return &registeredInstance{
		spec:       spec,
		instanceID: instanceID,
		statuses:   make(chan healthcheck.Status, 1),
	}
}

func (r *Registry) registerInstance(spec *instanceSpec) (*registeredInstance, error) {
	req := &api.InstanceRegisterRequest{
		InstanceRegisterRequest: model.InstanceRegisterRequest{
			Namespace:    r.cfg.Namespace,
			Service:      r.cfg.ServiceName,
			Host:         spec.host,
			Port:         spec.port,
			ServiceToken: spec.serviceToken,
			Weight:       cloneInt(spec.weight),
			Metadata:     cloneMetadata(spec.metadata),
			Location:     cloneLocation(spec.location),
			InstanceId:   spec.instanceID,
		},
	}
	if spec.setProtocol {
		protocol := spec.protocol
		req.Protocol = &protocol
	}
	if spec.version != "" {
		version := spec.version
		req.Version = &version
	}
	if !spec.disableHealthCheck {
		req.SetTTL(spec.ttl)
	}
	if err := req.Validate(); err != nil {
		return nil, &registrationValidationError{err: err}
	}

	resp, err := r.Provider.Register(req)
	if err != nil {
		return nil, err
	}
	instanceID := spec.instanceID
	if resp != nil && resp.InstanceID != "" {
		instanceID = resp.InstanceID
	}
	if instanceID == "" {
		return nil, fmt.Errorf("provider returned an empty instance id")
	}
	plog.GetBaseLogger().Debugf(
		"registered service=%s namespace=%s instanceID=%s address=%s protocol=%s autoHeartbeat=%v",
		r.cfg.ServiceName, r.cfg.Namespace, instanceID, instanceAddress(*spec), spec.protocol, spec.autoHeartbeat)
	return &registeredInstance{
		spec:       *spec,
		instanceID: instanceID,
		statuses:   make(chan healthcheck.Status, 1),
	}, nil
}

func (r *Registry) startManualHeartbeat(instance *registeredInstance) {
	if instance.spec.disableHealthCheck {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	instance.cancel = cancel

	r.statusMu.Lock()
	r.state.heartbeatStatuses[instance] = instance.statuses
	statusKnown := r.statusKnown
	serviceStatus := r.serviceStatus
	r.statusMu.Unlock()

	r.heartbeatWG.Add(1)
	go r.heartbeatLoop(ctx, instance, statusKnown, serviceStatus)
}

func (r *Registry) heartbeatLoop(
	ctx context.Context,
	instance *registeredInstance,
	statusKnown bool,
	serviceStatus healthcheck.Status,
) {
	defer r.heartbeatWG.Done()
	frozenTicker := &time.Ticker{}
	ticker := frozenTicker
	startTicker := func() {
		r.heartBeat(instance)
		ticker = time.NewTicker(time.Second * time.Duration(instance.spec.heartBeat))
	}

	if !statusKnown || serviceStatus == healthcheck.Serving {
		startTicker()
	} else {
		plog.GetBaseLogger().Debugf(
			"heartbeat delayed service=%s instanceID=%s until service status is serving",
			r.cfg.ServiceName, instance.instanceID)
	}

	for {
		select {
		case <-ctx.Done():
			if ticker != frozenTicker {
				ticker.Stop()
			}
			return
		case <-ticker.C:
			r.heartBeat(instance)
		case status := <-instance.statuses:
			if status != healthcheck.Serving && ticker != frozenTicker {
				plog.GetBaseLogger().Debugf(
					"heartbeat stopped service=%s instanceID=%s status=%v",
					r.cfg.ServiceName, instance.instanceID, status)
				ticker.Stop()
				ticker = frozenTicker
			} else if status == healthcheck.Serving && ticker == frozenTicker {
				startTicker()
			}
		}
	}
}

func (r *Registry) observeHealthStatus(status healthcheck.Status) {
	r.statusMu.Lock()
	r.statusKnown = true
	r.serviceStatus = status
	for _, statuses := range r.state.heartbeatStatuses {
		enqueueHealthStatus(statuses, status)
	}
	r.statusMu.Unlock()
}

func enqueueHealthStatus(statuses chan healthcheck.Status, status healthcheck.Status) {
	select {
	case statuses <- status:
		return
	default:
	}
	select {
	case <-statuses:
	default:
	}
	select {
	case statuses <- status:
	default:
	}
}

func (r *Registry) stopManualHeartbeats() {
	for _, instance := range r.state.instances {
		if instance.cancel != nil {
			instance.cancel()
		}
	}
	r.heartbeatWG.Wait()
	r.statusMu.Lock()
	for instance := range r.state.heartbeatStatuses {
		delete(r.state.heartbeatStatuses, instance)
	}
	r.statusMu.Unlock()
}

func (r *Registry) heartBeat(instance *registeredInstance) {
	req := &api.InstanceHeartbeatRequest{
		InstanceHeartbeatRequest: model.InstanceHeartbeatRequest{
			Service:      r.cfg.ServiceName,
			ServiceToken: instance.spec.serviceToken,
			Namespace:    r.cfg.Namespace,
			InstanceID:   instance.instanceID,
			Host:         instance.spec.host,
			Port:         instance.spec.port,
		},
	}
	if err := r.Provider.Heartbeat(req); err != nil {
		instance.heartbeatFailures++
		now := time.Now()
		if instance.heartbeatFailures == 1 ||
			instance.lastHeartbeatFailLog.IsZero() ||
			now.Sub(instance.lastHeartbeatFailLog) >= heartbeatErrorLogInterval {
			plog.GetBaseLogger().Errorf(
				"heartbeat report failed service=%s instanceID=%s address=%s protocol=%s failures=%d err=%v",
				r.cfg.ServiceName, instance.instanceID, instanceAddress(instance.spec),
				instance.spec.protocol, instance.heartbeatFailures, err)
			instance.lastHeartbeatFailLog = now
		}
		metrics.ReportHeartBeatFail(req)
		return
	}
	if instance.heartbeatFailures > 0 {
		plog.GetBaseLogger().Infof(
			"heartbeat report recovered service=%s instanceID=%s address=%s protocol=%s failures=%d",
			r.cfg.ServiceName, instance.instanceID, instanceAddress(instance.spec),
			instance.spec.protocol, instance.heartbeatFailures)
		instance.heartbeatFailures = 0
		instance.lastHeartbeatFailLog = time.Time{}
	}
	plog.GetBaseLogger().Debugf(
		"heartbeat report succeeded service=%s instanceID=%s address=%s protocol=%s",
		r.cfg.ServiceName, instance.instanceID, instanceAddress(instance.spec), instance.spec.protocol)
}

// Deregister anti-registration.
func (r *Registry) Deregister(_ string) error {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	r.stopManualHeartbeats()
	if !r.cfg.EnableRegister {
		r.state.instances = nil
		r.setRegisteredVersion("")
		return nil
	}
	remaining, err := r.deregisterInstances(r.state.instances)
	r.state.instances = remaining
	if len(remaining) == 0 {
		r.setRegisteredVersion("")
	}
	return err
}

func (r *Registry) deregisterInstances(instances []*registeredInstance) ([]*registeredInstance, error) {
	if len(instances) == 0 {
		return nil, nil
	}
	if r.Provider == nil {
		return instances, fmt.Errorf("deregister service %q: provider is nil", r.cfg.ServiceName)
	}
	remaining := make([]*registeredInstance, 0, len(instances))
	errs := make([]error, 0)
	for _, instance := range instances {
		if err := r.deregisterSpecWithID(&instance.spec, instance.instanceID); err != nil {
			remaining = append(remaining, instance)
			errs = append(errs, fmt.Errorf("deregister instance %s address=%s protocol=%s: %w",
				instance.instanceID, instanceAddress(instance.spec), instance.spec.protocol, err))
			continue
		}
		plog.GetBaseLogger().Debugf(
			"deregistered service=%s instanceID=%s address=%s protocol=%s",
			r.cfg.ServiceName, instance.instanceID, instanceAddress(instance.spec), instance.spec.protocol)
	}
	if len(errs) == 0 {
		return nil, nil
	}
	messages := make([]string, 0, len(errs))
	for _, err := range errs {
		messages = append(messages, err.Error())
	}
	return remaining, fmt.Errorf("%s", strings.Join(messages, "; "))
}

func (r *Registry) deregisterSpec(spec *instanceSpec) error {
	return r.deregisterSpecWithID(spec, spec.instanceID)
}

func (r *Registry) deregisterSpecWithID(spec *instanceSpec, instanceID string) error {
	req := &api.InstanceDeRegisterRequest{
		InstanceDeRegisterRequest: model.InstanceDeRegisterRequest{
			Service:      r.cfg.ServiceName,
			Namespace:    r.cfg.Namespace,
			InstanceID:   instanceID,
			ServiceToken: spec.serviceToken,
			Host:         spec.host,
			Port:         spec.port,
		},
	}
	return r.Provider.Deregister(req)
}

// SDKContext returns the Polaris SDK context backing this registry.
func (r *Registry) SDKContext() api.SDKContext {
	if r == nil {
		return nil
	}
	if r.sdkCtx != nil {
		return r.sdkCtx
	}
	if r.Provider != nil {
		return r.Provider.SDKContext()
	}
	return nil
}

// Version returns the instance version configured for this registry.
func (r *Registry) Version() string {
	if r == nil || r.cfg == nil {
		return ""
	}
	r.versionMu.RLock()
	registeredVersion := r.registeredVersion
	r.versionMu.RUnlock()
	if registeredVersion != "" {
		return registeredVersion
	}
	return r.cfg.Version
}

// Config returns a defensive copy of the immutable registration configuration.
// Extensions may use it to validate their own configuration before publishing a provider.
func (r *Registry) Config() *Config {
	if r == nil || r.cfg == nil {
		return nil
	}
	return cloneConfig(r.cfg)
}

func (r *Registry) setRegisteredVersion(version string) {
	r.versionMu.Lock()
	r.registeredVersion = version
	r.versionMu.Unlock()
}

// FromFramework returns the Polaris registry bound to service by tRPC-Go.
func FromFramework(service string) (*Registry, bool) {
	r, ok := registry.Get(service).(*Registry)
	if !ok || r == nil {
		return nil, false
	}
	return r, true
}

func cloneConfig(cfg *Config) *Config {
	cloned := *cfg
	cloned.Metadata = cloneMetadata(cfg.Metadata)
	cloned.Weight = cloneInt(cfg.Weight)
	cloned.InstanceLocation = cloneLocation(cfg.InstanceLocation)
	if cfg.Endpoints == nil {
		return &cloned
	}
	cloned.Endpoints = make([]Endpoint, len(cfg.Endpoints))
	for i, endpoint := range cfg.Endpoints {
		cloned.Endpoints[i] = endpoint
		cloned.Endpoints[i].Weight = cloneInt(endpoint.Weight)
		cloned.Endpoints[i].Metadata = cloneMetadata(endpoint.Metadata)
		cloned.Endpoints[i].InstanceLocation = cloneLocation(endpoint.InstanceLocation)
		cloned.Endpoints[i].AutoHeartbeat = cloneBool(endpoint.AutoHeartbeat)
		cloned.Endpoints[i].DisableHealthCheck = cloneBool(endpoint.DisableHealthCheck)
	}
	return &cloned
}

func applyConfigDefaults(cfg *Config) error {
	if cfg.ServiceName == "" {
		return fmt.Errorf("registry service name must not be empty")
	}
	if cfg.Namespace == "" {
		cfg.Namespace = "default"
	}
	if cfg.HeartBeat < 0 {
		return fmt.Errorf("registry service %q heartbeat interval must not be negative", cfg.ServiceName)
	}
	if cfg.HeartBeat == 0 {
		cfg.HeartBeat = defaultHeartBeat
	}
	if cfg.TTL < 0 {
		return fmt.Errorf("registry service %q TTL must not be negative", cfg.ServiceName)
	}
	if cfg.TTL == 0 {
		cfg.TTL = defaultTTL
	}
	if cfg.Weight == nil {
		cfg.Weight = &defaultWeight
	}
	if len(cfg.Endpoints) > 0 && (cfg.BindAddress != "" || cfg.PreferBindAddress) {
		return fmt.Errorf(
			"registry service %q endpoints cannot be combined with bind_address or prefer_bind_address",
			cfg.ServiceName)
	}
	if len(cfg.Endpoints) == 0 && cfg.EnableRegister && cfg.AutoHeartbeat && cfg.DisableHealthCheck {
		return fmt.Errorf("registry service %q auto heartbeat conflicts with disabled health check",
			cfg.ServiceName)
	}
	return nil
}

func parseInstanceAddress(address string) (string, int, error) {
	host, portRaw, err := net.SplitHostPort(address)
	if err != nil {
		return "", 0, err
	}
	if host == "" {
		return "", 0, fmt.Errorf("host is empty")
	}
	port, err := strconv.Atoi(portRaw)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port: %w", err)
	}
	if port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("port %d is outside 1-65535", port)
	}
	return host, port, nil
}

func instanceAddress(spec instanceSpec) string {
	return net.JoinHostPort(spec.host, strconv.Itoa(spec.port))
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneLocation(location *model.Location) *model.Location {
	if location == nil {
		return nil
	}
	cloned := *location
	return &cloned
}
