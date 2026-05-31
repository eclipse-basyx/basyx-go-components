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

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

const maxOIDCDiscoveryDocumentBytes = 1024 * 1024

func newOIDCProvider(ctx context.Context, issuer string, discoveryURL string) (*oidc.Provider, error) {
	discoveryURL = strings.TrimSpace(discoveryURL)
	if discoveryURL == "" {
		provider, err := oidc.NewProvider(ctx, issuer)
		if err != nil {
			return nil, fmt.Errorf("COMMON-OIDC-CREATEPROVIDER create OIDC provider: %w", err)
		}
		return provider, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("COMMON-OIDC-CREATEDISCOVERYREQUEST create OIDC discovery request: %w", err)
	}
	//nolint:gosec // Discovery URL is supplied by the service administrator.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("COMMON-OIDC-FETCHDISCOVERY fetch OIDC discovery metadata: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("COMMON-OIDC-FETCHDISCOVERY fetch OIDC discovery metadata: status %d", resp.StatusCode)
	}

	var config oidc.ProviderConfig
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxOIDCDiscoveryDocumentBytes))
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("COMMON-OIDC-PARSEDISCOVERY parse OIDC discovery metadata: %w", err)
	}
	if config.IssuerURL != issuer {
		return nil, fmt.Errorf("COMMON-OIDC-VALIDATEDISCOVERYISSUER discovery issuer mismatch: expected %q got %q", issuer, config.IssuerURL)
	}
	if strings.TrimSpace(config.JWKSURL) == "" {
		return nil, fmt.Errorf("COMMON-OIDC-VALIDATEDISCOVERYJWKS discovery metadata missing jwks_uri")
	}

	return config.NewProvider(ctx), nil
}
