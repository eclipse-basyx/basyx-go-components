package api

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/FriedJannik/aas-go-sdk/types"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistencepostgresql "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	"github.com/stretchr/testify/require"
)

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

	response, err := sut.GetSubmodelElementByPathSubmodelRepo(context.Background(), encodedSubmodelID, "a.b", "invalid-level", "")
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
	response, err := sut.InvokeOperationValueOnly(context.Background(), "", "", "", gen.OperationRequestValueOnly{}, false)
	require.NoError(t, err)
	require.Equal(t, 400, response.Code)
}
