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

// Package circuitbreaker is for circuit breaker configuration.
package circuitbreaker

import (
	"errors"
	"fmt"
	"time"

	"trpc.group/trpc-go/trpc-go/errs"
	"trpc.group/trpc-go/trpc-go/naming/circuitbreaker"
	"trpc.group/trpc-go/trpc-go/naming/registry"

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
	shouldCircuitBreak func(error, time.Duration) Should
}

// Available determines whether the node is available.
func (cb *CircuitBreaker) Available(node *registry.Node) bool {
	inst, ok := node.Metadata["instance"].(model.Instance)
	if !ok {
		return false
	}
	if inst.GetCircuitBreakerStatus() == nil {
		return true
	}
	return inst.GetCircuitBreakerStatus().IsAvailable()
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
	inst, ok := node.Metadata["instance"].(model.Instance)
	if !ok {
		return errors.New("report err: invalid instance")
	}

	return cb.consumer.UpdateServiceCallResult(&api.ServiceCallResult{
		ServiceCallResult: model.ServiceCallResult{
			CalledInstance: inst,
			RetStatus:      retStatus,
			Delay:          &cost,
			RetCode:        &retCode,
		},
	})
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
	inst, ok := node.Metadata["instance"].(model.Instance)
	if !ok {
		return errors.New("report err: invalid instance")
	}
	if err := consumer.UpdateServiceCallResult(&api.ServiceCallResult{
		ServiceCallResult: model.ServiceCallResult{
			CalledInstance: inst,
			RetStatus:      retStatus,
			Delay:          &cost,
			RetCode:        &retCode,
		},
	}); err != nil {
		return fmt.Errorf("report err: %v", err)
	}
	return nil
}

func canIgnoreError(e error, reportTimeout, cost time.Duration) bool {
	if e == nil {
		return true
	}
	// when errorCode==101 && cost < reportTimeout && errorType==framework,
	// the circuit breaker will consider it as a normal situation.
	err, ok := e.(*errs.Error)
	if ok &&
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
		if e, ok := err.(*errs.Error); ok &&
			e.Type == errs.ErrorTypeFramework &&
			(e.Code == errs.RetClientConnectFail ||
				e.Code == errs.RetClientNetErr ||
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
