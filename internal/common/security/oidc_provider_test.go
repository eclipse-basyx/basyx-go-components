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
*******************************************************************************/

package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewOIDCProvider_CustomDiscoveryURL(t *testing.T) {
	t.Parallel()

	_, issuer := newTestOIDCIssuer(t)
	customDiscovery := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/custom/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                issuer,
			"jwks_uri":                              issuer + "/keys",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	}))
	t.Cleanup(customDiscovery.Close)

	if _, err := newOIDCProvider(context.Background(), issuer, customDiscovery.URL+"/custom/openid-configuration"); err != nil {
		t.Fatalf("newOIDCProvider() error = %v", err)
	}
}

func TestNewOIDCProvider_CustomDiscoveryTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	t.Cleanup(server.Close)

	client := &http.Client{Timeout: 10 * time.Millisecond}
	if _, err := newOIDCProviderWithClient(context.Background(), "https://issuer.example", server.URL, client); err == nil {
		t.Fatalf("expected discovery timeout error")
	}
}

func TestNewOIDCProvider_RejectsOversizedCustomDiscovery(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat(" ", maxOIDCDiscoveryDocumentBytes+1)))
	}))
	t.Cleanup(server.Close)

	if _, err := newOIDCProvider(context.Background(), "https://issuer.example", server.URL); err == nil {
		t.Fatalf("expected oversized discovery metadata error")
	}
}

func TestNewOIDCProvider_CustomDiscoveryValidation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		metadata map[string]any
	}{
		{name: "issuer mismatch", metadata: map[string]any{"issuer": "https://unexpected.example", "jwks_uri": "https://issuer.example/keys"}},
		{name: "missing jwks", metadata: map[string]any{"issuer": "https://issuer.example"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(testCase.metadata)
			}))
			t.Cleanup(server.Close)

			if _, err := newOIDCProvider(context.Background(), "https://issuer.example", server.URL); err == nil {
				t.Fatalf("expected discovery metadata validation error")
			}
		})
	}
}
