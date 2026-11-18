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

// Package polaris contains helpers shared by the Polaris adapters.
package polaris

import (
	"errors"
	"fmt"

	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/config"
	"github.com/polarismesh/polaris-go/pkg/model"
	"github.com/polarismesh/polaris-go/pkg/plugin/circuitbreaker"
	"github.com/polarismesh/polaris-go/pkg/plugin/common"
)

// SourceServiceMetadataKey is the node metadata key used to retain the caller
// service through the split discovery, routing, and load-balancing stages.
const SourceServiceMetadataKey = "sourceService"

// CallerServiceMetadataKey is the circuit-breaker caller identity. It is kept
// separately from SourceServiceMetadataKey because service routing can be
// disabled without disabling caller-specific circuit-breaker rules.
const CallerServiceMetadataKey = "callerService"

// MethodResourceMetadataKey retains the method resource selected for a request
// so the report path can update the same resource that was checked.
const MethodResourceMetadataKey = "methodResource"

// AllowOpenMetadataKey records whether routing explicitly enabled recover-all
// behavior, which permits an Open instance when no better candidates remain.
const AllowOpenMetadataKey = "allowOpen"

// CircuitBreakerEnabled reports whether the SDK context initialized its breaker flow.
func CircuitBreakerEnabled(sdkCtx api.SDKContext) bool {
	if sdkCtx == nil {
		return false
	}
	cfg := sdkCtx.GetConfig()
	if cfg == nil {
		return false
	}
	consumer := cfg.GetConsumer()
	if consumer == nil {
		return false
	}
	circuitBreaker := consumer.GetCircuitBreaker()
	return circuitBreaker != nil && circuitBreaker.IsEnable()
}

// InstanceResource builds the instance-level resource used by the SDK breaker.
func InstanceResource(inst model.Instance, caller *model.ServiceKey) (*model.InstanceResource, error) {
	if inst == nil {
		return nil, errors.New("instance can not be nil")
	}
	if caller == nil {
		caller = model.EmptyServiceKey
	}
	return model.NewInstanceResource(
		&model.ServiceKey{Namespace: inst.GetNamespace(), Service: inst.GetService()},
		caller,
		inst.GetProtocol(),
		inst.GetHost(),
		inst.GetPort(),
	)
}

// ServiceResource builds the service-level resource used by the SDK breaker.
func ServiceResource(service *model.ServiceKey, caller *model.ServiceKey) (*model.ServiceResource, error) {
	if service == nil {
		return nil, errors.New("service can not be nil")
	}
	if caller == nil {
		caller = model.EmptyServiceKey
	}
	return model.NewServiceResource(service, caller)
}

// InstanceServiceResource builds the service resource represented by an
// instance. This keeps SERVICE and INSTANCE reporting on the same service key.
func InstanceServiceResource(inst model.Instance, caller *model.ServiceKey) (*model.ServiceResource, error) {
	if inst == nil {
		return nil, errors.New("instance can not be nil")
	}
	return ServiceResource(
		&model.ServiceKey{Namespace: inst.GetNamespace(), Service: inst.GetService()}, caller)
}

// MethodResource builds a method resource when a request path is available. An
// empty httpMethod is normalized to the wildcard by the SDK.
func MethodResource(
	service *model.ServiceKey,
	caller *model.ServiceKey,
	protocol string,
	httpMethod string,
	path string,
) (*model.MethodResource, error) {
	if path == "" {
		return nil, nil
	}
	if protocol == "" && httpMethod != "" {
		protocol = "http"
	}
	if caller == nil {
		caller = model.EmptyServiceKey
	}
	return model.NewMethodResourceWithAPI(service, caller, protocol, httpMethod, path)
}

// MethodResourcesEqual compares method bucket keys without relying on pointer
// identity. It is used to avoid re-acquiring the same breaker permission after
// an instance fallback selects an equivalent resource.
func MethodResourcesEqual(left, right *model.MethodResource) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.String() == right.String()
}

// SourceServiceKey converts a source service carried in node metadata into the
// service key used by circuit-breaker resources. An absent source is represented
// by an empty key, which is semantically equivalent to no caller for rule
// matching and avoids nil-pointer formatting in the SDK dependency.
func SourceServiceKey(source interface{}) *model.ServiceKey {
	if key, ok := source.(*model.ServiceKey); ok && key != nil {
		return key
	}
	info, ok := source.(*model.ServiceInfo)
	if !ok || info == nil {
		return model.EmptyServiceKey
	}
	return &model.ServiceKey{Namespace: info.Namespace, Service: info.Service}
}

// AcquireResourcePermission acquires permission from the caller-aware resource
// used by the SDK circuit breaker.
func AcquireResourcePermission(sdkCtx api.SDKContext, resource model.Resource) (bool, error) {
	if !CircuitBreakerEnabled(sdkCtx) {
		return true, nil
	}
	result, err := api.NewCircuitBreakerByContext(sdkCtx).Check(resource)
	if err != nil {
		return false, fmt.Errorf("check circuit breaker permission: %w", err)
	}
	if result == nil {
		return false, errors.New("check circuit breaker permission: nil result")
	}
	return result.Pass, nil
}

// PermissionResult identifies the resource that rejected a request.
type PermissionResult struct {
	Pass     bool
	Resource model.Resource
}

