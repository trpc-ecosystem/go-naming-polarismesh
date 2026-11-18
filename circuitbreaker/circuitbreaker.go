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

// Package circuitbreaker is for circuit breaker configuration.
package circuitbreaker

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"trpc.group/trpc-go/trpc-go/errs"
	"trpc.group/trpc-go/trpc-go/naming/circuitbreaker"
	"trpc.group/trpc-go/trpc-go/naming/registry"
	"trpc.group/trpc-go/trpc-naming-polarismesh/internal/polaris"

	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/model"
)

// Config is circuit breaker configuration.
type Config struct {
	// Name is the current name of plugin.
	Name string
	// ReportTimeout If ReportTimeout is set, when the downstream times out and the time is less than the set value,
	// the error will be ignored and not reported.
	ReportTimeout *time.Duration
}

const (
	errRetCode = 10000
	// sdkErrRetCode is the sentinel used by the SDK when a failed call has no
	// protocol response code. The legacy statistics code remains errRetCode.
	sdkErrRetCode int32 = -1
	// DeltaTimeout is the default minimum request cost to trigger circuit breaker.
	DeltaTimeout = time.Millisecond
)

// Setup is for setting up.
func Setup(sdkCtx api.SDKContext, cfg *Config, setDefault bool) error {
	minClientTimeout := DeltaTimeout
	if cfg != nil && cfg.ReportTimeout != nil {
		minClientTimeout = *cfg.ReportTimeout
	}
	name := "polarismesh"
	if cfg != nil && cfg.Name != "" {
		name = cfg.Name
	}
	cb := &CircuitBreaker{
		consumer:           api.NewConsumerAPIByContext(sdkCtx),
		circuitBreaker:     api.NewCircuitBreakerByContext(sdkCtx),
		shouldCircuitBreak: newShouldCircuitBreak(minClientTimeout),
	}
	circuitbreaker.Register(name, cb)
	if setDefault {
		circuitbreaker.SetDefaultCircuitBreaker(cb)
	}
	return nil
}

// CircuitBreaker is the circuit breaker structure.
type CircuitBreaker struct {
	consumer           api.ConsumerAPI
	circuitBreaker     api.CircuitBreakerAPI
	shouldCircuitBreak func(error, time.Duration) Should
}

// Available determines whether the node is available.
func (cb *CircuitBreaker) Available(node *registry.Node) bool {
	if node == nil || node.Metadata == nil {
		return false
	}
	allowOpen := false
	if value, ok := node.Metadata[polaris.AllowOpenMetadataKey].(bool); ok {
		allowOpen = value
	}
	inst, ok := node.Metadata["instance"].(model.Instance)
	if !ok {
		return false
	}
	status := inst.GetCircuitBreakerStatus()
	if status == nil {
		return true
	}
	if status.GetStatus() == model.HalfOpen {
		if halfOpen, ok := status.(*model.HalfOpenStatus); ok {
			return halfOpen.AllocatedCount() < int64(halfOpen.MaxRequest())
		}
	}
	return allowOpen || status.GetStatus() != model.Open
}

// Report reports the request status.
func (cb *CircuitBreaker) Report(node *registry.Node, cost time.Duration, err error) error {
	retStatus := model.RetSuccess
	var retCode int32
	if err != nil {
		switch cb.shouldCircuitBreak(err, cost) {
		case True:
			retStatus = model.RetFail
			retCode = errRetCode
		case False:
		default:
			// Unknown or Ignore will not be reported, will return directly.
			return nil
		}
	}
	return reportCallResult(cb.consumer, cb.circuitBreaker, node, cost, retStatus, retCode)
}

// Report reports the request status.
func Report(
	consumer api.ConsumerAPI,
	node *registry.Node,
	reportTimeout *time.Duration,
	cost time.Duration,
	err error,
) error {
	delta := DeltaTimeout
	if reportTimeout != nil {
		delta = *reportTimeout
	}
	retStatus := model.RetSuccess
	var retCode int32
	if !canIgnoreError(err, delta, cost) {
		retStatus = model.RetFail
		retCode = errRetCode
	}
	sdkCtx := consumer.SDKContext()
	return reportCallResult(consumer, api.NewCircuitBreakerByContext(sdkCtx),
		node, cost, retStatus, retCode)
}

