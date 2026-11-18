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

package registry

import "github.com/polarismesh/polaris-go/pkg/model"

// Endpoint describes one Polaris instance registered by a tRPC service.
type Endpoint struct {
	// Address is the instance network address in host:port form.
	Address string
	// Protocol is the protocol of this instance endpoint.
	Protocol string
	// Version is the optional instance version.
	Version string
	// ServiceToken is the optional service access token.
	ServiceToken string
	// InstanceID is an optional stable instance ID.
	InstanceID string
	// Weight is the optional instance weight.
	Weight *int
	// TTL is the heartbeat TTL in seconds.
	TTL int
	// HeartBeat is the manual heartbeat interval in seconds.
	HeartBeat int
	// AutoHeartbeat enables adapter-managed heartbeat reporting.
	AutoHeartbeat *bool
	// DisableHealthCheck disables TTL registration and heartbeat reporting.
	DisableHealthCheck *bool
	// Metadata is endpoint-specific metadata and overrides service metadata.
	Metadata map[string]string
	// InstanceLocation is the geographic location of the instance.
	InstanceLocation *model.Location
}

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
	// Version is the optional service instance version.
	Version string
	// InstanceProtocol is the protocol of registered instances.
	InstanceProtocol string
	// BindAddress specifies reporting address.
	BindAddress string
	// PreferBindAddress gives the BindAddress higher priority over service TRPC's opts.Address when it is true.
	PreferBindAddress bool
	// Metadata User-defined metadata information.
	Metadata map[string]string
	// DisableHealthCheck disables healthcheck.
	DisableHealthCheck bool
	// AutoHeartbeat enables adapter-managed heartbeat reporting.
	AutoHeartbeat bool
	// InstanceLocation is the geographic location of the instance.
	InstanceLocation *model.Location
	// Endpoints registers one Polaris instance per endpoint.
	Endpoints []Endpoint
}
