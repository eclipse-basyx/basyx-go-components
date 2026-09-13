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

package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

func TestAPIParameterConformance(t *testing.T) {
	testenv.RunAPIParameterConformance(t, aasEnvBaseURL, nil, []testenv.APIConformanceEndpoint{
		{Path: "/shells/dXJuOnRlc3Q/submodels/dXJuOnRlc3Q/submodel-elements"},
		{Path: "/shells/dXJuOnRlc3Q/submodels/dXJuOnRlc3Q/submodel-elements/$path"},
		{Path: "/shells", Filters: []string{"assetIds"}, IdentifierPath: "/shells/"},
		{Path: "/submodels", Filters: []string{"semanticId"}, IdentifierPath: "/submodels/"},
		{Path: "/concept-descriptions", Filters: []string{"isCaseOf", "dataSpecificationRef"}, IdentifierPath: "/concept-descriptions/"},
	})
}

func TestExactSubmodelReferenceConformance(t *testing.T) {
	testenv.RunSubmodelReferenceConformance(t, aasEnvBaseURL)
}

func TestExactConceptDescriptionReferenceConformance(t *testing.T) {
	testenv.RunConceptDescriptionReferenceConformance(t, aasEnvBaseURL)
}

func TestAssetPairConformance(t *testing.T) {
	testenv.RunAssetPairConformance(t, aasEnvBaseURL, "/shells", false)
}

func TestInvalidSubmodelIdentifierDoesNotMutateEnvironmentOrDescriptor(t *testing.T) {
	id := fmt.Sprintf("urn:conformance:invalid-put:%d", time.Now().UnixNano())
	payload := map[string]any{"modelType": "Submodel", "id": id, "idShort": "BeforeInvalidPut"}
	status, body, err := doAASEnvRawJSONRequest(http.MethodPost, aasEnvBaseURL+"/submodels", payload)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status, string(body))
	t.Cleanup(func() {
		_, _, _ = doAASEnvRawJSONRequest(http.MethodDelete, aasEnvBaseURL+"/submodels/"+common.EncodeString(id), nil)
	})
	payload["idShort"] = "AfterInvalidPut"
	status, body, err = doAASEnvRawJSONRequest(http.MethodPut, aasEnvBaseURL+"/submodels/"+common.EncodeString(id)+"=", payload)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, status, string(body))
	db, err := sql.Open("pgx", integrationTestDSN)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	for _, table := range []struct{ name, column string }{{"submodel", "submodel_identifier"}, {"submodel_descriptor", "id"}} {
		query, args, err := goqu.Dialect("postgres").From(table.name).Select("id_short").Where(goqu.C(table.column).Eq(id)).Prepared(true).ToSQL()
		require.NoError(t, err)
		var value string
		require.NoError(t, db.QueryRow(query, args...).Scan(&value))
		require.Equal(t, "BeforeInvalidPut", value)
	}
}
