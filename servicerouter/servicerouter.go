// Tencent is pleased to support the open source community by making tRPC available.
// Copyright (C) 2023 THL A29 Limited, a Tencent company. All rights reserved.
// If you have downloaded a copy of the tRPC source code from Tencent,
// please note that tRPC source code is licensed under the Apache 2.0 License,
// A copy of the Apache 2.0 License is included in this file.

// Package servicerouter is a service router.
package servicerouter

import (
	"errors"
	"fmt"
	"strings"

	"trpc.group/trpc-go/trpc-go/client"
	"trpc.group/trpc-go/trpc-go/codec"
	"trpc.group/trpc-go/trpc-go/log"
	"trpc.group/trpc-go/trpc-go/naming/registry"
	tsr "trpc.group/trpc-go/trpc-go/naming/servicerouter"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/config"
	"github.com/polarismesh/polaris-go/pkg/model"
	"github.com/polarismesh/polaris-go/pkg/model/pb"
	metric "github.com/polarismesh/polaris-go/pkg/model/pb/metric/v2"
	"github.com/polarismesh/polaris-go/pkg/plugin/common"
	"github.com/polarismesh/polaris-go/pkg/plugin/servicerouter"
	apimodel "github.com/polarismesh/specification/source/go/api/v1/model"
	apiservice "github.com/polarismesh/specification/source/go/api/v1/service_manage"
	"github.com/polarismesh/specification/source/go/api/v1/traffic_manage"
)

// CanaryKey is the trpc canary key.
var CanaryKey string = "trpc-canary"

var servicerouterGetFilterInstances = servicerouter.GetFilterInstances

// Setup is for setting up.
func Setup(sdkCtx api.SDKContext, cfg *Config, setDefault bool) error {
	s := &ServiceRouter{
		consumer: api.NewConsumerAPIByContext(sdkCtx),
		cfg:      cfg,
		sdkCtx:   sdkCtx,
	}

	// Initialize rule routing.
	ruleBased, err := sdkCtx.GetPlugins().GetPlugin(
		common.TypeServiceRouter, config.DefaultServiceRouterRuleBased)
	if err != nil {
		log.Errorf("get service router plugin err: %s\n", err.Error())
		return err
	}
	s.RuleBased = ruleBased.(servicerouter.ServiceRouter)

	// Initialize the nearest route.
	nearbyBased, err := sdkCtx.GetPlugins().GetPlugin(
		common.TypeServiceRouter, config.DefaultServiceRouterNearbyBased)
	if err != nil {
		log.Errorf("get service router plugin err: %s\n", err.Error())
		return err
	}
	s.NearbyBased = nearbyBased.(servicerouter.ServiceRouter)

	//Initialize packet routing.
	setDivison, err := sdkCtx.GetPlugins().GetPlugin(
		common.TypeServiceRouter, config.DefaultServiceRouterSetDivision)
	if err != nil {
		log.Errorf("get service router plugin err: %s\n", err.Error())
		return err
	}
	s.SetDivison = setDivison.(servicerouter.ServiceRouter)

	// Initialize to filter out unhealthy node routes.
	filterOnly, err := sdkCtx.GetPlugins().GetPlugin(
		common.TypeServiceRouter, config.DefaultServiceRouterFilterOnly)
	if err != nil {
		log.Errorf("get service router plugin err: %s\n", err.Error())
		return err
	}
	s.FilterOnly = filterOnly.(servicerouter.ServiceRouter)

	// Initialize routes filtered by meta.
	dstMeta, err := sdkCtx.GetPlugins().GetPlugin(
		common.TypeServiceRouter, config.DefaultServiceRouterDstMeta)
	if err != nil {
		log.Errorf("get service router plugin err: %s\n", err.Error())
		return err
	}
	s.DstMeta = dstMeta.(servicerouter.ServiceRouter)

	// Initialize the canary routing plugin.
	canary, err := sdkCtx.GetPlugins().GetPlugin(
		common.TypeServiceRouter, config.DefaultServiceRouterCanary)
	if err != nil {
		log.Errorf("get service router plugin err: %s\n", err.Error())
		return err
	}
	s.Canary = canary.(servicerouter.ServiceRouter)

	name := "polarismesh"
	if cfg != nil && cfg.Name != "" {
		name = cfg.Name
	}
	tsr.Register(name, s)
	if setDefault {
		tsr.SetDefaultServiceRouter(s)
	}
	return nil
}

// ServiceRouter is service routing.
type ServiceRouter struct {
	sdkCtx      api.SDKContext
	consumer    api.ConsumerAPI
	RuleBased   servicerouter.ServiceRouter
	NearbyBased servicerouter.ServiceRouter
	FilterOnly  servicerouter.ServiceRouter
	DstMeta     servicerouter.ServiceRouter
	SetDivison  servicerouter.ServiceRouter
	Canary      servicerouter.ServiceRouter
	cfg         *Config
}

