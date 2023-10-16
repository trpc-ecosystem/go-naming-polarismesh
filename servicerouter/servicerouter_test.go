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

// Package servicerouter is a service router.
package servicerouter

import (
	"testing"

	"trpc.group/trpc-go/trpc-go/naming/registry"
	tsr "trpc.group/trpc-go/trpc-go/naming/servicerouter"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/polarismesh/polaris-go/pkg/model"
	"github.com/polarismesh/polaris-go/pkg/plugin/servicerouter"
	apimodel "github.com/polarismesh/specification/source/go/api/v1/model"
	"github.com/polarismesh/specification/source/go/api/v1/traffic_manage"
	"github.com/stretchr/testify/assert"

	"trpc.group/trpc-go/trpc-naming-polarismesh/mock/mock_api"
	"trpc.group/trpc-go/trpc-naming-polarismesh/mock/mock_model"
	"trpc.group/trpc-go/trpc-naming-polarismesh/mock/mock_plugin"
	"trpc.group/trpc-go/trpc-naming-polarismesh/mock/mock_servicerouter"
)

func TestSetup(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	plugin := mock_servicerouter.NewMockServiceRouter(ctrl)
	pluginer := mock_plugin.NewMockManager(ctrl)
	pluginer.EXPECT().GetPlugin(gomock.Any(), gomock.Any()).Return(plugin, nil).AnyTimes()
	m := mock_api.NewMockSDKContext(ctrl)
	m.EXPECT().GetPlugins().Return(pluginer).AnyTimes()
	assert.Nil(t, Setup(m, &Config{Name: "polarismesh"}, true))
	assert.NotNil(t, tsr.Get("polarismesh"))
}

func TestInstanceToNode(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	serviceInstances := mock_model.NewMockServiceInstances(ctrl)
	inst := mock_model.NewMockInstance(ctrl)
	clustersMock := mock_model.NewMockServiceClusters(ctrl)
	clusters := model.NewCluster(clustersMock, nil)
	sr := ServiceRouter{cfg: &Config{}}
	nodes := sr.instanceToNode([]model.Instance{inst}, "env", clusters, serviceInstances)
	assert.Len(t, nodes, 1)
	node := nodes[0]
	assert.Equal(t, node.EnvKey, "env")
	assert.Equal(t, node.Metadata["serviceInstances"], serviceInstances)
	assert.Equal(t, node.Metadata["cluster"], clusters)
}

func TestInstanceToNode_ReturnAll(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	serviceInstances := mock_model.NewMockServiceInstances(ctrl)
	var instances []model.Instance
	for i := 0; i < 10; i++ {
		inst := mock_model.NewMockInstance(ctrl)
		inst.EXPECT().GetService().Return("service")
		inst.EXPECT().GetHost().Return("host")
		inst.EXPECT().GetPort().Return(uint32(i))
		inst.EXPECT().GetProtocol().Return("protocol")
		inst.EXPECT().GetWeight().Return(i)
		instances = append(instances, inst)
	}
	clustersMock := mock_model.NewMockServiceClusters(ctrl)
	clusters := model.NewCluster(clustersMock, nil)
	sr := ServiceRouter{cfg: &Config{NeedReturnAllNodes: true}}
	nodes := sr.instanceToNode(instances, "env", clusters, serviceInstances)
	assert.Len(t, nodes, 10)
	node := nodes[0]
	assert.Equal(t, node.EnvKey, "env")
	assert.Equal(t, node.Metadata["serviceInstances"], serviceInstances)
	assert.Equal(t, node.Metadata["cluster"], clusters)
}

func TestBuildRouteRules(t *testing.T) {
	serviceRule := buildRouteRules("sourceNamespace", "sourceServiceName",
		"sourceEnv", "destNamespace", []string{"env1", "env2"})
	assert.Equal(t, serviceRule.GetNamespace(), "sourceNamespace")
	assert.Equal(t, serviceRule.GetService(), "sourceServiceName")
	assert.Nil(t, serviceRule.GetValidateError())
}

