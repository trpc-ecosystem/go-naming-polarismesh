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
	"time"

	"github.com/polarismesh/polaris-go/pkg/config"
)

// Config selector configuration structure
type Config struct {
	// Name is the current name of plugin.
	Name string
	// RefreshInterval is the time to refresh the list, in ms.
	RefreshInterval int
	// ServerAddrs is service address.
	ServerAddrs []string
	// Protocol is protocol type.
	Protocol string
	// Enable is the state of the ServiceRouter.
	Enable bool
	// Timeout obtains information polaris mesh background timeout, in ms.
	Timeout int
	// ConnectTimeout is the timeout of connecting to polaris mesh background, in ms.
	ConnectTimeout int
	// Enable  is the state of the canary.
	EnableCanary bool
	// UseBuildin indicates whether to use the sdk default buried address.
	UseBuildin bool
	// ReportTimeout If ReportTimeout is set, when the downstream times out and the time is less than the set value,
	// the error will be ignored and not reported.
	ReportTimeout *time.Duration
	// EnableTransMeta When the setting is enabled,
	// remove the prefix from the transparent transmission field prefixed with 'selector-meta-'
	// and fill in the MetaData of SourceService.
	EnableTransMeta bool
	// Set the local cache storage address.
	LocalCachePersistDir string
	// Set the local IP address.
	BindIP string
}

const (
	setEnableKey       string = "internal-enable-set"
	setNameKey         string = "internal-set-name"
	setEnableValue     string = "Y"
	containerKey       string = "container_name"
	selectorMetaPrefix string = "selector-meta-"
)

// Deprecated: you can always use the original string defined in polaris-go.
const (
	// LoadBalanceWR is the random weight lb. It's default.
	LoadBalanceWR = "polaris_wr"
	// LoadBalanceHash is the common hash lb.
	LoadBalanceHash = "polaris_hash"
	// LoadBalancerRingHash is the lb based on consistent hash ring.
	LoadBalancerRingHash = "polaris_ring_hash"
	// LoadBalanceMaglev is the lb based on maglev hash.
	LoadBalanceMaglev = "polaris_maglev"
	// LoadBalanceL5Cst is the lb which is compatible with l5 consistent hash.
	LoadBalanceL5Cst = "polaris_l5cst"
)

// Deprecated: you can always use the original string defined in polaris-go.
var loadBalanceMap map[string]string = map[string]string{
	LoadBalanceWR:        config.DefaultLoadBalancerWR,
	LoadBalanceHash:      config.DefaultLoadBalancerHash,
	LoadBalancerRingHash: config.DefaultLoadBalancerRingHash,
	LoadBalanceMaglev:    config.DefaultLoadBalancerMaglev,
	LoadBalanceL5Cst:     config.DefaultLoadBalancerL5CST,
}