func hasEnv(r *traffic_manage.Route, env string) bool {
	var hasEnv bool
	for _, source := range r.GetSources() {
		if source.GetMetadata() == nil {
			continue
		}
		value, ok := source.GetMetadata()["env"]
		if !ok {
			continue
		}
		if value.GetValue().GetValue() == env {
			hasEnv = true
			break
		}
	}

	return hasEnv
}

func getDestination(r *traffic_manage.Route) []string {
	var result []string
	for _, dest := range r.GetDestinations() {
		if dest.GetMetadata() == nil {
			continue
		}
		value, ok := dest.GetMetadata()["env"]
		if !ok {
			continue
		}
		if dest.GetService().GetValue() == "*" && value.GetValue().GetValue() != "" {
			result = append(result, value.GetValue().GetValue())
		}
	}
	return result
}

// getEnvPriority finds the environment priority information from the service route.
func getEnvPriority(routes []*traffic_manage.Route, env string) string {
	result := []string{}
	for _, r := range routes {
		if !hasEnv(r, env) {
			continue
		}
		dest := getDestination(r)
		result = append(result, dest...)
	}
	return strings.Join(result, ",")
}

// getOutboundsRoute obtains the outgoing rule route of the service.
var getOutboundsRoute = func(
	rules *model.ServiceRuleResponse,
) []*traffic_manage.Route {
	if rules != nil && rules.GetType() == model.EventRouting {
		value, ok := rules.GetValue().(*traffic_manage.Routing)
		if ok {
			return value.GetOutbounds()
		}
	}
	return []*traffic_manage.Route{}
}

func (s *ServiceRouter) setEnable(
	srcServiceInfo *model.ServiceInfo,
	dstServiceInfo *model.ServiceInfo,
	opts *tsr.Options,
	chain []servicerouter.ServiceRouter,
) []servicerouter.ServiceRouter {
	sourceSetName := opts.SourceSetName
	dstSetName := opts.DestinationSetName
	if len(sourceSetName) != 0 || len(dstSetName) != 0 {
		//set grouping enabled.
		if len(sourceSetName) != 0 {
			if srcServiceInfo.Metadata == nil {
				srcServiceInfo.Metadata = map[string]string{
					setEnableKey: setEnableValue,
					setNameKey:   sourceSetName,
				}
			}
			srcServiceInfo.Metadata[setEnableKey] = setEnableValue
			srcServiceInfo.Metadata[setNameKey] = sourceSetName
		}
		if len(dstSetName) != 0 {
			if dstServiceInfo.Metadata == nil {
				dstServiceInfo.Metadata = map[string]string{
					setEnableKey: setEnableValue,
					setNameKey:   dstSetName,
				}
			}
			dstServiceInfo.Metadata[setEnableKey] = setEnableValue
			dstServiceInfo.Metadata[setNameKey] = dstSetName
		}
		chain = append(chain, s.SetDivison)
	}
	return chain
}

func (s *ServiceRouter) filterWithEnv(
	serviceInstances model.ServiceInstances,
	sourceService, destService *model.ServiceInfo, opts *tsr.Options) ([]*registry.Node, error) {
	envList := []string{}
	if len(opts.EnvTransfer) > 0 {
		envList = strings.Split(opts.EnvTransfer, ",")
	}

	sourceService.Metadata = map[string]string{
		"env": opts.SourceEnvName,
	}
	canaryValue := getCanaryValue(opts)
	routeRules := buildRouteRules(opts.SourceNamespace,
		opts.SourceServiceName, opts.SourceEnvName, opts.Namespace, envList)
	routeInfo := &servicerouter.RouteInfo{
		SourceService:    sourceService,
		SourceRouteRule:  routeRules,
		DestService:      destService,
		FilterOnlyRouter: s.FilterOnly,
		Canary:           canaryValue,
	}

	// Consider the set grouping situation.
	chain := []servicerouter.ServiceRouter{s.RuleBased}
	chain = s.setEnable(sourceService, destService, opts, chain)
	chain = append(chain, s.NearbyBased)
	if s.cfg.EnableCanary {
		chain = append(chain, s.Canary)
	}
	instances, cluster, _, err := servicerouterGetFilterInstances(s.sdkCtx.GetValueContext(),
		chain, routeInfo, serviceInstances)
	if err != nil {
		return nil, fmt.Errorf("filter instance with env err: %s", err.Error())
	}

	return s.instanceToNode(instances, opts.EnvTransfer, cluster, serviceInstances), nil
}

