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

package circuitbreaker

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"trpc.group/trpc-go/trpc-go/errs"
	"trpc.group/trpc-go/trpc-go/naming/circuitbreaker"
	"trpc.group/trpc-go/trpc-go/naming/registry"
	"trpc.group/trpc-go/trpc-naming-polarismesh/internal/mock/mock_api"
	"trpc.group/trpc-go/trpc-naming-polarismesh/internal/mock/mock_model"
	"trpc.group/trpc-go/trpc-naming-polarismesh/internal/polaris"

	"github.com/golang/mock/gomock"
	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/config"
	"github.com/polarismesh/polaris-go/pkg/model"
	"github.com/polarismesh/polaris-go/pkg/sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCircuitBreakerAPI struct {
	stats     []*model.ResourceStat
	reportErr func(*model.ResourceStat) error
}

func (f *fakeCircuitBreakerAPI) SDKContext() api.SDKContext { return nil }

func (f *fakeCircuitBreakerAPI) Check(model.Resource) (*model.CheckResult, error) {
	return nil, nil
}

func (f *fakeCircuitBreakerAPI) Report(stat *model.ResourceStat) error {
	f.stats = append(f.stats, stat)
	if f.reportErr != nil {
		return f.reportErr(stat)
	}
	return nil
}

func (f *fakeCircuitBreakerAPI) MakeFunctionDecorator(
	model.CustomerFunction, *api.RequestContext) model.DecoratorFunction {
	return nil
}

func (f *fakeCircuitBreakerAPI) MakeInvokeHandler(*api.RequestContext) model.InvokeHandler {
	return nil
}

type fakeReportEngine struct {
	sdk.Engine
	stats []*model.ResourceStat
}

func (e *fakeReportEngine) Report(stat *model.ResourceStat) error {
	e.stats = append(e.stats, stat)
	return nil
}

type fakeReportSDKContext struct {
	api.SDKContext
	engine sdk.Engine
	config config.Configuration
}

func (c *fakeReportSDKContext) GetEngine() sdk.Engine { return c.engine }

func (c *fakeReportSDKContext) GetConfig() config.Configuration { return c.config }

func newReportSDKContext(enabled bool) *fakeReportSDKContext {
	cfg := api.NewConfiguration()
	cfg.GetConsumer().GetCircuitBreaker().SetEnable(enabled)
	return &fakeReportSDKContext{config: cfg}
}

func TestSetUp(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_api.NewMockSDKContext(ctrl)
	assert.Nil(t, Setup(m, &Config{Name: "polarismesh"}, true))
	assert.NotNil(t, circuitbreaker.Get("polarismesh"))
	assert.NotNil(t, circuitbreaker.DefaultCircuitBreaker)
}

func TestAvailable(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cb := &CircuitBreaker{
		consumer:           mock_api.NewMockConsumerAPI(ctrl),
		shouldCircuitBreak: newShouldCircuitBreak(DeltaTimeout),
	}
	inst := mock_model.NewMockInstance(ctrl)
	status := mock_model.NewMockCircuitBreakerStatus(ctrl)
	inst.EXPECT().GetCircuitBreakerStatus().Return(status).AnyTimes()
	status.EXPECT().GetStatus().Return(model.Close).AnyTimes()
	available := cb.Available(&registry.Node{
		Metadata: map[string]interface{}{
			"instance": inst,
		},
	})
	assert.True(t, available)
	available = cb.Available(&registry.Node{
		Metadata: map[string]interface{}{},
	})
	assert.False(t, available)
}

func TestAvailableHalfOpenQuota(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cb := &CircuitBreaker{shouldCircuitBreak: newShouldCircuitBreak(DeltaTimeout)}
	status := model.NewHalfOpenStatus("test", time.Now(), 1).(*model.HalfOpenStatus)
	inst := mock_model.NewMockInstance(ctrl)
	inst.EXPECT().GetCircuitBreakerStatus().Return(status).AnyTimes()
	node := &registry.Node{Metadata: map[string]interface{}{"instance": inst}}

	assert.True(t, cb.Available(node))
	require.True(t, status.AcquirePermission())
	assert.False(t, cb.Available(node))
}

