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

// SignalContext returns a context that is canceled when the process receives
// os.Interrupt or SIGTERM, plus the cancel function returned by
// signal.NotifyContext. Callers should defer the cancel function in main so the
// signal handler is released when the service exits.
func SignalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.TODO(), os.Interrupt, syscall.SIGTERM)
}

// ServerAddress returns the HTTP listen address from cfg.Host and cfg.Port.
// Blank hosts are treated as the BaSyx default host, and the returned value is
// formatted with net.JoinHostPort so IPv6 hosts are bracketed correctly.
func ServerAddress(cfg ServerConfig) string {
	host := strings.TrimSpace(cfg.Host)
	if host == "" {
		host = DefaultConfig.ServerHost
	}
	return net.JoinHostPort(host, strconv.Itoa(cfg.Port))
}

// NewConfiguredHTTPServer creates an HTTP server with BaSyx timeout defaults.
// The ctx parameter becomes the base context for accepted connections, allowing
// request handlers to observe service shutdown through r.Context(). The cfg
// parameter supplies the listen address and timeout values; unset timeout values
// use secure BaSyx defaults. The returned server is not started.
func NewConfiguredHTTPServer(ctx context.Context, cfg ServerConfig, handler http.Handler) *http.Server {
	baseCtx := ctx
	if baseCtx == nil {
		baseCtx = context.TODO()
	}
	return &http.Server{
		Addr:              ServerAddress(cfg),
		Handler:           handler,
		ReadHeaderTimeout: serverTimeout(cfg.ReadHeaderTimeoutSeconds, DefaultConfig.ServerReadHeaderTimeoutSeconds),
		ReadTimeout:       serverTimeout(cfg.ReadTimeoutSeconds, DefaultConfig.ServerReadTimeoutSeconds),
		WriteTimeout:      serverTimeout(cfg.WriteTimeoutSeconds, DefaultConfig.ServerWriteTimeoutSeconds),
		IdleTimeout:       serverTimeout(cfg.IdleTimeoutSeconds, DefaultConfig.ServerIdleTimeoutSeconds),
		BaseContext: func(net.Listener) context.Context {
			return baseCtx
		},
	}
}

// StartHTTPServer binds and starts a configured HTTP server in a managed
// goroutine. The serviceCode parameter is normalized and used as the prefix for
// coded RUNSERVER errors. The ctx parameter is propagated to request contexts
// and is later passed to Wait to trigger graceful shutdown. The returned runner
// has already bound its listener; startup failures are returned immediately.
func StartHTTPServer(ctx context.Context, serviceCode string, cfg ServerConfig, handler http.Handler) (*HTTPServerRunner, error) {
	normalizedServiceCode := normalizeServiceCode(serviceCode)
	if ctx == nil {
		return nil, fmt.Errorf("%s-RUNSERVER-CONTEXT context must not be nil", normalizedServiceCode)
	}
	server := NewConfiguredHTTPServer(ctx, cfg, handler)
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return nil, fmt.Errorf("%s-RUNSERVER-LISTEN %w", normalizedServiceCode, err)
	}
	server.Addr = listener.Addr().String()
	runner := &HTTPServerRunner{
		server:          server,
		serviceCode:     normalizedServiceCode,
		shutdownTimeout: serverTimeout(cfg.ShutdownTimeoutSeconds, DefaultConfig.ServerShutdownTimeoutSeconds),
		serveErr:        make(chan error, 1),
	}
	go runner.serve(listener)
	return runner, nil
}

// RunHTTPServer starts a configured HTTP server and blocks until it stops. The
// ctx parameter is used both as the server base context and as the cancellation
// signal for graceful shutdown. The serviceCode parameter prefixes coded listen,
// shutdown, and context errors. The cfg parameter controls address and timeout
// values; the handler parameter serves all HTTP requests.
//
// Example:
//
//	ctx, stop := common.SignalContext()
//	defer stop()
//	if err := common.RunHTTPServer(ctx, "AASR", cfg.Server, router); err != nil {
//		log.Fatal(err)
//	}
func RunHTTPServer(ctx context.Context, serviceCode string, cfg ServerConfig, handler http.Handler) error {
	runner, err := StartHTTPServer(ctx, serviceCode, cfg, handler)
	if err != nil {
		return err
	}
	return runner.Wait(ctx)
}

// Wait blocks until the server stops externally or ctx is canceled. A canceled
// ctx starts graceful shutdown with the runner's configured shutdown timeout.
// The returned error is nil on clean shutdown or includes the normalized service
// code when listen, shutdown, or context handling fails.
func (runner *HTTPServerRunner) Wait(ctx context.Context) error {
	if runner == nil || runner.server == nil {
		return fmt.Errorf("HTTP-RUNSERVER-CONTEXT server runner must not be nil")
	}
	if ctx == nil {
		return fmt.Errorf("%s-RUNSERVER-CONTEXT context must not be nil", runner.serviceCode)
	}

	if ok, err := runner.pollServeError(); ok {
		return runner.listenError(err)
	}

	select {
	case err := <-runner.serveErr:
		return runner.listenError(err)
	case <-ctx.Done():
		log.Println("Shutting down server...")
		return runner.shutdown(ctx)
	}
}

func (runner *HTTPServerRunner) serve(listener net.Listener) {
	err := runner.server.Serve(listener)
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
		if ok, serveErr := runner.pollServeError(); ok {
			return runner.listenError(serveErr)
		}
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("%s-RUNSERVER-SHUTDOWN %w", runner.serviceCode, err)
	}

	select {
	case err := <-runner.serveErr:
		return runner.listenError(err)
	case <-shutdownCtx.Done():
		return fmt.Errorf("%s-RUNSERVER-SHUTDOWN %w", runner.serviceCode, shutdownCtx.Err())
	}
}

func (runner *HTTPServerRunner) pollServeError() (bool, error) {
	select {
	case err := <-runner.serveErr:
		return true, err
	default:
		return false, nil
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
