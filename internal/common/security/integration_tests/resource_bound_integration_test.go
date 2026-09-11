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

package integration_tests

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
	aasapi "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/api"
	aasdb "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	smapi "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/api"
	smdb "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	aasopenapi "github.com/eclipse-basyx/basyx-go-components/pkg/aasrepositoryapi/go"
	smopenapi "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

type accessClient struct {
	t       *testing.T
	db      *sql.DB
	scope   string
	server  *httptest.Server
	restart func() accessClient
}

func (c accessClient) request(method, path, user string, body any, etag string, status int) ([]byte, string) {
	c.t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		require.NoError(c.t, err)
	}
	data, headers := c.requestRaw(method, path, user, raw, etag, status)
	return data, headers.Get("ETag")
}

func (c accessClient) requestRaw(method, path, user string, raw []byte, etag string, status int) ([]byte, http.Header) {
	c.t.Helper()
	req, err := http.NewRequestWithContext(c.t.Context(), method, c.server.URL+path, bytes.NewReader(raw))
	require.NoError(c.t, err)
	req.Header.Set("X-Test-Subject", user)
	req.Header.Set("Content-Type", "application/json")
	if etag != "" {
		req.Header.Set("If-Match", etag)
	}
	response, err := c.server.Client().Do(req)
	require.NoError(c.t, err)
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(response.Body)
	require.NoError(c.t, err)
	require.Equal(c.t, status, response.StatusCode, "%s %s: %s", method, path, data)
	return data, response.Header
}
func (c accessClient) mutate(method, path, user string, body any, status int) []byte {
	c.t.Helper()
	index := bytes.Index([]byte(path), []byte("/$access"))
	_, etag := c.request(http.MethodGet, path[:index+len("/$access")], user, nil, "", http.StatusOK)
	data, _ := c.request(method, path, user, body, etag, status)
	return data
}
func principal(subject string) map[string]string {
	return map[string]string{"issuer": "https://rebac.test", "subject": subject}
}
func grant(subject string, rights ...string) map[string]any {
	return map[string]any{"principal": principal(subject), "rights": rights}
}
func rule(subject string, rights ...string) map[string]any {
	return map[string]any{"ACL": map[string]any{"ATTRIBUTES": []any{map[string]string{"CLAIM": "sub"}}, "RIGHTS": rights, "ACCESS": "ALLOW"}, "FORMULA": map[string]any{"$eq": []any{map[string]any{"$attribute": map[string]string{"CLAIM": "sub"}}, map[string]string{"$strVal": subject}}}}
}
func policy(resource map[string]string, rules ...any) map[string]any {
	if rules == nil {
		rules = []any{}
	}
	return map[string]any{"RESOURCE": resource, "rules": rules}
}
func encoded(id string) string { return base64.RawURLEncoding.EncodeToString([]byte(id)) }

type fallbackProvider struct{ model *auth.AccessModel }

func (p fallbackProvider) ActiveAccessModel() *auth.AccessModel { return p.model }

func newAccessClient(t *testing.T) accessClient { return newAccessClientWithBasePath(t, "") }

func newAccessClientWithBasePath(t *testing.T, basePath string) accessClient {
	t.Helper()
	dsn := os.Getenv("BASYX_REBAC_TEST_DSN")
	if dsn == "" {
		t.Skip("BASYX_REBAC_TEST_DSN must point to a database migrated to the current schema")
	}
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.PingContext(t.Context()))
	directory := t.TempDir()
	trust := filepath.Join(directory, "trustlist.json")
	require.NoError(t, os.WriteFile(trust, []byte("[]"), 0600))
	cfg := &common.Config{Security: common.SecurityConfig{AuthorizationMode: common.AuthorizationResourceBoundFirst}, ReBAC: common.ReBACConfig{PolicyScope: uuid.NewString(), GroupsClaim: "groups", BootstrapOwner: common.AccessPrincipal{Issuer: "https://rebac.test", Subject: "admin"}}, OIDC: common.OIDCConfig{TrustlistPath: trust}}
	cfg.Server.StrictVerification = "off"
	cfg.Server.ContextPath = basePath
	return startAccessClient(t, cfg, db)
}

