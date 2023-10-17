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

package selector

import (
	"context"
	"testing"
	"time"

	"trpc.group/trpc-go/trpc-go/codec"
	"trpc.group/trpc-go/trpc-go/naming/registry"
	"trpc.group/trpc-go/trpc-go/naming/selector"
	"trpc.group/trpc-go/trpc-naming-polarismesh/mock/mock_api"
	"trpc.group/trpc-go/trpc-naming-polarismesh/mock/mock_model"

	"github.com/golang/mock/gomock"
	"github.com/polarismesh/polaris-go/pkg/model"
	"github.com/stretchr/testify/assert"
)

func TestSetup(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := mock_api.NewMockSDKContext(ctrl)

	err := Setup(m, &Config{})
	assert.Nil(t, err)
}

func TestNew(t *testing.T) {
	_, err := New(&Config{
		RefreshInterval: 1000,
		ServerAddrs:     []string{"not_exist"},
	})
	assert.Nil(t, err)
}

func TestExtractSourceServiceRequestInfo(t *testing.T) {
	testOptions := []*selector.Options{
		// metadata and hosting service.
		{
			SourceMetadata: map[string]string{
				"a": "b",
			},
			SourceEnvName:     "source env",
			SourceServiceName: "service name",
		},
		// Hosting service only.
		{
			SourceServiceName: "service name",
		},
		// Only the calling namespace.
		{
			SourceNamespace: "test",
		},
		// Only metadata.
		{
			SourceMetadata: map[string]string{
				"a": "b",
			},
			SourceEnvName: "source env",
		},
		// metadata and calling service neither.
		{},
	}

	info := extractSourceServiceRequestInfo(testOptions[0], false)
	assert.Equal(t, info.Service, "service name")
	assert.Equal(t, info.Metadata["a"], "b")
	assert.Equal(t, info.Metadata["env"], "source env")

	info = extractSourceServiceRequestInfo(testOptions[1], false)
	assert.Equal(t, info.Service, "service name")
	assert.Equal(t, info.Namespace, "")
	assert.Equal(t, len(info.Metadata), 0)

	info = extractSourceServiceRequestInfo(testOptions[2], false)
	assert.Equal(t, info.Service, "")
	assert.Equal(t, info.Namespace, "test")
	assert.Equal(t, len(info.Metadata), 0)

	info = extractSourceServiceRequestInfo(testOptions[3], false)
	assert.Equal(t, info.Service, "")
	assert.Equal(t, info.Metadata["a"], "b")
	assert.Equal(t, info.Metadata["env"], "source env")

	info = extractSourceServiceRequestInfo(testOptions[4], false)
	assert.Nil(t, info)
}

func TestSelect(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	inst := mock_model.NewMockInstance(ctrl)
	inst.EXPECT().GetMetadata().Return(map[string]string{}).AnyTimes()
	inst.EXPECT().GetWeight().Return(100).AnyTimes()
	inst.EXPECT().GetHost().Return("host").AnyTimes()
	inst.EXPECT().GetPort().Return(uint32(1003)).AnyTimes()

	consumer := mock_api.NewMockConsumerAPI(ctrl)
	consumer.EXPECT().GetOneInstance(gomock.Any()).Return(&model.OneInstanceResponse{
		InstancesResponse: model.InstancesResponse{
			Instances: []model.Instance{inst},
		},
	}, nil).AnyTimes()
	s := &Selector{
		consumer: consumer,
		cfg:      &Config{},
	}

	node, err := s.Select("service name")
	assert.Nil(t, err)
	assert.Equal(t, node.Weight, 100)
}

func TestReport(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	inst := mock_model.NewMockInstance(ctrl)

	consumer := mock_api.NewMockConsumerAPI(ctrl)
	consumer.EXPECT().UpdateServiceCallResult(gomock.Any()).Return(nil).AnyTimes()
	s := &Selector{
		consumer: consumer,
		cfg:      &Config{},
	}

	node := &registry.Node{
		Metadata: map[string]interface{}{
			"instance": inst,
		},
	}

	assert.Nil(t, s.Report(node, time.Second, nil))
}

func TestSetTransSelectorMeta(t *testing.T) {
	ctx := context.Background()
	msg := trpc.Message(ctx)
	msg.WithServerMetaData(codec.MetaData{
		"selector-meta-k": []byte("v"),
		"k2":              []byte("v2"),
	})
	ctx = context.WithValue(ctx, codec.ContextKeyMessage, msg)
	opts := &selector.Options{Ctx: ctx}
	selectorMeta := make(map[string]string, 0)

	setTransSelectorMeta(opts, selectorMeta)
	assert.EqualValues(t, "v", selectorMeta["k"])
	assert.EqualValues(t, "", selectorMeta["k2"])
	opts.Ctx = nil
	selectorMeta = make(map[string]string, 0)
	setTransSelectorMeta(opts, selectorMeta)
	assert.EqualValues(t, 0, len(selectorMeta))
}
