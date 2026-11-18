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

package polaris

import (
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"trpc.group/trpc-go/trpc-naming-polarismesh/internal/mock/mock_api"
	"trpc.group/trpc-go/trpc-naming-polarismesh/internal/mock/mock_plugin"

	"github.com/golang/mock/gomock"
	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/config"
	"github.com/polarismesh/polaris-go/pkg/model"
	"github.com/stretchr/testify/require"
)

func TestAcquireResourcePermissionsRejectsExhaustedLowerHalfOpenWithoutUpperLeak(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service := &model.ServiceKey{Namespace: "default", Service: "service"}
	caller := model.EmptyServiceKey
	serviceResource, err := ServiceResource(service, caller)
	require.NoError(t, err)
	methodResource, err := MethodResource(service, caller, "grpc", "GET", "/Service/Call")
	require.NoError(t, err)
	serviceHalfOpen := model.NewHalfOpenStatus("service", time.Now(), 2).(*model.HalfOpenStatus)
	methodHalfOpen := model.NewHalfOpenStatus("method", time.Now(), 1).(*model.HalfOpenStatus)
	sdkCtx, _ := circuitBreakerTestContext(t, ctrl, &fakeCircuitBreaker{
		statuses: map[string]model.CircuitBreakerStatus{
			serviceResource.String(): serviceHalfOpen,
			methodResource.String():  methodHalfOpen,
		},
	})

	permission, err := AcquireResourcePermissions(sdkCtx, serviceResource, methodResource)
	require.NoError(t, err)
	require.True(t, permission.Pass)
	require.Equal(t, int64(1), serviceHalfOpen.AllocatedCount())
	require.Equal(t, int64(1), methodHalfOpen.AllocatedCount())

	permission, err = AcquireResourcePermissions(sdkCtx, serviceResource, methodResource)
	require.NoError(t, err)
	require.False(t, permission.Pass)
	require.Equal(t, methodResource, permission.Resource)
	require.Equal(t, int64(1), serviceHalfOpen.AllocatedCount())
}

func TestAcquireResourcePermissionsRejectsOpenTransitionDuringAcquisition(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service := &model.ServiceKey{Namespace: "default", Service: "transition"}
	serviceResource, err := ServiceResource(service, model.EmptyServiceKey)
	require.NoError(t, err)
	halfOpen := model.NewHalfOpenStatus("transition", time.Now(), 1).(*model.HalfOpenStatus)
	breaker := &fakeCircuitBreaker{
		statuses: map[string]model.CircuitBreakerStatus{
			serviceResource.String(): halfOpen,
		},
		checkStatus: func(_ model.Resource, check int) model.CircuitBreakerStatus {
			if check == 2 {
				return model.NewCircuitBreakerStatus("transition", model.Open, time.Now())
			}
			return halfOpen
		},
	}
	sdkCtx, _ := circuitBreakerTestContext(t, ctrl, breaker)

	permission, err := AcquireResourcePermissions(sdkCtx, serviceResource)
	require.NoError(t, err)
	require.False(t, permission.Pass)
	require.Equal(t, serviceResource, permission.Resource)
	require.Equal(t, int64(0), halfOpen.AllocatedCount())
}

