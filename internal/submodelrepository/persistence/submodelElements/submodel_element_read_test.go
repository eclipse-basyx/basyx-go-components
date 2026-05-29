package submodelelements

import (
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestBuildSubmodelElementReferenceBuildsKeyChainForNestedPathWithListIndex(t *testing.T) {
	t.Parallel()

	reference, err := buildSubmodelElementReference("sm-1", types.ModelTypeSubmodelElementList, "test.test[0]")
	require.NoError(t, err)

	keys := reference.Keys()
	require.Len(t, keys, 4)

	require.Equal(t, types.KeyTypesSubmodel, keys[0].Type())
	require.Equal(t, "sm-1", keys[0].Value())
	require.Equal(t, types.KeyTypesSubmodelElementCollection, keys[1].Type())
	require.Equal(t, "test", keys[1].Value())
	require.Equal(t, types.KeyTypesSubmodelElementCollection, keys[2].Type())
	require.Equal(t, "test", keys[2].Value())
	require.Equal(t, types.KeyTypesSubmodelElementList, keys[3].Type())
	require.Equal(t, "0", keys[3].Value())
}

func TestBuildSubmodelElementReferenceBuildsKeyChainForNestedDotPath(t *testing.T) {
	t.Parallel()

	reference, err := buildSubmodelElementReference("sm-1", types.ModelTypeProperty, "parent.child")
	require.NoError(t, err)

	keys := reference.Keys()
	require.Len(t, keys, 3)

	require.Equal(t, types.KeyTypesSubmodel, keys[0].Type())
	require.Equal(t, "sm-1", keys[0].Value())
	require.Equal(t, types.KeyTypesSubmodelElementCollection, keys[1].Type())
	require.Equal(t, "parent", keys[1].Value())
	require.Equal(t, types.KeyTypesProperty, keys[2].Type())
	require.Equal(t, "child", keys[2].Value())
}
