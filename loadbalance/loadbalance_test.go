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

package loadbalance

import (
	"testing"

	"trpc.group/trpc-go/trpc-go/naming/registry"

	"trpc.group/trpc-go/trpc-naming-polarismesh/mock/mock_api"
	"trpc.group/trpc-go/trpc-naming-polarismesh/mock/mock_loadbalancer"
	"trpc.group/trpc-go/trpc-naming-polarismesh/mock/mock_model"
	"trpc.group/trpc-go/trpc-naming-polarismesh/mock/mock_plugin"

	"github.com/golang/mock/gomock"
	"github.com/polarismesh/polaris-go/pkg/config"
	"github.com/polarismesh/polaris-go/pkg/model"
	"github.com/polarismesh/polaris-go/pkg/plugin/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSetup(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	inst := mock_model.NewMockInstance(ctrl)

	plugin := mock_loadbalancer.NewMockLoadBalancer(ctrl)
	plugin.EXPECT().ChooseInstance(gomock.Any(), gomock.Any()).Return(inst, nil).AnyTimes()

	pluginer := mock_plugin.NewMockManager(ctrl)
	pluginer.EXPECT().GetPlugin(common.TypeLoadBalancer, config.DefaultLoadBalancerWR).Return(plugin, nil).AnyTimes()
	m := mock_api.NewMockSDKContext(ctrl)
	m.EXPECT().GetPlugins().Return(pluginer).AnyTimes()

	assert.Nil(t, Setup(m, "polaris_wr", true))
}

func TestSelect(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	inst := mock_model.NewMockInstance(ctrl)
	inst.EXPECT().GetMetadata().Return(map[string]string{}).AnyTimes()
	inst.EXPECT().GetWeight().Return(100).AnyTimes()
	inst.EXPECT().GetHost().Return("host").AnyTimes()
	inst.EXPECT().GetPort().Return(uint32(1003)).AnyTimes()

	plugin := mock_loadbalancer.NewMockLoadBalancer(ctrl)
	plugin.EXPECT().ChooseInstance(gomock.Any(), gomock.Any()).Return(inst, nil).AnyTimes()

	m := mock_api.NewMockSDKContext(ctrl)
	m.EXPECT().GetValueContext().Return(mock_model.NewMockValueContext(ctrl)).AnyTimes()

	clustersMock := mock_model.NewMockServiceClusters(ctrl)
	clustersMock.EXPECT().GetServiceInstances().Return(mock_model.NewMockServiceInstances(ctrl)).AnyTimes()

	lb := &WRLoadBalancer{
		sdkCtx: m,
		lb:     plugin,
	}

	list := []*registry.Node{
		{
			Metadata: map[string]interface{}{
				"cluster":          model.NewCluster(clustersMock, nil),
				"serviceInstances": mock_model.NewMockServiceInstances(ctrl),
			},
		},
	}

	node, err := lb.Select("service", list)
	assert.Nil(t, err)
	assert.Equal(t, node.Weight, 100)

	_, err = lb.Select("service", nil)
	assert.NotNil(t, err)
}

func TestAsPluginCfgs(t *testing.T) {
	newYamlCfgs := func(cfg string) map[string]yaml.Node {
		yamlCfgs := make(map[string]yaml.Node)
		require.Nil(t, yaml.Unmarshal([]byte(cfg), &yamlCfgs))
		return yamlCfgs
	}
	t.Run("unimplemented lb", func(t *testing.T) {
		_, err := AsPluginCfgs(newYamlCfgs(`unimplemented_lb: {}`))
		require.NotNil(t, err)
	})
	t.Run("lb does not support to config", func(t *testing.T) {
		_, err := AsPluginCfgs(newYamlCfgs(`polaris_maglev: {}`))
		require.NotNil(t, err)
	})
	t.Run("ring hash", func(t *testing.T) {
		_, err := AsPluginCfgs(newYamlCfgs(`
polaris_ring_hash:
  vnodeCount: 1024`))
		require.Nil(t, err)
	})
	t.Run("mixed lbs", func(t *testing.T) {
		_, err := AsPluginCfgs(newYamlCfgs(`
polaris_ring_hash:
  vnodeCount: 1024
unimplemented_lb: {}`))
		require.NotNil(t, err)
	})
}
