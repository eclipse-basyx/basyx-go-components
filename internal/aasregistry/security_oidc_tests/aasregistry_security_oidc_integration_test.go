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

package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
)

const (
	oidcTestAudience  = "basyx-api"
	oidcTestKeyID     = "basyx-oidc-e2e-key"
	scpRegistryURL    = "http://localhost:6104"
	rolesRegistryURL  = "http://localhost:6204"
	securityEnvVar    = "OIDC_TEST_SECURITY_ENV"
	composeConfigPath = "docker_compose/docker_compose.yml"
)

type testIssuer struct {
	privateKey *rsa.PrivateKey
	issuerURL  string
	server     *http.Server
}

func TestOIDCSignedJWTProviders(t *testing.T) {
	t.Run("delegated scp permission", func(t *testing.T) {
		assertStatus(t, scpRegistryURL+"/shell-descriptors", signedToken(t, map[string]any{
			"scp": "access_as_user profile",
		}), http.StatusOK)
	})

	t.Run("missing delegated permission", func(t *testing.T) {
		assertStatus(t, scpRegistryURL+"/shell-descriptors", signedToken(t, map[string]any{
			"scp": "profile",
		}), http.StatusForbidden)
	})

	t.Run("wrong audience", func(t *testing.T) {
		assertStatus(t, scpRegistryURL+"/shell-descriptors", signedToken(t, map[string]any{
			"aud": "other-api",
			"scp": "access_as_user",
		}), http.StatusUnauthorized)
	})

	t.Run("wrong issuer", func(t *testing.T) {
		assertStatus(t, scpRegistryURL+"/shell-descriptors", signedToken(t, map[string]any{
			"iss": oidcIssuer.issuerURL + "/unexpected",
			"scp": "access_as_user",
		}), http.StatusUnauthorized)
	})

	t.Run("expired token", func(t *testing.T) {
		assertStatus(t, scpRegistryURL+"/shell-descriptors", signedToken(t, map[string]any{
			"exp": time.Now().Add(-time.Minute).Unix(),
			"scp": "access_as_user",
		}), http.StatusUnauthorized)
	})

	t.Run("opaque token", func(t *testing.T) {
		assertStatus(t, scpRegistryURL+"/shell-descriptors", "opaque-token", http.StatusUnauthorized)
	})

	t.Run("mapped app role", func(t *testing.T) {
		assertStatus(t, rolesRegistryURL+"/shell-descriptors", signedToken(t, map[string]any{
			"roles": []string{"admin"},
		}), http.StatusOK)
	})

	t.Run("mapped app role uses exact scalar equality", func(t *testing.T) {
		assertStatus(t, rolesRegistryURL+"/shell-descriptors", signedToken(t, map[string]any{
			"roles": []string{"administrator"},
		}), http.StatusForbidden)
	})
}

var oidcIssuer *testIssuer

func TestMain(m *testing.M) {
	issuer, err := startTestIssuer()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "COMMON-OIDC-E2E-STARTISSUER failed to start OIDC issuer: %v\n", err)
		os.Exit(1)
	}
	oidcIssuer = issuer

	securityEnv, err := writeSecurityEnvironment(issuer.issuerURL)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "COMMON-OIDC-E2E-WRITESECURITYENV failed to write security environment: %v\n", err)
		issuer.close()
		os.Exit(1)
	}
	if err := os.Setenv(securityEnvVar, securityEnv); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "COMMON-OIDC-E2E-SETSECURITYENV failed to set security environment: %v\n", err)
		issuer.close()
		_ = os.RemoveAll(securityEnv)
		os.Exit(1)
	}

	code := testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile: composeConfigPath,
		WaitForReady: func() error {
			if err := testenv.WaitHealthyURL(scpRegistryURL+"/health", 2*time.Minute); err != nil {
				return err
			}
			return testenv.WaitHealthyURL(rolesRegistryURL+"/health", 2*time.Minute)
		},
	})

	issuer.close()
	_ = os.RemoveAll(securityEnv)
	os.Exit(code)
}

func startTestIssuer() (*testIssuer, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	//nolint:gosec // The test issuer must be reachable from Docker containers through host-gateway.
	listener, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		return nil, err
	}
	issuerURL := fmt.Sprintf("http://localhost:%d", listener.Addr().(*net.TCPAddr).Port)
	handler := &oidctest.Server{
		PublicKeys: []oidctest.PublicKey{{
			PublicKey: privateKey.Public(),
			KeyID:     oidcTestKeyID,
			Algorithm: oidc.RS256,
		}},
	}
	handler.SetIssuer(issuerURL)
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		_ = server.Serve(listener)
	}()
	return &testIssuer{privateKey: privateKey, issuerURL: issuerURL, server: server}, nil
}

func (issuer *testIssuer) close() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = issuer.server.Shutdown(ctx)
}

