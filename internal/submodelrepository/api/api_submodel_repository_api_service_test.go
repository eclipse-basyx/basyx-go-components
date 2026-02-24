package api

import (
	"testing"

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