func (s *ServiceRouter) filter(
	serviceInstances model.ServiceInstances,
	sourceService, destService *model.ServiceInfo, opts *tsr.Options) ([]*registry.Node, error) {

	sourceRouteRules, err := s.consumer.GetRouteRule(&api.GetServiceRuleRequest{
		GetServiceRuleRequest: model.GetServiceRuleRequest{
			Namespace: sourceService.Namespace,
			Service:   sourceService.Service,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get source service ns: %s, service: %s route rule err: %s",
			sourceService.Namespace, sourceService.Service, err.Error())
	}
	canaryValue := getCanaryValue(opts)

	// First consider if there is a rule.
	// If there is no outgoing rule, skip the service route directly, and only filter unhealthy nodes.
	// Otherwise, use the env and key of this node to filter out its own rules.
	chain := []servicerouter.ServiceRouter{}
	var newEnvStr string
	outbounds := getOutboundsRoute(sourceRouteRules)
	if len(outbounds) == 0 {
		chain = s.setEnable(sourceService, destService, opts, chain)
		chain = append(chain, s.NearbyBased)
		if s.cfg.EnableCanary {
			chain = append(chain, s.Canary)
		}
	} else {
		// Calling service metadata, used for rule routing.
		sourceService.Metadata = make(map[string]string)
		for key, value := range opts.SourceMetadata {
			if len(key) > 0 && len(value) > 0 {
				sourceService.Metadata[key] = value
			}
		}

		// Configure environment routing. If you set an environment key, use the environment key first.
		if len(opts.EnvKey) > 0 {
			sourceService.Metadata["key"] = opts.EnvKey
		} else {
			sourceService.Metadata["env"] = opts.SourceEnvName
		}
		newEnvStr = getEnvPriority(outbounds, opts.SourceEnvName)

		chain = append(chain, s.RuleBased)
		chain = s.setEnable(sourceService, destService, opts, chain)
		chain = append(chain, s.NearbyBased)
		if s.cfg.EnableCanary {
			chain = append(chain, s.Canary)
		}
	}

	routeInfo := &servicerouter.RouteInfo{
		SourceService:    sourceService,
		SourceRouteRule:  sourceRouteRules,
		DestService:      destService,
		FilterOnlyRouter: s.FilterOnly,
		Canary:           canaryValue,
	}
	instances, cluster, _, err := servicerouterGetFilterInstances(
		s.sdkCtx.GetValueContext(),
		chain,
		routeInfo,
		serviceInstances,
	)
	if err != nil {
		return nil, fmt.Errorf("filter instances without transfer env err: %s", err.Error())
	}
	if len(instances) == 0 {
		return nil, fmt.Errorf("env %s do not have instances, key: %s",
			opts.SourceEnvName, opts.EnvKey)
	}

	return s.instanceToNode(instances, newEnvStr, cluster, serviceInstances), nil
}

func (s *ServiceRouter) filterWithoutServiceRouter(
	serviceInstances model.ServiceInstances,
	sourceService, destService *model.ServiceInfo, opts *tsr.Options) ([]*registry.Node, error) {
	chain := []servicerouter.ServiceRouter{}
	if len(opts.DestinationEnvName) > 0 {
		chain = append(chain, s.DstMeta)
		destService.Metadata = map[string]string{
			"env": opts.DestinationEnvName,
		}
	}
	canaryValue := getCanaryValue(opts)
	chain = s.setEnable(sourceService, destService, opts, chain)
	chain = append(chain, s.NearbyBased)
	if s.cfg.EnableCanary {
		chain = append(chain, s.Canary)
	}
	routeInfo := &servicerouter.RouteInfo{
		SourceService:    sourceService,
		DestService:      destService,
		FilterOnlyRouter: s.FilterOnly,
		Canary:           canaryValue,
	}
	instances, cluster, _, err := servicerouterGetFilterInstances(
		s.sdkCtx.GetValueContext(), chain, routeInfo, serviceInstances)
	if err != nil {
		return nil, fmt.Errorf("filter instances err: %s", err.Error())
	}
	if len(instances) == 0 {
		return nil, errors.New("filter instances no instances available")
	}
	return s.instanceToNode(instances, "", cluster, serviceInstances), nil
}

// Filter filters instances based on routing rules.
func (s *ServiceRouter) Filter(serviceName string,
	nodes []*registry.Node, opt ...tsr.Option) ([]*registry.Node, error) {
	if len(nodes) == 0 {
		return nil, errors.New("servicerouter: no node available")
	}
	serviceInstances, ok := nodes[0].Metadata["service_instances"].(model.ServiceInstances)
	if !ok {
		return nil, errors.New("service instances invalid")
	}

	opts := &tsr.Options{}
	for _, o := range opt {
		o(opts)
	}
	log.Tracef("[NAMING-POLARISMESH] servicerouter options: %+v", opts)
	sourceService := &model.ServiceInfo{
		Service:   opts.SourceServiceName,
		Namespace: opts.SourceNamespace,
	}
	destService := &model.ServiceInfo{
		Service:   serviceName,
		Namespace: opts.Namespace,
	}

	// If the main calling service information does not exist, the service route will not be taken.
	if len(sourceService.Service) == 0 ||
		len(sourceService.Namespace) == 0 ||
		opts.DisableServiceRouter ||
		!s.cfg.Enable {
		return s.filterWithoutServiceRouter(serviceInstances, sourceService, destService, opts)
	}

	// If there is no transparent transmission of environmental information.
	if len(opts.EnvTransfer) == 0 {
		return s.filter(serviceInstances, sourceService, destService, opts)
	}
	return s.filterWithEnv(serviceInstances, sourceService, destService, opts)
}

// buildRouteRules builds query rules based on the transparent environment priority list.
func buildRouteRules(sourceNamespace, sourceServiceName,
	sourceEnv, destNamespace string, envList []string) model.ServiceRule {
	route := &traffic_manage.Route{
		Sources: []*traffic_manage.Source{
			{
				Namespace: &wrappers.StringValue{
					Value: sourceNamespace,
				},
				Service: &wrappers.StringValue{
					Value: sourceServiceName,
				},
				Metadata: map[string]*apimodel.MatchString{
					"env": {
						Type: apimodel.MatchString_EXACT,
						Value: &wrappers.StringValue{
							Value: sourceEnv,
						},
					},
				},
			},
		},
	}
	dests := []*traffic_manage.Destination{}
	for i, env := range envList {
		dest := &traffic_manage.Destination{
			Namespace: &wrappers.StringValue{
				Value: destNamespace,
			},
			Service: &wrappers.StringValue{
				Value: "*",
			},
			Priority: &wrappers.UInt32Value{
				Value: uint32(i),
			},
			Weight: &wrappers.UInt32Value{
				Value: 100,
			},
			Metadata: map[string]*apimodel.MatchString{
				"env": {
					Type: apimodel.MatchString_EXACT,
					Value: &wrappers.StringValue{
						Value: env,
					},
				},
			},
		}
		dests = append(dests, dest)
	}

	route.Destinations = dests
	value := &traffic_manage.Routing{
		Namespace: &wrappers.StringValue{
			Value: sourceNamespace,
		},
		Service: &wrappers.StringValue{
			Value: sourceServiceName,
		},
		Outbounds: []*traffic_manage.Route{route},
	}

	rule := &apiservice.DiscoverResponse{
		Code: &wrappers.UInt32Value{Value: uint32(metric.ExecuteSuccess)},
		Info: &wrappers.StringValue{Value: "create from local"},
		Type: apiservice.DiscoverResponse_ROUTING,
		Service: &apiservice.Service{
			Name:      &wrappers.StringValue{Value: sourceServiceName},
			Namespace: &wrappers.StringValue{Value: sourceNamespace},
		},
		Instances: nil,
		Routing:   value,
	}
	return pb.NewRoutingRuleInProto(rule)
}

func (s *ServiceRouter) instanceToNode(instances []model.Instance,
	env string, cluster *model.Cluster, resp model.ServiceInstances) []*registry.Node {
	if len(instances) == 0 {
		return nil
	}
	list := make([]*registry.Node, 0, len(instances))
	if s.cfg.NeedReturnAllNodes {
		for _, ins := range instances {
			list = append(list, &registry.Node{
				ServiceName: ins.GetService(),
				Address:     fmt.Sprintf("%s:%d", ins.GetHost(), ins.GetPort()),
				Protocol:    ins.GetProtocol(),
				Weight:      ins.GetWeight(),
			})
		}
	} else {
		list = append(list, &registry.Node{})
	}
	list[0].EnvKey = env
	list[0].Metadata = map[string]interface{}{
		"serviceInstances": resp,
		"cluster":          cluster,
	}
	return list
}

func getCanaryValue(opts *tsr.Options) string {
	if opts.Ctx == nil {
		return ""
	}
	ctx := opts.Ctx
	msg := codec.Message(ctx)
	metaData := msg.ClientMetaData()
	if metaData == nil {
		return ""
	}
	return string(metaData[CanaryKey])
}

// WithCanary sets canary metadata.
func WithCanary(val string) client.Option {
	return func(o *client.Options) {
		client.WithMetaData(CanaryKey, []byte(val))(o)
	}
}
