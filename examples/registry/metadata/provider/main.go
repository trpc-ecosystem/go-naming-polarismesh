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

// Package main demonstrates dynamic metadata registration through Polaris.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"

	"trpc.group/trpc-go/trpc-go"
	"trpc.group/trpc-go/trpc-go/server"
	_ "trpc.group/trpc-go/trpc-naming-polarismesh"
	"trpc.group/trpc-go/trpc-naming-polarismesh/examples/internal/localip"
	polarisregistry "trpc.group/trpc-go/trpc-naming-polarismesh/registry"

	"github.com/polarismesh/polaris-go/api"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	defaultNamespace = "go-naming-polarismesh-e2e"
	defaultService   = "trpc.polaris.registry.metadata.Echo"
)

var (
	configPath       string
	polarisServer    string
	polarisPort      int
	namespace        string
	serviceName      string
	advertiseHost    string
	version          string
	metadataMode     string
	listenPort       int
	registrationPort int
	debug            bool
)

type echoService interface {
	Echo(context.Context, *wrapperspb.StringValue) (*wrapperspb.StringValue, error)
}

type echoServiceImpl struct{}

func (s *echoServiceImpl) Echo(
	_ context.Context,
	req *wrapperspb.StringValue,
) (*wrapperspb.StringValue, error) {
	return wrapperspb.String("registry metadata: " + req.GetValue()), nil
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
	switch metadataMode {
	case "dynamic", "none", "failure":
	default:
		log.Fatalf("unsupported metadata mode %q", metadataMode)
	}
	if registrationPort == 0 {
		registrationPort = listenPort
	}
	if registrationPort < 1 || registrationPort > 65535 {
		log.Fatalf("registration port %d must be in 1..65535", registrationPort)
	}
	localHost, err := localip.FromServer(polarisServer, polarisPort)
	if err != nil {
		log.Fatalf("resolve local host from Polaris connection: %v", err)
	}
	if advertiseHost != "" {
		localHost = advertiseHost
	}
	setEnvironment(localHost)
	trpc.ServerConfigPath = configPath

	if debug {
		if err := api.SetLoggersLevel(api.DebugLog); err != nil {
			log.Printf("[WARN] enable Polaris debug logs: %v", err)
		}
	}
	if metadataMode != "none" {
		provider, err := dynamicMetadata(localHost)
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
	if err := service.Register(serviceDesc, &echoServiceImpl{}); err != nil {
		log.Fatalf("register echo service: %v", err)
	}

	log.Printf("dynamic metadata provider listening on %s:%d as %s",
		localHost, listenPort, serviceName)
	if err := s.Serve(); err != nil {
		log.Fatalf("serve registry metadata example: %v", err)
	}
}

func dynamicMetadata(localHost string) (polarisregistry.MetadataProvider, error) {
	payload, err := json.Marshal(struct {
		Service  string `json:"service"`
		Version  string `json:"version"`
		Endpoint string `json:"endpoint"`
	}{serviceName, version, net.JoinHostPort(localHost, strconv.Itoa(listenPort))})
	if err != nil {
		return nil, err
	}
	return polarisregistry.RegistrationSnapshotProviderFunc(func() (polarisregistry.RegistrationSnapshot, error) {
		if metadataMode == "failure" {
			return polarisregistry.RegistrationSnapshot{}, fmt.Errorf("snapshot unavailable")
		}
		return polarisregistry.RegistrationSnapshot{
			Host:             localHost,
			Metadata:         map[string]string{"deployment": "green", "descriptor": string(payload)},
			Version:          version,
			RegistrationPort: registrationPort,
			Protocol:         "",
			SetProtocol:      true,
		}, nil
	}), nil
}

func initArgs() {
	flag.StringVar(&configPath, "conf", "trpc_go.yaml", "tRPC configuration path")
	flag.StringVar(&polarisServer, "polaris-server", os.Getenv("POLARIS_SERVER"),
		"Polaris host without scheme")
	flag.IntVar(&polarisPort, "polaris-port", envInt("POLARIS_GRPC_PORT", 8091),
		"Polaris gRPC port")
	flag.StringVar(&namespace, "namespace", envDefault("POLARIS_NAMESPACE", defaultNamespace),
		"Polaris namespace")
	flag.StringVar(&serviceName, "service", envDefault("POLARIS_SERVICE", defaultService),
		"Polaris service name")
	flag.StringVar(&advertiseHost, "host", os.Getenv("PROVIDER_HOST"),
		"registration host; derived from Polaris when empty")
	flag.StringVar(&version, "version", envDefault("POLARIS_VERSION", "v1.2.3"),
		"registered instance version")
	flag.StringVar(&metadataMode, "metadata-mode", envDefault("METADATA_MODE", "dynamic"),
		"metadata mode: dynamic, none or failure")
	flag.IntVar(&listenPort, "port", envInt("PROVIDER_PORT", 28271),
		"provider listen port")
	flag.IntVar(&registrationPort, "registration-port", envInt("REGISTRATION_PORT", 0),
		"Polaris registration port; zero uses the listen port")
	flag.BoolVar(&debug, "debug", false, "enable Polaris debug logs")
}

func setEnvironment(localHost string) {
	setenv("POLARIS_SERVER", polarisServer)
	setenv("POLARIS_GRPC_PORT", strconv.Itoa(polarisPort))
	setenv("POLARIS_NAMESPACE", namespace)
	setenv("POLARIS_SERVICE", serviceName)
	setenv("POLARIS_VERSION", version)
	setenv("PROVIDER_HOST", localHost)
	setenv("PROVIDER_PORT", strconv.Itoa(listenPort))
	setenv("REGISTRATION_PORT", strconv.Itoa(registrationPort))
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
