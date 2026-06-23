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

package api

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncbulk"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistencepostgresql "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi"
	"github.com/stretchr/testify/require"
)

func contextWithABACDisabled(t *testing.T) context.Context {
	t.Helper()

	cfg := &common.Config{}
	var cfgCtx context.Context
	handler := common.ConfigMiddleware(cfg)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		cfgCtx = r.Context()
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	require.NotNil(t, cfgCtx)
	return cfgCtx
}

func TestResolveModelReferencePathKeysUsesEntityForParentSegment(t *testing.T) {
	t.Parallel()

	keyTypes, keyValues, err := resolveModelReferencePathKeys(
		"DemoEntity.StatementProperty1",
		"Property",
		func(path string) (string, error) {
			if path == "DemoEntity" {
				return "Entity", nil
			}
			return "", nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, []string{"Entity", "Property"}, keyTypes)
	require.Equal(t, []string{"DemoEntity", "StatementProperty1"}, keyValues)
}

func TestResolveModelReferencePathKeysUsesAnnotatedRelationshipElementForParentSegment(t *testing.T) {
	t.Parallel()

	keyTypes, keyValues, err := resolveModelReferencePathKeys(
		"DemoAnnotatedRelationshipElement.AnnotationProperty1",
		"Property",
		func(path string) (string, error) {
			if path == "DemoAnnotatedRelationshipElement" {
				return "AnnotatedRelationshipElement", nil
			}
			return "", nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, []string{"AnnotatedRelationshipElement", "Property"}, keyTypes)
	require.Equal(t, []string{"DemoAnnotatedRelationshipElement", "AnnotationProperty1"}, keyValues)
}

func TestResolveModelReferencePathKeysBuildsListIndexSegment(t *testing.T) {
	t.Parallel()

	keyTypes, keyValues, err := resolveModelReferencePathKeys(
		"test.test[0]",
		"SubmodelElementList",
		func(path string) (string, error) {
			switch path {
			case "test":
				return "SubmodelElementCollection", nil
			case "test.test":
				return "SubmodelElementCollection", nil
			default:
				return "", nil
			}
		},
	)
	require.NoError(t, err)
	require.Equal(t, []string{"SubmodelElementCollection", "SubmodelElementCollection", "SubmodelElementList"}, keyTypes)
	require.Equal(t, []string{"test", "test", "0"}, keyValues)
}

func TestGetSubmodelElementByPathSubmodelRepoRejectsInvalidLevel(t *testing.T) {
	t.Parallel()

	sut := NewSubmodelRepositoryAPIAPIService(persistencepostgresql.SubmodelDatabase{})
	encodedSubmodelID := base64.RawStdEncoding.EncodeToString([]byte("sm-1"))

	response, err := sut.GetSubmodelElementByPathSubmodelRepo(contextWithABACDisabled(t), encodedSubmodelID, "a.b", "invalid-level", "")
	require.NoError(t, err)
	require.Equal(t, 400, response.Code)
}

func TestGetSubmodelByIDPathRejectsInvalidLevel(t *testing.T) {
	t.Parallel()

	sut := NewSubmodelRepositoryAPIAPIService(persistencepostgresql.SubmodelDatabase{})
	encodedSubmodelID := common.EncodeString("sm-1")

	response, err := sut.GetSubmodelByIDPath(contextWithABACDisabled(t), encodedSubmodelID, "invalid-level")
	require.NoError(t, err)
	require.Equal(t, 400, response.Code)
}

func TestInvokeOperationValueOnlyReturnsBadRequest(t *testing.T) {
	t.Parallel()

	sut := NewSubmodelRepositoryAPIAPIService(persistencepostgresql.SubmodelDatabase{})
	response, err := sut.InvokeOperationValueOnly(contextWithABACDisabled(t), "", "", "", gen.OperationRequestValueOnly{}, false)
	require.NoError(t, err)
	require.Equal(t, 400, response.Code)
}

func TestGetOperationAsyncStatusReturnsRedirectWithLocation(t *testing.T) {
	sut := NewSubmodelRepositoryAPIAPIService(persistencepostgresql.SubmodelDatabase{})

	decodedSubmodelID := "sm-redirect"
	encodedSubmodelID := base64.RawURLEncoding.EncodeToString([]byte(decodedSubmodelID))
	handleID, err := sut.asyncManager.Start("anonymous")
	require.NoError(t, err)
	sut.asyncManager.Update(handleID, func(record asyncbulk.Record) asyncbulk.Record {
		record.ExecutionState = "Completed"
		record.Metadata = map[string]string{
			delegatedAsyncSubmodelIdentifierMetadataKey: decodedSubmodelID,
			delegatedAsyncIDShortPathMetadataKey:        "Ops.Add",
		}
		return record
	})

	response, err := sut.GetOperationAsyncStatus(contextWithABACDisabled(t), encodedSubmodelID, "Ops.Add", handleID)
	require.NoError(t, err)
	require.Equal(t, 302, response.Code)

	redirect, ok := response.Body.(openapi.Redirect)
	require.True(t, ok)
	require.True(t, strings.Contains(redirect.Location, "/operation-results/"))
}
