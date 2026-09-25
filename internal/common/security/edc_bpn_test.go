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
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestSecurityMiddlewareInjectsEdcBpnBeforeABAC(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		header  string
		want    string
	}{
		{name: "disabled preserves token claim", header: "BPN_HEADER", want: "BPN_TOKEN"},
		{name: "enabled overrides token claim", enabled: true, header: " BPN_HEADER ", want: "BPN_HEADER"},
		{name: "missing header preserves token claim", enabled: true, want: "BPN_TOKEN"},
		{name: "blank header preserves token claim", enabled: true, header: "  ", want: "BPN_TOKEN"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := chi.NewRouter()
			oidc := func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					claims := Claims{"Edc-Bpn": "BPN_TOKEN", "sub": "test-user"}
					next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ClaimsKey, claims)))
				})
			}
			abac := func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					require.Equal(t, test.want, FromContext(r)["Edc-Bpn"])
					require.Equal(t, "test-user", FromContext(r)["sub"])
					next.ServeHTTP(w, r)
				})
			}
			applySecurityMiddleware(router, oidc, abac, test.enabled)
			router.Get("/*", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
			for _, path := range []string{"/submodels", "/shell-descriptors", "/lookup/shells", "/shells", "/concept-descriptions"} {
				ctx := common.ContextWithConfig(t.Context(), &common.Config{ABAC: common.ABACConfig{Enabled: false}})
				request := httptest.NewRequestWithContext(ctx, http.MethodGet, path, nil)
				request.Header.Set("Edc-Bpn", test.header)
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, request)
				require.Equal(t, http.StatusNoContent, recorder.Code)
			}
		})
	}
}

func TestSecuritySetupHonorsEdcBpnHeaderFlag(t *testing.T) {
	for _, useProvider := range []bool{false, true} {
		for _, abacEnabled := range []bool{false, true} {
			for _, injectionEnabled := range []bool{false, true} {
				t.Run(fmt.Sprintf("provider=%t/abac=%t/injection=%t", useProvider, abacEnabled, injectionEnabled), func(t *testing.T) {
					testSecuritySetupEdcBpn(t, useProvider, abacEnabled, injectionEnabled)
				})
			}
		}
	}
}

func testSecuritySetupEdcBpn(t *testing.T, useProvider, abacEnabled, injectionEnabled bool) {
	t.Helper()
	policy := []byte(`{"AllAccessPermissionRules":{"rules":[{
		"ACL":{"ACCESS":"ALLOW","RIGHTS":["ALL"],"ATTRIBUTES":[{"CLAIM":"Edc-Bpn"}]},
		"OBJECTS":[{"ROUTE":"/*"}],
		"FORMULA":{"$eq":[{"$attribute":{"CLAIM":"Edc-Bpn"}},{"$strVal":"BPN_ALLOWED"}]}
	}]}}`)
	dir := t.TempDir()
	cfg := &common.Config{
		General: common.GeneralConfig{EnableCustomMiddlewareHeaderInjection: injectionEnabled},
		ABAC:    common.ABACConfig{Enabled: abacEnabled, ModelPath: filepath.Join(dir, "policy.json")},
		OIDC:    common.OIDCConfig{TrustlistPath: filepath.Join(dir, "trustlist.json")},
	}
	require.NoError(t, os.WriteFile(cfg.ABAC.ModelPath, policy, 0o600))
	require.NoError(t, os.WriteFile(cfg.OIDC.TrustlistPath, []byte(`[]`), 0o600))
	router := chi.NewRouter()
	ctx := common.ContextWithConfig(t.Context(), cfg)
	if useProvider {
		model, err := ParseAccessModel(policy, router, "")
		require.NoError(t, err)
		require.NoError(t, SetupSecurityWithAccessModelProvider(ctx, cfg, router, edcBpnModelProvider{model: model}))
	} else {
		require.NoError(t, SetupSecurity(ctx, cfg, router))
	}
	handler := func(w http.ResponseWriter, r *http.Request) {
		if !abacEnabled {
			require.Nil(t, FromContext(r))
		}
		w.WriteHeader(http.StatusNoContent)
	}
	router.Get("/submodels", handler)
	router.Get("/shells", handler)
	for _, header := range []string{"BPN_ALLOWED", "BPN_DENIED", ""} {
		for _, path := range []string{"/submodels", "/shells"} {
			request := httptest.NewRequestWithContext(ctx, http.MethodGet, path, nil)
			request.Header.Set("Edc-Bpn", header)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			want := http.StatusForbidden
			if !abacEnabled || injectionEnabled && header == "BPN_ALLOWED" {
				want = http.StatusNoContent
			}
			require.Equal(t, want, recorder.Code, recorder.Body.String())
		}
	}
}

type edcBpnModelProvider struct {
	model *AccessModel
}

func (p edcBpnModelProvider) ActiveAccessModel() *AccessModel {
	return p.model
}

func TestEdcBpnHeaderMiddlewareDoesNotCreateClaimsWithoutAuthenticationMiddleware(t *testing.T) {
	handler := EdcBpnHeaderMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Nil(t, FromContext(r))
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := common.ContextWithConfig(t.Context(), &common.Config{ABAC: common.ABACConfig{Enabled: false}})
	request := httptest.NewRequestWithContext(ctx, http.MethodGet, "/submodels", nil)
	request.Header.Set("Edc-Bpn", "BPN_HEADER")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusNoContent, recorder.Code)
}