func startAccessClient(t *testing.T, cfg *common.Config, db *sql.DB) accessClient {
	t.Helper()
	basePath := cfg.Server.ContextPath
	ctx := common.ContextWithConfig(t.Context(), cfg)
	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(common.ContextWithConfig(r.Context(), cfg)))
		})
	})
	allRoutes := []any{map[string]string{"ROUTE": "/*"}}
	allResources := []any{
		map[string]string{"IDENTIFIABLE": "$aas(\"*\")"}, map[string]string{"IDENTIFIABLE": "$sm(\"*\")"},
		map[string]string{"IDENTIFIABLE": "$cd(\"*\")"}, map[string]string{"DESCRIPTOR": "$aasdesc(\"*\")"},
		map[string]string{"DESCRIPTOR": "$smdesc(\"*\")"},
	}
	adminRoutes := rule("admin", "ALL")
	adminRoutes["OBJECTS"] = allRoutes
	adminResources := rule("admin", "ALL")
	adminResources["OBJECTS"] = allResources
	auditorRoutes := rule("auditor", "ALL")
	auditorRoutes["OBJECTS"] = allRoutes
	auditorResources := rule("auditor", "ALL")
	auditorResources["OBJECTS"] = allResources
	creatorCollections := rule("creator", "CREATE", "READ")
	creatorCollections["OBJECTS"] = []any{map[string]string{"ROUTE": "/shells"}, map[string]string{"ROUTE": "/submodels"}, map[string]string{"ROUTE": "/submodels/*"}}
	readerCollections := rule("reader", "READ")
	readerCollections["OBJECTS"] = []any{map[string]string{"ROUTE": "/submodels"}}
	fallbackData, err := json.Marshal(map[string]any{"AllAccessPermissionRules": map[string]any{"rules": []any{
		adminRoutes, adminResources, auditorRoutes, auditorResources, creatorCollections, readerCollections,
	}}})
	require.NoError(t, err)
	materializedFallback, err := auth.MaterializeABACPolicy(fallbackData, router, basePath)
	require.NoError(t, err)
	fallback, err := auth.AccessModelFromMaterializedRules(materializedFallback.PolicyID, materializedFallback.Rules, router, basePath)
	require.NoError(t, err)
	require.NoError(t, auth.SetupResourceBoundSecurity(ctx, cfg, router, db, fallbackProvider{fallback}, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Test-Subject") == "" {
				next.ServeHTTP(w, r)
				return
			}
			claims := auth.Claims{"iss": "https://rebac.test", "sub": r.Header.Get("X-Test-Subject")}
			if r.Header.Get("X-Test-Subject") == "group-member" {
				claims["groups"] = []string{"/inspectors", "/bootstrap-owners"}
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), auth.ClaimsKey, claims)))
		})
	}))
	submodels, err := smdb.NewSubmodelDatabaseFromDB(db, nil, "off")
	require.NoError(t, err)
	shells, err := aasdb.NewAssetAdministrationShellDatabaseFromDB(db, "off")
	require.NoError(t, err)
	smController := smopenapi.NewSubmodelRepositoryAPIAPIController(smapi.NewSubmodelRepositoryAPIAPIService(ctx, *submodels), "", "off")
	aasController := aasopenapi.NewAssetAdministrationShellRepositoryAPIAPIController(aasapi.NewAssetAdministrationShellRepositoryAPIAPIService(ctx, shells, submodels, true), "", "off")
	for _, route := range smController.Routes() {
		router.Method(route.Method, route.Pattern, route.HandlerFunc)
	}
	for _, route := range aasController.Routes() {
		router.Method(route.Method, route.Pattern, route.HandlerFunc)
	}
	router.Method(http.MethodPost, "/lookup/shells/{aasIdentifier}", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	var handler http.Handler = router
	if basePath != "" {
		root := chi.NewRouter()
		root.Mount(basePath, router)
		handler = root
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return accessClient{t: t, db: db, scope: cfg.ReBAC.PolicyScope, server: server, restart: func() accessClient { return startAccessClient(t, cfg, db) }}
}

func TestResourceBoundDiscoveryPostUsesABACCreationPolicy(t *testing.T) {
	c := newAccessClient(t)
	path := "/lookup/shells/" + encoded("urn:rebac:discovery:"+uuid.NewString())
	c.request(http.MethodPost, path, "admin", []any{map[string]string{"name": "serial", "value": "1"}}, "", http.StatusCreated)
	c.request(http.MethodPost, path, "outsider", []any{map[string]string{"name": "serial", "value": "1"}}, "", http.StatusForbidden)
}

func TestPersistedCollectionPoliciesAreIgnored(t *testing.T) {
	c := newAccessClient(t)
	raw, err := json.Marshal(policy(map[string]string{"ROUTE": "/shells"}, rule("outsider", "ALL")))
	require.NoError(t, err)
	insert := goqu.Dialect("postgres").Insert("rebac_access").Rows(goqu.Record{"scope": c.scope, "collection": "/shells", "policy": raw}).Prepared(true)
	query, args, err := insert.ToSQL()
	require.NoError(t, err)
	_, err = c.db.ExecContext(t.Context(), query, args...)
	require.NoError(t, err)

	c.request(http.MethodGet, "/shells", "outsider", nil, "", http.StatusForbidden)
	c.request(http.MethodGet, "/shells/$access", "outsider", nil, "", http.StatusNotFound)
	shell := map[string]any{
		"modelType":        "AssetAdministrationShell",
		"id":               "urn:ignored-collection-policy:" + uuid.NewString(),
		"assetInformation": map[string]string{"assetKind": "Instance"},
	}
	c.request(http.MethodPost, "/shells", "outsider", shell, "", http.StatusForbidden)
}

func TestResourceBoundAccessLifecycle(t *testing.T) {
	client := newAccessClient(t)
	id := "urn:rebac:" + uuid.NewString()
	path := "/submodels/" + encoded(id)
	access := path + "/$access"
	model := map[string]any{"modelType": "Submodel", "id": id, "idShort": "Bridge", "submodelElements": []any{
		map[string]any{"modelType": "Property", "idShort": "Public", "valueType": "xs:string", "value": "visible"},
		map[string]any{"modelType": "Property", "idShort": "Private", "valueType": "xs:string", "value": "secret"},
	}}
	client.request("POST", "/submodels", "admin", model, "", 201)
	data, _ := client.request("GET", access, "admin", nil, "", 200)
	require.Contains(t, string(data), `"subject":"admin"`)
	client.request("GET", access, "outsider", nil, "", 403)
	client.mutate("POST", access+"/grants", "admin", grant("reader", "READ"), 409)
	client.request("PUT", access+"/policy", "admin", policy(map[string]string{"IDENTIFIABLE": "$sm(" + strconvQuote(id) + ")"}), "", 428)
	client.mutate("PUT", access+"/policy", "admin", policy(map[string]string{"IDENTIFIABLE": "$sm(" + strconvQuote(id) + ")"}), 200)
	client.request("GET", path, "admin", nil, "", 200)
	client.mutate("POST", access+"/grants", "admin", grant("admin", "ALL"), 201)
	client.mutate("POST", access+"/grants", "admin", grant("reader", "READ"), 201)
	client.request("GET", path, "reader", nil, "", 200)
	child := path + "/submodel-elements/Private/$access"
	client.mutate("PUT", child+"/policy", "admin", policy(map[string]string{"REFERABLE": "$sme(" + strconvQuote(id) + ").Private"}, rule("manufacturer", "READ")), 200)
	data, _ = client.request("GET", path, "reader", nil, "", 200)
	require.NotContains(t, string(data), "secret")
	require.Contains(t, string(data), "visible")
	client.request("GET", path+"/submodel-elements/Private", "reader", nil, "", 403)
	client.request("GET", path+"/submodel-elements/Private", "manufacturer", nil, "", 200)
	client.request("GET", path+"/submodel-elements/Private", "auditor", nil, "", 200)
	client.mutate("PUT", access+"/managers", "admin", []any{principal("manager")}, 200)
	client.request("GET", access, "manager", nil, "", 200)
	client.mutate("PUT", access+"/owners", "manager", []any{principal("manager")}, 403)
	client.request("GET", child, "manager", nil, "", 403)
	client.mutate("PUT", access+"/owners", "admin", []any{}, 400)
	_, etag := client.request("GET", access, "admin", nil, "", 200)
	client.mutate("POST", access+"/grants", "admin", grant("other", "READ"), 201)
	client.request("PUT", access+"/managers", "admin", []any{}, etag, 412)
	client.mutate("DELETE", child+"/policy", "admin", nil, 204)
	client.request("GET", child, "manager", nil, "", 200)
	client.request("GET", path+"/$history", "admin", nil, "", 501)
	client.request("DELETE", path, "admin", nil, "", 204)
	client.request("GET", access, "admin", nil, "", 404)
}
func strconvQuote(value string) string { raw, _ := json.Marshal(value); return string(raw) }

func TestResourceBoundCreatorAndRecreation(t *testing.T) {
	c := newAccessClient(t)
	id := "urn:creator:" + uuid.NewString()
	path := "/submodels/" + encoded(id)
	access := path + "/$access"
	model := map[string]any{"modelType": "Submodel", "id": id, "idShort": "Created"}
	c.request("PUT", path, "creator", model, "", 201)
	c.request("GET", access, "creator", nil, "", 200)
	c.request("GET", path, "creator", nil, "", 200)
	c.request("DELETE", path, "creator", nil, "", 204)
	c.request("POST", "/submodels", "admin", model, "", 201)
	c.request("GET", access, "creator", nil, "", 403)
	c.request("DELETE", path, "admin", nil, "", 204)
}

func TestResourceBoundNestedAliasesAndAmbiguousInheritance(t *testing.T) {
	c := newAccessClient(t)
	smID := "urn:nested:" + uuid.NewString()
	smPath := "/submodels/" + encoded(smID)
	c.request("POST", "/submodels", "admin", map[string]any{"modelType": "Submodel", "id": smID, "idShort": "Shared"}, "", 201)
	var shellPaths []string
	for _, reader := range []string{"first", "second"} {
		id := "urn:aas:" + uuid.NewString()
		path := "/shells/" + encoded(id)
		shellPaths = append(shellPaths, path)
		shell := map[string]any{"modelType": "AssetAdministrationShell", "id": id, "idShort": "Bridge", "assetInformation": map[string]any{"assetKind": "Instance", "globalAssetId": id + ":asset"}, "submodels": []any{map[string]any{"type": "ModelReference", "keys": []any{map[string]string{"type": "Submodel", "value": smID}}}}}
		c.request("POST", "/shells", "admin", shell, "", 201)
		c.mutate("PUT", path+"/$access/policy", "admin", policy(map[string]string{"IDENTIFIABLE": "$aas(" + strconvQuote(id) + ")"}, rule("admin", "ALL"), rule(reader, "READ")), 200)
	}
	c.request("GET", smPath, "first", nil, "", 403)
	c.request("GET", shellPaths[0]+smPath, "first", nil, "", 200)
	c.request("GET", shellPaths[1]+smPath, "first", nil, "", 403)
	_, directTag := c.request("GET", smPath+"/$access", "admin", nil, "", 200)
	_, nestedTag := c.request("GET", shellPaths[0]+smPath+"/$access", "admin", nil, "", 200)
	require.NotEqual(t, directTag, nestedTag)
	wrong := "/shells/" + encoded("urn:missing") + smPath + "/$access"
	c.request("GET", wrong, "admin", nil, "", 404)
	c.mutate("PUT", smPath+"/$access/policy", "admin", policy(map[string]string{"IDENTIFIABLE": "$sm(" + strconvQuote(smID) + ")"}, rule("first", "READ"), rule("admin", "ALL")), 200)
	c.request("GET", smPath, "first", nil, "", 200)
	c.request("DELETE", smPath, "admin", nil, "", 204)
	for _, path := range shellPaths {
		c.request("DELETE", path, "admin", nil, "", 204)
	}
}

func TestResourceBoundAndABACPermissionsAreCombined(t *testing.T) {
	c := newAccessClient(t)
	id := "urn:filter:" + uuid.NewString()
	path := "/submodels/" + encoded(id)
	access := path + "/$access"
	c.request("POST", "/submodels", "admin", map[string]any{"modelType": "Submodel", "id": id, "idShort": "Filter", "submodelElements": []any{map[string]any{"modelType": "Property", "idShort": "Cost", "valueType": "xs:string", "value": "hidden"}}}, "", 201)
	filtered := rule("auditor", "READ")
	filtered["FILTER"] = map[string]any{"FRAGMENT": "$sme#value", "CONDITION": map[string]any{"$boolean": false}}
	c.mutate("PUT", access+"/policy", "admin", policy(map[string]string{"IDENTIFIABLE": "$sm(" + strconvQuote(id) + ")"}, filtered), 200)
	data, _ := c.request("GET", path, "auditor", nil, "", 200)
	require.Contains(t, string(data), "hidden")
	conditional := rule("auditor", "READ")
	conditional["FORMULA"] = map[string]any{"$and": []any{conditional["FORMULA"], map[string]any{"$eq": []any{map[string]any{"$field": "$sm#idShort"}, map[string]any{"$strVal": "Different"}}}}}
	c.mutate("PUT", access+"/policy", "admin", policy(map[string]string{"IDENTIFIABLE": "$sm(" + strconvQuote(id) + ")"}, conditional), 200)
	data, _ = c.request("GET", path, "auditor", nil, "", 200)
	require.Contains(t, string(data), "hidden")
	c.mutate("DELETE", access+"/policy", "admin", nil, 204)
	c.request("DELETE", path, "admin", nil, "", 204)
}

func TestResourceBoundGroupGrantAndOwnerAdministration(t *testing.T) {
	c := newAccessClient(t)
	id := "urn:group-access:" + uuid.NewString()
	path := "/submodels/" + encoded(id)
	access := path + "/$access"
	c.request(http.MethodPost, "/submodels", "admin", map[string]any{"modelType": "Submodel", "id": id, "idShort": "GroupAccess"}, "", http.StatusCreated)
	c.mutate(http.MethodPut, access+"/policy", "admin", policy(map[string]string{"IDENTIFIABLE": "$sm(" + strconvQuote(id) + ")"}), http.StatusOK)
	c.mutate(http.MethodPost, access+"/grants", "admin", map[string]any{
		"principal": map[string]string{"type": "group", "issuer": "https://rebac.test", "subject": "/inspectors"},
		"rights":    []string{"READ"},
	}, http.StatusCreated)
	c.request(http.MethodGet, path, "group-member", nil, "", http.StatusOK)
	c.request(http.MethodGet, path, "group-outsider", nil, "", http.StatusForbidden)
	c.mutate(http.MethodPut, access+"/managers", "admin", []any{
		map[string]string{"type": "group", "issuer": "https://rebac.test", "subject": "/inspectors"},
	}, http.StatusOK)
	c.request(http.MethodGet, access, "group-member", nil, "", http.StatusOK)
	c.mutate(http.MethodPut, access+"/owners", "admin", []any{
		principal("admin"),
		map[string]string{"type": "group", "issuer": "https://rebac.test", "subject": "/inspectors"},
	}, http.StatusOK)
	c.mutate(http.MethodPut, access+"/managers", "group-member", []any{}, http.StatusOK)
	c.request(http.MethodGet, access, "group-member", nil, "", http.StatusOK)
	c.request(http.MethodDelete, path, "admin", nil, "", http.StatusNoContent)
}

func TestResourceBoundShareLinkLifecycle(t *testing.T) {
	c := newAccessClient(t)
	id := "urn:share-link:" + uuid.NewString()
	path := "/submodels/" + encoded(id)
	access := path + "/$access"
	c.request(http.MethodPost, "/submodels", "admin", map[string]any{"modelType": "Submodel", "id": id, "idShort": "Shared"}, "", http.StatusCreated)
	c.mutate(http.MethodPost, access+"/share-links", "admin", map[string]any{"rights": []string{"READ"}}, http.StatusConflict)
	c.mutate(http.MethodPut, access+"/policy", "admin", policy(map[string]string{"IDENTIFIABLE": "$sm(" + strconvQuote(id) + ")"}), http.StatusOK)
	c.mutate(http.MethodPost, access+"/share-links", "admin", map[string]any{"rights": []string{"READ"}, "expiresInSeconds": int64(^uint64(0) >> 1)}, http.StatusBadRequest)

	_, etag := c.request(http.MethodGet, access, "admin", nil, "", http.StatusOK)
	created, headers := c.requestRaw(http.MethodPost, access+"/share-links", "admin", mustMarshal(t, map[string]any{"rights": []string{"READ"}, "expiresInSeconds": 3600}), etag, http.StatusCreated)
	require.Equal(t, "no-store", headers.Get("Cache-Control"))
	require.Equal(t, "no-referrer", headers.Get("Referrer-Policy"))
	require.Equal(t, etag, headers.Get("ETag"))
	var invitation struct {
		ID        string `json:"id"`
		ShareLink string `json:"shareLink"`
	}
	require.NoError(t, json.Unmarshal(created, &invitation))
	require.NotEmpty(t, invitation.ID)
	parts := strings.Split(invitation.ShareLink, "token=")
	require.Len(t, parts, 2)
	token := parts[1]
	_, headers = c.requestRaw(http.MethodPost, "/security/rebac/share-links/redeem", "share-recipient", mustMarshal(t, map[string]string{"token": token}), "", http.StatusCreated)
	require.Equal(t, "no-store", headers.Get("Cache-Control"))
	require.Equal(t, "no-referrer", headers.Get("Referrer-Policy"))
	c.request(http.MethodGet, path, "share-recipient", nil, "", http.StatusOK)
	c.request(http.MethodPost, "/security/rebac/share-links/redeem", "share-recipient", map[string]string{"token": token}, "", http.StatusNotFound)

	revoked := c.mutate(http.MethodPost, access+"/share-links", "admin", map[string]any{"rights": []string{"READ"}}, http.StatusCreated)
	require.NoError(t, json.Unmarshal(revoked, &invitation))
	parts = strings.Split(invitation.ShareLink, "token=")
	c.mutate(http.MethodDelete, access+"/share-links/"+invitation.ID, "admin", nil, http.StatusNoContent)
	c.request(http.MethodPost, "/security/rebac/share-links/redeem", "other-recipient", map[string]string{"token": parts[1]}, "", http.StatusNotFound)

	bound := c.mutate(http.MethodPost, access+"/share-links", "admin", map[string]any{"rights": []string{"READ"}, "expectedPrincipal": principal("intended-recipient")}, http.StatusCreated)
	require.NoError(t, json.Unmarshal(bound, &invitation))
	parts = strings.Split(invitation.ShareLink, "token=")
	c.request(http.MethodPost, "/security/rebac/share-links/redeem", "wrong-recipient", map[string]string{"token": parts[1]}, "", http.StatusNotFound)
	c.request(http.MethodPost, "/security/rebac/share-links/redeem", "intended-recipient", map[string]string{"token": parts[1]}, "", http.StatusCreated)

	expired := c.mutate(http.MethodPost, access+"/share-links", "admin", map[string]any{"rights": []string{"READ"}}, http.StatusCreated)
	require.NoError(t, json.Unmarshal(expired, &invitation))
	parts = strings.Split(invitation.ShareLink, "token=")
	expire := goqu.Dialect("postgres").Update("rebac_share_invitation").Set(goqu.Record{
		"created_at": goqu.L("CURRENT_TIMESTAMP - INTERVAL '2 seconds'"),
		"expires_at": goqu.L("CURRENT_TIMESTAMP - INTERVAL '1 second'"),
	}).Where(goqu.Ex{"id": invitation.ID}).Prepared(true)
	query, args, err := expire.ToSQL()
	require.NoError(t, err)
	_, err = c.db.ExecContext(t.Context(), query, args...)
	require.NoError(t, err)
	c.request(http.MethodPost, "/security/rebac/share-links/redeem", "other-recipient", map[string]string{"token": parts[1]}, "", http.StatusNotFound)

	stale := c.mutate(http.MethodPost, access+"/share-links", "admin", map[string]any{"rights": []string{"READ"}}, http.StatusCreated)
	require.NoError(t, json.Unmarshal(stale, &invitation))
	parts = strings.Split(invitation.ShareLink, "token=")
	c.mutate(http.MethodPost, access+"/grants", "admin", grant("revision-change", "VIEW"), http.StatusCreated)
	c.request(http.MethodPost, "/security/rebac/share-links/redeem", "other-recipient", map[string]string{"token": parts[1]}, "", http.StatusNotFound)
}

func TestResourceBoundBootstrapGroupOwnsExistingResources(t *testing.T) {
	c := newAccessClient(t)
	id := "urn:bootstrap-group:" + uuid.NewString()
	path := "/shells/" + encoded(id)
	shell := map[string]any{"modelType": "AssetAdministrationShell", "id": id, "idShort": "BootstrapGroup", "assetInformation": map[string]string{"assetKind": "Instance"}}
	c.request(http.MethodPost, "/shells", "admin", shell, "", http.StatusCreated)

	trust := filepath.Join(t.TempDir(), "trustlist.json")
	require.NoError(t, os.WriteFile(trust, []byte("[]"), 0600))
	cfg := &common.Config{
		Security: common.SecurityConfig{AuthorizationMode: common.AuthorizationResourceBoundFirst},
		ReBAC: common.ReBACConfig{
			PolicyScope:    uuid.NewString(),
			GroupsClaim:    "groups",
			BootstrapOwner: common.AccessPrincipal{Type: common.AccessPrincipalGroup, Issuer: "https://rebac.test", Subject: "/bootstrap-owners"},
		},
		OIDC: common.OIDCConfig{TrustlistPath: trust},
	}
	cfg.Server.StrictVerification = "off"
	groupClient := startAccessClient(t, cfg, c.db)
	groupClient.request(http.MethodGet, path, "group-member", nil, "", http.StatusOK)
	groupClient.request(http.MethodGet, path+"/$access", "group-member", nil, "", http.StatusOK)
	groupClient.request(http.MethodDelete, path, "group-member", nil, "", http.StatusNoContent)
}

func TestAdminCreatedAASIsHiddenWithoutReBACOrABACRead(t *testing.T) {
	c := newAccessClient(t)
	adminID := "urn:admin-owned:" + uuid.NewString()
	adminPath := "/shells/" + encoded(adminID)
	shell := map[string]any{
		"modelType":        "AssetAdministrationShell",
		"id":               adminID,
		"idShort":          "AdminOwned",
		"assetInformation": map[string]string{"assetKind": "Instance"},
	}
	c.request(http.MethodPost, "/shells", "admin", shell, "", http.StatusCreated)
	c.request(http.MethodGet, adminPath, "creator", nil, "", http.StatusForbidden)

	creatorID := "urn:creator-owned:" + uuid.NewString()
	creatorPath := "/shells/" + encoded(creatorID)
	shell["id"] = creatorID
	shell["idShort"] = "CreatorOwned"
	c.request(http.MethodPost, "/shells", "creator", shell, "", http.StatusCreated)
	c.request(http.MethodGet, creatorPath, "creator", nil, "", http.StatusOK)

	data, _ := c.request(http.MethodGet, "/shells", "creator", nil, "", http.StatusOK)
	require.Contains(t, string(data), creatorID)
	require.NotContains(t, string(data), adminID)
	data, _ = c.request(http.MethodGet, "/shells", "auditor", nil, "", http.StatusOK)
	require.Contains(t, string(data), creatorID)
	require.Contains(t, string(data), adminID)

	c.request(http.MethodDelete, creatorPath, "creator", nil, "", http.StatusNoContent)
	c.request(http.MethodDelete, adminPath, "admin", nil, "", http.StatusNoContent)
}

func TestAASUpdateCapabilitiesUseEffectiveAuthorization(t *testing.T) {
	c := newAccessClient(t)

	id := "urn:capabilities:" + uuid.NewString()
	path := "/shells/" + encoded(id)
	access := path + "/$access"
	capabilities := access + "/capabilities"
	shell := map[string]any{
		"modelType":        "AssetAdministrationShell",
		"id":               id,
		"idShort":          "CapabilityTarget",
		"assetInformation": map[string]string{"assetKind": "Instance"},
	}
	c.request(http.MethodPost, "/shells", "admin", shell, "", http.StatusCreated)
	filteredUpdate := rule("filtered", "UPDATE")
	filteredUpdate["FORMULA"] = map[string]any{"$and": []any{
		filteredUpdate["FORMULA"],
		map[string]any{"$eq": []any{map[string]any{"$field": "$aas#idShort"}, map[string]string{"$strVal": "Different"}}},
	}}
	binding := map[string]string{"IDENTIFIABLE": "$aas(" + strconvQuote(id) + ")"}
	c.mutate(http.MethodPut, access+"/policy", "admin", policy(binding, rule("admin", "ALL"), rule("builder", "READ"), rule("filtered", "READ"), filteredUpdate), http.StatusOK)

	assertAASCapability(t, c, capabilities, "builder", false)
	c.request(http.MethodGet, access, "builder", nil, "", http.StatusForbidden)
	shell["idShort"] = "ForbiddenUpdate"
	c.request(http.MethodPut, path, "builder", shell, "", http.StatusForbidden)
	shell["idShort"] = "CapabilityTarget"

	c.mutate(http.MethodPost, access+"/grants", "admin", grant("builder", "UPDATE"), http.StatusCreated)
	assertAASCapability(t, c, capabilities, "builder", true)
	assertAASCapability(t, c, capabilities, "auditor", true)
	assertAASCapability(t, c, capabilities, "filtered", false)

	filteredUpdate = rule("filtered", "UPDATE")
	filteredUpdate["FORMULA"] = map[string]any{"$and": []any{
		filteredUpdate["FORMULA"],
		map[string]any{"$eq": []any{map[string]any{"$field": "$aas#idShort"}, map[string]string{"$strVal": "CapabilityTarget"}}},
	}}
	c.mutate(http.MethodPut, access+"/policy", "admin", policy(binding, rule("admin", "ALL"), rule("builder", "READ", "UPDATE"), rule("filtered", "READ"), filteredUpdate), http.StatusOK)
	assertAASCapability(t, c, capabilities, "filtered", true)

	c.mutate(http.MethodPut, access+"/managers", "admin", []any{principal("manager")}, http.StatusOK)
	assertAASCapability(t, c, capabilities, "manager", true)
	c.request(http.MethodDelete, path, "admin", nil, "", http.StatusNoContent)
}

func TestAASUpdateCapabilitiesRespectOwnershipInheritanceAndHaveNoSideEffects(t *testing.T) {
	c := newAccessClient(t)

	ownedID := "urn:capabilities:owned:" + uuid.NewString()
	ownedPath := "/shells/" + encoded(ownedID)
	ownedAccess := ownedPath + "/$access"
	shell := map[string]any{
		"modelType":        "AssetAdministrationShell",
		"id":               ownedID,
		"idShort":          "OwnedCapabilityTarget",
		"assetInformation": map[string]string{"assetKind": "Instance"},
	}
	c.request(http.MethodPost, "/shells", "creator", shell, "", http.StatusCreated)
	resourceBefore, _ := c.request(http.MethodGet, ownedPath, "creator", nil, "", http.StatusOK)
	before, beforeETag := c.request(http.MethodGet, ownedAccess, "creator", nil, "", http.StatusOK)
	capabilityBody, headers := c.requestRaw(http.MethodGet, ownedAccess+"/capabilities", "creator", nil, "", http.StatusOK)
	require.JSONEq(t, `{"canUpdate":true}`, string(capabilityBody))
	require.Equal(t, "no-store", headers.Get("Cache-Control"))
	require.Empty(t, headers.Get("ETag"))
	after, afterETag := c.request(http.MethodGet, ownedAccess, "creator", nil, "", http.StatusOK)
	resourceAfter, _ := c.request(http.MethodGet, ownedPath, "creator", nil, "", http.StatusOK)
	require.JSONEq(t, string(resourceBefore), string(resourceAfter))
	require.JSONEq(t, string(before), string(after))
	require.Equal(t, beforeETag, afterETag)

	inheritedID := "urn:capabilities:inherited:" + uuid.NewString()
	inheritedPath := "/shells/" + encoded(inheritedID)
	shell["id"] = inheritedID
	shell["idShort"] = "InheritedCapabilityTarget"
	c.request(http.MethodPost, "/shells", "admin", shell, "", http.StatusCreated)
	c.mutate(http.MethodPut, inheritedPath+"/$access/policy", "admin", policy(
		map[string]string{"IDENTIFIABLE": "$aas(" + strconvQuote(inheritedID) + ")"},
		rule("admin", "ALL"),
		rule("inherited", "READ", "UPDATE"),
	), http.StatusOK)
	assertAASCapability(t, c, inheritedPath+"/$access/capabilities", "inherited", true)
	c.mutate(http.MethodPut, inheritedPath+"/$access/policy", "admin", policy(
		map[string]string{"IDENTIFIABLE": "$aas(" + strconvQuote(inheritedID) + ")"},
		rule("admin", "ALL"),
		rule("inherited", "READ"),
	), http.StatusOK)
	assertAASCapability(t, c, inheritedPath+"/$access/capabilities", "inherited", false)

	c.request(http.MethodDelete, ownedPath, "creator", nil, "", http.StatusNoContent)
	c.request(http.MethodDelete, inheritedPath, "admin", nil, "", http.StatusNoContent)
}

func TestAASUpdateCapabilitiesHideUnknownAndInvisibleResources(t *testing.T) {
	c := newAccessClient(t)
	id := "urn:capabilities:hidden:" + uuid.NewString()
	path := "/shells/" + encoded(id)
	shell := map[string]any{
		"modelType":        "AssetAdministrationShell",
		"id":               id,
		"idShort":          "HiddenCapabilityTarget",
		"assetInformation": map[string]string{"assetKind": "Instance"},
	}
	c.request(http.MethodPost, "/shells", "admin", shell, "", http.StatusCreated)

	hiddenBody, hiddenHeaders := c.requestRaw(http.MethodGet, path+"/$access/capabilities", "outsider", nil, "", http.StatusNotFound)
	unknownPath := "/shells/" + encoded("urn:capabilities:unknown:"+uuid.NewString()) + "/$access/capabilities"
	unknownBody, unknownHeaders := c.requestRaw(http.MethodGet, unknownPath, "outsider", nil, "", http.StatusNotFound)
	require.Equal(t, hiddenBody, unknownBody)
	assertNeutralCapabilityHeaders(t, hiddenHeaders)
	assertNeutralCapabilityHeaders(t, unknownHeaders)
	require.Equal(t, hiddenHeaders.Get("Cache-Control"), unknownHeaders.Get("Cache-Control"))
	require.Equal(t, hiddenHeaders.Get("Content-Type"), unknownHeaders.Get("Content-Type"))

	unauthenticatedExisting, existingHeaders := c.requestRaw(http.MethodGet, path+"/$access/capabilities", "", nil, "", http.StatusUnauthorized)
	unauthenticatedUnknown, missingHeaders := c.requestRaw(http.MethodGet, unknownPath, "", nil, "", http.StatusUnauthorized)
	require.Equal(t, unauthenticatedExisting, unauthenticatedUnknown)
	require.Equal(t, existingHeaders.Get("Cache-Control"), missingHeaders.Get("Cache-Control"))
	require.Equal(t, existingHeaders.Get("Content-Type"), missingHeaders.Get("Content-Type"))
	c.request(http.MethodDelete, path, "admin", nil, "", http.StatusNoContent)
}

func TestAASUpdateCapabilitiesFailClosedAndAuthenticateBeforeEvaluation(t *testing.T) {
	c := newAccessClient(t)
	id := "urn:capabilities:failure:" + uuid.NewString()
	path := "/shells/" + encoded(id)
	shell := map[string]any{
		"modelType":        "AssetAdministrationShell",
		"id":               id,
		"idShort":          "FailedCapabilityTarget",
		"assetInformation": map[string]string{"assetKind": "Instance"},
	}
	c.request(http.MethodPost, "/shells", "admin", shell, "", http.StatusCreated)
	invalidRequest, err := http.NewRequestWithContext(t.Context(), http.MethodGet, c.server.URL+path+"/$access/capabilities", nil)
	require.NoError(t, err)
	invalidRequest.Header.Set("Authorization", "Bearer invalid")
	invalidResponse, err := c.server.Client().Do(invalidRequest)
	require.NoError(t, err)
	defer func() { _ = invalidResponse.Body.Close() }()
	require.Equal(t, http.StatusUnauthorized, invalidResponse.StatusCode)
	require.Equal(t, "no-store", invalidResponse.Header.Get("Cache-Control"))

	require.NoError(t, c.db.Close())

	unknownPath := "/shells/" + encoded("urn:capabilities:unknown:"+uuid.NewString()) + "/$access/capabilities"
	c.requestRaw(http.MethodGet, unknownPath, "", nil, "", http.StatusUnauthorized)
	body, headers := c.requestRaw(http.MethodGet, path+"/$access/capabilities", "admin", nil, "", http.StatusInternalServerError)
	require.JSONEq(t, `{"message":"capability check failed"}`, string(body))
	require.NotContains(t, string(body), id)
	require.NotContains(t, string(body), "canUpdate")
	assertNeutralCapabilityHeaders(t, headers)
}

func assertAASCapability(t *testing.T, c accessClient, path, user string, expected bool) {
	t.Helper()
	data, headers := c.requestRaw(http.MethodGet, path, user, nil, "", http.StatusOK)
	var response map[string]bool
	require.NoError(t, json.Unmarshal(data, &response))
	require.Equal(t, map[string]bool{"canUpdate": expected}, response)
	require.Equal(t, "no-store", headers.Get("Cache-Control"))
	require.Empty(t, headers.Get("ETag"))
	require.Empty(t, headers.Get("Last-Modified"))
}

func assertNeutralCapabilityHeaders(t *testing.T, headers http.Header) {
	t.Helper()
	require.Equal(t, "no-store", headers.Get("Cache-Control"))
	require.Equal(t, "application/json", headers.Get("Content-Type"))
	require.Empty(t, headers.Get("ETag"))
	require.Empty(t, headers.Get("Last-Modified"))
}

func TestResourceBoundGrantRoundTripAndContextPath(t *testing.T) {
	c := newAccessClientWithBasePath(t, "/bridge/api")
	id := "urn:bridge:ä/" + uuid.NewString()
	path := "/bridge/api/submodels/" + encoded(id)
	access := path + "/$access"
	c.request("POST", "/bridge/api/submodels", "admin", map[string]any{"modelType": "Submodel", "id": id}, "", 201)
	c.request("GET", access+"/policy", "admin", nil, "", 404)
	c.mutate("PUT", access+"/policy", "admin", policy(map[string]string{"IDENTIFIABLE": "$sm(" + strconvQuote(id) + ")"}, rule("admin", "ALL")), 200)
	data := c.mutate("POST", access+"/grants", "admin", grant("reader", "READ"), 201)
	var created map[string]any
	require.NoError(t, json.Unmarshal(data, &created))
	grantPath := access + "/grants/" + created["id"].(string)
	c.request("GET", path, "reader", nil, "", 200)
	c.mutate("PUT", grantPath, "admin", grant("writer", "UPDATE"), 200)
	c.request("GET", path, "reader", nil, "", 403)
	c.mutate("PUT", access+"/managers", "admin", []any{principal("manager")}, 200)
	duplicate := c.mutate("POST", access+"/grants", "admin", grant("manager", "ALL"), 201)
	require.NoError(t, json.Unmarshal(duplicate, &created))
	c.mutate("DELETE", access+"/grants/"+created["id"].(string), "admin", nil, 204)
	c.request("GET", path, "manager", nil, "", 200)
	c.mutate("DELETE", grantPath, "manager", nil, 204)
	c.mutate("PUT", access+"/policy", "manager", policy(map[string]string{"IDENTIFIABLE": "$sm(" + strconvQuote(id) + ")"}), 409)
}

func TestResourceBoundVisibilityBeforePagination(t *testing.T) {
	c := newAccessClient(t)
	id := "urn:rebac:page:" + uuid.NewString()
	path := "/submodels/" + encoded(id)
	c.request("POST", "/submodels", "admin", map[string]any{"modelType": "Submodel", "id": id}, "", 201)
	c.mutate("PUT", path+"/$access/policy", "admin", policy(map[string]string{"IDENTIFIABLE": "$sm(" + strconvQuote(id) + ")"}, rule("reader", "READ"), rule("direct-only", "READ")), 200)
	c.request("GET", path, "direct-only", nil, "", http.StatusOK)
	c.request("GET", "/submodels", "direct-only", nil, "", http.StatusForbidden)
	data, _ := c.request("GET", "/submodels?limit=1", "reader", nil, "", 200)
	var page struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(data, &page))
	require.Len(t, page.Result, 1)
	require.Equal(t, id, page.Result[0].ID)
}

