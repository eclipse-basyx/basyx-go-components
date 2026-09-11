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

package registrysync

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

var specificationVersionPattern = regexp.MustCompile(`^V?(\d+)\.(\d+)\.\d+$`)

// TestDescriptorInterfaceVersionsMatchRepositoryContracts prevents descriptor interface versions from drifting from the repository OpenAPI contracts.
func TestDescriptorInterfaceVersionsMatchRepositoryContracts(t *testing.T) {
	tests := []struct {
		name            string
		openAPIPath     string
		interfaceName   string
		descriptorValue string
	}{
		{
			name:            "AAS repository",
			openAPIPath:     filepath.Join("..", "..", "cmd", "aasrepositoryservice", "openapi.yaml"),
			interfaceName:   "AAS",
			descriptorValue: aasDescriptorInterface,
		},
		{
			name:            "Submodel repository",
			openAPIPath:     filepath.Join("..", "..", "cmd", "submodelrepositoryservice", "openapi.yaml"),
			interfaceName:   "SUBMODEL",
			descriptorValue: submodelDescriptorInterface,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contractVersion := readRepositoryContractVersion(t, test.openAPIPath)
			require.Equal(t, test.interfaceName+"-"+contractVersion, test.descriptorValue)
		})
	}
}

func readRepositoryContractVersion(t *testing.T, openAPIPath string) string {
	t.Helper()

	content, err := os.ReadFile(openAPIPath)
	require.NoError(t, err)

	var contract struct {
		Info struct {
			Version string `yaml:"version"`
		} `yaml:"info"`
	}
	require.NoError(t, yaml.Unmarshal(content, &contract))

	matches := specificationVersionPattern.FindStringSubmatch(contract.Info.Version)
	require.Len(t, matches, 3, "repository OpenAPI info.version must use V<major>.<minor>.<patch>")
	return matches[1] + "." + matches[2]
}