func TestAcquireResourcePermissionsRejectsLowerOpenTransitionWithoutUpperLeak(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service := &model.ServiceKey{Namespace: "default", Service: "transition"}
	serviceResource, err := ServiceResource(service, model.EmptyServiceKey)
	require.NoError(t, err)
	methodResource, err := MethodResource(service, model.EmptyServiceKey, "grpc", "GET", "/Service/Call")
	require.NoError(t, err)
	serviceHalfOpen := model.NewHalfOpenStatus("service", time.Now(), 2).(*model.HalfOpenStatus)
	methodHalfOpen := model.NewHalfOpenStatus("method", time.Now(), 1).(*model.HalfOpenStatus)
	breaker := &fakeCircuitBreaker{
		statuses: map[string]model.CircuitBreakerStatus{
			serviceResource.String(): serviceHalfOpen,
			methodResource.String():  methodHalfOpen,
		},
		checkStatus: func(resource model.Resource, check int) model.CircuitBreakerStatus {
			if check == 4 {
				return model.NewCircuitBreakerStatus("transition", model.Open, time.Now())
			}
			if resource.String() == serviceResource.String() {
				return serviceHalfOpen
			}
			return methodHalfOpen
		},
	}
	sdkCtx, _ := circuitBreakerTestContext(t, ctrl, breaker)

	permission, err := AcquireResourcePermissions(sdkCtx, serviceResource, methodResource)
	require.NoError(t, err)
	require.False(t, permission.Pass)
	require.Equal(t, methodResource, permission.Resource)
	require.Equal(t, int64(0), serviceHalfOpen.AllocatedCount())
	require.Equal(t, int64(0), methodHalfOpen.AllocatedCount())
}

func TestAcquireResourcePermissionsRollsBackAfterExternalSDKReservation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service := &model.ServiceKey{Namespace: "default", Service: "exhaustion"}
	serviceResource, err := ServiceResource(service, model.EmptyServiceKey)
	require.NoError(t, err)
	methodResource, err := MethodResource(service, model.EmptyServiceKey, "grpc", "GET", "/Service/Call")
	require.NoError(t, err)
	serviceHalfOpen := model.NewHalfOpenStatus("service", time.Now(), 2).(*model.HalfOpenStatus)
	methodHalfOpen := model.NewHalfOpenStatus("method", time.Now(), 1).(*model.HalfOpenStatus)
	breaker := &fakeCircuitBreaker{
		checkStatus: func(resource model.Resource, check int) model.CircuitBreakerStatus {
			if check == 4 {
				require.Equal(t, int64(1), serviceHalfOpen.AllocatedCount())
				// Direct SDK calls reserve the same state without adapter coordination.
				require.True(t, methodHalfOpen.AcquirePermission())
			}
			if resource.String() == serviceResource.String() {
				return serviceHalfOpen
			}
			return methodHalfOpen
		},
	}
	sdkCtx, _ := circuitBreakerTestContext(t, ctrl, breaker)

	permission, err := AcquireResourcePermissions(sdkCtx, serviceResource, methodResource)
	require.NoError(t, err)
	require.False(t, permission.Pass)
	require.Equal(t, methodResource, permission.Resource)
	require.Equal(t, int64(0), serviceHalfOpen.AllocatedCount())
	require.Equal(t, int64(1), methodHalfOpen.AllocatedCount())
	require.Zero(t, serviceHalfOpen.FinishedCount())
	require.Equal(t, model.HalfOpen, serviceHalfOpen.CalNextStatus())
	require.True(t, methodHalfOpen.CancelPermission())
	permission, err = AcquireResourcePermissions(sdkCtx, serviceResource, methodResource)
	require.NoError(t, err)
	require.True(t, permission.Pass)
	require.Equal(t, int64(1), serviceHalfOpen.AllocatedCount())
	require.Equal(t, int64(1), methodHalfOpen.AllocatedCount())
}

