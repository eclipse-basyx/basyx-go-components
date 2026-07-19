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

package aasenvironment

import (
	"bytes"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	aasx "github.com/aas-core-works/aas-package3-golang"
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

func TestSerializeEnvironmentToAASXPackageDoesNotRequireWritableTempDirectory(t *testing.T) {
	tempRoot := t.TempDir()
	noWriteTemp := filepath.Join(tempRoot, "no-write")
	require.NoError(t, os.Mkdir(noWriteTemp, 0o500))
	t.Cleanup(func() {
		// #nosec G302 -- test cleanup restores owner access on a temporary directory created by this test.
		_ = os.Chmod(noWriteTemp, 0o700)
	})
	t.Setenv("TMPDIR", noWriteTemp)

	payload, err := serializeEnvironmentToAASXPackage(
		[]byte(`{"assetAdministrationShells":[],"submodels":[],"conceptDescriptions":[]}`),
		"application/json",
		serializationAASXJSONSpecURI,
		nil,
		nil,
	)
	require.NoError(t, err)
	require.NotEmpty(t, payload)

	packageReader, err := aasx.NewPackaging().OpenReadFromStream(bytes.NewReader(payload))
	require.NoError(t, err)
	defer func() {
		_ = packageReader.Close()
	}()

	specs, err := packageReader.Specs()
	require.NoError(t, err)
	require.Len(t, specs, 1)
	require.Equal(t, serializationAASXJSONSpecURI, normalizePartURI(specs[0].URI))
}

func TestSerializeEnvironmentToAASXPackagePreservesManagedPaths(t *testing.T) {
	managedSupplementaryPath := "/aasx/files/file-token/service%20manual.pdf"
	managedThumbnailPath := "/aasx/files/thumbnail-token/preview%20image.png"
	supplementaryURI, err := url.Parse(managedSupplementaryPath)
	require.NoError(t, err)
	thumbnailPart, err := buildSerializationThumbnailPart(
		"aas-id", "preview image.png", "image/png", managedThumbnailPath, []byte("thumbnail"),
	)
	require.NoError(t, err)
	require.Equal(t, managedThumbnailPath, thumbnailPart.URI.String())

	payload, err := serializeEnvironmentToAASXPackage(
		[]byte(`{"assetAdministrationShells":[],"submodels":[],"conceptDescriptions":[]}`),
		"application/json",
		serializationAASXJSONSpecURI,
		[]serializationThumbnailPart{thumbnailPart},
		[]serializationSupplementaryPart{{
			URI: supplementaryURI, ContentType: "application/pdf", Content: []byte("manual"),
		}},
	)
	require.NoError(t, err)

	packageReader, err := aasx.NewPackaging().OpenReadFromStream(bytes.NewReader(payload))
	require.NoError(t, err)
	defer func() { _ = packageReader.Close() }()
	relationships, err := packageReader.SupplementaryRelationships()
	require.NoError(t, err)
	require.Len(t, relationships, 2)
	paths := make(map[string]struct{}, len(relationships))
	for _, relationship := range relationships {
		paths[relationship.Supplementary.URI.String()] = struct{}{}
	}
	require.Contains(t, paths, managedSupplementaryPath)
	require.Contains(t, paths, managedThumbnailPath)
}