func TestResourceBoundCompoundPutRequiresLifecycleRights(t *testing.T) {
	c := newAccessClient(t)
	id := "urn:rebac:compound:" + uuid.NewString()
	path := "/submodels/" + encoded(id)
	root := map[string]any{"modelType": "Submodel", "id": id}
	binding := map[string]string{"IDENTIFIABLE": "$sm(" + strconvQuote(id) + ")"}
	c.request("POST", "/submodels", "admin", root, "", 201)
	c.mutate("PUT", path+"/$access/policy", "admin", policy(binding, rule("editor", "UPDATE")), 200)
	root["submodelElements"] = []any{map[string]any{"modelType": "Property", "idShort": "Added", "valueType": "xs:string", "value": "new"}}
	c.request("PUT", path, "editor", root, "", 403)
	c.mutate("PUT", path+"/$access/policy", "admin", policy(binding, rule("editor", "UPDATE", "CREATE")), 200)
	c.request("PUT", path, "editor", root, "", 204)
	childAccess := path + "/submodel-elements/Added/$access"
	c.request("GET", childAccess, "editor", nil, "", 200)
	c.mutate("PUT", childAccess+"/owners", "editor", []any{principal("admin")}, 200)
	delete(root, "submodelElements")
	c.request("PUT", path, "editor", root, "", 403)
	c.mutate("PUT", path+"/$access/policy", "admin", policy(binding, rule("editor", "UPDATE", "DELETE")), 200)
	c.request("PUT", path, "editor", root, "", 204)
}