func reportCallResult(
	consumer api.ConsumerAPI,
	circuitBreaker api.CircuitBreakerAPI,
	node *registry.Node,
	cost time.Duration,
	retStatus model.RetStatus,
	retCode int32,
) error {
	if node == nil || node.Metadata == nil {
		return errors.New("report err: invalid node metadata")
	}
	inst, ok := node.Metadata["instance"].(model.Instance)
	if !ok {
		return errors.New("report err: invalid instance")
	}

	sdkCtx := consumer.SDKContext()
	if sdkCtx == nil {
		return errors.New("report err: nil sdk context")
	}
	var reportErr error
	if polaris.CircuitBreakerEnabled(sdkCtx) {
		caller := nodeCallerServiceKey(node)
		serviceResource, err := polaris.InstanceServiceResource(inst, caller)
		if err != nil {
			return fmt.Errorf("report err: build service resource: %w", err)
		}
		instanceResource, err := polaris.InstanceResource(inst, caller)
		if err != nil {
			return fmt.Errorf("report err: build instance resource: %w", err)
		}
		resources := []model.Resource{serviceResource, instanceResource}
		if methodResource, ok := node.Metadata[polaris.MethodResourceMetadataKey].(*model.MethodResource); ok && methodResource != nil {
			resources = []model.Resource{serviceResource, methodResource, instanceResource}
		}
		resourceRetCode := retCode
		if retStatus == model.RetFail {
			resourceRetCode = sdkErrRetCode
		}
		if circuitBreaker == nil {
			reportErr = errors.New("report circuit breaker err: nil circuit breaker api")
		} else {
			for _, resource := range resources {
				if err := circuitBreaker.Report(&model.ResourceStat{
					Resource:  resource,
					RetCode:   strconv.Itoa(int(resourceRetCode)),
					Delay:     cost,
					RetStatus: retStatus,
				}); err != nil && reportErr == nil {
					reportErr = fmt.Errorf("report circuit breaker err: %w", err)
				}
			}
		}
	}

	if err := consumer.UpdateServiceCallResult(&api.ServiceCallResult{
		ServiceCallResult: model.ServiceCallResult{
			CalledInstance: inst,
			RetStatus:      retStatus,
			Delay:          &cost,
			RetCode:        &retCode,
		},
	}); err != nil && reportErr == nil {
		return fmt.Errorf("report err: %w", err)
	}
	return reportErr
}

func nodeCallerServiceKey(node *registry.Node) *model.ServiceKey {
	if value, ok := node.Metadata[polaris.CallerServiceMetadataKey]; ok {
		return polaris.SourceServiceKey(value)
	}
	return polaris.SourceServiceKey(node.Metadata[polaris.SourceServiceMetadataKey])
}

func canIgnoreError(e error, reportTimeout, cost time.Duration) bool {
	if e == nil {
		return true
	}
	// when errorCode==101 && cost < reportTimeout && errorType==framework,
	// the circuit breaker will consider it as a normal situation.
	var err *errs.Error
	if errors.As(e, &err) && err != nil &&
		err.Code == errs.RetClientTimeout &&
		err.Type == errs.ErrorTypeFramework &&
		cost < reportTimeout {
		return true
	}
	return false
}

// ShouldCircuitBreak judges whether an error should be counted as a circuit breaker by f.
//
// True indicates that it should be counted as a blown error.
// False means that it should not be counted as a circuit breaker error, and the circuit breaker will report success.
// Ignore means that the error should be ignored by the circuit breaker without any reporting.
// Unknown means that f cannot be judged,
// and this error will be handed over to the original shouldCircuitBreak function to continue to judge.
//
// If the final result is still Unknown, the circuit breaker will skip this error report.
//
// This function is not concurrency safe.
// Must be set at the beginning of the main function. Executing after tRPC has started has no effect.
//
// Before the trpc-go main library deletes the circuit breaker error logic,
// the incoming error parameter only has the following three framework error codes:
//
//	errs.RetClientConnectFail
//	errs.RetClientNetErr
//	errs.RetClientTimeout
func ShouldCircuitBreak(f func(error) Should) {
	oldNew := newShouldCircuitBreak
	newShouldCircuitBreak = func(minClientTimeout time.Duration) func(error, time.Duration) Should {
		shouldCircuitBreak := oldNew(minClientTimeout)
		return func(err error, cost time.Duration) Should {
			should := f(err)
			if should == Unknown {
				return shouldCircuitBreak(err, cost)
			}
			return should
		}
	}
}

// newShouldCircuitBreak creates a default circuit breaker error checking strategy, which is also a bottom-up strategy.
var newShouldCircuitBreak = func(minClientTimeout time.Duration) func(error, time.Duration) Should {
	return func(err error, cost time.Duration) Should {
		var e *errs.Error
		if errors.As(err, &e) && e != nil &&
			e.Type == errs.ErrorTypeFramework &&
			(e.Code == errs.RetClientConnectFail ||
				e.Code == errs.RetClientNetErr ||
				e.Code == errs.RetClientReadFrameErr ||
				e.Code == errs.RetClientTimeout && cost >= minClientTimeout) {
			return True
		}
		return False
	}
}

// Should determines whether it should be counted as a circuit breaker error.
type Should int

// True/False indicates whether the current error should be counted as a circuit breaker error.
// Ignore means that the current error should be ignored by the circuit breaker,
// and no report will be made.
// Unknown means that we don't know whether it should be regarded as a circuit breaker error,
// and it will be handed over to the next function to judge.
const (
	Unknown Should = iota
	True
	False
	Ignore
)
