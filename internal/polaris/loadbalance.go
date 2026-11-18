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

	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/model"
	"github.com/polarismesh/polaris-go/pkg/plugin/loadbalancer"
)

// ChooseAvailableInstance includes half-open instances in normal load-balancing
// and acquires SDK permission before returning the selected instance.
func ChooseAvailableInstance(
	sdkCtx api.SDKContext,
	lb loadbalancer.LoadBalancer,
	cluster *model.Cluster,
	serviceInstances model.ServiceInstances,
	criteria loadbalancer.Criteria,
	caller *model.ServiceKey,
) (model.Instance, error) {
	inst, err := ChooseInstanceCandidate(sdkCtx, lb, cluster, serviceInstances, criteria)
	if err != nil {
		return nil, err
	}
	pass, err := InstancePermissionAvailable(sdkCtx, inst, cluster.HasLimitedInstances, caller)
	if err != nil {
		return nil, err
	}
	if pass {
		return inst, nil
	}
	return ChoosePermittedInstance(sdkCtx, lb, cluster, serviceInstances, criteria, caller, inst)
}

// ChoosePermittedInstance chooses an instance with caller-aware permission.
// Instances already rejected by the caller can be excluded to avoid checking
// the same candidate twice.
func ChoosePermittedInstance(
	sdkCtx api.SDKContext,
	lb loadbalancer.LoadBalancer,
	cluster *model.Cluster,
	serviceInstances model.ServiceInstances,
	criteria loadbalancer.Criteria,
	caller *model.ServiceKey,
	excluded ...model.Instance,
) (model.Instance, error) {
	return choosePermittedInstance(sdkCtx, lb, cluster, serviceInstances, criteria, caller, excluded...)
}

// ChooseInstanceCandidate chooses an instance without acquiring circuit-breaker
// permission. It includes endpoint-shared Open instances in its fallback so a
// caller whose own state is Close can still be evaluated by the SDK.
func ChooseInstanceCandidate(
	sdkCtx api.SDKContext,
	lb loadbalancer.LoadBalancer,
	cluster *model.Cluster,
	serviceInstances model.ServiceInstances,
	criteria loadbalancer.Criteria,
) (model.Instance, error) {
	selectCluster := model.NewCluster(cluster.GetClusters(), cluster)
	selectCluster.HasLimitedInstances = cluster.HasLimitedInstances
	selectCluster.SetReuse(false)
	selectCluster.IncludeHalfOpen = true
	criteria.Cluster = selectCluster

	inst, err := lb.ChooseInstance(&criteria, serviceInstances)
	if err != nil {
		if !isInstanceNotFound(err) {
			return nil, err
		}
		return chooseCandidateFromList(lb, cluster, serviceInstances, criteria,
			fallbackCandidates(cluster))
	}
	if inst == nil {
		return nil, model.NewSDKError(model.ErrCodeAPIInstanceNotFound, nil,
			"load balancer returned nil instance")
	}
	return inst, nil
}

func isInstanceNotFound(err error) bool {
	var sdkErr model.SDKError
	return errors.As(err, &sdkErr) && sdkErr.ErrorCode() == model.ErrCodeAPIInstanceNotFound
}

func choosePermittedInstance(
	sdkCtx api.SDKContext,
	lb loadbalancer.LoadBalancer,
	cluster *model.Cluster,
	serviceInstances model.ServiceInstances,
	criteria loadbalancer.Criteria,
	caller *model.ServiceKey,
	excluded ...model.Instance,
) (model.Instance, error) {
	candidates := fallbackCandidates(cluster)
	for _, inst := range excluded {
		candidates = removeInstance(candidates, inst)
	}
	for len(candidates) > 0 {
		inst, err := chooseCandidateFromList(lb, cluster, serviceInstances, criteria, candidates)
		if err != nil {
			return nil, err
		}
		pass, err := InstancePermissionAvailable(sdkCtx, inst, cluster.HasLimitedInstances, caller)
		if err != nil {
			return nil, err
		}
		if pass {
			return inst, nil
		}
		before := len(candidates)
		candidates = removeInstance(candidates, inst)
		if len(candidates) == before {
			return nil, fmt.Errorf("load balancer selected instance outside candidate set: %s",
				inst.GetInstanceKey().String())
		}
	}
	return nil, model.NewSDKError(model.ErrCodeAPIInstanceNotFound, nil,
		"no instance has circuit breaker permission")
}

func chooseCandidateFromList(
	lb loadbalancer.LoadBalancer,
	cluster *model.Cluster,
	serviceInstances model.ServiceInstances,
	criteria loadbalancer.Criteria,
	candidates []model.Instance,
) (model.Instance, error) {
	filteredInstances := model.NewDefaultServiceInstances(
		model.ServiceInfo{
			Service:   serviceInstances.GetService(),
			Namespace: serviceInstances.GetNamespace(),
			Metadata:  serviceInstances.GetMetadata(),
		},
		candidates,
	)
	candidateCluster := model.NewCluster(filteredInstances.GetServiceClusters(), nil)
	candidateCluster.SetReuse(false)
	candidateCluster.IncludeHalfOpen = true
	// Include endpoint-shared Open instances so the caller-aware SDK check
	// can distinguish this caller's state from another caller's state.
	candidateCluster.HasLimitedInstances = true
	criteria.Cluster = candidateCluster

	inst, err := lb.ChooseInstance(&criteria, filteredInstances)
	if err != nil {
		return nil, err
	}
	if inst == nil {
		return nil, model.NewSDKError(model.ErrCodeAPIInstanceNotFound, nil,
			"load balancer returned nil instance")
	}
	return inst, nil
}

func fallbackCandidates(cluster *model.Cluster) []model.Instance {
	candidates, _ := cluster.GetAllInstances()
	return selectableCandidates(candidates, cluster.HasLimitedInstances)
}

func selectableCandidates(instances []model.Instance, includeUnhealthy bool) []model.Instance {
	candidates := make([]model.Instance, 0, len(instances))
	for _, inst := range instances {
		if inst == nil || inst.IsIsolated() || inst.GetWeight() == 0 {
			continue
		}
		if !includeUnhealthy && !inst.IsHealthy() {
			continue
		}
		candidates = append(candidates, inst)
	}
	return candidates
}

func removeInstance(instances []model.Instance, removed model.Instance) []model.Instance {
	for i, inst := range instances {
		if sameInstance(inst, removed) {
			return append(instances[:i:i], instances[i+1:]...)
		}
	}
	return instances
}

func sameInstance(left, right model.Instance) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	if left == right {
		return true
	}
	leftID, rightID := left.GetId(), right.GetId()
	if leftID != "" && rightID != "" {
		return leftID == rightID
	}
	return left.GetInstanceKey() == right.GetInstanceKey()
}
