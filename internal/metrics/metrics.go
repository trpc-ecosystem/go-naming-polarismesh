// Tencent is pleased to support the open source community by making tRPC available.
// Copyright (C) 2023 THL A29 Limited, a Tencent company. All rights reserved.
// If you have downloaded a copy of the tRPC source code from Tencent,
// please note that tRPC source code is licensed under the Apache 2.0 License,
// A copy of the Apache 2.0 License is included in this file.

// Package metrics 定义和上报metrics
package metrics

import (
	"strconv"

	"trpc.group/trpc-go/trpc-go/metrics"

	"github.com/polarismesh/polaris-go/api"
	plog "github.com/polarismesh/polaris-go/pkg/log"
)

const (
	polarisMetricsKey          = "polaris_metrics"
	polarisServiceKey          = "polaris_service"
	polarisServiceNamespaceKey = "polaris_namespace"
	polarisServiceHostKey      = "polaris_host"
	polarisServicePortKey      = "polaris_port"
)

// ReportHeartBeatFail report service heartbeat fails
func ReportHeartBeatFail(req *api.InstanceHeartbeatRequest) {
	dims := []*metrics.Dimension{
		{
			Name:  polarisServiceKey,
			Value: req.Service,
		},
		{
			Name:  polarisServiceNamespaceKey,
			Value: req.Namespace,
		},
		{
			Name:  polarisServiceHostKey,
			Value: req.Host,
		},
		{
			Name:  polarisServicePortKey,
			Value: strconv.FormatInt(int64(req.Port), 10),
		},
	}
	indices := []*metrics.Metrics{
		metrics.NewMetrics("trpc.PolarisHeartBeatFail", float64(1), metrics.PolicySUM),
	}
	err := metrics.ReportMultiDimensionMetricsX(polarisMetricsKey, dims, indices)
	if err != nil {
		plog.GetBaseLogger().Errorf("heartbeat metrics report err: %v\n", err)
	}
}
