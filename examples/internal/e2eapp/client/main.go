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

// Package main exposes the HTTP control surface used by feature examples.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"trpc.group/trpc-go/trpc-go"
	"trpc.group/trpc-go/trpc-go/client"
	"trpc.group/trpc-go/trpc-go/codec"
	"trpc.group/trpc-go/trpc-go/naming/registry"
	_ "trpc.group/trpc-go/trpc-naming-polarismesh"

	"github.com/polarismesh/polaris-go/api"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var (
	listen    string
	service   string
	namespace string
	caller    string
	debug     bool
)

type callResult struct {
	Success   bool   `json:"success"`
	Address   string `json:"address"`
	Response  string `json:"response"`
	Error     string `json:"error,omitempty"`
	ElapsedMS int64  `json:"elapsed_ms"`
}

type sequenceResult struct {
	Total     int            `json:"total"`
	Successes int            `json:"successes"`
	Failures  int            `json:"failures"`
	Addresses map[string]int `json:"addresses"`
	Calls     []callResult   `json:"calls"`
}

type invokeOptions struct {
	mode           string
	lb             string
	key            string
	callerEnv      string
	calleeEnv      string
	destMetadata   string
	sourceMetadata string
	callerService  string
	disableRouter  bool
}

func initArgs() {
	flag.StringVar(&listen, "listen", "127.0.0.1:18080", "HTTP control listen address")
	flag.StringVar(&service, "service", "trpc.polaris.e2e.Echo", "callee service")
	flag.StringVar(&namespace, "namespace", "go-naming-polarismesh-e2e", "Polaris namespace")
	flag.StringVar(&caller, "caller", "trpc.polaris.e2e.Caller", "caller service")
	flag.BoolVar(&debug, "debug", false, "enable Polaris debug logs")
}

func main() {
	initArgs()
	flag.Parse()
	if debug {
		if err := api.SetLoggersLevel(api.DebugLog); err != nil {
			log.Printf("[WARN] enable Polaris debug logs: %v", err)
		}
	}

	_ = trpc.NewServer()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/call", handleCall)
	mux.HandleFunc("/sequence", handleSequence)

	httpServer := &http.Server{
		Addr:              listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("feature example client listening on %s", listen)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP control server: %v", err)
	}
}

func handleCall(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, invoke(r.Context(), optionsFromRequest(r)))
}

func handleSequence(w http.ResponseWriter, r *http.Request) {
	count, err := strconv.Atoi(r.URL.Query().Get("count"))
	if err != nil || count <= 0 || count > 4096 {
		http.Error(w, "count must be in [1,4096]", http.StatusBadRequest)
		return
	}
	options := optionsFromRequest(r)
	keyPrefix := r.URL.Query().Get("key")
	if keyPrefix == "" {
		keyPrefix = "e2e"
	}
	result := sequenceResult{
		Total:     count,
		Addresses: make(map[string]int),
		Calls:     make([]callResult, 0, count),
	}
	for i := 0; i < count; i++ {
		options.key = fmt.Sprintf("%s-%d", keyPrefix, i)
		call := invoke(r.Context(), options)
		result.Calls = append(result.Calls, call)
		if call.Success {
			result.Successes++
		} else {
			result.Failures++
		}
		if call.Address != "" {
			result.Addresses[call.Address]++
		}
	}
	writeJSON(w, result)
}

func optionsFromRequest(r *http.Request) invokeOptions {
	query := r.URL.Query()
	return invokeOptions{
		mode:           query.Get("mode"),
		lb:             query.Get("lb"),
		key:            query.Get("key"),
		callerEnv:      query.Get("caller_env"),
		calleeEnv:      query.Get("callee_env"),
		destMetadata:   query.Get("dest_meta"),
		sourceMetadata: query.Get("source_meta"),
		callerService:  query.Get("source_svc"),
		disableRouter:  query.Get("disable_router") == "true",
	}
}

func invoke(ctx context.Context, options invokeOptions) callResult {
	start := time.Now()
	ctx, msg := codec.WithCloneMessage(ctx)
	defer codec.PutBackMessage(msg)
	msg.WithClientRPCName("/" + service + "/Echo")
	msg.WithCalleeServiceName(service)
	msg.WithCalleeApp("polaris")
	msg.WithCalleeServer("e2e")
	msg.WithCalleeService("Echo")
	msg.WithCalleeMethod("Echo")
	msg.WithSerializationType(codec.SerializationTypePB)

	callerName := caller
	if options.callerService != "" {
		callerName = options.callerService
	}
	opts := []client.Option{
		client.WithNamespace(namespace),
		client.WithCallerNamespace(namespace),
		client.WithCallerServiceName(callerName),
		client.WithProtocol("trpc"),
		client.WithNetwork("tcp"),
		client.WithTimeout(3 * time.Second),
		client.WithShouldErrReportToSelector(func(err error) bool { return err != nil }),
	}
	if options.lb != "" {
		opts = append(opts, client.WithBalancerName(options.lb))
	}
	if options.key != "" {
		opts = append(opts, client.WithKey(options.key))
	}
	if options.callerEnv != "" {
		opts = append(opts, client.WithCallerEnvName(options.callerEnv))
	}
	if options.calleeEnv != "" {
		opts = append(opts, client.WithCalleeEnvName(options.calleeEnv))
	}
	if options.disableRouter {
		opts = append(opts, client.WithDisableServiceRouter())
	}
	for key, value := range parseMetadata(options.sourceMetadata) {
		opts = append(opts, client.WithCallerMetadata(key, value))
	}
	for key, value := range parseMetadata(options.destMetadata) {
		opts = append(opts, client.WithCalleeMetadata(key, value))
	}
	switch options.mode {
	case "target":
		opts = append(opts, client.WithTarget("polarismesh://"+service))
	case "service":
		opts = append(opts, client.WithServiceName(service))
	default:
		return callResult{
			Error:     "mode must be target or service",
			ElapsedMS: time.Since(start).Milliseconds(),
		}
	}

	var selected registry.Node
	opts = append(opts, client.WithSelectorNode(&selected))
	req := wrapperspb.String("hello")
	rsp := wrapperspb.String("")
	err := client.DefaultClient.Invoke(ctx, req, rsp, opts...)
	result := callResult{
		Success:   err == nil,
		Address:   selected.Address,
		Response:  rsp.GetValue(),
		ElapsedMS: time.Since(start).Milliseconds(),
	}
	if err != nil {
		result.Error = err.Error()
	}
	return result
}

func parseMetadata(raw string) map[string]string {
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

func writeJSON(w http.ResponseWriter, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}
