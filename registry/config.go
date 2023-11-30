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

package registry

import "github.com/polarismesh/polaris-go/pkg/model"

// Config is registry configuration.
type Config struct {
	// ServiceToken Service Access Token.
	ServiceToken string
	// Protocol Server access method, support http grpc, default grpc.
	Protocol string
	// HeartBeat Reporting heartbeat time interval, the default is recommended as TTL/2.
	HeartBeat int
	// EnableRegister By default, only report heartbeat, do not register service, if true, start registration.
	EnableRegister bool
	// Weight.
	Weight *int
	// TTL Unit s, the cycle for the server to check whether the periodic instance is healthy.
	TTL int
	// InstanceID instance name.
	InstanceID string
	// Namespace namespace.
	Namespace string
	// ServiceName Service Name.
	ServiceName string
	// BindAddress specifies reporting address.
	BindAddress string
	// PreferBindAddress gives the BindAddress higher priority over service TRPC's opts.Address when it is true.
	PreferBindAddress bool
	// Metadata User-defined metadata information.
	Metadata map[string]string
	// DisableHealthCheck disables healthcheck.
	DisableHealthCheck bool
	// InstanceLocation is the geographic location of the instance.
	InstanceLocation *model.Location
}
