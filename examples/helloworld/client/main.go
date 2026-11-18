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

// Package main calls the HelloWorld service through both naming paths.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	trpc "trpc.group/trpc-go/trpc-go"
	"trpc.group/trpc-go/trpc-go/client"
	"trpc.group/trpc-go/trpc-go/naming/registry"
	_ "trpc.group/trpc-go/trpc-naming-polarismesh"
	pb "trpc.group/trpc-go/trpc-naming-polarismesh/examples/helloworld/proto"
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
	caller        string
	message       string
)

type callResult struct {
	Mode     string
	Address  string
	Response string
}

func main() {
	initArgs()
	flag.Parse()
	if polarisServer == "" {
		log.Fatal("--polaris-server or POLARIS_SERVER is required")
	}
	setEnvironment()
	trpc.ServerConfigPath = configPath

	_ = trpc.NewServer()
	proxy := pb.NewGreeterClientProxy()
	results := []callResult{
		mustCall(proxy, "target", "Hello"),
		mustCall(proxy, "service", "Hello"),
		mustCall(proxy, "service", "Hi"),
	}
	for _, result := range results {
		fmt.Printf("%-16s address=%-21s response=%s\n",
			result.Mode, result.Address, result.Response)
	}
}

func mustCall(
	proxy pb.GreeterClientProxy,
	path string,
	method string,
) callResult {
	var node registry.Node
	opts := baseOptions(&node)
	if path == "target" {
		opts = append(opts, client.WithTarget("polarismesh://"+serviceName))
	} else {
		opts = append(opts, client.WithServiceName(serviceName))
	}
	req := &pb.HelloRequest{Msg: message}
	var (
		rsp *pb.HelloReply
		err error
	)
	switch method {
	case "Hello":
		rsp, err = proxy.Hello(context.Background(), req, opts...)
	case "Hi":
		rsp, err = proxy.Hi(context.Background(), req, opts...)
	default:
		log.Fatalf("unsupported method %q", method)
	}
	if err != nil {
		log.Fatalf("%s://%s failed: %v", path, method, err)
	}
	return callResult{
		Mode:     path + "://" + method,
		Address:  node.Address,
		Response: rsp.GetMsg(),
	}
}

func baseOptions(node *registry.Node) []client.Option {
	return []client.Option{
		client.WithNamespace(namespace),
		client.WithCallerNamespace(namespace),
		client.WithCallerServiceName(caller),
		client.WithSelectorNode(node),
		client.WithProtocol("trpc"),
		client.WithNetwork("tcp"),
		client.WithTimeout(3 * time.Second),
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
	flag.StringVar(&caller, "caller", envDefault("HELLOWORLD_CALLER",
		"trpc.examples.helloworld.Client"), "caller service name")
	flag.StringVar(&message, "message", envDefault("HELLOWORLD_MESSAGE", "tRPC-Go"),
		"message sent to the server")
}

func setEnvironment() {
	setenv("POLARIS_SERVER", polarisServer)
	setenv("POLARIS_GRPC_PORT", strconv.Itoa(polarisPort))
	setenv("POLARIS_NAMESPACE", namespace)
	setenv("POLARIS_SERVICE", serviceName)
	cacheDir, err := filepath.Abs(filepath.Join(filepath.Dir(configPath), ".polaris-cache"))
	if err != nil {
		log.Fatalf("resolve Polaris cache directory: %v", err)
	}
	setenv("POLARIS_CACHE_DIR", cacheDir)
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
