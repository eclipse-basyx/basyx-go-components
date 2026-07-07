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

	server := NewConfiguredHTTPServer(t.Context(), cfg, http.NewServeMux())

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

	server := NewConfiguredHTTPServer(t.Context(), cfg, http.NewServeMux())

	if server.ReadHeaderTimeout != 15*time.Second ||
		server.ReadTimeout != 300*time.Second ||
		server.WriteTimeout != 300*time.Second ||
		server.IdleTimeout != 60*time.Second {
		t.Fatalf("unexpected default server timeouts: %+v", server)
	}
	ctx, cancel := context.WithCancel(t.Context())
	runner, err := StartHTTPServer(ctx, "test", ServerConfig{Host: "127.0.0.1"}, http.NotFoundHandler())
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}
	if runner.shutdownTimeout != 10*time.Second {
		t.Fatalf("expected default shutdown timeout, got %v", runner.shutdownTimeout)
	}
	cancel()
	if err := runner.Wait(ctx); err != nil {
		t.Fatalf("unexpected runner wait error: %v", err)
	}
}

func TestRunServerContextCancellationShutsDownHTTPServer(t *testing.T) {
	cfg := ServerConfig{
		Host:                   "127.0.0.1",
		ShutdownTimeoutSeconds: 1,
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	runner, err := StartHTTPServer(ctx, "test", cfg, handler)
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}
	waitErr := make(chan error, 1)
	go func() {
		waitErr <- runner.Wait(ctx)
	}()

	url := fmt.Sprintf("http://%s", runner.server.Addr)
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

func TestRunServerContextCancellationCancelsRequestContext(t *testing.T) {
	cfg := ServerConfig{
		Host:                   "127.0.0.1",
		ShutdownTimeoutSeconds: 1,
	}
	requestStarted := make(chan struct{})
	requestCanceled := make(chan struct{})
	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-r.Context().Done()
		close(requestCanceled)
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	runner, err := StartHTTPServer(ctx, "test", cfg, handler)
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}
	waitErr := make(chan error, 1)
	go func() {
		waitErr <- runner.Wait(ctx)
	}()

	clientDone := make(chan struct{})
	client := &http.Client{Timeout: 2 * time.Second}
	go func() {
		response, _ := client.Get(fmt.Sprintf("http://%s", runner.server.Addr))
		if response != nil {
			_ = response.Body.Close()
		}
		close(clientDone)
	}()

	select {
	case <-requestStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not receive request")
	}
	cancel()

	select {
	case <-requestCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("request context was not canceled during shutdown")
	}
	select {
	case err := <-waitErr:
		if err != nil {
			t.Fatalf("unexpected wait error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not shut down after request context cancellation")
	}
	select {
	case <-clientDone:
	case <-time.After(2 * time.Second):
		t.Fatal("client request did not finish")
	}
}

func TestStartHTTPServerReturnsListenErrorBeforeServing(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("bind occupied listener: %v", err)
	}
	defer func() {
		_ = listener.Close()
	}()
	port := listener.Addr().(*net.TCPAddr).Port

	runner, err := StartHTTPServer(t.Context(), "test", ServerConfig{Host: "127.0.0.1", Port: port}, http.NotFoundHandler())
	if err == nil {
		t.Fatal("expected listen error for occupied port")
	}
	if runner != nil {
		t.Fatal("expected nil runner on listen failure")
	}
	if !strings.Contains(err.Error(), "TEST-RUNSERVER-LISTEN") {
		t.Fatalf("expected TEST-RUNSERVER-LISTEN error, got %v", err)
	}
}

func TestLoadConfigAppliesServerTimeoutConfig(t *testing.T) {
	unsetServerTimeoutEnv(t)
	path := writeTempConfig(t, `server:
  readHeaderTimeoutSeconds: 11
  readTimeoutSeconds: 301
  writeTimeoutSeconds: 302
  idleTimeoutSeconds: 61
  shutdownTimeoutSeconds: 12
`)

	cfg, err := LoadConfig(path, QUIET)
	if err != nil {
		t.Fatalf("unexpected config load error: %v", err)
	}

	assertServerTimeouts(t, cfg.Server, 11, 301, 302, 61, 12)
}