func TestResourceBoundReferenceRequiresSubmodelAdministration(t *testing.T) {
	c := newAccessClient(t)
	id := "urn:rebac:owned:" + uuid.NewString()
	c.request("POST", "/submodels", "creator", map[string]any{"modelType": "Submodel", "id": id}, "", 201)
	aasID := "urn:rebac:aas:" + uuid.NewString()
	ref := map[string]any{"type": "ModelReference", "keys": []any{map[string]string{"type": "Submodel", "value": id}}}
	aas := map[string]any{"modelType": "AssetAdministrationShell", "id": aasID, "assetInformation": map[string]string{"assetKind": "Instance"}, "submodels": []any{ref}}
	c.request("POST", "/shells", "admin", aas, "", 403)
	access := "/submodels/" + encoded(id) + "/$access"
	c.mutate("PUT", access+"/policy", "creator", policy(map[string]string{"IDENTIFIABLE": "$sm(" + strconvQuote(id) + ")"}), 200)
	c.mutate("PUT", access+"/managers", "creator", []any{principal("admin")}, 200)
	c.request("POST", "/shells", "admin", aas, "", 201)
}

func TestResourceBoundViewOnlyReferences(t *testing.T) {
	c := newAccessClient(t)
	id := "urn:rebac:view:" + uuid.NewString()
	smPath := "/submodels/" + encoded(id)
	c.request("POST", "/submodels", "admin", map[string]any{"modelType": "Submodel", "id": id}, "", 201)
	aasID := "urn:rebac:view-aas:" + uuid.NewString()
	aasPath := "/shells/" + encoded(aasID)
	ref := map[string]any{"type": "ModelReference", "keys": []any{map[string]string{"type": "Submodel", "value": id}}}
	aas := map[string]any{"modelType": "AssetAdministrationShell", "id": aasID, "assetInformation": map[string]string{"assetKind": "Instance"}, "submodels": []any{ref}}
	c.request("POST", "/shells", "admin", aas, "", 201)
	c.mutate("PUT", aasPath+"/$access/policy", "admin", policy(map[string]string{"IDENTIFIABLE": "$aas(" + strconvQuote(aasID) + ")"}, rule("viewer", "READ")), 200)
	c.mutate("PUT", smPath+"/$access/policy", "admin", policy(map[string]string{"IDENTIFIABLE": "$sm(" + strconvQuote(id) + ")"}), 200)
	hidden, _ := c.request("GET", aasPath, "viewer", nil, "", 200)
	require.NotContains(t, string(hidden), id)
	c.mutate("POST", smPath+"/$access/grants", "admin", grant("viewer", "VIEW"), 201)
	c.request("GET", smPath, "viewer", nil, "", 403)
	c.request("GET", smPath+"/$reference", "viewer", nil, "", 200)
	visible, _ := c.request("GET", aasPath, "viewer", nil, "", 200)
	require.Contains(t, string(visible), id)
	refs, _ := c.request("GET", aasPath+"/submodel-refs?limit=1", "viewer", nil, "", 200)
	require.Contains(t, string(refs), id)
}

