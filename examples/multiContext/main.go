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

// Package main demonstrates two isolated Polaris selector configurations.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"time"

	"trpc.group/trpc-go/trpc-go"
	"trpc.group/trpc-go/trpc-go/client"
	"trpc.group/trpc-go/trpc-go/codec"
	"trpc.group/trpc-go/trpc-go/naming/registry"
	adapternaming "trpc.group/trpc-go/trpc-naming-polarismesh"

	"github.com/polarismesh/polaris-go/api"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var (
	server    string
	namespace string
	caller    string
	serviceA  string
	serviceB  string
	debug     bool
)

type result struct {
	Success  bool   `json:"success"`
	Selector string `json:"selector"`
	Service  string `json:"service"`
	Address  string `json:"address"`
	Response string `json:"response"`
	Error    string `json:"error,omitempty"`
}

func main() {
	flag.StringVar(&server, "polaris-server", "", "Polaris host:port")
	flag.StringVar(&namespace, "namespace", "go-naming-polarismesh-e2e", "Polaris namespace")
	flag.StringVar(&caller, "caller", "trpc.polaris.e2e.MultiContextCaller", "caller service")
	flag.StringVar(&serviceA, "service-a", "trpc.polaris.e2e.MultiContextA", "first service")
	flag.StringVar(&serviceB, "service-b", "trpc.polaris.e2e.MultiContextB", "second service")
	flag.BoolVar(&debug, "debug", false, "enable Polaris debug logs")
	flag.Parse()
	if server == "" {
		log.Fatal("--polaris-server is required")
	}
	if debug {
		if err := api.SetLoggersLevel(api.DebugLog); err != nil {
			log.Printf("[WARN] enable debug logs: %v", err)
		}
	}

	_ = trpc.NewServer()
	for name := range map[string]struct{}{"polaris-a": {}, "polaris-b": {}} {
		persist := fmt.Sprintf("./polaris-%s-cache", name)
		if err := adapternaming.SetupWithConfig(&adapternaming.Config{
			Name:        name,
			Protocol:    "grpc",
			AddressList: server,
			PersistDir:  &persist,
			Default:     boolPtr(false),
		}); err != nil {
			log.Fatalf("setup selector %s: %v", name, err)
		}
	}

	results := []result{
		invoke("polaris-a", serviceA),
		invoke("polaris-b", serviceB),
	}
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(data))
	for _, item := range results {
		if !item.Success {
			log.Fatalf("selector %s call failed: %s", item.Selector, item.Error)
		}
	}
}

func invoke(selectorName, service string) result {
	ctx, msg := codec.WithCloneMessage(context.Background())
	defer codec.PutBackMessage(msg)
	msg.WithClientRPCName("/" + service + "/Echo")
	msg.WithCalleeServiceName(service)
	msg.WithCalleeApp("polaris")
	msg.WithCalleeServer("multi-context")
	msg.WithCalleeService("Echo")
	msg.WithCalleeMethod("Echo")
	msg.WithSerializationType(codec.SerializationTypePB)

	var selected registry.Node
	req := wrapperspb.String("multi-context")
	rsp := wrapperspb.String("")
	err := client.DefaultClient.Invoke(ctx, req, rsp,
		client.WithTarget(selectorName+"://"+service),
		client.WithNamespace(namespace),
		client.WithCallerNamespace(namespace),
		client.WithCallerServiceName(caller),
		client.WithSelectorNode(&selected),
		client.WithProtocol("trpc"),
		client.WithNetwork("tcp"),
		client.WithTimeout(3*time.Second),
	)
	item := result{
		Success:  err == nil,
		Selector: selectorName,
		Service:  service,
		Address:  selected.Address,
		Response: rsp.GetValue(),
	}
	if err != nil {
		item.Error = err.Error()
	}
	return item
}

func boolPtr(value bool) *bool {
	return &value
}
