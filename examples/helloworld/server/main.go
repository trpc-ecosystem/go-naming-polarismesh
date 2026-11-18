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

// Package main starts a HelloWorld provider registered through Polaris.
package main

//go:generate trpc create -p ../proto/helloworld.proto --api-version 2 --rpconly -o ../proto --protodir . --mock=false --nogomod

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"

	trpc "trpc.group/trpc-go/trpc-go"
	_ "trpc.group/trpc-go/trpc-naming-polarismesh"
	pb "trpc.group/trpc-go/trpc-naming-polarismesh/examples/helloworld/proto"
	"trpc.group/trpc-go/trpc-naming-polarismesh/examples/internal/localip"
)

const (
	defaultNamespace = "go-naming-polarismesh-e2e"
	defaultService   = "trpc.examples.helloworld.Greeter"
)

var (
	configPath    string
	polarisServer string
	polarisPort   int
	namespace     string
	serviceName   string
	version       string
	listenPort    int
)

type greeterService struct {
	version string
}

func (s *greeterService) Hello(
	_ context.Context,
	req *pb.HelloRequest,
) (*pb.HelloReply, error) {
	return &pb.HelloReply{
		Msg: fmt.Sprintf("Hello, %s! (server %s)", req.GetMsg(), s.version),
	}, nil
}

func (s *greeterService) Hi(
	_ context.Context,
	req *pb.HelloRequest,
) (*pb.HelloReply, error) {
	return &pb.HelloReply{
		Msg: fmt.Sprintf("Hi, %s! (server %s)", req.GetMsg(), s.version),
	}, nil
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
	setEnvironment(localHost)
	trpc.ServerConfigPath = configPath

	s := trpc.NewServer()
	if err := pb.RegisterGreeterService(
		s.Service(serviceName),
		&greeterService{version: version},
	); err != nil {
		log.Fatalf("register HelloWorld service: %v", err)
	}

	log.Printf("HelloWorld server listening on %s:%d as %s", localHost, listenPort, serviceName)
	if err := s.Serve(); err != nil {
		log.Fatalf("serve HelloWorld: %v", err)
	}
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
	flag.StringVar(&version, "version", envDefault("POLARIS_VERSION", "v1.0.0"),
		"registered instance version")
	flag.IntVar(&listenPort, "port", envInt("HELLOWORLD_PORT", 18191),
		"HelloWorld listen port")
}

func setEnvironment(localHost string) {
	setenv("POLARIS_SERVER", polarisServer)
	setenv("POLARIS_GRPC_PORT", strconv.Itoa(polarisPort))
	setenv("POLARIS_NAMESPACE", namespace)
	setenv("POLARIS_SERVICE", serviceName)
	setenv("POLARIS_VERSION", version)
	setenv("HELLOWORLD_PORT", strconv.Itoa(listenPort))
	setenv("HELLOWORLD_HOST", localHost)
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
