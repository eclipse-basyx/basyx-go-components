package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aas-core-works/aas-core3.1-golang/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistencepostgresql "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi/go"
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

func TestShouldForwardAuthorizationHeaderTrustedByDefaultLocalhost(t *testing.T) {
	t.Parallel()

	require.True(t, isTrustedDelegationHost("localhost"))
	require.True(t, isTrustedDelegationHost("127.0.0.1"))
	require.False(t, isTrustedDelegationHost("example.com"))
}

func TestShouldForwardAuthorizationHeaderTrustedByAllowlist(t *testing.T) {
	t.Setenv(delegationTrustedHostsKey, "delegate.example.com")
	require.True(t, isTrustedDelegationHost("delegate.example.com"))
}

func TestParseDelegationAsyncTTLUsesDefaultOnInvalidValue(t *testing.T) {
	t.Setenv(delegationAsyncTTLKey, "invalid")
	require.Equal(t, defaultDelegationAsyncTTL, parseDelegationAsyncTTL())
}

func TestGetOperationAsyncStatusReturnsRedirectWithLocation(t *testing.T) {
	sut := NewSubmodelRepositoryAPIAPIService(persistencepostgresql.SubmodelDatabase{})

	delegatedOperationAsyncState.Lock()
	delegatedOperationAsyncState.records = map[string]delegatedOperationAsyncRecord{}
	delegatedOperationAsyncState.lastCleanupAt = time.Time{}
	delegatedOperationAsyncState.Unlock()

	decodedSubmodelID := "sm-redirect"
	encodedSubmodelID := base64.RawURLEncoding.EncodeToString([]byte(decodedSubmodelID))
	handleID := "handle-redirect"
	persistDelegatedAsyncRecord(handleID, delegatedOperationAsyncRecord{
		SubmodelIdentifier: decodedSubmodelID,
		IDShortPath:        "Ops.Add",
		State:              "Completed",
	})

	response, err := sut.GetOperationAsyncStatus(contextWithABACDisabled(t), encodedSubmodelID, "Ops.Add", handleID)
	require.NoError(t, err)
	require.Equal(t, 302, response.Code)

	redirect, ok := response.Body.(openapi.Redirect)
	require.True(t, ok)
	require.True(t, strings.Contains(redirect.Location, "/operation-results/"))
}