func TestAcquireResourcePermissionsRollsBackAllQuotasAfterInstanceRejection(t *testing.T) {
	for _, rejection := range []string{"open", "exhausted"} {
		t.Run(rejection, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			service := &model.ServiceKey{Namespace: "default", Service: "instance"}
			serviceResource, err := ServiceResource(service, model.EmptyServiceKey)
			require.NoError(t, err)
			methodResource, err := MethodResource(service, model.EmptyServiceKey, "grpc", "", "/Service/Call")
			require.NoError(t, err)
			instanceResource, err := model.NewInstanceResource(service, model.EmptyServiceKey, "grpc", "127.0.0.1", 8080)
			require.NoError(t, err)
			serviceHalfOpen := model.NewHalfOpenStatus("service", time.Now(), 2).(*model.HalfOpenStatus)
			methodHalfOpen := model.NewHalfOpenStatus("method", time.Now(), 2).(*model.HalfOpenStatus)
			instanceHalfOpen := model.NewHalfOpenStatus("instance", time.Now(), 1).(*model.HalfOpenStatus)
			statuses := map[string]model.CircuitBreakerStatus{
				serviceResource.String():  serviceHalfOpen,
				methodResource.String():   methodHalfOpen,
				instanceResource.String(): instanceHalfOpen,
			}
			breaker := &fakeCircuitBreaker{
				checkStatus: func(resource model.Resource, check int) model.CircuitBreakerStatus {
					if check == 6 {
						require.Equal(t, int64(1), serviceHalfOpen.AllocatedCount())
						require.Equal(t, int64(1), methodHalfOpen.AllocatedCount())
						if rejection == "open" {
							return model.NewCircuitBreakerStatus("instance", model.Open, time.Now())
						}
						require.True(t, instanceHalfOpen.AcquirePermission())
					}
					return statuses[resource.String()]
				},
			}
			sdkCtx, _ := circuitBreakerTestContext(t, ctrl, breaker)

			permission, err := AcquireResourcePermissions(sdkCtx, serviceResource, methodResource, instanceResource)
			require.NoError(t, err)
			require.False(t, permission.Pass)
			require.Equal(t, instanceResource, permission.Resource)
			require.Zero(t, serviceHalfOpen.AllocatedCount())
			require.Zero(t, methodHalfOpen.AllocatedCount())
			require.Zero(t, serviceHalfOpen.FinishedCount())
			require.Zero(t, methodHalfOpen.FinishedCount())
			require.Equal(t, model.HalfOpen, serviceHalfOpen.CalNextStatus())
			require.Equal(t, model.HalfOpen, methodHalfOpen.CalNextStatus())
			if rejection == "exhausted" {
				require.Equal(t, int64(1), instanceHalfOpen.AllocatedCount())
			} else {
				require.Zero(t, instanceHalfOpen.AllocatedCount())
			}
		})
	}
}

func TestAcquireResourcePermissionsCancelsOriginalStateAfterReplacement(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	service := &model.ServiceKey{Namespace: "default", Service: "replacement"}
	serviceResource, err := ServiceResource(service, model.EmptyServiceKey)
	require.NoError(t, err)
	methodResource, err := MethodResource(service, model.EmptyServiceKey, "grpc", "", "/Service/Call")
	require.NoError(t, err)
	original := model.NewHalfOpenStatus("service", time.Now(), 1).(*model.HalfOpenStatus)
	replacement := model.NewHalfOpenStatus("service", time.Now(), 1).(*model.HalfOpenStatus)
	methodHalfOpen := model.NewHalfOpenStatus("method", time.Now(), 1).(*model.HalfOpenStatus)
	current := original
	sdkCtx, _ := circuitBreakerTestContext(t, ctrl, &fakeCircuitBreaker{
		checkStatus: func(resource model.Resource, check int) model.CircuitBreakerStatus {
			if check == 4 {
				require.Equal(t, int64(1), original.AllocatedCount())
				current = replacement
				require.True(t, replacement.AcquirePermission())
				return model.NewCircuitBreakerStatus("method", model.Open, time.Now())
			}
			if resource.String() == serviceResource.String() {
				return current
			}
			return methodHalfOpen
		},
	})

	permission, err := AcquireResourcePermissions(sdkCtx, serviceResource, methodResource)
	require.NoError(t, err)
	require.False(t, permission.Pass)
	require.Equal(t, methodResource, permission.Resource)
	require.Zero(t, original.AllocatedCount())
	require.Equal(t, int64(1), replacement.AllocatedCount())
}