// AcquireResourcePermissions validates all resources before acquiring HalfOpen
// quotas. If a later resource rejects the request, all permissions acquired by
// this call are canceled on their original states without recording a result.
func AcquireResourcePermissions(
	sdkCtx api.SDKContext,
	resources ...model.Resource,
) (PermissionResult, error) {
	if !CircuitBreakerEnabled(sdkCtx) {
		return PermissionResult{Pass: true}, nil
	}
	breaker, err := circuitBreakerPlugin(sdkCtx)
	if err != nil {
		return PermissionResult{}, err
	}

	for _, resource := range resources {
		if resource == nil {
			continue
		}
		status := breaker.CheckResource(resource)
		if status == nil {
			continue
		}
		switch status.GetStatus() {
		case model.Open:
			return PermissionResult{Resource: resource}, nil
		case model.HalfOpen:
			halfOpen, ok := status.(*model.HalfOpenStatus)
			if ok && halfOpen.AllocatedCount() >= int64(halfOpen.MaxRequest()) {
				return PermissionResult{Resource: resource}, nil
			}
		}
	}
	acquired := make([]*model.HalfOpenStatus, 0, len(resources))
	passed := false
	defer func() {
		if !passed {
			for i := len(acquired) - 1; i >= 0; i-- {
				acquired[i].CancelPermission()
			}
		}
	}()
	for _, resource := range resources {
		if resource == nil {
			continue
		}
		status := breaker.CheckResource(resource)
		if status == nil {
			continue
		}
		if status.GetStatus() == model.Open {
			return PermissionResult{Resource: resource}, nil
		}
		if status.GetStatus() != model.HalfOpen {
			continue
		}
		halfOpen, ok := status.(*model.HalfOpenStatus)
		if !ok {
			continue
		}
		if !halfOpen.AcquirePermission() {
			return PermissionResult{Resource: resource}, nil
		}
		acquired = append(acquired, halfOpen)
	}
	passed = true
	return PermissionResult{Pass: true}, nil
}

// InstancePermissionAvailable checks whether an instance can be selected
// without consuming its HalfOpen probe quota. Final permission is acquired by
// AcquireResourcePermissions after the request's method resource is known.
func InstancePermissionAvailable(
	sdkCtx api.SDKContext,
	inst model.Instance,
	allowOpen bool,
	caller *model.ServiceKey,
) (bool, error) {
	if !CircuitBreakerEnabled(sdkCtx) {
		return true, nil
	}
	if status := inst.GetCircuitBreakerStatus(); status != nil &&
		status.GetStatus() == model.Open && allowOpen {
		return true, nil
	}
	resource, err := InstanceResource(inst, caller)
	if err != nil {
		return false, fmt.Errorf("build instance resource: %w", err)
	}
	breaker, err := circuitBreakerPlugin(sdkCtx)
	if err != nil {
		return false, err
	}
	status := breaker.CheckResource(resource)
	if status == nil {
		return true, nil
	}
	if status.GetStatus() == model.Open {
		return false, nil
	}
	if halfOpen, ok := status.(*model.HalfOpenStatus); ok &&
		halfOpen.AllocatedCount() >= int64(halfOpen.MaxRequest()) {
		return false, nil
	}
	return true, nil
}

// IsRecoverAllOpenInstance reports whether endpoint recover-all routing
// explicitly permits this caller's Open instance.
func IsRecoverAllOpenInstance(inst model.Instance, allowOpen bool) bool {
	status := inst.GetCircuitBreakerStatus()
	return allowOpen && status != nil && status.GetStatus() == model.Open
}

func circuitBreakerPlugin(sdkCtx api.SDKContext) (circuitbreaker.CircuitBreaker, error) {
	cfg := sdkCtx.GetConfig()
	name := config.DefaultCircuitBreaker
	if cfg != nil && cfg.GetConsumer() != nil && cfg.GetConsumer().GetCircuitBreaker() != nil {
		if chain := cfg.GetConsumer().GetCircuitBreaker().GetChain(); len(chain) > 0 {
			name = chain[0]
		}
	}
	plugin, err := sdkCtx.GetPlugins().GetPlugin(common.TypeCircuitBreaker, name)
	if err != nil {
		return nil, fmt.Errorf("get circuit breaker plugin %s: %w", name, err)
	}
	breaker, ok := plugin.(circuitbreaker.CircuitBreaker)
	if !ok {
		return nil, fmt.Errorf("plugin %s is not a circuit breaker", name)
	}
	return breaker, nil
}

// AcquireInstancePermission acquires one probe quota from the same
// caller-aware instance resource used for reporting. The endpoint-shared status
// is only consulted for the explicit recover-all bypass; all other states are
// resolved by the SDK so a status written by another caller cannot consume this
// caller's quota.
func AcquireInstancePermission(
	sdkCtx api.SDKContext,
	inst model.Instance,
	allowOpen bool,
	caller *model.ServiceKey,
) (bool, error) {
	if !CircuitBreakerEnabled(sdkCtx) {
		return true, nil
	}
	status := inst.GetCircuitBreakerStatus()
	if status != nil && status.GetStatus() == model.Open && allowOpen {
		return true, nil
	}

	resource, err := InstanceResource(inst, caller)
	if err != nil {
		return false, fmt.Errorf("build instance resource: %w", err)
	}
	return AcquireResourcePermission(sdkCtx, resource)
}
