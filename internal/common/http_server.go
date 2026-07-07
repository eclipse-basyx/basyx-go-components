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
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// HTTPServerRunner manages one configured HTTP server instance.
type HTTPServerRunner struct {
	server          *http.Server
	serviceCode     string
	shutdownTimeout time.Duration
	serveErr        chan error
}

// SignalContext returns a context that is canceled on process interrupt or SIGTERM.
func SignalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.TODO(), os.Interrupt, syscall.SIGTERM)
}

// ServerAddress returns the configured HTTP listen address.
func ServerAddress(cfg ServerConfig) string {
	host := strings.TrimSpace(cfg.Host)
	if host == "" {
		host = DefaultConfig.ServerHost
	}
	return net.JoinHostPort(host, strconv.Itoa(cfg.Port))
}

// NewConfiguredHTTPServer creates an HTTP server with BaSyx timeout defaults.
func NewConfiguredHTTPServer(cfg ServerConfig, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              ServerAddress(cfg),
		Handler:           handler,
		ReadHeaderTimeout: serverTimeout(cfg.ReadHeaderTimeoutSeconds, DefaultConfig.ServerReadHeaderTimeoutSeconds),
		ReadTimeout:       serverTimeout(cfg.ReadTimeoutSeconds, DefaultConfig.ServerReadTimeoutSeconds),
		WriteTimeout:      serverTimeout(cfg.WriteTimeoutSeconds, DefaultConfig.ServerWriteTimeoutSeconds),
		IdleTimeout:       serverTimeout(cfg.IdleTimeoutSeconds, DefaultConfig.ServerIdleTimeoutSeconds),
	}
}

// StartHTTPServer starts a configured HTTP server in a managed goroutine.
func StartHTTPServer(serviceCode string, cfg ServerConfig, handler http.Handler) *HTTPServerRunner {
	runner := &HTTPServerRunner{
		server:          NewConfiguredHTTPServer(cfg, handler),
		serviceCode:     normalizeServiceCode(serviceCode),
		shutdownTimeout: serverTimeout(cfg.ShutdownTimeoutSeconds, DefaultConfig.ServerShutdownTimeoutSeconds),
		serveErr:        make(chan error, 1),
	}
	go runner.listenAndServe()
	return runner
}

// RunHTTPServer starts a configured HTTP server and blocks until it stops.
func RunHTTPServer(ctx context.Context, serviceCode string, cfg ServerConfig, handler http.Handler) error {
	return StartHTTPServer(serviceCode, cfg, handler).Wait(ctx)
}

// Wait blocks until the server fails to start, stops externally, or ctx is canceled.
func (runner *HTTPServerRunner) Wait(ctx context.Context) error {
	if runner == nil || runner.server == nil {
		return fmt.Errorf("HTTP-RUNSERVER-CONTEXT server runner must not be nil")
	}
	if ctx == nil {
		return fmt.Errorf("%s-RUNSERVER-CONTEXT context must not be nil", runner.serviceCode)
	}

	select {
	case err := <-runner.serveErr:
		return runner.listenError(err)
	case <-ctx.Done():
		log.Println("Shutting down server...")
		return runner.shutdown(ctx)
	}
}

func (runner *HTTPServerRunner) listenAndServe() {
	err := runner.server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		runner.serveErr <- nil
		return
	}
	runner.serveErr <- err
}

func (runner *HTTPServerRunner) shutdown(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runner.shutdownTimeout)
	defer cancel()

	if err := runner.server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("%s-RUNSERVER-SHUTDOWN %w", runner.serviceCode, err)
	}

	select {
	case err := <-runner.serveErr:
		return runner.listenError(err)
	case <-shutdownCtx.Done():
		return fmt.Errorf("%s-RUNSERVER-SHUTDOWN %w", runner.serviceCode, shutdownCtx.Err())
	}
}

func (runner *HTTPServerRunner) listenError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s-RUNSERVER-LISTEN %w", runner.serviceCode, err)
}

func serverTimeout(seconds int, defaultSeconds int) time.Duration {
	if seconds <= 0 {
		seconds = defaultSeconds
	}
	return time.Duration(seconds) * time.Second
}

func normalizeServiceCode(serviceCode string) string {
	normalized := strings.ToUpper(strings.TrimSpace(serviceCode))
	if normalized == "" {
		return "HTTP"
	}
	return normalized
}