func signedToken(t *testing.T, overrides map[string]any) string {
	t.Helper()

	claims := map[string]any{
		"iss": oidcIssuer.issuerURL,
		"sub": "oidc-e2e-subject",
		"aud": oidcTestAudience,
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	for key, value := range overrides {
		claims[key] = value
	}
	rawClaims, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("COMMON-OIDC-E2E-MARSHALCLAIMS failed to marshal claims: %v", err)
	}
	return oidctest.SignIDToken(oidcIssuer.privateKey, oidcTestKeyID, oidc.RS256, string(rawClaims))
}

func assertStatus(t *testing.T, endpoint string, token string, expected int) {
	t.Helper()

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("COMMON-OIDC-E2E-CREATEREQUEST failed to create request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	//nolint:gosec // Integration tests call fixed local registry endpoints only.
	response, err := testenv.HTTPClient().Do(request)
	if err != nil {
		t.Fatalf("COMMON-OIDC-E2E-EXECREQUEST failed to execute request: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != expected {
		t.Fatalf("status = %d, want %d", response.StatusCode, expected)
	}
}

func writeSecurityEnvironment(issuerURL string) (string, error) {
	directory, err := os.MkdirTemp(".", ".basyx-oidc-security-*")
	if err != nil {
		return "", err
	}
	directory, err = filepath.Abs(directory)
	if err != nil {
		_ = os.RemoveAll(directory)
		return "", err
	}

	files := map[string]any{
		"scp-trustlist.json": []map[string]any{{
			"issuer":   issuerURL,
			"audience": oidcTestAudience,
			"scopes":   []string{"access_as_user"},
		}},
		"roles-trustlist.json": []map[string]any{{
			"issuer":   issuerURL,
			"audience": oidcTestAudience,
			"claimMappings": []map[string]any{{
				"target":  "role",
				"mode":    "scalar",
				"sources": []string{"/roles"},
			}},
		}},
		"scp-access-rules.json":   delegatedAccessRules,
		"roles-access-rules.json": rolesAccessRules,
	}
	for name, content := range files {
		if err := writeSecurityFile(filepath.Join(directory, name), content); err != nil {
			_ = os.RemoveAll(directory)
			return "", err
		}
	}
	return directory, nil
}

func writeSecurityFile(path string, content any) error {
	var data []byte
	switch value := content.(type) {
	case string:
		data = []byte(value)
	default:
		var err error
		data, err = json.MarshalIndent(content, "", "  ")
		if err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o600)
}

const delegatedAccessRules = `{
  "AllAccessPermissionRules": {
    "DEFATTRIBUTES": [
      { "name": "anonymous", "attributes": [ { "GLOBAL": "ANONYMOUS" } ] },
      { "name": "scopes", "attributes": [ { "CLAIM": "basyx.scopes" } ] }
    ],
    "DEFOBJECTS": [
      { "name": "health", "objects": [ { "ROUTE": "/health" } ] },
      { "name": "shells", "objects": [ { "ROUTE": "/shell-descriptors" } ] }
    ],
    "DEFACLS": [
      { "name": "read-health", "acl": { "USEATTRIBUTES": "anonymous", "RIGHTS": [ "READ" ], "ACCESS": "ALLOW" } },
      { "name": "read-shells", "acl": { "USEATTRIBUTES": "scopes", "RIGHTS": [ "READ" ], "ACCESS": "ALLOW" } }
    ],
    "rules": [
      { "USEACL": "read-health", "USEOBJECTS": [ "health" ], "FORMULA": { "$boolean": true } },
      { "USEACL": "read-shells", "USEOBJECTS": [ "shells" ], "FORMULA": { "$boolean": true } }
    ]
  }
}`

const rolesAccessRules = `{
  "AllAccessPermissionRules": {
    "DEFATTRIBUTES": [
      { "name": "anonymous", "attributes": [ { "GLOBAL": "ANONYMOUS" } ] },
      { "name": "role", "attributes": [ { "CLAIM": "basyx.role" } ] }
    ],
    "DEFOBJECTS": [
      { "name": "health", "objects": [ { "ROUTE": "/health" } ] },
      { "name": "shells", "objects": [ { "ROUTE": "/shell-descriptors" } ] }
    ],
    "DEFACLS": [
      { "name": "read-health", "acl": { "USEATTRIBUTES": "anonymous", "RIGHTS": [ "READ" ], "ACCESS": "ALLOW" } },
      { "name": "read-shells", "acl": { "USEATTRIBUTES": "role", "RIGHTS": [ "READ" ], "ACCESS": "ALLOW" } }
    ],
    "rules": [
      { "USEACL": "read-health", "USEOBJECTS": [ "health" ], "FORMULA": { "$boolean": true } },
      {
        "USEACL": "read-shells",
        "USEOBJECTS": [ "shells" ],
        "FORMULA": { "$eq": [ { "$attribute": { "CLAIM": "basyx.role" } }, { "$strVal": "admin" } ] }
      }
    ]
  }
}`
