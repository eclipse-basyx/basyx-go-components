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
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/FriedJannik/aas-go-sdk/types"
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

func TestParseDelegationTimeoutParsesISO8601Duration(t *testing.T) {
	t.Parallel()

	duration, err := parseDelegationTimeout("PT5.5S")
	require.NoError(t, err)
	require.Equal(t, 5500*time.Millisecond, duration)
}

func TestParseDelegationTimeoutRejectsUnsupportedYears(t *testing.T) {
	t.Parallel()

	_, err := parseDelegationTimeout("P1Y")
	require.Error(t, err)
}

func TestResolveDelegationURLReadsInvocationDelegationQualifier(t *testing.T) {
	t.Parallel()

	operation := types.NewOperation()
	qualifier := types.Qualifier{}
	qualifier.SetType(invocationDelegationQualifierType)
	valueType := types.DataTypeDefXSDString
	qualifier.SetValueType(valueType)
	delegationURL := "http://delegation.internal/invoke"
	qualifier.SetValue(&delegationURL)
	operation.SetQualifiers([]types.IQualifier{&qualifier})

	resolvedURL, err := resolveDelegationURL(operation)
	require.NoError(t, err)
	require.Equal(t, delegationURL, resolvedURL)
}

func TestInvokeOperationValueOnlyReturnsBadRequest(t *testing.T) {
	t.Parallel()

	sut := NewSubmodelRepositoryAPIAPIService(persistencepostgresql.SubmodelDatabase{})
	response, err := sut.InvokeOperationValueOnly(contextWithABACDisabled(t), "", "", "", gen.OperationRequestValueOnly{}, false)
	require.NoError(t, err)
	require.Equal(t, 400, response.Code)
}

func TestToDelegatedOperationResultPayloadFromBodyForArrayKeepsInoutputEmpty(t *testing.T) {
	t.Parallel()

	delegatedBody := []types.IOperationVariable{&types.OperationVariable{}}
	resultPayload, ok := toDelegatedOperationResultPayloadFromBody(delegatedBody)
	require.True(t, ok)
	resultPayloadBytes, err := json.Marshal(resultPayload)
	require.NoError(t, err)

	resultPayloadJSON := map[string]any{}
	require.NoError(t, json.Unmarshal(resultPayloadBytes, &resultPayloadJSON))

	outputArguments, outputOK := resultPayloadJSON["outputArguments"].([]any)
	require.True(t, outputOK)
	require.Len(t, outputArguments, 1)

	inoutputArguments, inoutputOK := resultPayloadJSON["inoutputArguments"].([]any)
	require.True(t, inoutputOK)
	require.Len(t, inoutputArguments, 0)
}

func TestToDelegatedOperationResultPayloadFromBodyForMapSeparatesOutputAndInoutput(t *testing.T) {
	t.Parallel()

	delegatedBody := map[string]any{
		"outputArguments": []map[string]any{{
			"value": map[string]any{"modelType": "Property", "idShort": "out", "valueType": "xs:string", "value": "output"},
		}},
		"inoutputArguments": []map[string]any{{
			"value": map[string]any{"modelType": "Property", "idShort": "inout", "valueType": "xs:string", "value": "inoutput"},
		}},
	}

	resultPayload, ok := toDelegatedOperationResultPayloadFromBody(delegatedBody)
	require.True(t, ok)
	resultPayloadBytes, err := json.Marshal(resultPayload)
	require.NoError(t, err)

	resultPayloadJSON := map[string]any{}
	require.NoError(t, json.Unmarshal(resultPayloadBytes, &resultPayloadJSON))

	outputArguments, outputOK := resultPayloadJSON["outputArguments"].([]any)
	require.True(t, outputOK)
	require.Len(t, outputArguments, 1)

	inoutputArguments, inoutputOK := resultPayloadJSON["inoutputArguments"].([]any)
	require.True(t, inoutputOK)
	require.Len(t, inoutputArguments, 1)
}

func mustParseDelegationTestURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()

	parsedURL, err := url.Parse(rawURL)
	require.NoError(t, err)
	return parsedURL
}

func TestDelegationAuthorityRejectsImplicitLocalAndInternalTrust(t *testing.T) {
	t.Setenv(delegationTrustedHostsKey, "")

	require.False(t, isTrustedDelegationAuthority("localhost:80"))
	require.False(t, isTrustedDelegationAuthority("127.0.0.1:80"))
	require.False(t, isTrustedDelegationAuthority("10.0.0.1:80"))
	require.False(t, isTrustedDelegationAuthority("service.internal:80"))
	require.False(t, isTrustedDelegationAuthority("service.svc:80"))
	require.False(t, isTrustedDelegationAuthority("service.cluster.local:80"))
}

func TestDelegationAuthorityRequiresPortAndSupportsWildcardPort(t *testing.T) {
	t.Setenv(delegationTrustedHostsKey, "delegate.example.com,delegate.example.com:8080,localhost:*,*:*,*.svc:*")

	require.False(t, isTrustedDelegationAuthority("delegate.example.com:80"))
	require.True(t, isTrustedDelegationAuthority("delegate.example.com:8080"))
	require.True(t, isTrustedDelegationAuthority("localhost:12345"))
	require.False(t, isTrustedDelegationAuthority("service.svc:12345"))
}