func TestReport(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_api.NewMockConsumerAPI(ctrl)
	m.EXPECT().SDKContext().Return(newReportSDKContext(true)).AnyTimes()
	m.EXPECT().UpdateServiceCallResult(gomock.Any()).Return(nil).AnyTimes()
	inst := expectReportInstance(ctrl)
	cb := &CircuitBreaker{
		consumer:           m,
		circuitBreaker:     &fakeCircuitBreakerAPI{},
		shouldCircuitBreak: newShouldCircuitBreak(DeltaTimeout),
	}
	node := &registry.Node{
		Metadata: map[string]interface{}{
			"instance": inst,
		},
	}
	err := cb.Report(node, time.Second, nil)
	assert.Nil(t, err)
	require.Len(t, cb.circuitBreaker.(*fakeCircuitBreakerAPI).stats, 2)
	require.IsType(t, &model.ServiceResource{}, cb.circuitBreaker.(*fakeCircuitBreakerAPI).stats[0].Resource)
	err = cb.Report(&registry.Node{}, time.Second, errors.New("not success"))
	assert.NotNil(t, err)
}

func TestReportUsesSDKFailureSentinel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	consumer := mock_api.NewMockConsumerAPI(ctrl)
	consumer.EXPECT().SDKContext().Return(newReportSDKContext(true)).AnyTimes()
	var legacyResult *api.ServiceCallResult
	consumer.EXPECT().UpdateServiceCallResult(gomock.Any()).
		Do(func(result *api.ServiceCallResult) { legacyResult = result }).
		Return(nil).Times(1)
	breaker := &fakeCircuitBreakerAPI{}
	cb := &CircuitBreaker{
		consumer:           consumer,
		circuitBreaker:     breaker,
		shouldCircuitBreak: newShouldCircuitBreak(DeltaTimeout),
	}

	err := cb.Report(&registry.Node{
		Metadata: map[string]interface{}{"instance": expectReportInstance(ctrl)},
	}, time.Second, errs.NewFrameError(errs.RetClientNetErr, "network failure"))

	require.NoError(t, err)
	require.Len(t, breaker.stats, 2)
	require.IsType(t, &model.ServiceResource{}, breaker.stats[0].Resource)
	require.IsType(t, &model.InstanceResource{}, breaker.stats[1].Resource)
	assert.Equal(t, "-1", breaker.stats[0].RetCode)
	assert.Equal(t, model.RetFail, breaker.stats[0].RetStatus)
	require.NotNil(t, legacyResult)
	require.NotNil(t, legacyResult.RetCode)
	assert.Equal(t, int32(errRetCode), *legacyResult.RetCode)
}

func TestReportContinuesAfterCircuitBreakerError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	consumer := mock_api.NewMockConsumerAPI(ctrl)
	consumer.EXPECT().SDKContext().Return(newReportSDKContext(true)).AnyTimes()
	var legacyResult *api.ServiceCallResult
	consumer.EXPECT().UpdateServiceCallResult(gomock.Any()).
		Do(func(result *api.ServiceCallResult) { legacyResult = result }).
		Return(nil).
		Times(1)
	breaker := &fakeCircuitBreakerAPI{
		reportErr: func(stat *model.ResourceStat) error {
			if _, ok := stat.Resource.(*model.ServiceResource); ok {
				return errors.New("service report failed")
			}
			return nil
		},
	}
	methodResource, err := polaris.MethodResource(
		&model.ServiceKey{Namespace: "default", Service: "service"},
		model.EmptyServiceKey,
		"grpc",
		"",
		"/service/method",
	)
	require.NoError(t, err)
	cb := &CircuitBreaker{
		consumer:           consumer,
		circuitBreaker:     breaker,
		shouldCircuitBreak: newShouldCircuitBreak(DeltaTimeout),
	}

	err = cb.Report(&registry.Node{
		Metadata: map[string]interface{}{
			"instance":                        expectReportInstance(ctrl),
			polaris.MethodResourceMetadataKey: methodResource,
		},
	}, time.Second, errs.NewFrameError(errs.RetClientNetErr, "network failure"))

	require.Error(t, err)
	require.Len(t, breaker.stats, 3)
	require.IsType(t, &model.ServiceResource{}, breaker.stats[0].Resource)
	require.IsType(t, &model.MethodResource{}, breaker.stats[1].Resource)
	require.IsType(t, &model.InstanceResource{}, breaker.stats[2].Resource)
	require.NotNil(t, legacyResult)
}

func TestReportWhenSDKCircuitBreakerDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	consumer := mock_api.NewMockConsumerAPI(ctrl)
	consumer.EXPECT().SDKContext().Return(newReportSDKContext(false)).AnyTimes()
	var legacyResult *api.ServiceCallResult
	consumer.EXPECT().UpdateServiceCallResult(gomock.Any()).
		Do(func(result *api.ServiceCallResult) { legacyResult = result }).
		Return(nil).Times(1)
	breaker := &fakeCircuitBreakerAPI{}
	cb := &CircuitBreaker{
		consumer:           consumer,
		circuitBreaker:     breaker,
		shouldCircuitBreak: newShouldCircuitBreak(DeltaTimeout),
	}

	err := cb.Report(&registry.Node{
		Metadata: map[string]interface{}{"instance": expectReportInstance(ctrl)},
	}, time.Second, errs.NewFrameError(errs.RetClientNetErr, "network failure"))

	require.NoError(t, err)
	assert.Empty(t, breaker.stats)
	require.NotNil(t, legacyResult)
	assert.Equal(t, model.RetFail, legacyResult.RetStatus)
}

func expectReportInstance(ctrl *gomock.Controller) *mock_model.MockInstance {
	inst := mock_model.NewMockInstance(ctrl)
	inst.EXPECT().GetNamespace().Return("default").AnyTimes()
	inst.EXPECT().GetService().Return("service").AnyTimes()
	inst.EXPECT().GetProtocol().Return("grpc").AnyTimes()
	inst.EXPECT().GetHost().Return("127.0.0.1").AnyTimes()
	inst.EXPECT().GetPort().Return(uint32(8000)).AnyTimes()
	return inst
}

func TestCanIgnoreError(t *testing.T) {
	type args struct {
		cost time.Duration
		err  error
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "normal",
			args: args{
				cost: time.Microsecond,
				err: &errs.Error{
					Type: errs.ErrorTypeFramework,
					Code: errs.RetClientTimeout,
				},
			},
			want: true,
		},
		{
			name: "cost > 1ms",
			args: args{
				cost: time.Second,
				err: &errs.Error{
					Type: errs.ErrorTypeFramework,
					Code: errs.RetClientTimeout,
				},
			},
			want: false,
		},
		{
			name: "type business",
			args: args{
				cost: time.Microsecond,
				err: &errs.Error{
					Type: errs.ErrorTypeBusiness,
					Code: errs.RetClientTimeout,
				},
			},
			want: false,
		},
		{
			name: "nil",
			args: args{},
			want: true,
		},
		{
			name: "not errs.Error",
			args: args{
				cost: time.Microsecond,
				err:  errors.New("invalid error"),
			},
			want: false,
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	inst := expectReportInstance(ctrl)
	node := &registry.Node{Metadata: map[string]interface{}{"instance": inst}}
	consumer := mock_api.NewMockConsumerAPI(ctrl)
	engine := &fakeReportEngine{}
	sdkCtx := newReportSDKContext(true)
	sdkCtx.engine = engine
	consumer.EXPECT().SDKContext().Return(sdkCtx).AnyTimes()
	consumer.EXPECT().UpdateServiceCallResult(gomock.Any()).Return(nil).AnyTimes()
	reportTimeout := time.Millisecond

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, canIgnoreError(tt.args.err, DeltaTimeout, tt.args.cost), tt.want)
			require.Nil(t, Report(consumer, node, &reportTimeout, tt.args.cost, tt.args.err))
			require.NotEmpty(t, engine.stats)
		})
	}
}

