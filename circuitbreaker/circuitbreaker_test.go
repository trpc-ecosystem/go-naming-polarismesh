// Tencent is pleased to support the open source community by making tRPC available.
// Copyright (C) 2023 THL A29 Limited, a Tencent company. All rights reserved.
// If you have downloaded a copy of the tRPC source code from Tencent,
// please note that tRPC source code is licensed under the Apache 2.0 License,
// A copy of the Apache 2.0 License is included in this file.

package circuitbreaker

import (
	"errors"
	"strings"
	"testing"
	"time"

	"trpc.group/trpc-go/trpc-go/errs"
	"trpc.group/trpc-go/trpc-go/naming/circuitbreaker"
	"trpc.group/trpc-go/trpc-go/naming/registry"
	"trpc.group/trpc-go/trpc-naming-polaris/mock/mock_api"
	"trpc.group/trpc-go/trpc-naming-polaris/mock/mock_model"

	"github.com/golang/mock/gomock"
	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetUp(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_api.NewMockSDKContext(ctrl)
	assert.Nil(t, Setup(m, &Config{Name: "polaris"}, true))
	assert.NotNil(t, circuitbreaker.Get("polaris"))
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
	status.EXPECT().IsAvailable().Return(true).AnyTimes()
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

func TestReport(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_api.NewMockConsumerAPI(ctrl)
	m.EXPECT().UpdateServiceCallResult(gomock.Any()).Return(nil).AnyTimes()
	cb := &CircuitBreaker{
		consumer:           m,
		shouldCircuitBreak: newShouldCircuitBreak(DeltaTimeout),
	}
	inst := mock_model.NewMockInstance(ctrl)
	node := &registry.Node{
		Metadata: map[string]interface{}{
			"instance": inst,
		},
	}
	err := cb.Report(node, time.Second, nil)
	assert.Nil(t, err)
	err = cb.Report(&registry.Node{}, time.Second, errors.New("not success"))
	assert.NotNil(t, err)
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
	inst := mock_model.NewMockInstance(ctrl)
	node := &registry.Node{Metadata: map[string]interface{}{"instance": inst}}
	consumer := mock_api.NewMockConsumerAPI(ctrl)
	consumer.EXPECT().UpdateServiceCallResult(gomock.Any()).Return(nil).AnyTimes()
	reportTimeout := time.Millisecond

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, canIgnoreError(tt.args.err, DeltaTimeout, tt.args.cost), tt.want)
			require.Nil(t, Report(consumer, node, &reportTimeout, tt.args.cost, tt.args.err))
		})
	}
}

func TestShouldCircuitBreak(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	inst := mock_model.NewMockInstance(ctrl)
	node := &registry.Node{Metadata: map[string]interface{}{"instance": inst}}

	newCB := func() (*CircuitBreaker, *mock_api.MockConsumerAPI) {
		consumer := mock_api.NewMockConsumerAPI(ctrl)
		cb := CircuitBreaker{
			consumer:           consumer,
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