func TestFilterWithoutBoundRules(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	sdkCtx := mock_api.NewMockSDKContext(ctrl)
	sdkCtx.EXPECT().GetValueContext().Return(model.NewValueContext()).AnyTimes()
	consumer := mock_api.NewMockConsumerAPI(ctrl)
	consumer.EXPECT().GetRouteRule(gomock.Any()).Return(nil, nil).AnyTimes()

	oldServicerouterGetFilterInstances := servicerouterGetFilterInstances
	defer func() {
		servicerouterGetFilterInstances = oldServicerouterGetFilterInstances
	}()
	inst := mock_model.NewMockInstance(ctrl)
	servicerouterGetFilterInstances = func(model.ValueContext, []servicerouter.ServiceRouter,
		*servicerouter.RouteInfo, model.ServiceInstances) ([]model.Instance,
		*model.Cluster, *model.ServiceInfo, error) {
		return []model.Instance{inst}, nil, nil, nil
	}

	serviceRouter := &ServiceRouter{
		sdkCtx:   sdkCtx,
		consumer: consumer,
		cfg:      &Config{},
	}
	serviceInstances := mock_model.NewMockServiceInstances(ctrl)
	n := &registry.Node{
		Metadata: map[string]interface{}{
			"service_instances": serviceInstances,
		},
	}

	nodes, err := serviceRouter.Filter("service name", []*registry.Node{n})
	assert.Nil(t, err)
	assert.Len(t, nodes, 1)

	nodes, err = serviceRouter.Filter("service name", []*registry.Node{n}, tsr.WithNamespace("hahaha"))
	assert.Nil(t, err)
	assert.Len(t, nodes, 1)
}

func TestFilterBoundRules(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	sdkCtx := mock_api.NewMockSDKContext(ctrl)
	sdkCtx.EXPECT().GetValueContext().Return(model.NewValueContext()).AnyTimes()
	consumer := mock_api.NewMockConsumerAPI(ctrl)
	consumer.EXPECT().GetRouteRule(gomock.Any()).Return(nil, nil).AnyTimes()

	oldGetOutboundsRoute := getOutboundsRoute
	defer func() {
		getOutboundsRoute = oldGetOutboundsRoute
	}()
	getOutboundsRoute = func(_ *model.ServiceRuleResponse) []*traffic_manage.Route {
		return []*traffic_manage.Route{&traffic_manage.Route{}}
	}

	odlServicerouterGetFilterInstances := servicerouterGetFilterInstances
	defer func() {
		servicerouterGetFilterInstances = odlServicerouterGetFilterInstances
	}()
	inst := mock_model.NewMockInstance(ctrl)
	servicerouterGetFilterInstances = func(model.ValueContext, []servicerouter.ServiceRouter,
		*servicerouter.RouteInfo, model.ServiceInstances) ([]model.Instance,
		*model.Cluster, *model.ServiceInfo, error) {
		return []model.Instance{inst}, nil, nil, nil
	}

	serviceRouter := &ServiceRouter{
		sdkCtx:   sdkCtx,
		consumer: consumer,
		cfg: &Config{
			Enable: true,
		},
	}

	serviceInstances := mock_model.NewMockServiceInstances(ctrl)

	n := &registry.Node{
		Metadata: map[string]interface{}{
			"service_instances": serviceInstances,
		},
	}

	nodes, err := serviceRouter.Filter("service name", []*registry.Node{n},
		tsr.WithNamespace("hahaha"),
		tsr.WithSourceServiceName("source service"),
		tsr.WithSourceNamespace("source namespace"),
	)
	assert.Nil(t, err)
	assert.Len(t, nodes, 1)

	nodes, err = serviceRouter.Filter("service name", []*registry.Node{n},
		tsr.WithNamespace("hahaha"),
		tsr.WithSourceServiceName("source service"),
		tsr.WithSourceNamespace("source namespace"),
		tsr.WithEnvTransfer("vdf,fvdf"),
	)
	assert.Nil(t, err)
	assert.Len(t, nodes, 1)
}

func TestSetEnable(t *testing.T) {
	serviceRouter := &ServiceRouter{}
	srcServiceInfo := &model.ServiceInfo{}
	dstServiceInfo := &model.ServiceInfo{}
	opts := &tsr.Options{
		SourceSetName:      "SourceSetName",
		DestinationSetName: "DestinationSetName",
	}
	chain := []servicerouter.ServiceRouter{}

	chain = serviceRouter.setEnable(srcServiceInfo, dstServiceInfo, opts, chain)
	assert.Len(t, chain, 1)
}

func TestGetEnvPriority(t *testing.T) {
	routes := []*traffic_manage.Route{&traffic_manage.Route{
		Sources: []*traffic_manage.Source{
			{
				Metadata: map[string]*apimodel.MatchString{
					"env": {
						Value: &wrappers.StringValue{
							Value: "hhha",
						},
					},
				},
			},
		},
		Destinations: []*traffic_manage.Destination{
			{
				Service: &wrappers.StringValue{
					Value: "*",
				},
				Metadata: map[string]*apimodel.MatchString{
					"env": {
						Value: &wrappers.StringValue{
							Value: "hhha1",
						},
					},
				},
			},
			{
				Service: &wrappers.StringValue{
					Value: "*",
				},
				Metadata: map[string]*apimodel.MatchString{
					"env": {
						Value: &wrappers.StringValue{
							Value: "hhha2",
						},
					},
				},
			},
		},
	}}

	assert.Equal(t, getEnvPriority(routes, "hhha"), "hhha1,hhha2")
}
