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

package naming

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"trpc.group/trpc-go/trpc-go"
	"trpc.group/trpc-go/trpc-go/codec"
	thttp "trpc.group/trpc-go/trpc-go/http"
	trpcregistry "trpc.group/trpc-go/trpc-go/naming/registry"
	trpcselector "trpc.group/trpc-go/trpc-go/naming/selector"
	adaptercb "trpc.group/trpc-go/trpc-naming-polarismesh/circuitbreaker"
	adapterpolaris "trpc.group/trpc-go/trpc-naming-polarismesh/internal/polaris"
	adapterselector "trpc.group/trpc-go/trpc-naming-polarismesh/selector"

	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/config"
	"github.com/polarismesh/polaris-go/pkg/model"
	sdkbreaker "github.com/polarismesh/polaris-go/pkg/plugin/circuitbreaker"
	"github.com/polarismesh/polaris-go/pkg/plugin/common"
	"github.com/polarismesh/polaris-go/test/mock"
	"github.com/polarismesh/specification/source/go/api/v1/fault_tolerance"
	apispecmodel "github.com/polarismesh/specification/source/go/api/v1/model"
	apiservice "github.com/polarismesh/specification/source/go/api/v1/service_manage"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type reviewNamingServer struct {
	mock.NamingServer
	rules []*fault_tolerance.CircuitBreakerRule
}

func (s *reviewNamingServer) Discover(stream apiservice.PolarisGRPC_DiscoverServer) error {
	for {
		req, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		resp := &apiservice.DiscoverResponse{
			Code:    wrapperspb.UInt32(200000),
			Service: req.Service,
			Type:    apiservice.DiscoverResponse_DiscoverResponseType(req.Type),
		}
		switch req.Type {
		case apiservice.DiscoverRequest_INSTANCE:
			resp.Instances = s.GetServiceInstances(&model.ServiceKey{
				Namespace: req.Service.GetNamespace().GetValue(),
				Service:   req.Service.GetName().GetValue(),
			})
			resp.Service.Revision = wrapperspb.String("review-instances-v1")
		case apiservice.DiscoverRequest_CIRCUIT_BREAKER:
			resp.CircuitBreaker = &fault_tolerance.CircuitBreaker{
				Revision: wrapperspb.String("review-rules-v1"),
				Rules:    s.rules,
			}
		}
		if err := stream.Send(resp); err != nil {
			return err
		}
	}
}

func reviewErrorRateWindow(cfg *Config, window time.Duration) {
	cfg.CircuitBreaker.ErrorRate = &struct {
		ErrorRateThreshold     *float64       `yaml:"errorRateThreshold"`
		MetricNumBuckets       *int           `yaml:"metricNumBuckets"`
		MetricStatTimeWindow   *time.Duration `yaml:"metricStatTimeWindow"`
		RequestVolumeThreshold *int           `yaml:"requestVolumeThreshold"`
	}{MetricStatTimeWindow: &window}
}

func reviewLiveContext(
	t *testing.T,
	rules []*fault_tolerance.CircuitBreakerRule,
	rateWindow time.Duration,
) (api.SDKContext, model.Instance) {
	t.Helper()
	logDir, err := os.MkdirTemp("", "go-naming-review-logs-")
	require.NoError(t, err)
	t.Cleanup(func() {
		removeDirWithRetry(t, logDir)
	})
	require.NoError(t, api.SetLoggersDir(logDir))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := &reviewNamingServer{NamingServer: mock.NewNamingServer(), rules: rules}
	_, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	var portInt int
	_, err = fmt.Sscan(port, &portInt)
	require.NoError(t, err)
	server.RegisterServerServices("127.0.0.1", portInt)
	svc := &apiservice.Service{
		Name:      wrapperspb.String("review-service"),
		Namespace: wrapperspb.String("cr-review"),
	}
	server.RegisterService(svc)
	server.RegisterServiceInstances(svc, []*apiservice.Instance{{
		Id:        wrapperspb.String("review-instance"),
		Namespace: svc.Namespace,
		Service:   svc.Name,
		Host:      wrapperspb.String("127.0.0.1"),
		Port:      wrapperspb.UInt32(8000),
		Protocol:  wrapperspb.String("grpc"),
		Healthy:   wrapperspb.Bool(true),
		Weight:    wrapperspb.UInt32(100),
	}})

	grpcServer := grpc.NewServer()
	apiservice.RegisterPolarisGRPCServer(grpcServer, server)
	done := make(chan error, 1)
	go func() { done <- grpcServer.Serve(listener) }()
	t.Cleanup(func() {
		grpcServer.Stop()
		<-done
	})

	persistDir := t.TempDir()
	cfg := &Config{
		Protocol:    "grpc",
		AddressList: listener.Addr().String(),
		PersistDir:  &persistDir,
	}
	if rateWindow != 0 {
		reviewErrorRateWindow(cfg, rateWindow)
	}
	ctx, err := newSDKContext(cfg)
	require.NoError(t, err)
	t.Cleanup(ctx.Destroy)

	resp, err := api.NewConsumerAPIByContext(ctx).GetAllInstances(&api.GetAllInstancesRequest{
		GetAllInstancesRequest: model.GetAllInstancesRequest{
			Namespace: "cr-review",
			Service:   "review-service",
		},
	})
	require.NoError(t, err)
	require.Len(t, resp.Instances, 1)
	return ctx, resp.Instances[0]
}

