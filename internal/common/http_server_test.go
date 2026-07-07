/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

package common

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestServerAddressUsesConfiguredHostAndPort(t *testing.T) {
	cfg := ServerConfig{Host: "127.0.0.1", Port: 8081}

	if actual := ServerAddress(cfg); actual != "127.0.0.1:8081" {
		t.Fatalf("expected configured address, got %q", actual)
	}
}

func TestServerAddressDefaultsBlankHost(t *testing.T) {
	cfg := ServerConfig{Port: 8081}

	if actual := ServerAddress(cfg); actual != "0.0.0.0:8081" {
		t.Fatalf("expected default host address, got %q", actual)
	}
}

func TestNewConfiguredHTTPServerAppliesTimeouts(t *testing.T) {
	cfg := ServerConfig{
		Host:                     "::1",
		Port:                     8082,
		ReadHeaderTimeoutSeconds: 1,
		ReadTimeoutSeconds:       2,
		WriteTimeoutSeconds:      3,
		IdleTimeoutSeconds:       4,
		ShutdownTimeoutSeconds:   5,
	}

	server := NewConfiguredHTTPServer(cfg, http.NewServeMux())

	if server.Addr != "[::1]:8082" {
		t.Fatalf("expected IPv6 address, got %q", server.Addr)
	}
	if server.ReadHeaderTimeout != time.Second ||
		server.ReadTimeout != 2*time.Second ||
		server.WriteTimeout != 3*time.Second ||
		server.IdleTimeout != 4*time.Second {
		t.Fatalf("unexpected server timeouts: %+v", server)
	}
}

func TestNewConfiguredHTTPServerDefaultsZeroTimeouts(t *testing.T) {
	cfg := ServerConfig{Port: 8083}

	server := NewConfiguredHTTPServer(cfg, http.NewServeMux())

	if server.ReadHeaderTimeout != 15*time.Second ||
		server.ReadTimeout != 30*time.Second ||
		server.WriteTimeout != 30*time.Second ||
		server.IdleTimeout != 60*time.Second {
		t.Fatalf("unexpected default server timeouts: %+v", server)
	}
	runner := StartHTTPServer("test", ServerConfig{Host: "127.0.0.1", Port: reserveHTTPServerTestPort(t)}, http.NotFoundHandler())
	if runner.shutdownTimeout != 10*time.Second {
		t.Fatalf("expected default shutdown timeout, got %v", runner.shutdownTimeout)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := runner.Wait(ctx); err != nil {
		t.Fatalf("unexpected runner wait error: %v", err)
	}
}

func TestRunServerContextCancellationShutsDownHTTPServer(t *testing.T) {
	port := reserveHTTPServerTestPort(t)
	cfg := ServerConfig{
		Host:                   "127.0.0.1",
		Port:                   port,
		ShutdownTimeoutSeconds: 1,
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	runner := StartHTTPServer("test", cfg, handler)
	waitErr := make(chan error, 1)
	go func() {
		waitErr <- runner.Wait(ctx)
	}()

	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitForHTTPServer(t, url)
	cancel()

	select {
	case err := <-waitErr:
		if err != nil {
			t.Fatalf("unexpected wait error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not shut down after context cancellation")
	}
}

func TestLoadConfigRejectsNonPositiveServerTimeout(t *testing.T) {
	for _, key := range []string{
		"SERVER_READHEADERTIMEOUTSECONDS",
		"SERVER_READTIMEOUTSECONDS",
		"SERVER_WRITETIMEOUTSECONDS",
		"SERVER_IDLETIMEOUTSECONDS",
		"SERVER_SHUTDOWNTIMEOUTSECONDS",
	} {
		withUnsetEnv(t, key)
	}
	captureLogOutput(t)
	path := writeTempConfig(t, "server:\n  readTimeoutSeconds: 0\n")

	_, err := LoadConfig(path, QUIET)
	if err == nil {
		t.Fatal("expected config load error for non-positive server timeout")
	}
	if !strings.Contains(err.Error(), "CONFIG-SERVER-TIMEOUT") {
		t.Fatalf("expected CONFIG-SERVER-TIMEOUT error, got %v", err)
	}
}

func reserveHTTPServerTestPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve local port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("close local port reservation: %v", err)
	}
	return port
}

func waitForHTTPServer(t *testing.T, url string) {
	t.Helper()
	client := &http.Client{Timeout: 100 * time.Millisecond}
	deadline := time.Now().Add(2 * time.Second)
	for {
		response, err := client.Get(url)
		if err == nil {
			_ = response.Body.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("server at %s did not become reachable: %v", url, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
