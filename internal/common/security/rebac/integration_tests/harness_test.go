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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

// Package rebacintegration verifies relationship-based access control against
// real PostgreSQL, OpenFGA and Keycloak containers and all covered services.
package rebacintegration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the pgx database/sql driver
	"github.com/stretchr/testify/require"
)

const (
	userPassword  = "pwd"
	composeFile   = "docker_compose/docker_compose.yml"
	healthTimeout = 240 * time.Second
)

var (
	composeRuntime *testenv.ComposeRuntime
	securityEnv    string
	submodelURL    string
	aasURL         string
	cdURL          string
	environmentURL string
	packagesURL    string
	passportsURL   string
	tokenURL       string
	issuer         string
	tokens         *testenv.PasswordGrantTokenProvider
	uniqueCounter  int
	uniqueMu       sync.Mutex
)

func TestMain(m *testing.M) {
	composeRuntime = testenv.NewComposeRuntimeOrExit("rebac-it", []testenv.PortBinding{
		{Name: "sm", EnvVar: "BASYX_IT_SM_PORT"},
		{Name: "aas", EnvVar: "BASYX_IT_AAS_PORT"},
		{Name: "cd", EnvVar: "BASYX_IT_CD_PORT"},
		{Name: "env", EnvVar: "BASYX_IT_ENV_PORT"},
		{Name: "aasx", EnvVar: "BASYX_IT_AASX_PORT"},
		{Name: "dpp", EnvVar: "BASYX_IT_DPP_PORT"},
		{Name: "db", EnvVar: "BASYX_IT_DB_PORT"},
		{Name: "keycloak", EnvVar: "BASYX_IT_KEYCLOAK_PORT"},
	})
	submodelURL = composeRuntime.LocalhostURL("sm")
	aasURL = composeRuntime.LocalhostURL("aas")
	cdURL = composeRuntime.LocalhostURL("cd")
	environmentURL = composeRuntime.LocalhostURL("env")
	packagesURL = composeRuntime.LocalhostURL("aasx")
	passportsURL = composeRuntime.LocalhostURL("dpp")
	issuer = composeRuntime.LocalhostURL("keycloak") + "/realms/basyx"
	tokenURL = issuer + "/protocol/openid-connect/token"
	tokens = testenv.NewPasswordGrantTokenProvider(tokenURL, "basyx-ui", 10*time.Second)
	securityEnv = testenv.PrepareSecurityEnvOrExit("security_env", map[string]string{
		"http://localhost:8080": composeRuntime.LocalhostURL("keycloak"),
	})
	code := testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile:        composeFile,
		ProjectName:        composeRuntime.ProjectName,
		Env:                composeRuntime.EnvWith("BASYX_IT_SECURITY_ENV=" + securityEnv),
		PreDownBeforeUp:    true,
		SkipDownAfterTests: os.Getenv("BASYX_IT_KEEP_STACK") == "1",
		UpTimeout:          20 * time.Minute,
		WaitForReady:       waitForServices,
	})
	_ = os.RemoveAll(securityEnv)
	os.Exit(code)
}

func waitForServices() error {
	for _, base := range []string{submodelURL, aasURL, cdURL, environmentURL, packagesURL, passportsURL} {
		if err := testenv.WaitHealthyURL(base+"/health", healthTimeout); err != nil {
			return err
		}
	}
	return nil
}

// compose runs a docker compose command against the suite's project.
func compose(t *testing.T, args ...string) {
	t.Helper()
	engine, base, err := testenv.FindCompose()
	require.NoError(t, err)
	command := append(append(base, "-f", composeFile, "-p", composeRuntime.ProjectName), args...)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	require.NoError(t, testenv.RunComposeWithEnv(ctx, engine, composeRuntime.EnvWith("BASYX_IT_SECURITY_ENV="+securityEnv), command...))
}

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", composeRuntime.PostgresKeywordDSN("db", "basyxTestDB"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func unique(prefix string) string {
	uniqueMu.Lock()
	defer uniqueMu.Unlock()
	uniqueCounter++
	return fmt.Sprintf("urn:rebac-it:%s:%d:%d", prefix, time.Now().UnixNano(), uniqueCounter)
}

func enc(identifier string) string {
	return common.EncodeString(identifier)
}

// response is one HTTP exchange.
type response struct {
	status int
	body   []byte
	header http.Header
}

func (r response) json(t *testing.T) map[string]any {
	t.Helper()
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(r.body, &decoded), string(r.body))
	return decoded
}

func call(t *testing.T, user string, method string, url string, body any, headers map[string]string) response {
	t.Helper()
	if body == nil {
		return callRaw(t, user, method, url, nil, "", headers)
	}
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	return callRaw(t, user, method, url, bytes.NewReader(payload), "application/json", headers)
}

// callRaw sends body with contentType, e.g. a multipart upload.
func callRaw(t *testing.T, user string, method string, url string, body io.Reader, contentType string, headers map[string]string) response {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), method, url, body)
	require.NoError(t, err)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	if user != "" {
		request.Header.Set("Authorization", "Bearer "+token(t, user))
	}
	result, err := testenv.HTTPClient().Do(request)
	require.NoError(t, err)
	defer func() { _ = result.Body.Close() }()
	payload, err := io.ReadAll(result.Body)
	require.NoError(t, err)
	return response{status: result.StatusCode, body: payload, header: result.Header}
}

func token(t *testing.T, user string) string {
	t.Helper()
	accessToken, err := tokens.GetAccessToken(&testenv.TokenCredentials{User: user, Password: userPassword})
	require.NoError(t, err)
	return accessToken
}