func TestResolveTrustedURLTargetRequiresOriginalAndResolvedAddresses(t *testing.T) {
	resolveToLoopback := func(_ context.Context, host string) ([]netip.Addr, error) {
		require.Equal(t, "localhost", host)
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}

	t.Setenv(delegationTrustedHostsKey, "localhost:*")
	guard := newDelegationAddressGuard(resolveToLoopback, nil)
	_, err := guard.resolveTrustedURLTarget(context.Background(), mustParseDelegationTestURL(t, "http://localhost:1234/delegate"))
	require.ErrorContains(t, err, "UNTRUSTEDRESOLVED")

	t.Setenv(delegationTrustedHostsKey, "localhost:*,127.0.0.1:*")
	guard = newDelegationAddressGuard(resolveToLoopback, nil)
	target, err := guard.resolveTrustedURLTarget(context.Background(), mustParseDelegationTestURL(t, "http://localhost:1234/delegate"))
	require.NoError(t, err)
	require.Equal(t, "localhost:1234", target.originalAuthority)
	require.Equal(t, "127.0.0.1:1234", target.resolvedAuthority)
}

func TestResolveTrustedURLTargetUsesDefaultHTTPAndHTTPSPorts(t *testing.T) {
	t.Setenv(delegationTrustedHostsKey, "example.com:80,secure.example.com:443,93.184.216.34:80,93.184.216.34:443")

	resolveToDocumentationIP := func(_ context.Context, _ string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
	}
	guard := newDelegationAddressGuard(resolveToDocumentationIP, nil)

	httpTarget, httpErr := guard.resolveTrustedURLTarget(context.Background(), mustParseDelegationTestURL(t, "http://example.com/delegate"))
	require.NoError(t, httpErr)
	require.Equal(t, "example.com:80", httpTarget.originalAuthority)
	require.Equal(t, "93.184.216.34:80", httpTarget.resolvedAuthority)

	httpsTarget, httpsErr := guard.resolveTrustedURLTarget(context.Background(), mustParseDelegationTestURL(t, "https://secure.example.com/delegate"))
	require.NoError(t, httpsErr)
	require.Equal(t, "secure.example.com:443", httpsTarget.originalAuthority)
	require.Equal(t, "93.184.216.34:443", httpsTarget.resolvedAuthority)
}

func TestDelegationHTTPClientRejectsDisallowedResolvedIPBeforeDial(t *testing.T) {
	t.Setenv(delegationTrustedHostsKey, "service.internal:8080")

	dialed := false
	resolveToPrivateIP := func(_ context.Context, host string) ([]netip.Addr, error) {
		require.Equal(t, "service.internal", host)
		return []netip.Addr{netip.MustParseAddr("10.0.0.1")}, nil
	}
	failOnDial := func(_ context.Context, _ string, _ string) (net.Conn, error) {
		dialed = true
		return nil, errors.New("dial should not be called")
	}

	guard := newDelegationAddressGuard(resolveToPrivateIP, failOnDial)
	client := newDelegationHTTPClient(time.Second, guard)
	_, err := client.Post("http://service.internal:8080/delegate", "application/json", strings.NewReader("[]"))
	require.ErrorContains(t, err, "UNTRUSTEDRESOLVED")
	require.False(t, dialed)
}

func TestDelegationHTTPClientRechecksRedirectTargets(t *testing.T) {
	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://192.0.2.1:8080/delegate", http.StatusFound)
	}))
	defer redirectServer.Close()

	serverURL := mustParseDelegationTestURL(t, redirectServer.URL)
	t.Setenv(delegationTrustedHostsKey, serverURL.Host)

	guard := newDelegationAddressGuard(nil, nil)
	client := newDelegationHTTPClient(2*time.Second, guard)
	_, err := client.Get(redirectServer.URL)
	require.ErrorContains(t, err, "UNTRUSTED")
}

func TestDelegatedOperationForwardsAuthorizationAfterStrictTrust(t *testing.T) {
	capturedAuthorization := ""
	delegationServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuthorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	}))
	defer delegationServer.Close()

	serverURL := mustParseDelegationTestURL(t, delegationServer.URL)
	t.Setenv(delegationTrustedHostsKey, serverURL.Host)

	ctx := common.WithAuthorizationHeader(contextWithABACDisabled(t), "Bearer delegated")
	statusCode, _, err := doDelegatedOperationCall(ctx, delegationServer.URL, []types.IOperationVariable{}, time.Second)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode)
	require.Equal(t, "Bearer delegated", capturedAuthorization)
}

func TestDelegatedOperationRejectsUntrustedTargetBeforeForwardingAuthorization(t *testing.T) {
	delegationServerCalled := false
	delegationServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		delegationServerCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer delegationServer.Close()

	t.Setenv(delegationTrustedHostsKey, "")
	ctx := common.WithAuthorizationHeader(contextWithABACDisabled(t), "Bearer delegated")
	_, _, err := doDelegatedOperationCall(ctx, delegationServer.URL, []types.IOperationVariable{}, time.Second)
	require.ErrorContains(t, err, "UNTRUSTED")
	require.False(t, delegationServerCalled)
}

func TestParseDelegationAsyncTTLUsesDefaultOnInvalidValue(t *testing.T) {
	t.Setenv(delegationAsyncTTLKey, "invalid")
	require.Equal(t, defaultDelegationAsyncTTL, parseDelegationAsyncTTL())
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
