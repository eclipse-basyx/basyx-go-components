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
	"github.com/stretchr/testify/require"
)

func mustParseDelegationTestURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()

	parsedURL, err := url.Parse(rawURL)
	require.NoError(t, err)
	return parsedURL
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

func TestToDelegatedOperationResultPayloadFromBodyForArrayKeepsInoutputEmpty(t *testing.T) {
	t.Parallel()

	property := &types.Property{}
	property.SetIDShort(stringPointer("out"))
	property.SetValueType(types.DataTypeDefXSDString)
	property.SetValue(stringPointer("output"))

	operationVariable := &types.OperationVariable{}
	operationVariable.SetValue(property)

	delegatedBody := []types.IOperationVariable{operationVariable}
	resultPayload, ok := toDelegatedOperationResultPayloadFromBody(delegatedBody)
	require.True(t, ok)
	resultPayloadBytes, err := json.Marshal(resultPayload)
	require.NoError(t, err)

	resultPayloadJSON := map[string]any{}
	require.NoError(t, json.Unmarshal(resultPayloadBytes, &resultPayloadJSON))

	outputArguments, outputOK := resultPayloadJSON["outputArguments"].([]any)
	require.True(t, outputOK)
	require.Len(t, outputArguments, 1)

	firstOutput, firstOutputOK := outputArguments[0].(map[string]any)
	require.True(t, firstOutputOK)
	outputValue, outputValueOK := firstOutput["value"].(map[string]any)
	require.True(t, outputValueOK)
	require.Equal(t, "output", outputValue["value"])

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

	firstOutput, firstOutputOK := outputArguments[0].(map[string]any)
	require.True(t, firstOutputOK)
	outputValue, outputValueOK := firstOutput["value"].(map[string]any)
	require.True(t, outputValueOK)
	require.Equal(t, "output", outputValue["value"])

	inoutputArguments, inoutputOK := resultPayloadJSON["inoutputArguments"].([]any)
	require.True(t, inoutputOK)
	require.Len(t, inoutputArguments, 1)

	firstInoutput, firstInoutputOK := inoutputArguments[0].(map[string]any)
	require.True(t, firstInoutputOK)
	inoutputValue, inoutputValueOK := firstInoutput["value"].(map[string]any)
	require.True(t, inoutputValueOK)
	require.Equal(t, "inoutput", inoutputValue["value"])
}

func TestToDelegatedOperationResultPayloadFromBodyForAnySliceUsesJsonization(t *testing.T) {
	t.Parallel()

	delegatedBody := []any{
		map[string]any{
			"value": map[string]any{"modelType": "Property", "idShort": "sum", "valueType": "xs:int", "value": "8"},
		},
		map[string]any{
			"value": map[string]any{"modelType": "Property", "idShort": "diff", "valueType": "xs:int", "value": "2"},
		},
	}

	resultPayload, ok := toDelegatedOperationResultPayloadFromBody(delegatedBody)
	require.True(t, ok)
	resultPayloadBytes, err := json.Marshal(resultPayload)
	require.NoError(t, err)

	resultPayloadJSON := map[string]any{}
	require.NoError(t, json.Unmarshal(resultPayloadBytes, &resultPayloadJSON))

	outputArguments, outputOK := resultPayloadJSON["outputArguments"].([]any)
	require.True(t, outputOK)
	require.Len(t, outputArguments, 2)

	firstOutput, firstOutputOK := outputArguments[0].(map[string]any)
	require.True(t, firstOutputOK)
	firstValue, firstValueOK := firstOutput["value"].(map[string]any)
	require.True(t, firstValueOK)
	require.Equal(t, "8", firstValue["value"])

	secondOutput, secondOutputOK := outputArguments[1].(map[string]any)
	require.True(t, secondOutputOK)
	secondValue, secondValueOK := secondOutput["value"].(map[string]any)
	require.True(t, secondValueOK)
	require.Equal(t, "2", secondValue["value"])

	inoutputArguments, inoutputOK := resultPayloadJSON["inoutputArguments"].([]any)
	require.True(t, inoutputOK)
	require.Len(t, inoutputArguments, 0)
}

func stringPointer(value string) *string {
	return &value
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

func TestDelegationDialFallsBackAcrossTrustedResolvedAddresses(t *testing.T) {
	t.Setenv(delegationTrustedHostsKey, "localhost:*,127.0.0.1:*,[::1]:*")

	resolveToLoopback := func(_ context.Context, host string) ([]netip.Addr, error) {
		require.Equal(t, "localhost", host)
		return []netip.Addr{netip.MustParseAddr("::1"), netip.MustParseAddr("127.0.0.1")}, nil
	}

	dialedAddresses := []string{}
	dialContext := func(_ context.Context, _ string, address string) (net.Conn, error) {
		dialedAddresses = append(dialedAddresses, address)
		if address == "[::1]:1234" {
			return nil, errors.New("first endpoint is unavailable")
		}

		clientConn, serverConn := net.Pipe()
		_ = serverConn.Close()
		return clientConn, nil
	}

	guard := newDelegationAddressGuard(resolveToLoopback, dialContext)
	conn, err := guard.dialTrustedContext(context.Background(), "tcp", "localhost:1234")
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	require.Equal(t, []string{"[::1]:1234", "127.0.0.1:1234"}, dialedAddresses)
}

func TestResolveTrustedURLTargetUnmapsIPv4MappedResolvedAddresses(t *testing.T) {
	t.Setenv(delegationTrustedHostsKey, "host.docker.internal:*,192.168.65.254:*")

	resolveToIPv4MappedGateway := func(_ context.Context, host string) ([]netip.Addr, error) {
		require.Equal(t, "host.docker.internal", host)
		return []netip.Addr{netip.MustParseAddr("::ffff:192.168.65.254")}, nil
	}

	guard := newDelegationAddressGuard(resolveToIPv4MappedGateway, nil)
	target, err := guard.resolveTrustedURLTarget(context.Background(), mustParseDelegationTestURL(t, "http://host.docker.internal:1234/delegate"))
	require.NoError(t, err)
	require.Equal(t, "host.docker.internal:1234", target.originalAuthority)
	require.Equal(t, "192.168.65.254:1234", target.resolvedAuthority)
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

func TestNewDelegationHTTPTransportClonesDefaultTransportSettings(t *testing.T) {
	guard := newDelegationAddressGuard(nil, nil)
	transport := newDelegationHTTPTransport(guard)
	defaultTransport := http.DefaultTransport.(*http.Transport)

	require.NotSame(t, defaultTransport, transport)
	require.Nil(t, transport.Proxy)
	require.NotNil(t, transport.DialContext)
	require.Equal(t, defaultTransport.ForceAttemptHTTP2, transport.ForceAttemptHTTP2)
	require.Equal(t, defaultTransport.MaxIdleConns, transport.MaxIdleConns)
	require.Equal(t, defaultTransport.IdleConnTimeout, transport.IdleConnTimeout)
	require.Equal(t, defaultTransport.TLSHandshakeTimeout, transport.TLSHandshakeTimeout)
	require.Equal(t, defaultTransport.ExpectContinueTimeout, transport.ExpectContinueTimeout)
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