// subject returns the OIDC subject of a test user.
func subject(t *testing.T, user string) string {
	t.Helper()
	parts := strings.Split(token(t, user), ".")
	require.Len(t, parts, 3)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var claims struct {
		Subject string `json:"sub"`
	}
	require.NoError(t, json.Unmarshal(payload, &claims))
	require.NotEmpty(t, claims.Subject)
	return claims.Subject
}

func expectStatus(t *testing.T, expected int, got response, step string) {
	t.Helper()
	require.Equalf(t, expected, got.status, "%s: %s", step, string(got.body))
}

func property(idShort string, value string) map[string]any {
	return map[string]any{"idShort": idShort, "modelType": "Property", "valueType": "xs:string", "value": value}
}

func collection(idShort string, children ...map[string]any) map[string]any {
	value := make([]any, 0, len(children))
	for _, child := range children {
		value = append(value, child)
	}
	return map[string]any{"idShort": idShort, "modelType": "SubmodelElementCollection", "value": value}
}

func submodel(identifier string, idShort string, elements ...map[string]any) map[string]any {
	value := make([]any, 0, len(elements))
	for _, element := range elements {
		value = append(value, element)
	}
	return map[string]any{"id": identifier, "idShort": idShort, "modelType": "Submodel", "submodelElements": value}
}

func shell(identifier string, submodelIDs ...string) map[string]any {
	references := make([]any, 0, len(submodelIDs))
	for _, submodelID := range submodelIDs {
		references = append(references, map[string]any{
			"type": "ModelReference",
			"keys": []any{map[string]any{"type": "Submodel", "value": submodelID}},
		})
	}
	return map[string]any{
		"id": identifier, "idShort": "shell", "modelType": "AssetAdministrationShell",
		"assetInformation": map[string]any{"assetKind": "Instance", "globalAssetId": identifier + ":asset"},
		"submodels":        references,
	}
}

func conceptDescription(identifier string) map[string]any {
	return map[string]any{"id": identifier, "idShort": "cd", "modelType": "ConceptDescription"}
}

// grant is one requested direct grant.
type grant struct {
	Relation    string `json:"relation"`
	SubjectType string `json:"subjectType"`
	Issuer      string `json:"issuer"`
	Subject     string `json:"subject"`
}

func userGrant(t *testing.T, relation string, user string) grant {
	t.Helper()
	return grant{Relation: relation, SubjectType: "user", Issuer: issuer, Subject: subject(t, user)}
}

func groupGrant(relation string, group string) grant {
	return grant{Relation: relation, SubjectType: "group", Issuer: issuer, Subject: group}
}

// readAccess returns the grants document and ETag of an $access resource.
func readAccess(t *testing.T, user string, accessURL string) ([]grant, string) {
	t.Helper()
	result := call(t, user, http.MethodGet, accessURL, nil, nil)
	expectStatus(t, http.StatusOK, result, "read $access")
	var document struct {
		Grants []grant `json:"grants"`
	}
	require.NoError(t, json.Unmarshal(result.body, &document))
	for index := range document.Grants {
		if document.Grants[index].Issuer == "" {
			document.Grants[index].Issuer = issuer
		}
	}
	return document.Grants, result.header.Get("ETag")
}

// setGrants replaces the grants of an $access resource and expects the
// change to be applied synchronously.
func setGrants(t *testing.T, user string, accessURL string, grants []grant) response {
	t.Helper()
	_, etag := readAccess(t, user, accessURL)
	result := call(t, user, http.MethodPut, accessURL+"/grants", map[string]any{"grants": grants}, map[string]string{"If-Match": etag})
	expectStatus(t, http.StatusOK, result, "replace grants")
	return result
}

// addGrants appends grants to the current grants of an $access resource.
func addGrants(t *testing.T, user string, accessURL string, grants ...grant) {
	t.Helper()
	current, _ := readAccess(t, user, accessURL)
	setGrants(t, user, accessURL, append(current, grants...))
}

func submodelAccess(base string, identifier string) string {
	return base + "/submodels/" + enc(identifier) + "/$access"
}

func elementAccess(base string, identifier string, path string) string {
	return base + "/submodels/" + enc(identifier) + "/submodel-elements/" + path + "/$access"
}

var bootstrapOnce sync.Once

// bootstrapCreators lets alice and carol create identifiables, descriptors
// and discovery entries. The operators group is configured as ReBAC
// administrator of the suite.
func bootstrapCreators(t *testing.T) {
	t.Helper()
	bootstrapOnce.Do(func() {
		for _, repository := range []struct {
			base string
			kind string
		}{
			{submodelURL, "submodel"}, {aasURL, "aas"}, {cdURL, "concept_description"},
			{environmentURL, "aas_descriptor"}, {environmentURL, "submodel_descriptor"}, {environmentURL, "asset_links"},
			{packagesURL, "aasx_package"},
		} {
			accessURL := repository.base + "/security/rebac/repositories/" + repository.kind + "/$access"
			setGrants(t, "dave", accessURL, []grant{
				userGrant(t, "creator", "alice"),
				userGrant(t, "creator", "carol"),
				groupGrant("admin", "operators"),
			})
		}
	})
}

// createSubmodel lets owner create a Submodel on base and returns its id.
func createSubmodel(t *testing.T, base string, owner string, idShort string, elements ...map[string]any) string {
	t.Helper()
	bootstrapCreators(t)
	identifier := unique("sm")
	result := call(t, owner, http.MethodPost, base+"/submodels", submodel(identifier, idShort, elements...), nil)
	expectStatus(t, http.StatusCreated, result, "create submodel")
	return identifier
}