func TestAcquireResourcePermissionsConcurrentLowerExhaustionDoesNotLeakUpperQuota(t *testing.T) {
	const requests = 32
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service := &model.ServiceKey{Namespace: "default", Service: "concurrent"}
	serviceResource, err := ServiceResource(service, model.EmptyServiceKey)
	require.NoError(t, err)
	methodResource, err := MethodResource(service, model.EmptyServiceKey, "grpc", "", "/Service/Call")
	require.NoError(t, err)
	serviceHalfOpen := model.NewHalfOpenStatus("service", time.Now(), requests).(*model.HalfOpenStatus)
	methodHalfOpen := model.NewHalfOpenStatus("method", time.Now(), 1).(*model.HalfOpenStatus)
	statuses := map[string]model.CircuitBreakerStatus{
		serviceResource.String(): serviceHalfOpen,
		methodResource.String():  methodHalfOpen,
	}
	var validated sync.WaitGroup
	validated.Add(requests)
	var gateMu sync.Mutex
	gates := make(map[string]bool)
	sdkCtx, _ := circuitBreakerTestContext(t, ctrl, &fakeCircuitBreaker{
		checkStatus: func(resource model.Resource, _ int) model.CircuitBreakerStatus {
			if status := statuses[resource.String()]; status != nil {
				return status
			}
			gateMu.Lock()
			checked := gates[resource.String()]
			gates[resource.String()] = true
			gateMu.Unlock()
			if !checked {
				validated.Done()
				validated.Wait()
			}
			return nil
		},
	})

	type result struct {
		permission PermissionResult
		err        error
	}
	results := make(chan result, requests)
	start := make(chan struct{})
	var ready sync.WaitGroup
	ready.Add(requests)
	for i := 0; i < requests; i++ {
		gate, err := ServiceResource(&model.ServiceKey{
			Namespace: "default", Service: "gate-" + strconv.Itoa(i),
		}, model.EmptyServiceKey)
		require.NoError(t, err)
		go func(gate model.Resource) {
			ready.Done()
			<-start
			permission, err := AcquireResourcePermissions(sdkCtx, serviceResource, methodResource, gate)
			results <- result{permission: permission, err: err}
		}(gate)
	}
	ready.Wait()
	close(start)
	passed := 0
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for i := 0; i < requests; i++ {
		select {
		case result := <-results:
			require.NoError(t, result.err)
			if result.permission.Pass {
				passed++
			} else {
				require.Equal(t, methodResource, result.permission.Resource)
			}
		case <-deadline.C:
			t.Fatal("permission acquisition timed out")
		}
	}
	require.Equal(t, 1, passed)
	require.Equal(t, int64(passed), serviceHalfOpen.AllocatedCount())
	require.Equal(t, int64(passed), methodHalfOpen.AllocatedCount())
}

func circuitBreakerTestContext(
	t *testing.T, ctrl *gomock.Controller, breaker *fakeCircuitBreaker,
) (api.SDKContext, *fakeCircuitBreaker) {
	t.Helper()
	sdkConfig := config.NewDefaultConfiguration([]string{"127.0.0.1:0"})
	sdkConfig.GetConsumer().GetCircuitBreaker().SetEnable(true)
	pluginManager := mock_plugin.NewMockManager(ctrl)
	pluginManager.EXPECT().
		GetPlugin(gomock.Any(), config.DefaultCircuitBreaker).
		Return(breaker, nil).AnyTimes()
	sdkCtx := mock_api.NewMockSDKContext(ctrl)
	sdkCtx.EXPECT().GetConfig().Return(sdkConfig).AnyTimes()
	sdkCtx.EXPECT().GetPlugins().Return(pluginManager).AnyTimes()
	return sdkCtx, breaker
}

type fakeCircuitBreaker struct {
	mock_plugin.MockPlugin
	statuses    map[string]model.CircuitBreakerStatus
	checks      int64
	checkStatus func(model.Resource, int) model.CircuitBreakerStatus
}

func (b *fakeCircuitBreaker) CheckResource(resource model.Resource) model.CircuitBreakerStatus {
	if b.checkStatus != nil {
		check := atomic.AddInt64(&b.checks, 1)
		return b.checkStatus(resource, int(check))
	}
	return b.statuses[resource.String()]
}

func (b *fakeCircuitBreaker) Report(*model.ResourceStat) error {
	return nil
}
