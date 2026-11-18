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

// Package main demonstrates one service registered with multiple endpoints.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"

	"trpc.group/trpc-go/trpc-go"
	"trpc.group/trpc-go/trpc-go/server"
	_ "trpc.group/trpc-go/trpc-naming-polarismesh"
	"trpc.group/trpc-go/trpc-naming-polarismesh/examples/internal/localip"

	"github.com/polarismesh/polaris-go/api"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var (
	configPath    string
	polarisServer string
	polarisPort   int
	namespace     string
	serviceName   string
	providerHost  string
	trpcPort      int
	httpPort      int
	versionA      string
	versionB      string
	weightA       int
	weightB       int
	instanceA     string
	instanceB     string
	debug         bool
)

type echoService interface {
	Echo(context.Context, *wrapperspb.StringValue) (*wrapperspb.StringValue, error)
}

type echoServiceImpl struct{}

func (s *echoServiceImpl) Echo(
	_ context.Context,
	req *wrapperspb.StringValue,
) (*wrapperspb.StringValue, error) {
	return wrapperspb.String("multi transport: " + req.GetValue()), nil
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

func main() {
	initArgs()
	flag.Parse()
	if polarisServer == "" {
		log.Fatal("--polaris-server or POLARIS_SERVER is required")
	}
	if providerHost == "" {
		host, err := localip.FromServer(polarisServer, polarisPort)
		if err != nil {
			log.Fatalf("resolve local host from Polaris connection: %v", err)
		}
		providerHost = host
	}
	setEnvironment()
	trpc.ServerConfigPath = configPath

	if debug {
		if err := api.SetLoggersLevel(api.DebugLog); err != nil {
			log.Printf("[WARN] enable Polaris debug logs: %v", err)
		}
	}
	if err := startHTTPTransports(); err != nil {
		log.Fatalf("start HTTP transports: %v", err)
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
	if err := service.Register(serviceDesc, &echoServiceImpl{}); err != nil {
		log.Fatalf("register echo service: %v", err)
	}

	log.Printf("multi-endpoint provider listening on %s:%d as %s", providerHost, trpcPort, serviceName)
	if err := s.Serve(); err != nil {
		log.Fatalf("serve multi-endpoint provider: %v", err)
	}
}

func startHTTPTransports() error {
	ports := []int{httpPort}
	for _, port := range ports {
		if port <= 0 {
			continue
		}
		address := net.JoinHostPort("", strconv.Itoa(port))
		listener, err := net.Listen("tcp", address)
		if err != nil {
			return fmt.Errorf("listen %s: %w", address, err)
		}
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"status":"ok"}`))
		})
		go func(port int, listener net.Listener, handler http.Handler) {
			if err := http.Serve(listener, handler); err != nil {
				log.Printf("[WARN] HTTP transport on port %d stopped: %v", port, err)
			}
		}(port, listener, mux)
	}
	return nil
}

func initArgs() {
	flag.StringVar(&configPath, "conf", "trpc_go.yaml", "tRPC configuration path")
	flag.StringVar(&polarisServer, "polaris-server", os.Getenv("POLARIS_SERVER"),
		"Polaris host without scheme")
	flag.IntVar(&polarisPort, "polaris-port", envInt("POLARIS_GRPC_PORT", 8091),
		"Polaris gRPC port")
	flag.StringVar(&namespace, "namespace", envDefault("POLARIS_NAMESPACE", "default"),
		"Polaris namespace")
	flag.StringVar(&serviceName, "service", envDefault("POLARIS_SERVICE", ""),
		"registered service name")
	flag.StringVar(&providerHost, "host", os.Getenv("PROVIDER_HOST"),
		"registered host; derived from the Polaris connection when empty")
	flag.IntVar(&trpcPort, "trpc-port", envInt("TRPC_PORT", 28351), "tRPC listener port")
	flag.IntVar(&httpPort, "http-port", envInt("HTTP_PORT", 28352), "HTTP listener port")
	flag.StringVar(&versionA, "version-a", envDefault("VERSION_A", "v1.0.0"),
		"first endpoint version")
	flag.StringVar(&versionB, "version-b", envDefault("VERSION_B", "v2.0.0"),
		"second endpoint version")
	flag.IntVar(&weightA, "weight-a", envInt("WEIGHT_A", 80), "first endpoint weight")
	flag.IntVar(&weightB, "weight-b", envInt("WEIGHT_B", 120), "second endpoint weight")
	flag.StringVar(&instanceA, "instance-a", envDefault("INSTANCE_A", ""),
		"first endpoint instance ID")
	flag.StringVar(&instanceB, "instance-b", envDefault("INSTANCE_B", ""),
		"second endpoint instance ID")
	flag.BoolVar(&debug, "debug", false, "enable Polaris debug logs")
}

func setEnvironment() {
	setenv("POLARIS_SERVER", polarisServer)
	setenv("POLARIS_GRPC_PORT", strconv.Itoa(polarisPort))
	setenv("POLARIS_NAMESPACE", namespace)
	setenv("POLARIS_SERVICE", serviceName)
	setenv("PROVIDER_HOST", providerHost)
	setenv("TRPC_PORT", strconv.Itoa(trpcPort))
	setenv("HTTP_PORT", strconv.Itoa(httpPort))
	setenv("VERSION_A", versionA)
	setenv("VERSION_B", versionB)
	setenv("WEIGHT_A", strconv.Itoa(weightA))
	setenv("WEIGHT_B", strconv.Itoa(weightB))
	setenv("INSTANCE_A", instanceA)
	setenv("INSTANCE_B", instanceB)
}

func setenv(key, value string) {
	if err := os.Setenv(key, value); err != nil {
		log.Fatalf("set %s: %v", key, err)
	}
}

func envDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	raw := envDefault(key, strconv.Itoa(fallback))
	value, err := strconv.Atoi(raw)
	if err == nil {
		return value
	}
	return fallback
}