func TestResourceBoundConcurrentMutationsAndRestart(t *testing.T) {
	c := newAccessClient(t)
	id := "urn:rebac:concurrent:" + uuid.NewString()
	path := "/submodels/" + encoded(id)
	access := path + "/$access"
	c.request("POST", "/submodels", "admin", map[string]any{"modelType": "Submodel", "id": id}, "", 201)
	c.mutate("PUT", access+"/policy", "admin", policy(map[string]string{"IDENTIFIABLE": "$sm(" + strconvQuote(id) + ")"}, rule("admin", "ALL")), 200)
	_, etag := c.request("GET", access, "admin", nil, "", 200)
	results := make(chan int, 2)
	for _, user := range []string{"reader", "viewer"} {
		body, err := json.Marshal(grant(user, "READ"))
		require.NoError(t, err)
		req, err := http.NewRequestWithContext(t.Context(), "POST", c.server.URL+access+"/grants", bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("X-Test-Subject", "admin")
		req.Header.Set("If-Match", etag)
		go func() {
			response, err := c.server.Client().Do(req)
			if err != nil {
				results <- 0
				return
			}
			_ = response.Body.Close()
			results <- response.StatusCode
		}()
	}
	require.ElementsMatch(t, []int{201, 412}, []int{<-results, <-results})
	before, etag := c.request("GET", access, "admin", nil, "", 200)
	c.server.Close()
	c = c.restart()
	after, restartedETag := c.request("GET", access, "admin", nil, "", 200)
	require.JSONEq(t, string(before), string(after))
	require.Equal(t, etag, restartedETag)
	c.mutate("DELETE", access+"/policy", "admin", nil, 204)
	c.server.Close()
	c = c.restart()
	c.request("GET", access+"/policy", "admin", nil, "", 404)
	c.request("GET", path, "admin", nil, "", 200)
}

func TestResourceBoundListReplacementDoesNotReuseAccess(t *testing.T) {
	c := newAccessClient(t)
	id := "urn:rebac:list:" + uuid.NewString()
	path := "/submodels/" + encoded(id)
	first := map[string]any{"modelType": "Property", "valueType": "xs:string", "value": "first"}
	second := map[string]any{"modelType": "Property", "valueType": "xs:string", "value": "second"}
	list := map[string]any{"modelType": "SubmodelElementList", "idShort": "Items", "typeValueListElement": "Property", "valueTypeListElement": "xs:string", "value": []any{first, second}}
	root := map[string]any{"modelType": "Submodel", "id": id, "submodelElements": []any{list}}
	c.request("POST", "/submodels", "admin", root, "", 201)
	entry := path + "/submodel-elements/Items[0]/$access"
	c.mutate("PUT", entry+"/policy", "admin", policy(map[string]string{"REFERABLE": "$sme(" + strconvQuote(id) + ").Items[0]"}, rule("reader", "READ"), rule("admin", "ALL")), 200)
	_, stale := c.request("GET", entry, "admin", nil, "", 200)
	list["value"] = []any{second}
	c.request("PUT", path, "admin", root, "", 204)
	c.request("GET", entry+"/policy", "admin", nil, "", 404)
	c.request("GET", path+"/submodel-elements/Items[0]", "reader", nil, "", 403)
	c.request("PUT", entry+"/owners", "admin", []any{principal("admin")}, stale, 412)
}

func TestResourceBoundInvalidPredicateUsesABACFallback(t *testing.T) {
	c := newAccessClient(t)
	id := "urn:rebac:error:" + uuid.NewString()
	path := "/submodels/" + encoded(id)
	c.request("POST", "/submodels", "admin", map[string]any{"modelType": "Submodel", "id": id, "idShort": "Bridge"}, "", 201)
	failing := rule("auditor", "READ")
	failing["FORMULA"] = map[string]any{"$regex": []any{map[string]string{"$field": "$sm#idShort"}, map[string]string{"$strVal": "["}}}
	c.mutate("PUT", path+"/$access/policy", "admin", policy(map[string]string{"IDENTIFIABLE": "$sm(" + strconvQuote(id) + ")"}, failing), 200)
	c.request("GET", path, "auditor", nil, "", 200)
	c.mutate("DELETE", path+"/$access/policy", "admin", nil, 204)
	c.request("GET", path, "auditor", nil, "", 200)
}

func TestResourceBoundAccessRoutesAcrossAliases(t *testing.T) {
	c := newAccessClient(t)
	id := "urn:rebac:routes:" + uuid.NewString()
	smPath := "/submodels/" + encoded(id)
	property := map[string]any{"modelType": "Property", "idShort": "Reading", "valueType": "xs:string"}
	group := map[string]any{"modelType": "SubmodelElementCollection", "idShort": "Inspection", "value": []any{map[string]any{"modelType": "SubmodelElementCollection", "idShort": "Sensor", "value": []any{property}}}}
	c.request("POST", "/submodels", "admin", map[string]any{"modelType": "Submodel", "id": id, "submodelElements": []any{group}}, "", 201)
	aasID := "urn:rebac:routes-aas:" + uuid.NewString()
	aasPath := "/shells/" + encoded(aasID)
	ref := map[string]any{"type": "ModelReference", "keys": []any{map[string]string{"type": "Submodel", "value": id}}}
	c.request("POST", "/shells", "admin", map[string]any{"modelType": "AssetAdministrationShell", "id": aasID, "assetInformation": map[string]string{"assetKind": "Instance"}, "submodels": []any{ref}}, "", 201)
	childPath := smPath + "/submodel-elements/Inspection.Sensor.Reading"
	for _, path := range []string{"/shells", "/submodels", "/shell-descriptors", "/submodel-descriptors", "/concept-descriptions", "/lookup/shells"} {
		c.request("GET", path+"/$access", "admin", nil, "", http.StatusNotFound)
	}
	for _, path := range []string{aasPath, smPath, childPath, aasPath + smPath, aasPath + childPath} {
		t.Run(path, func(t *testing.T) { local := c; local.t = t; exerciseAccessRoutes(local, path) })
	}
}

func exerciseAccessRoutes(c accessClient, path string) {
	c.t.Helper()
	access := path + "/$access"
	c.mutate("PUT", access+"/policy", "admin", policy(map[string]string{"ROUTE": path}, rule("admin", "ALL")), 200)
	c.request("GET", access+"/policy", "admin", nil, "", 200)
	raw := c.mutate("POST", access+"/grants", "admin", grant("reader", "READ"), 201)
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(c.t, json.Unmarshal(raw, &created))
	c.mutate("PUT", access+"/grants/"+created.ID, "admin", grant("viewer", "VIEW"), 200)
	c.mutate("PUT", access+"/grants/"+created.ID, "admin", grant("viewer", "INVALID"), 400)
	c.mutate("DELETE", access+"/grants/"+created.ID, "admin", nil, 204)
	c.mutate("PUT", access+"/managers", "admin", []any{principal("manager")}, 200)
	c.mutate("PUT", access+"/owners", "admin", []any{principal("admin"), principal("coowner")}, 200)
	c.mutate("PUT", access+"/managers", "manager", []any{}, 200)
	c.mutate("PUT", access+"/owners", "coowner", []any{principal("admin")}, 200)
	c.mutate("DELETE", access+"/policy", "admin", nil, 204)
	c.request("GET", access+"/policy", "admin", nil, "", 404)
}

func TestResourceBoundAnonymousReadKeepsAdministrationProtected(t *testing.T) {
	c := newAccessClient(t)
	id := "urn:rebac:public:" + uuid.NewString()
	path := "/submodels/" + encoded(id)
	c.request("POST", "/submodels", "admin", map[string]any{"modelType": "Submodel", "id": id}, "", 201)
	anonymous := map[string]any{"ACL": map[string]any{"ATTRIBUTES": []any{map[string]string{"GLOBAL": "ANONYMOUS"}}, "RIGHTS": []string{"READ"}, "ACCESS": "ALLOW"}, "FORMULA": map[string]bool{"$boolean": true}}
	c.mutate("PUT", path+"/$access/policy", "admin", policy(map[string]string{"IDENTIFIABLE": "$sm(" + strconvQuote(id) + ")"}, anonymous), 200)
	c.request("GET", path, "", nil, "", 200)
	c.request("GET", path+"/$access", "", nil, "", 401)
}

func TestResourceBoundAccessInputValidationAndResponseMetadata(t *testing.T) {
	c := newAccessClient(t)
	id := "urn:rebac:validation:" + uuid.NewString()
	path := "/submodels/" + encoded(id)
	access := path + "/$access"
	c.request("POST", "/submodels", "admin", map[string]any{"modelType": "Submodel", "id": id}, "", http.StatusCreated)
	c.mutate("PUT", access+"/policy", "admin", policy(map[string]string{"IDENTIFIABLE": "$sm(" + strconvQuote(id) + ")"}, rule("admin", "ALL")), http.StatusOK)
	baseline, baselineETag := c.request("GET", access, "admin", nil, "", http.StatusOK)

	cases := []struct {
		name, method, suffix, body, message string
		status                              int
	}{
		{"trailing JSON", http.MethodPost, "/grants", `{"principal":{"issuer":"https://rebac.test","subject":"reader"},"rights":["READ"]}{}`, "multiple JSON values", http.StatusBadRequest},
		{"whitespace principal", http.MethodPost, "/grants", `{"principal":{"issuer":"https://rebac.test","subject":" "},"rights":["READ"]}`, "non-whitespace", http.StatusBadRequest},
		{"duplicate rights", http.MethodPost, "/grants", `{"principal":{"issuer":"https://rebac.test","subject":"reader"},"rights":["READ","READ"]}`, "duplicate right READ", http.StatusBadRequest},
		{"duplicate managers", http.MethodPut, "/managers", `[{"issuer":"https://rebac.test","subject":"manager"},{"issuer":"https://rebac.test","subject":"manager"}]`, "duplicate principal", http.StatusBadRequest},
		{"invalid grant ID", http.MethodPut, "/grants/not-a-uuid", `{"principal":{"issuer":"https://rebac.test","subject":"reader"},"rights":["READ"]}`, "invalid grant ID", http.StatusBadRequest},
		{"unknown grant", http.MethodDelete, "/grants/" + uuid.NewString(), "", "unknown managed grant", http.StatusNotFound},
		{"unsupported method", http.MethodPost, "/policy", `{}`, "expected PUT or DELETE", http.StatusMethodNotAllowed},
		{"mismatched resource", http.MethodPut, "/policy", `{"RESOURCE":{"IDENTIFIABLE":"$sm(\"different\")"},"rules":[]}`, "binding differs", http.StatusBadRequest},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			local := c
			local.t = t
			data, _ := local.requestRaw(test.method, access+test.suffix, "admin", []byte(test.body), baselineETag, test.status)
			require.Contains(t, string(data), test.message)
			current, currentETag := local.request("GET", access, "admin", nil, "", http.StatusOK)
			require.JSONEq(t, string(baseline), string(current))
			require.Equal(t, baselineETag, currentETag)
		})
	}

	oversized := bytes.Repeat([]byte(" "), (1<<20)+1)
	data, _ := c.requestRaw(http.MethodPost, access+"/grants", "admin", oversized, baselineETag, http.StatusRequestEntityTooLarge)
	require.Contains(t, string(data), "exceeds 1 MiB")

	createdData, headers := c.requestRaw(http.MethodPost, access+"/grants", "admin", mustMarshal(t, grant("reader", "READ")), baselineETag, http.StatusCreated)
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(createdData, &created))
	require.Equal(t, access+"/grants/"+created.ID, headers.Get("Location"))
	require.NotEmpty(t, headers.Get("ETag"))
	require.Equal(t, "application/json", headers.Get("Content-Type"))

	c.request("POST", "/submodels", " ", map[string]any{"modelType": "Submodel", "id": "urn:rebac:blank:" + uuid.NewString()}, "", http.StatusUnauthorized)
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return data
}