func TestLoadConfigAppliesReadableServerTimeoutEnvironmentOverrides(t *testing.T) {
	unsetServerTimeoutEnv(t)
	t.Setenv("SERVER_READ_HEADER_TIMEOUT_SECONDS", "12")
	t.Setenv("BASYX_SERVER_READ_TIMEOUT_SECONDS", "303")
	t.Setenv("SERVER_WRITE_TIMEOUT_SECONDS", "304")
	t.Setenv("BASYX_SERVER_IDLE_TIMEOUT_SECONDS", "62")
	t.Setenv("SERVER_SHUTDOWN_TIMEOUT_SECONDS", "13")

	cfg, err := LoadConfig("", QUIET)
	if err != nil {
		t.Fatalf("unexpected config load error: %v", err)
	}

	assertServerTimeouts(t, cfg.Server, 12, 303, 304, 62, 13)
}

func TestLoadConfigRejectsNonPositiveServerTimeouts(t *testing.T) {
	for _, tc := range []struct {
		name    string
		yamlKey string
		value   int
	}{
		{name: "read header zero", yamlKey: "readHeaderTimeoutSeconds", value: 0},
		{name: "read negative", yamlKey: "readTimeoutSeconds", value: -1},
		{name: "write zero", yamlKey: "writeTimeoutSeconds", value: 0},
		{name: "idle negative", yamlKey: "idleTimeoutSeconds", value: -1},
		{name: "shutdown zero", yamlKey: "shutdownTimeoutSeconds", value: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			unsetServerTimeoutEnv(t)
			path := writeTempConfig(t, fmt.Sprintf("server:\n  %s: %d\n", tc.yamlKey, tc.value))

			_, err := LoadConfig(path, QUIET)
			if err == nil {
				t.Fatal("expected config load error for non-positive server timeout")
			}
			if !strings.Contains(err.Error(), "CONFIG-SERVER-TIMEOUT") {
				t.Fatalf("expected CONFIG-SERVER-TIMEOUT error, got %v", err)
			}
			if !strings.Contains(err.Error(), tc.yamlKey) {
				t.Fatalf("expected error to name %s, got %v", tc.yamlKey, err)
			}
		})
	}
}

func unsetServerTimeoutEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"SERVER_READHEADERTIMEOUTSECONDS",
		"SERVER_READTIMEOUTSECONDS",
		"SERVER_WRITETIMEOUTSECONDS",
		"SERVER_IDLETIMEOUTSECONDS",
		"SERVER_SHUTDOWNTIMEOUTSECONDS",
		"SERVER_READ_HEADER_TIMEOUT_SECONDS",
		"SERVER_READ_TIMEOUT_SECONDS",
		"SERVER_WRITE_TIMEOUT_SECONDS",
		"SERVER_IDLE_TIMEOUT_SECONDS",
		"SERVER_SHUTDOWN_TIMEOUT_SECONDS",
		"BASYX_SERVER_READ_HEADER_TIMEOUT_SECONDS",
		"BASYX_SERVER_READ_TIMEOUT_SECONDS",
		"BASYX_SERVER_WRITE_TIMEOUT_SECONDS",
		"BASYX_SERVER_IDLE_TIMEOUT_SECONDS",
		"BASYX_SERVER_SHUTDOWN_TIMEOUT_SECONDS",
	} {
		withUnsetEnv(t, key)
	}
}

func assertServerTimeouts(t *testing.T, cfg ServerConfig, readHeader int, read int, write int, idle int, shutdown int) {
	t.Helper()
	if cfg.ReadHeaderTimeoutSeconds != readHeader ||
		cfg.ReadTimeoutSeconds != read ||
		cfg.WriteTimeoutSeconds != write ||
		cfg.IdleTimeoutSeconds != idle ||
		cfg.ShutdownTimeoutSeconds != shutdown {
		t.Fatalf("unexpected server timeouts: %+v", cfg)
	}
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