func removeDirWithRetry(t *testing.T, dir string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var lastErr error
	for {
		lastErr = os.RemoveAll(dir)
		if lastErr == nil {
			time.Sleep(100 * time.Millisecond)
			lastErr = os.RemoveAll(dir)
			if lastErr == nil {
				if _, err := os.Stat(dir); os.IsNotExist(err) {
					return
				}
			}
		}
		if time.Now().After(deadline) {
			require.NoError(t, lastErr)
			require.NoFileExists(t, dir)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func reviewRule(caller string, level fault_tolerance.Level, sleep uint32) *fault_tolerance.CircuitBreakerRule {
	return &fault_tolerance.CircuitBreakerRule{
		Id:       "review-" + caller + "-" + level.String(),
		Name:     "review-" + caller + "-" + level.String(),
		Revision: "v1",
		Enable:   true,
		Level:    level,
		RuleMatcher: &fault_tolerance.RuleMatcher{
			Source: &fault_tolerance.RuleMatcher_SourceService{
				Namespace: "callers",
				Service:   caller,
			},
			Destination: &fault_tolerance.RuleMatcher_DestinationService{
				Namespace: "cr-review",
				Service:   "review-service",
			},
		},
		RecoverCondition: &fault_tolerance.RecoverCondition{
			SleepWindow:        sleep,
			ConsecutiveSuccess: 1,
		},
		BlockConfigs: []*fault_tolerance.BlockConfig{{
			Name: "review-block",
			ErrorConditions: []*fault_tolerance.ErrorCondition{{
				InputType: fault_tolerance.ErrorCondition_RET_CODE,
				Condition: &apispecmodel.MatchString{
					Type:  apispecmodel.MatchString_RANGE,
					Value: wrapperspb.String("500~599"),
				},
			}},
			TriggerConditions: []*fault_tolerance.TriggerCondition{{
				TriggerType: fault_tolerance.TriggerCondition_CONSECUTIVE_ERROR,
				ErrorCount:  1,
			}},
		}},
	}
}

func reviewResource(t *testing.T, inst model.Instance, caller string) *model.InstanceResource {
	t.Helper()
	resource, err := model.NewInstanceResource(
		&model.ServiceKey{Namespace: inst.GetNamespace(), Service: inst.GetService()},
		&model.ServiceKey{Namespace: "callers", Service: caller},
		inst.GetProtocol(), inst.GetHost(), inst.GetPort())
	require.NoError(t, err)
	return resource
}

func reviewRawStatus(t *testing.T, ctx api.SDKContext, resource model.Resource) model.CircuitBreakerStatus {
	t.Helper()
	plugin, err := ctx.GetPlugins().GetPlugin(common.TypeCircuitBreaker, config.DefaultCircuitBreaker)
	require.NoError(t, err)
	raw, ok := plugin.(sdkbreaker.CircuitBreaker)
	require.True(t, ok)
	return raw.CheckResource(resource)
}

func TestReviewSubsecondErrorRateWindowFailsFast(t *testing.T) {
	for _, window := range []time.Duration{500 * time.Millisecond, 1500 * time.Millisecond} {
		cfg := &Config{Protocol: "grpc", AddressList: "127.0.0.1:0"}
		reviewErrorRateWindow(cfg, window)
		_, err := newSDKContext(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "requires a whole number of seconds")
	}
}

func TestReviewOneSecondErrorRateWindowReportsWithoutPanic(t *testing.T) {
	ctx, inst := reviewLiveContext(t, nil, time.Second)
	consumer := api.NewConsumerAPIByContext(ctx)
	require.NotPanics(t, func() {
		require.NoError(t, adaptercb.Report(consumer, &trpcregistry.Node{
			Metadata: map[string]interface{}{"instance": inst},
		}, nil, time.Millisecond, nil))
	})
}

func TestReviewOtherCallerCannotConsumeProbeQuota(t *testing.T) {
	ctx, inst := reviewLiveContext(t, []*fault_tolerance.CircuitBreakerRule{
		reviewRule("orders", fault_tolerance.Level_INSTANCE, 1),
		reviewRule("payments", fault_tolerance.Level_INSTANCE, 1),
	}, 0)
	ordersResource := reviewResource(t, inst, "orders")
	paymentsResource := reviewResource(t, inst, "payments")
	breaker := api.NewCircuitBreakerByContext(ctx)
	for _, resource := range []*model.InstanceResource{ordersResource, paymentsResource} {
		require.NoError(t, breaker.Report(&model.ResourceStat{
			Resource: resource, RetCode: "0", RetStatus: model.RetSuccess,
		}))
	}
	require.Eventually(t, func() bool {
		return reviewRawStatus(t, ctx, ordersResource) != nil &&
			reviewRawStatus(t, ctx, paymentsResource) != nil
	}, 3*time.Second, 10*time.Millisecond)
	require.NoError(t, breaker.Report(&model.ResourceStat{
		Resource: ordersResource, RetCode: "-1", RetStatus: model.RetFail,
	}))
	require.Eventually(t, func() bool {
		status := reviewRawStatus(t, ctx, ordersResource)
		return status != nil && status.GetStatus() == model.HalfOpen
	}, 4*time.Second, 10*time.Millisecond)

	name := "review-other-caller"
	require.NoError(t, adapterselector.Setup(ctx, &adapterselector.Config{Name: name, Enable: true}))
	selector := trpcselector.Get(name)
	node, err := selector.Select("review-service",
		trpcselector.WithNamespace("cr-review"),
		trpcselector.WithSourceNamespace("callers"),
		trpcselector.WithSourceServiceName("payments"))
	require.NoError(t, err)
	require.NoError(t, selector.Report(node, time.Millisecond, nil))
	require.Equal(t, model.HalfOpen, reviewRawStatus(t, ctx, ordersResource).GetStatus())

	node, err = selector.Select("review-service",
		trpcselector.WithNamespace("cr-review"),
		trpcselector.WithSourceNamespace("callers"),
		trpcselector.WithSourceServiceName("orders"))
	require.NoError(t, err)
	require.NoError(t, selector.Report(node, time.Millisecond, nil))
	require.Eventually(t, func() bool {
		status := reviewRawStatus(t, ctx, ordersResource)
		return status != nil && status.GetStatus() == model.Close
	}, 3*time.Second, 10*time.Millisecond)
}

func TestReviewDisabledRoutingPreservesCaller(t *testing.T) {
	for _, mode := range []string{"config-disabled", "request-disabled"} {
		t.Run(mode, func(t *testing.T) {
			ctx, inst := reviewLiveContext(t, []*fault_tolerance.CircuitBreakerRule{
				reviewRule("orders", fault_tolerance.Level_INSTANCE, 30),
			}, 0)
			name := "review-disabled-routing-" + mode
			require.NoError(t, adapterselector.Setup(ctx, &adapterselector.Config{
				Name:   name,
				Enable: mode != "config-disabled",
			}))
			selector := trpcselector.Get(name)
			options := []trpcselector.Option{
				trpcselector.WithNamespace("cr-review"),
				trpcselector.WithSourceNamespace("callers"),
				trpcselector.WithSourceServiceName("orders"),
			}
			if mode == "request-disabled" {
				options = append(options, trpcselector.WithDisableServiceRouter())
			}
			node, err := selector.Select("review-service", options...)
			require.NoError(t, err)
			require.Nil(t, node.Metadata[adapterpolaris.SourceServiceMetadataKey])
			require.Equal(t, &model.ServiceKey{Namespace: "callers", Service: "orders"},
				node.Metadata[adapterpolaris.CallerServiceMetadataKey])

			require.NoError(t, selector.Report(node, time.Millisecond, nil))
			resource := reviewResource(t, inst, "orders")
			require.Eventually(t, func() bool {
				return reviewRawStatus(t, ctx, resource) != nil
			}, 3*time.Second, 10*time.Millisecond)
			require.NoError(t, selector.Report(node, time.Millisecond, fmt.Errorf("network failure")))
			require.Eventually(t, func() bool {
				result, err := api.NewCircuitBreakerByContext(ctx).Check(resource)
				return err == nil && result != nil && !result.Pass
			}, 3*time.Second, 10*time.Millisecond)
		})
	}
}

func TestReviewServiceRuleCannotDisableAllProtection(t *testing.T) {
	rule := reviewRule("orders", fault_tolerance.Level_SERVICE, 30)
	ctx, inst := reviewLiveContext(t, []*fault_tolerance.CircuitBreakerRule{rule}, 0)
	serviceResource, err := model.NewServiceResource(
		&model.ServiceKey{Namespace: inst.GetNamespace(), Service: inst.GetService()},
		&model.ServiceKey{Namespace: "callers", Service: "orders"})
	require.NoError(t, err)
	name := "review-service-rule"
	require.NoError(t, adapterselector.Setup(ctx, &adapterselector.Config{Name: name, Enable: true}))
	selector := trpcselector.Get(name)
	node, err := selector.Select("review-service",
		trpcselector.WithNamespace("cr-review"),
		trpcselector.WithSourceNamespace("callers"),
		trpcselector.WithSourceServiceName("orders"))
	require.NoError(t, err)
	require.NoError(t, selector.Report(node, time.Millisecond, nil))
	require.Eventually(t, func() bool {
		result, err := api.NewCircuitBreakerByContext(ctx).Check(serviceResource)
		return err == nil && result != nil && result.RuleName == rule.Name
	}, 3*time.Second, 10*time.Millisecond)
	require.NoError(t, selector.Report(node, time.Millisecond, fmt.Errorf("network failure")))
	require.Eventually(t, func() bool {
		result, err := api.NewCircuitBreakerByContext(ctx).Check(serviceResource)
		return err == nil && result != nil && !result.Pass
	}, 3*time.Second, 10*time.Millisecond)

	_, err = selector.Select("review-service",
		trpcselector.WithNamespace("cr-review"),
		trpcselector.WithSourceNamespace("callers"),
		trpcselector.WithSourceServiceName("orders"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "denied by circuit breaker")
}

func TestReviewMethodRuleUsesRequestPath(t *testing.T) {
	rule := reviewRule("orders", fault_tolerance.Level_METHOD, 30)
	rule.BlockConfigs = []*fault_tolerance.BlockConfig{{
		Name: "review-method-block",
		Api: &apispecmodel.API{
			Protocol: "grpc",
			Method:   "GET",
			Path:     &apispecmodel.MatchString{Value: wrapperspb.String("/review.Service/Call")},
		},
		ErrorConditions: []*fault_tolerance.ErrorCondition{{
			InputType: fault_tolerance.ErrorCondition_RET_CODE,
			Condition: &apispecmodel.MatchString{
				Type:  apispecmodel.MatchString_RANGE,
				Value: wrapperspb.String("500~599"),
			},
		}},
		TriggerConditions: []*fault_tolerance.TriggerCondition{{
			TriggerType: fault_tolerance.TriggerCondition_CONSECUTIVE_ERROR,
			ErrorCount:  1,
		}},
	}}
	ctx, inst := reviewLiveContext(t, []*fault_tolerance.CircuitBreakerRule{rule}, 0)
	methodResource, err := model.NewMethodResourceWithAPI(
		&model.ServiceKey{Namespace: inst.GetNamespace(), Service: inst.GetService()},
		&model.ServiceKey{Namespace: "callers", Service: "orders"},
		"grpc", "GET", "/review.Service/Call")
	require.NoError(t, err)
	name := "review-method-rule"
	require.NoError(t, adapterselector.Setup(ctx, &adapterselector.Config{Name: name, Enable: true}))
	selector := trpcselector.Get(name)

	msgCtx := trpc.Message(context.Background())
	msgCtx.WithClientRPCName("/review.Service/Call")
	msgCtx.WithCalleeMethod("/review.Service/Call")
	msgCtx.WithClientReqHead(&thttp.ClientReqHeader{Method: "GET"})
	requestCtx := context.WithValue(context.Background(), codec.ContextKeyMessage, msgCtx)
	node, err := selector.Select("review-service",
		trpcselector.WithContext(requestCtx),
		trpcselector.WithNamespace("cr-review"),
		trpcselector.WithSourceNamespace("callers"),
		trpcselector.WithSourceServiceName("orders"))
	require.NoError(t, err)
	require.Equal(t, methodResource, node.Metadata[adapterpolaris.MethodResourceMetadataKey])
	require.NoError(t, selector.Report(node, time.Millisecond, nil))
	require.Eventually(t, func() bool {
		result, err := api.NewCircuitBreakerByContext(ctx).Check(methodResource)
		return err == nil && result != nil && result.RuleName == rule.Name
	}, 3*time.Second, 10*time.Millisecond)
	require.NoError(t, selector.Report(node, time.Millisecond, fmt.Errorf("network failure")))
	require.Eventually(t, func() bool {
		result, err := api.NewCircuitBreakerByContext(ctx).Check(methodResource)
		return err == nil && result != nil && !result.Pass
	}, 3*time.Second, 10*time.Millisecond)
}
