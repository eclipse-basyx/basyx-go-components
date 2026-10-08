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
	"testing"
)

func TestContextWithConfig_RoundTrip(t *testing.T) {
	cfg := &Config{Server: ServerConfig{Port: 6004}}

	ctx := ContextWithConfig(context.Background(), cfg)
	resolved, ok := ConfigFromContext(ctx)
	if !ok {
		t.Fatalf("expected config in context")
	}
	if resolved != cfg {
		t.Fatalf("expected same config pointer from context")
	}
}

func TestUploadMaxSizeBytesFromContext(t *testing.T) {
	if actual := UploadMaxSizeBytesFromContext(t.Context()); actual != DefaultConfig.GeneralUploadMaxSizeBytes {
		t.Fatalf("expected default upload limit %d, got %d", DefaultConfig.GeneralUploadMaxSizeBytes, actual)
	}

	cfg := &Config{}
	cfg.General.UploadMaxSizeBytes = 4096
	if actual := UploadMaxSizeBytesFromContext(ContextWithConfig(t.Context(), cfg)); actual != 4096 {
		t.Fatalf("expected configured upload limit 4096, got %d", actual)
	}
}

func TestPaginationFromContext(t *testing.T) {
	expected := PaginationConfig{DefaultLimit: DefaultConfig.ServerPaginationDefaultLimit, MaxLimit: DefaultConfig.ServerPaginationMaxLimit}
	if actual := PaginationFromContext(t.Context()); actual != expected {
		t.Fatalf("expected default pagination %+v, got %+v", expected, actual)
	}

	invalid := &Config{Server: ServerConfig{Pagination: PaginationConfig{DefaultLimit: 10, MaxLimit: 5}}}
	if actual := PaginationFromContext(ContextWithConfig(t.Context(), invalid)); actual != expected {
		t.Fatalf("expected invalid pagination to fall back to %+v, got %+v", expected, actual)
	}

	cfg := &Config{Server: ServerConfig{Pagination: PaginationConfig{DefaultLimit: 5, MaxLimit: 20}}}
	ctx := ContextWithConfig(t.Context(), cfg)
	if actual := PaginationFromContext(ctx); actual != cfg.Server.Pagination {
		t.Fatalf("expected configured pagination %+v, got %+v", cfg.Server.Pagination, actual)
	}
	if actual := DefaultPageLimit(ctx); actual != 5 {
		t.Fatalf("expected default page limit 5, got %d", actual)
	}
}
