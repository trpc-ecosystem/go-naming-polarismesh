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

// Package main runs the shared provider used by feature examples.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"trpc.group/trpc-go/trpc-go"
	"trpc.group/trpc-go/trpc-go/server"
	_ "trpc.group/trpc-go/trpc-naming-polarismesh"
	"trpc.group/trpc-go/trpc-naming-polarismesh/examples/internal/localip"
	polarisregistry "trpc.group/trpc-go/trpc-naming-polarismesh/registry"

	"github.com/polarismesh/polaris-go/api"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var (
	serviceName   string
	nodeID        string
	namespace     string
	version       string
	metadataMode  string
	labels        string
	debug         bool
	port          int
	polarisServer string
	polarisPort   int
)

type echoService interface {
	Echo(context.Context, *wrapperspb.StringValue) (*wrapperspb.StringValue, error)
}

type echoServiceImpl struct {
	nodeID   string
	port     int
	version  string
	metadata string
}

func (s *echoServiceImpl) Echo(
	_ context.Context,
	req *wrapperspb.StringValue,
) (*wrapperspb.StringValue, error) {
	return wrapperspb.String(fmt.Sprintf(
		"%s|%s|%d|%s|%s", s.nodeID, s.metadata, s.port, s.version, req.GetValue())), nil
}

func echoHandler(
	svr interface{},
	ctx context.Context,
	filterFactory server.FilterFunc,
) (interface{}, error) {
	req := &wrapperspb.StringValue{}
	filters, err := filterFactory(req)
	if err != nil {
		return nil, err
	}
	handle := func(ctx context.Context, reqBody interface{}) (interface{}, error) {
		return svr.(echoService).Echo(ctx, reqBody.(*wrapperspb.StringValue))
	}
	return filters.Filter(ctx, req, handle)
}

func initArgs() {
	flag.StringVar(&serviceName, "service", "trpc.polaris.e2e.Echo", "tRPC service name")
	flag.StringVar(&nodeID, "node-id", "provider", "unique provider node id")
	flag.StringVar(&namespace, "namespace", "go-naming-polarismesh-e2e", "Polaris namespace")
	flag.StringVar(&version, "version", "v1.0.0", "registered instance version")
	flag.StringVar(&metadataMode, "metadata-mode", "dynamic",
		"metadata mode: dynamic, none or conflict")
	flag.StringVar(&labels, "labels", "", "comma-separated dynamic metadata key=value pairs")
	flag.BoolVar(&debug, "debug", false, "enable Polaris debug logs")
	flag.IntVar(&port, "port", 18191, "provider listen port")
	flag.StringVar(&polarisServer, "polaris-server", os.Getenv("POLARIS_SERVER"),
		"Polaris host without scheme")
	flag.IntVar(&polarisPort, "polaris-port", 8091, "Polaris gRPC port")
}

func main() {
	initArgs()
	flag.Parse()
	if polarisServer == "" {
		log.Fatal("--polaris-server or POLARIS_SERVER is required")
	}
	localHost, err := localip.FromServer(polarisServer, polarisPort)
	if err != nil {
		log.Fatalf("resolve local host from Polaris connection: %v", err)
	}
	if err := os.Setenv("PROVIDER_HOST", localHost); err != nil {
		log.Fatalf("set PROVIDER_HOST: %v", err)
	}
	if debug {
		if err := api.SetLoggersLevel(api.DebugLog); err != nil {
			log.Printf("[WARN] enable debug logs: %v", err)
		}
	}

	if metadataMode != "none" {
		provider, err := dynamicMetadata()
		if err != nil {
			log.Fatalf("build metadata provider: %v", err)
		}
		if err := polarisregistry.RegisterMetadataProvider(serviceName, provider); err != nil {
			log.Fatalf("register metadata provider: %v", err)
		}
		defer polarisregistry.UnregisterMetadataProvider(serviceName)
	}

	s := trpc.NewServer()
	service := s.Service(serviceName)
	if service == nil {
		log.Fatalf("service %s is not configured", serviceName)
	}
	serviceDesc := &server.ServiceDesc{
		ServiceName: serviceName,
		HandlerType: (*echoService)(nil),
		Methods: []server.Method{{
			Name: "/" + serviceName + "/Echo",
			Func: echoHandler,
		}},
	}
	if err := service.Register(serviceDesc, &echoServiceImpl{
		nodeID: nodeID, port: port, version: version, metadata: metadataMode,
	}); err != nil {
		log.Fatalf("register echo service: %v", err)
	}
	if err := s.Serve(); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

func dynamicMetadata() (polarisregistry.MetadataProvider, error) {
	return polarisregistry.MetadataProviderFunc(func() (map[string]string, error) {
		metadata := map[string]string{
			"e2e-node-id":   nodeID,
			"e2e-owner":     "go-naming-polarismesh",
			"e2e-namespace": namespace,
		}
		for key, value := range parseLabels(labels) {
			metadata[key] = value
		}
		return metadata, nil
	}), nil
}

func parseLabels(raw string) map[string]string {
	result := make(map[string]string)
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			continue
		}
		result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return result
}
