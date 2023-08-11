// Tencent is pleased to support the open source community by making tRPC available.
// Copyright (C) 2023 THL A29 Limited, a Tencent company. All rights reserved.
// If you have downloaded a copy of the tRPC source code from Tencent,
// please note that tRPC source code is licensed under the Apache 2.0 License,
// A copy of the Apache 2.0 License is included in this file.

package servicerouter

// Config configuration.
type Config struct {
	// Name is the current name of plugin.
	Name string
	// Enable configures whether to enable the service routing function.
	Enable bool
	// EnableCanary configures whether to enable the canary function.
	EnableCanary bool
	// NeedReturnAllNodes expands all nodes into registry.Node and return.
	NeedReturnAllNodes bool
}

const (
	setEnableKey   string = "internal-enable-set"
	setNameKey     string = "internal-set-name"
	setEnableValue string = "Y"
)
