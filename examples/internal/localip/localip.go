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

// Package localip resolves the local address used to reach a remote server.
package localip

import (
	"fmt"
	"net"
	"strconv"
	"time"
)

const connectTimeout = 3 * time.Second

// FromServer dials serverHost:serverPort and returns the local IP of that socket.
func FromServer(serverHost string, serverPort int) (string, error) {
	if serverHost == "" {
		return "", fmt.Errorf("server host is empty")
	}
	address := net.JoinHostPort(serverHost, strconv.Itoa(serverPort))
	conn, err := net.DialTimeout("tcp", address, connectTimeout)
	if err != nil {
		return "", fmt.Errorf("dial %s: %w", address, err)
	}
	defer func() {
		_ = conn.Close()
	}()

	tcpAddr, ok := conn.LocalAddr().(*net.TCPAddr)
	if !ok || tcpAddr.IP == nil {
		return "", fmt.Errorf("unexpected local address %q", conn.LocalAddr())
	}
	return tcpAddr.IP.String(), nil
}
