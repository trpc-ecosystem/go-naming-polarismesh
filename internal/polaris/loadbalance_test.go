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
	"errors"
	"fmt"
	"testing"

	"github.com/polarismesh/polaris-go/pkg/model"
	"github.com/polarismesh/polaris-go/pkg/model/local"
	"github.com/polarismesh/polaris-go/pkg/model/pb"
	apiservice "github.com/polarismesh/specification/source/go/api/v1/service_manage"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestIsInstanceNotFoundHandlesWrappedSDKError(t *testing.T) {
	sdkErr := model.NewSDKError(model.ErrCodeAPIInstanceNotFound, nil, "instance not found")
	require.True(t, isInstanceNotFound(fmt.Errorf("wrapped: %w", sdkErr)))
	require.False(t, isInstanceNotFound(fmt.Errorf("wrapped: %w", errors.New("other error"))))
}

func TestRemoveInstanceMatchesEquivalentInstance(t *testing.T) {
	service := &model.ServiceKey{Namespace: "default", Service: "service"}
	newInstance := func(id string, port uint32) model.Instance {
		return pb.NewInstanceInProto(&apiservice.Instance{
			Id:       wrapperspb.String(id),
			Host:     wrapperspb.String("127.0.0.1"),
			Port:     wrapperspb.UInt32(port),
			Protocol: wrapperspb.String("trpc"),
			Weight:   wrapperspb.UInt32(100),
			Healthy:  wrapperspb.Bool(true),
		}, service, local.NewInstanceLocalValue())
	}

	selected := newInstance("same-instance", 8000)
	equivalent := newInstance("same-instance", 8000)
	other := newInstance("other-instance", 8001)

	remaining := removeInstance([]model.Instance{selected, other}, equivalent)
	require.Len(t, remaining, 1)
	require.Same(t, other, remaining[0])
}

func TestRemoveInstanceMatchesEndpointWithoutID(t *testing.T) {
	service := &model.ServiceKey{Namespace: "default", Service: "service"}
	newInstance := func(protocol string) model.Instance {
		return pb.NewInstanceInProto(&apiservice.Instance{
			Host:     wrapperspb.String("127.0.0.1"),
			Port:     wrapperspb.UInt32(8000),
			Protocol: wrapperspb.String(protocol),
			Weight:   wrapperspb.UInt32(100),
			Healthy:  wrapperspb.Bool(true),
		}, service, local.NewInstanceLocalValue())
	}
	selected := newInstance("trpc")
	equivalent := newInstance("http")

	remaining := removeInstance([]model.Instance{selected}, equivalent)
	require.Empty(t, remaining)
}

func TestRemoveInstanceKeepsCandidatesWhenInstanceIsAbsent(t *testing.T) {
	service := &model.ServiceKey{Namespace: "default", Service: "service"}
	instance := pb.NewInstanceInProto(&apiservice.Instance{
		Id:       wrapperspb.String("instance"),
		Host:     wrapperspb.String("127.0.0.1"),
		Port:     wrapperspb.UInt32(8000),
		Protocol: wrapperspb.String("trpc"),
		Weight:   wrapperspb.UInt32(100),
		Healthy:  wrapperspb.Bool(true),
	}, service, local.NewInstanceLocalValue())
	absent := pb.NewInstanceInProto(&apiservice.Instance{
		Id:       wrapperspb.String("absent"),
		Host:     wrapperspb.String("127.0.0.1"),
		Port:     wrapperspb.UInt32(8001),
		Protocol: wrapperspb.String("trpc"),
		Weight:   wrapperspb.UInt32(100),
		Healthy:  wrapperspb.Bool(true),
	}, service, local.NewInstanceLocalValue())

	remaining := removeInstance([]model.Instance{instance}, absent)
	require.Len(t, remaining, 1)
	require.Same(t, instance, remaining[0])
}