func TestWrappedFrameworkErrorsPreserveCircuitBreakerClassification(t *testing.T) {
	classify := newShouldCircuitBreak(DeltaTimeout)
	for _, tt := range []struct {
		name   string
		err    error
		cost   time.Duration
		ignore bool
		should Should
	}{
		{"short timeout", &errs.Error{Type: errs.ErrorTypeFramework, Code: errs.RetClientTimeout}, time.Microsecond, true, False},
		{"long timeout", &errs.Error{Type: errs.ErrorTypeFramework, Code: errs.RetClientTimeout}, time.Second, false, True},
		{"connect failure", &errs.Error{Type: errs.ErrorTypeFramework, Code: errs.RetClientConnectFail}, time.Microsecond, false, True},
		{"business timeout", &errs.Error{Type: errs.ErrorTypeBusiness, Code: errs.RetClientTimeout}, time.Microsecond, false, False},
		{"unrelated error", errors.New("unrelated"), time.Microsecond, false, False},
		{"typed nil", (*errs.Error)(nil), time.Microsecond, false, False},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := fmt.Errorf("call failed: %w", tt.err)
			require.Equal(t, tt.ignore, canIgnoreError(wrapped, DeltaTimeout, tt.cost))
			require.Equal(t, tt.should, classify(wrapped, tt.cost))
		})
	}
}

func TestShouldCircuitBreak(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	inst := expectReportInstance(ctrl)
	node := &registry.Node{Metadata: map[string]interface{}{"instance": inst}}

	newCB := func() (*CircuitBreaker, *mock_api.MockConsumerAPI) {
		consumer := mock_api.NewMockConsumerAPI(ctrl)
		consumer.EXPECT().SDKContext().Return(newReportSDKContext(true)).AnyTimes()
		cb := CircuitBreaker{
			consumer:           consumer,
			circuitBreaker:     &fakeCircuitBreakerAPI{},
			shouldCircuitBreak: newShouldCircuitBreak(DeltaTimeout)}
		return &cb, consumer
	}

	ShouldCircuitBreak(func(err error) Should {
		errStr := err.Error()
		if strings.HasPrefix(errStr, "true") {
			return True
		} else if strings.HasPrefix(errStr, "false") {
			return False
		} else if strings.HasPrefix(errStr, "ignore") {
			return Ignore
		} else {
			return Unknown
		}
	})

	for _, c := range []struct {
		name    string
		err     error
		success bool
		called  bool
	}{
		{name: "any is not a circuit breaker error",
			err: errors.New("any"), success: true, called: true},
		{name: "explicitly not a circuit breaker error",
			err: errors.New("false"), success: true, called: true},
		{name: "ignore this error and report not called",
			err: errors.New("ignore"), success: true, called: false},
		{name: "explicitly a circuit breaker error",
			err: errors.New("true"), success: false, called: true},
		{name: "frame client net error is always a circuit breaker error",
			err: errs.NewFrameError(errs.RetClientNetErr, "any"), success: false, called: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			cb, consumer := newCB()
			call := consumer.EXPECT().
				UpdateServiceCallResult(gomock.Any()).
				Do(func(res *api.ServiceCallResult) {
					require.Equal(t, c.success, res.RetStatus == model.RetSuccess)
				})
			if c.called {
				call.MinTimes(1)
			} else {
				call.MaxTimes(0)
			}
			require.Nil(t, cb.Report(node, time.Second, c.err))
		})
	}
}

func TestShouldCircuitBreakReadFrameError(t *testing.T) {
	shouldCircuitBreak := newShouldCircuitBreak(DeltaTimeout)
	assert.Equal(t, True, shouldCircuitBreak(
		errs.NewFrameError(errs.RetClientReadFrameErr, "read EOF"),
		DeltaTimeout,
	))
}
