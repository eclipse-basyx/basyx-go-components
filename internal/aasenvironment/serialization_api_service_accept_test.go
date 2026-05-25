package aasenvironment

import (
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

func TestNegotiateSerializationContentTypeSupportsAASXAliasMediaTypes(t *testing.T) {
	testCases := []struct {
		name         string
		acceptHeader string
		expected     string
	}{
		{
			name:         "AliasXML",
			acceptHeader: "application/asset-administration-shell+xml",
			expected:     serializationContentTypeAASXXMLAlt,
		},
		{
			name:         "AliasJSON",
			acceptHeader: "application/asset-administration-shell+json",
			expected:     serializationContentTypeAASXJSONAlt,
		},
		{
			name:         "PackageXML",
			acceptHeader: "application/asset-administration-shell-package+xml",
			expected:     serializationContentTypeAASXXMLPkg,
		},
		{
			name:         "PackageJSON",
			acceptHeader: "application/asset-administration-shell-package+json",
			expected:     serializationContentTypeAASXJSONPkg,
		},
		{
			name:         "AliasXMLWithQuality",
			acceptHeader: "application/asset-administration-shell+xml;q=1.0, application/json;q=0.5",
			expected:     serializationContentTypeAASXXMLAlt,
		},
		{
			name:         "PackageXMLWithQuality",
			acceptHeader: "application/asset-administration-shell-package+xml;q=1.0, application/json;q=0.5",
			expected:     serializationContentTypeAASXXMLPkg,
		},
		{
			name:         "AliasJSONWithTrailingSemicolon",
			acceptHeader: "application/asset-administration-shell+json;",
			expected:     serializationContentTypeAASXJSONAlt,
		},
		{
			name:         "PackageJSONWithTrailingSemicolon",
			acceptHeader: "application/asset-administration-shell-package+json;",
			expected:     serializationContentTypeAASXJSONPkg,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			contentType, err := negotiateSerializationContentType(testCase.acceptHeader)
			require.NoError(t, err)
			require.Equal(t, testCase.expected, contentType)
		})
	}
}

func TestNegotiateSerializationContentTypeReturnsBadRequestForUnsupportedType(t *testing.T) {
	contentType, err := negotiateSerializationContentType("application/pdf")
	require.Error(t, err)
	require.True(t, common.IsErrBadRequest(err))
	require.Empty(t, contentType)
}
